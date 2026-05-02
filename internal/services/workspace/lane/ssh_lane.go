package lane

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// ErrPrimingTimeout is returned when the PTY shell does not emit the sentinel
// response within the priming deadline. Callers can detect it via errors.Is to
// implement fallback strategies (e.g. non-lane SSH exec).
var ErrPrimingTimeout = errors.New("lane: priming exec failed")

// passwordPromptRe matches sudo/su password prompts emitted by common locales.
var passwordPromptRe = regexp.MustCompile(`(?im)(\[sudo\][^\n]*password|password\s*(?:for[^\n]*)?:|sorry,\s*try\s*again|암호\s*:)`)

func makePasswordPromptMatcher() promptMatcher {
	return func(buf []byte) (bool, int) {
		loc := passwordPromptRe.FindIndex(buf)
		if loc == nil {
			return false, 0
		}
		return true, loc[1]
	}
}

// sshLane is a persistent PTY-backed shell on a remote host. All Exec /
// EnterAccount / ExitAccount calls serialize through mu — the underlying PTY
// is a single resource.
type sshLane struct {
	alias  string
	client *ssh.Client

	session *ssh.Session
	stdin   io.WriteCloser
	reader  *streamReader

	mu        sync.Mutex
	stack     AccountStack
	cwd       string
	user      string
	privState string
	lastUsed  time.Time
	closed    atomic.Bool
}

// openSSHLane starts a fresh PTY shell on client and primes it with the lane
// init script. The login user (client.User if available, else "") seeds the
// account stack as element 0.
func openSSHLane(ctx context.Context, client *ssh.Client, alias, loginUser, defaultCWD string) (*sshLane, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("lane: open ssh session: %w", err)
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
		return nil, fmt.Errorf("lane: request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("lane: stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("lane: stdout pipe: %w", err)
	}

	if err := session.Shell(); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("lane: start shell: %w", err)
	}

	stack := AccountStack{}
	if u := strings.TrimSpace(loginUser); u != "" {
		stack = stack.Push(u)
	}

	l := &sshLane{
		alias:    alias,
		client:   client,
		session:  session,
		stdin:    stdin,
		reader:   newStreamReader(stdout),
		stack:    stack,
		cwd:      strings.TrimSpace(defaultCWD),
		user:     loginUser,
		lastUsed: time.Now(),
	}

	if _, err := io.WriteString(stdin, LaneInitScript); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("lane: write init script: %w", err)
	}

	primingCtx := ctx
	if _, deadline := primingCtx.Deadline(); !deadline {
		var cancel context.CancelFunc
		// 5s is plenty: the priming printf executes within a few ms once the
		// shell is ready, and a slow profile.d should never block this long.
		// A short budget means a misbehaving remote falls back to the legacy
		// per-channel exec path almost immediately instead of stalling the
		// user for 30s on every connect.
		primingCtx, cancel = context.WithTimeout(primingCtx, 5*time.Second)
		defer cancel()
	}

	codec := NewCodec()
	if _, err := io.WriteString(stdin, codec.Wrap(":")); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("lane: write priming command: %w", err)
	}
	res, err := l.reader.waitFor(primingCtx, codec)
	if err != nil {
		if os.Getenv("ARGUS_LANE_DEBUG") != "" {
			buf := l.reader.snapshotBuffer()
			fmt.Fprintf(os.Stderr, "[lane-debug] priming timeout for %s: nonce=%s err=%v buf_len=%d buf=%q\n",
				alias, codec.Nonce, err, len(buf), buf)
		}
		_ = l.Close()
		return nil, fmt.Errorf("%w: %w", ErrPrimingTimeout, err)
	}

	l.mu.Lock()
	if res.CWD != "" {
		l.cwd = res.CWD
	}
	if res.User != "" {
		l.user = res.User
		if len(l.stack) == 0 || l.stack.Top() != res.User {
			l.stack = AccountStack{res.User}
		}
	}
	l.mu.Unlock()
	l.reader.dropAll()
	return l, nil
}

func (l *sshLane) ID() LaneID {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LaneIDFor(l.alias, l.stack)
}

func (l *sshLane) Snapshot() LaneSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LaneSnapshot{
		ID:           LaneIDFor(l.alias, l.stack),
		Alias:        l.alias,
		AccountStack: l.stack.Clone(),
		CWD:          l.cwd,
		PrivState:    l.privState,
		LastUsed:     l.lastUsed,
	}
}

