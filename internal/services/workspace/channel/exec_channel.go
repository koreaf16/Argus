package channel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/koreaf16/argus/internal/services/workspace/lane"
)

// ErrPrimingTimeout is returned when the PTY shell fails to settle the lane
// init script before the priming deadline. Callers can use errors.Is to
// distinguish this from authentication or transport errors.
var ErrPrimingTimeout = errors.New("channel: priming exec failed")

// execChannel is a persistent PTY-backed shell scoped to a single PrivilegeKey.
// Default-privilege channels are spawned directly from the master *ssh.Client;
// non-default channels are spawned by sending an enter command on top of the
// default channel before the codec sentinel takes over.
type execChannel struct {
	key       ChannelKey
	loginUser string
	defaultCWD string

	session *ssh.Session
	stdin   io.WriteCloser
	reader  *lane.StreamReader

	mu        sync.Mutex
	stack     lane.AccountStack
	cwd       string
	user      string
	privState string
	lastUsed  time.Time
	lastErr   string
	state     ChannelState
	closed    atomic.Bool
}

// openExecChannel starts a fresh PTY shell on client and primes it. The
// resulting channel represents PrivilegeDefault — non-default privilege
// channels are produced by ChannelManager calling EnterAccount on a default
// channel and re-keying the result.
func openExecChannel(ctx context.Context, client *ssh.Client, alias, loginUser, defaultCWD string) (*execChannel, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("channel: open ssh session: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.ECHOCTL:       0,
		ssh.ICANON:        0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("dumb", 200, 80, modes); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("channel: request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("channel: stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("channel: stdout pipe: %w", err)
	}
	if err := session.Shell(); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("channel: start shell: %w", err)
	}

	stack := lane.AccountStack{}
	if u := strings.TrimSpace(loginUser); u != "" {
		stack = stack.Push(u)
	}

	c := &execChannel{
		key: ChannelKey{
			Alias:     alias,
			Privilege: PrivilegeDefault,
			Purpose:   PurposeExec,
		},
		loginUser:  loginUser,
		defaultCWD: defaultCWD,
		session:    session,
		stdin:      stdin,
		reader:     lane.NewStreamReader(stdout),
		stack:      stack,
		cwd:        strings.TrimSpace(defaultCWD),
		user:       loginUser,
		state:      StateOpening,
		lastUsed:   time.Now(),
	}

	if _, err := io.WriteString(stdin, lane.LaneInitScript); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("channel: write init script: %w", err)
	}

	primingCtx := ctx
	if _, hasDeadline := primingCtx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		primingCtx, cancel = context.WithTimeout(primingCtx, 5*time.Second)
		defer cancel()
	}

	codec := lane.NewCodec()
	if _, err := io.WriteString(stdin, codec.Wrap(":")); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("channel: write priming command: %w", err)
	}
	res, err := c.reader.WaitFor(primingCtx, codec)
	if err != nil {
		if os.Getenv("ARGUS_LANE_DEBUG") != "" {
			buf := c.reader.SnapshotBuffer()
			fmt.Fprintf(os.Stderr, "[channel-debug] priming timeout for %s: nonce=%s err=%v buf_len=%d buf=%q\n",
				alias, codec.Nonce, err, len(buf), buf)
		}
		_ = c.Close()
		return nil, fmt.Errorf("%w: %w", ErrPrimingTimeout, err)
	}
	c.mu.Lock()
	if res.CWD != "" {
		c.cwd = res.CWD
	}
	if sanitizeUser(res.User) != "" {
		c.user = res.User
		if len(c.stack) == 0 || c.stack.Top() != res.User {
			c.stack = lane.AccountStack{res.User}
		}
	}
	c.state = StateReady
	c.mu.Unlock()
	c.reader.DropAll()
	return c, nil
}

func (c *execChannel) Key() ChannelKey {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.key
}

func (c *execChannel) Snapshot() ChannelSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ChannelSnapshot{
		Key:          c.key,
		LoginUser:    c.loginUser,
		AccountStack: c.stack.Clone(),
		CWD:          c.cwd,
		State:        c.state,
		LastUsed:     c.lastUsed,
		LastError:    c.lastErr,
	}
}