func (l *sshLane) Exec(ctx context.Context, cmd string, opts ExecOptions) (ExecResult, error) {
	if l.closed.Load() {
		return ExecResult{}, errors.New("lane: closed")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.execLocked(ctx, cmd, opts)
}

// execLocked runs cmd assuming l.mu is already held. Used by Exec, by the
// init-script priming, and re-entrantly by EnterAccount/ExitAccount when they
// need to settle the lane after a transition.
func (l *sshLane) execLocked(ctx context.Context, cmd string, opts ExecOptions) (ExecResult, error) {
	execCtx := ctx
	timeout := opts.Timeout
	if timeout <= 0 {
		// Fall back budget so a misbehaving remote shell can't strand the
		// caller forever. The legacy non-lane Exec path is fast enough that a
		// 15s ceiling for any single lane Exec leaves plenty of headroom for
		// healthy commands while bounding the worst case.
		timeout = 15 * time.Second
	}
	{
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(execCtx, timeout)
		defer cancel()
	}

	codec := NewCodec()
	wrapped := cmd
	if wd := strings.TrimSpace(opts.WorkingDir); wd != "" && wd != l.cwd {
		wrapped = "cd " + posixSingleQuote(wd) + " 2>/dev/null || true\n" + cmd
	}

	if opts.OnChunk != nil {
		fullSentinel := SentinelPrefix + codec.Nonce + ":"
		filter := NewSentinelFilter(opts.OnChunk, fullSentinel)
		l.reader.setChunkCallback(filter.OnChunk)
		defer l.reader.setChunkCallback(nil)
	}

	if _, err := io.WriteString(l.stdin, codec.Wrap(wrapped)); err != nil {
		return ExecResult{}, fmt.Errorf("lane: write command: %w", err)
	}

	var matcher promptMatcher
	pwInjected := 0
	if strings.TrimSpace(opts.SudoPassword) != "" {
		matcher = makePasswordPromptMatcher()
	}

	debugLane := os.Getenv("ARGUS_LANE_DEBUG") != ""
	startedAt := time.Now()
	defer func() {
		if debugLane {
			fmt.Fprintf(os.Stderr, "[lane-debug] exec finished alias=%s nonce=%s elapsed=%s cmd_len=%d\n",
				l.alias, codec.Nonce, time.Since(startedAt), len(cmd))
		}
	}()

	for {
		res, err := l.reader.waitForWithMatch(execCtx, codec, matcher)
		if err == nil {
			if res.CWD != "" {
				l.cwd = res.CWD
			}
			if res.User != "" {
				l.user = res.User
			}
			l.lastUsed = time.Now()
			return ExecResult{
				Stdout: strings.TrimRight(res.Stdout, "\r\n"),
				Code:   res.Code,
				CWD:    res.CWD,
				User:   res.User,
			}, nil
		}
		if errors.Is(err, errPromptMatched) {
			if pwInjected >= 1 {
				return ExecResult{}, errors.New("lane: sudo password rejected")
			}
			if _, werr := io.WriteString(l.stdin, opts.SudoPassword+"\n"); werr != nil {
				return ExecResult{}, fmt.Errorf("lane: inject sudo password: %w", werr)
			}
			pwInjected++
			continue
		}
		if debugLane {
			buf := l.reader.snapshotBuffer()
			fmt.Fprintf(os.Stderr, "[lane-debug] exec failed alias=%s nonce=%s err=%v elapsed=%s cmd_len=%d buf_len=%d buf=%q\n",
				l.alias, codec.Nonce, err, time.Since(startedAt), len(cmd), len(buf), buf)
		}
		return ExecResult{}, err
	}
}

// EnterAccount transitions the lane into a child shell as user via method
// ("su" or "sudo"). On return the lane's account stack has been pushed and the
// effective cwd/user reflect the new shell.
func (l *sshLane) EnterAccount(ctx context.Context, user, method, password string) error {
	if l.closed.Load() {
		return errors.New("lane: closed")
	}
	user = strings.TrimSpace(user)
	if user == "" {
		return errors.New("lane: target user is required")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	enterCmd := buildEnterCommand(user, method)
	if _, err := io.WriteString(l.stdin, enterCmd+"\n"); err != nil {
		return fmt.Errorf("lane: write enter command: %w", err)
	}

	if strings.TrimSpace(password) != "" {
		matcher := makePasswordPromptMatcher()
		injected := 0
		injectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	authLoop:
		for injected < 1 {
			fakeCodec := &Codec{Nonce: "no-match-during-auth"}
			_, err := l.reader.waitForWithMatch(injectCtx, fakeCodec, matcher)
			if errors.Is(err, errPromptMatched) {
				if _, werr := io.WriteString(l.stdin, password+"\n"); werr != nil {
					return fmt.Errorf("lane: write password: %w", werr)
				}
				injected++
				continue
			}
			break authLoop
		}
	}

	if _, err := io.WriteString(l.stdin, LaneInitScript); err != nil {
		return fmt.Errorf("lane: re-init child shell: %w", err)
	}

	settleCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	codec := NewCodec()
	if _, err := io.WriteString(l.stdin, codec.Wrap(":")); err != nil {
		return fmt.Errorf("lane: settle child shell: %w", err)
	}
	res, err := l.reader.waitFor(settleCtx, codec)
	if err != nil {
		return fmt.Errorf("lane: settle child shell: %w", err)
	}

	l.stack = l.stack.Push(strings.TrimSpace(res.User))
	l.cwd = res.CWD
	l.user = strings.TrimSpace(res.User)
	l.privState = "fresh"
	l.lastUsed = time.Now()
	return nil
}

// ExitAccount leaves the topmost child shell, popping the account stack. If
// the stack only holds the login user, ExitAccount returns an error rather
// than killing the lane.
func (l *sshLane) ExitAccount(ctx context.Context) error {
	if l.closed.Load() {
		return errors.New("lane: closed")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.stack) <= 1 {
		return errors.New("lane: cannot exit login shell")
	}

	if _, err := io.WriteString(l.stdin, "exit\n"); err != nil {
		return fmt.Errorf("lane: write exit: %w", err)
	}

	settleCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	codec := NewCodec()
	if _, err := io.WriteString(l.stdin, codec.Wrap(":")); err != nil {
		return fmt.Errorf("lane: settle parent shell: %w", err)
	}
	res, err := l.reader.waitFor(settleCtx, codec)
	if err != nil {
		return fmt.Errorf("lane: settle parent shell: %w", err)
	}

	l.stack, _ = l.stack.Pop()
	l.cwd = res.CWD
	l.user = strings.TrimSpace(res.User)
	l.lastUsed = time.Now()
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

func (l *sshLane) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}
	if l.stdin != nil {
		_, _ = io.WriteString(l.stdin, "exit\n")
		_ = l.stdin.Close()
	}
	if l.session != nil {
		return l.session.Close()
	}
	return nil
}

// posixSingleQuote wraps s in single quotes, escaping any embedded ' for POSIX shells.
func posixSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