func (c *execChannel) Exec(ctx context.Context, req ExecRequest) (ExecOutcome, error) {
	if c.closed.Load() {
		return ExecOutcome{}, errors.New("channel: closed")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = StateBusy
	defer func() { c.state = StateReady }()

	return c.execLocked(ctx, req)
}

func (c *execChannel) execLocked(ctx context.Context, req ExecRequest) (ExecOutcome, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	codec := lane.NewCodec()
	wrapped := req.Command
	if wd := strings.TrimSpace(req.WorkingDir); wd != "" && wd != c.cwd {
		wrapped = "cd " + posixSingleQuote(wd) + " 2>/dev/null || true\n" + req.Command
	}

	if req.OnChunk != nil {
		fullSentinel := lane.SentinelPrefix + codec.Nonce + ":"
		filter := lane.NewSentinelFilter(req.OnChunk, fullSentinel)
		c.reader.SetChunkCallback(filter.OnChunk)
		defer c.reader.SetChunkCallback(nil)
	}

	if _, err := io.WriteString(c.stdin, codec.Wrap(wrapped)); err != nil {
		c.lastErr = err.Error()
		return ExecOutcome{}, fmt.Errorf("channel: write command: %w", err)
	}

	var matcher lane.PromptMatcher
	pwInjected := 0
	if strings.TrimSpace(req.SudoPassword) != "" {
		matcher = lane.MakePasswordPromptMatcher()
	}

	debugCh := os.Getenv("ARGUS_LANE_DEBUG") != ""
	startedAt := time.Now()
	defer func() {
		if debugCh {
			fmt.Fprintf(os.Stderr, "[channel-debug] exec finished alias=%s nonce=%s elapsed=%s cmd_len=%d\n",
				c.key.Alias, codec.Nonce, time.Since(startedAt), len(req.Command))
		}
	}()

	for {
		res, err := c.reader.WaitForWithMatch(execCtx, codec, matcher)
		if err == nil {
			if res.CWD != "" {
				c.cwd = res.CWD
			}
			if sanitizeUser(res.User) != "" {
				c.user = res.User
			}
			c.lastUsed = time.Now()
			c.lastErr = ""
			return ExecOutcome{
				Stdout: strings.TrimRight(res.Stdout, "\r\n"),
				Code:   res.Code,
				CWD:    res.CWD,
				User:   res.User,
			}, nil
		}
		if errors.Is(err, lane.ErrPromptMatched) {
			if pwInjected >= 1 {
				c.lastErr = "sudo password rejected"
				return ExecOutcome{}, errors.New("channel: sudo password rejected")
			}
			if _, werr := io.WriteString(c.stdin, req.SudoPassword+"\n"); werr != nil {
				c.lastErr = werr.Error()
				return ExecOutcome{}, fmt.Errorf("channel: inject sudo password: %w", werr)
			}
			pwInjected++
			continue
		}
		c.lastErr = err.Error()
		if debugCh {
			buf := c.reader.SnapshotBuffer()
			fmt.Fprintf(os.Stderr, "[channel-debug] exec failed alias=%s nonce=%s err=%v elapsed=%s cmd_len=%d buf_len=%d buf=%q\n",
				c.key.Alias, codec.Nonce, err, time.Since(startedAt), len(req.Command), len(buf), buf)
		}
		return ExecOutcome{}, err
	}
}

// EnterAccount transitions the underlying PTY into a child shell as user via
// method ("su" or "sudo"). The channel's stack/cwd/user are updated to reflect
// the post-push state. ChannelManager re-keys the channel so future Acquires
// for the new privilege resolve to this same execChannel.
func (c *execChannel) EnterAccount(ctx context.Context, user, method, password string) error {
	if c.closed.Load() {
		return errors.New("channel: closed")
	}
	user = strings.TrimSpace(user)
	if user == "" {
		return errors.New("channel: target user is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	enterCmd := buildEnterCommand(user, method)
	if _, err := io.WriteString(c.stdin, enterCmd+"\n"); err != nil {
		return fmt.Errorf("channel: write enter command: %w", err)
	}

	if strings.TrimSpace(password) != "" {
		matcher := lane.MakePasswordPromptMatcher()
		injected := 0
		injectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	authLoop:
		for injected < 1 {
			fakeCodec := &lane.Codec{Nonce: "no-match-during-auth"}
			_, err := c.reader.WaitForWithMatch(injectCtx, fakeCodec, matcher)
			if errors.Is(err, lane.ErrPromptMatched) {
				if _, werr := io.WriteString(c.stdin, password+"\n"); werr != nil {
					return fmt.Errorf("channel: write password: %w", werr)
				}
				injected++
				continue
			}
			break authLoop
		}
	}

	if _, err := io.WriteString(c.stdin, lane.LaneInitScript); err != nil {
		return fmt.Errorf("channel: re-init child shell: %w", err)
	}

	settleCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	codec := lane.NewCodec()
	if _, err := io.WriteString(c.stdin, codec.Wrap(":")); err != nil {
		return fmt.Errorf("channel: settle child shell: %w", err)
	}
	res, err := c.reader.WaitFor(settleCtx, codec)
	if err != nil {
		return fmt.Errorf("channel: settle child shell: %w", err)
	}

	c.stack = c.stack.Push(sanitizeUser(res.User))
	c.cwd = res.CWD
	c.user = sanitizeUser(res.User)
	c.privState = "fresh"
	c.lastUsed = time.Now()
	c.key = ChannelKey{
		Alias:     c.key.Alias,
		Privilege: PrivilegeKeyFromStack(c.loginUser, c.stack),
		Purpose:   PurposeExec,
	}
	return nil
}

// ExitAccount leaves the topmost child shell, popping the account stack. Pops
// past the login user are rejected so the channel is never killed accidentally.
func (c *execChannel) ExitAccount(ctx context.Context) error {
	if c.closed.Load() {
		return errors.New("channel: closed")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.stack) <= 1 {
		return errors.New("channel: cannot exit login shell")
	}

	if _, err := io.WriteString(c.stdin, "exit\n"); err != nil {
		return fmt.Errorf("channel: write exit: %w", err)
	}

	settleCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	codec := lane.NewCodec()
	if _, err := io.WriteString(c.stdin, codec.Wrap(":")); err != nil {
		return fmt.Errorf("channel: settle parent shell: %w", err)
	}
	res, err := c.reader.WaitFor(settleCtx, codec)
	if err != nil {
		return fmt.Errorf("channel: settle parent shell: %w", err)
	}
	c.stack, _ = c.stack.Pop()
	c.cwd = res.CWD
	c.user = sanitizeUser(res.User)
	c.lastUsed = time.Now()
	c.key = ChannelKey{
		Alias:     c.key.Alias,
		Privilege: PrivilegeKeyFromStack(c.loginUser, c.stack),
		Purpose:   PurposeExec,
	}
	return nil
}

// StartStreaming runs a command and exposes the result as a streaming handle
// so callers used to workspace.ExecHandle keep working. Account transitions
// (Enter/Exit) are handled by ChannelManager *before* invoking StartStreaming;
// this method always runs a plain Exec on the current shell.
func (c *execChannel) StartStreaming(ctx context.Context, req ExecRequest) (*StreamHandle, error) {
	if c.closed.Load() {
		return nil, errors.New("channel: closed")
	}
	streamCh := make(chan string, 64)
	resultCh := make(chan ExecOutcome, 1)
	cancelCtx, cancel := context.WithCancel(ctx)
	var killOnce sync.Once

	handle := &StreamHandle{
		Stream: streamCh,
		Result: resultCh,
		Write: func(string) error {
			return errors.New("channel: stdin writes are not supported on exec channels")
		},
		Kill: func() { killOnce.Do(cancel) },
	}

	chunked := req
	chunked.OnChunk = func(chunk string) {
		if req.OnChunk != nil {
			req.OnChunk(chunk)
		}
		select {
		case streamCh <- chunk:
		default:
		}
	}

	go func() {
		defer close(streamCh)
		defer close(resultCh)
		defer cancel()

		out, err := c.Exec(cancelCtx, chunked)
		if err != nil {
			resultCh <- ExecOutcome{Code: 1, Stderr: err.Error()}
			return
		}
		resultCh <- out
	}()

	return handle, nil
}

func (c *execChannel) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	c.state = StateClosed
	c.mu.Unlock()
	if c.stdin != nil {
		_, _ = io.WriteString(c.stdin, "exit\n")
		_ = c.stdin.Close()
	}
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

func buildEnterCommand(user, method string) string {
	user = strings.TrimSpace(user)
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "sudo":
		return "sudo -i -u " + posixSingleQuote(user)
	default:
		return "su -s /bin/bash - " + posixSingleQuote(user)
	}
}

// posixSingleQuote wraps s in single quotes, escaping any embedded ' for POSIX shells.
func posixSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
