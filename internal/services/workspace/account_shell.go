package workspace

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ReuseSessionAuto     = "auto"
	ReuseSessionNever    = "never"
	ReuseSessionRequired = "required"

	PrivilegeNone = "none"
	PrivilegeSudo = "sudo"
	PrivilegeSU   = "su"

	PTYAuto   = "auto"
	PTYAlways = "always"
	PTYNever  = "never"
)

var accountShellSeq atomic.Uint64

const defaultAccountShellPrimeTimeout = 15 * time.Second

type accountShellSession struct {
	id              string
	alias           string
	user            string
	shell           string
	privilegeMethod string
	role            string
	channel         string

	stdin   io.WriteCloser
	closeFn func() error

	out    chan string
	closed chan struct{}

	runMu   sync.Mutex
	stateMu sync.RWMutex
	cwd     string

	closeOnce  sync.Once
	closeErr   error
	closedFlag atomic.Bool
	primed     atomic.Bool
}

func newAccountShellSession(alias, sessionID, user, shellName, privilegeMethod, role, channel, cwd string, stdin io.WriteCloser, readers []io.Reader, closeFn func() error) *accountShellSession {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		id = fmt.Sprintf("%s-%06d", strings.TrimSpace(alias), accountShellSeq.Add(1))
	}
	if strings.TrimSpace(user) == "" {
		user = "unknown"
	}
	if strings.TrimSpace(shellName) == "" {
		shellName = "bash"
	}
	if strings.TrimSpace(privilegeMethod) == "" {
		privilegeMethod = PrivilegeNone
	}
	s := &accountShellSession{
		id:              id,
		alias:           alias,
		user:            user,
		shell:           shellName,
		privilegeMethod: privilegeMethod,
		role:            strings.TrimSpace(role),
		channel:         strings.TrimSpace(channel),
		stdin:           stdin,
		closeFn:         closeFn,
		out:             make(chan string, 512),
		closed:          make(chan struct{}),
		cwd:             strings.TrimSpace(cwd),
	}
	var wg sync.WaitGroup
	for _, r := range readers {
		if r == nil {
			continue
		}
		wg.Add(1)
		go func(reader io.Reader) {
			defer wg.Done()
			buf := make([]byte, 8192)
			for {
				n, err := reader.Read(buf)
				if n > 0 {
					chunk := string(buf[:n])
					select {
					case s.out <- chunk:
					case <-s.closed:
						return
					}
				}
				if err != nil {
					return
				}
			}
		}(r)
	}
	go func() {
		wg.Wait()
		s.closedFlag.Store(true)
		close(s.out)
	}()
	return s
}

func (s *accountShellSession) Info() AccountShellInfo {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return AccountShellInfo{
		ID:              s.id,
		Alias:           s.alias,
		User:            s.user,
		Shell:           s.shell,
		PrivilegeMethod: s.privilegeMethod,
		Role:            s.role,
		Channel:         s.channel,
		CWD:             s.cwd,
		Active:          !s.closedFlag.Load(),
	}
}

func (s *accountShellSession) Run(ctx context.Context, command string, opts ExecOptions, subQuery ExecuteSubQueryFunc) (ExecResult, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	if s.closedFlag.Load() {
		return ExecResult{}, fmt.Errorf("account shell is closed: %s", s.id)
	}

	// On first call, handle any startup prompts (e.g., sudo password from "sudo -u <user> bash -li")
	// BEFORE writing the command. Without this, the script would be fed to sudo as password
	// attempts, corrupting the auth flow and causing "prime account shell: context deadline exceeded".
	if !s.primed.Swap(true) {
		if err := s.waitAndHandleSudoPrompt(ctx, opts, 300*time.Millisecond); err != nil {
			return ExecResult{Code: 1, CWD: s.currentCWD(), User: s.user, Stderr: err.Error()}, err
		}
	}
	s.drainOutput()

	execCWD := strings.TrimSpace(opts.WorkingDir)
	if execCWD == "" {
		execCWD = s.currentCWD()
	}

	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	metaToken := "__ARG_M_" + id + "__"
	wrapped := buildInteractiveBashExecCommand(command, execCWD, metaToken)
	if _, err := io.WriteString(s.stdin, wrapped+"\n"); err != nil {
		return ExecResult{}, err
	}

	var stdoutBuf strings.Builder
	var recentLines []string
	pwInjectCount := 0
	retryCount := 0
	lastPasswordPrompt := ""
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ExecResult{
				Code:   1,
				CWD:    s.currentCWD(),
				User:   s.user,
				Stderr: ctx.Err().Error(),
			}, ctx.Err()
		case chunk, ok := <-s.out:
			if !ok {
				return ExecResult{
					Code:   1,
					CWD:    s.currentCWD(),
					User:   s.user,
					Stderr: "account shell closed",
				}, fmt.Errorf("account shell closed: %s", s.id)
			}
			stdoutBuf.WriteString(chunk)
			if opts.ChunkCallback != nil {
				opts.ChunkCallback(chunk)
			}
			recentLines = appendRecentLines(recentLines, chunk, 40)
			if strings.Contains(stdoutBuf.String(), metaToken+":") {
				parsedStdout, code, pwd, user, err := parseExecOutput(stdoutBuf.String(), metaToken)
				if err != nil {
					return ExecResult{
						Stdout: strings.TrimSpace(stdoutBuf.String()),
						Code:   1,
						CWD:    s.currentCWD(),
						User:   s.user,
					}, err
				}
				s.stateMu.Lock()
				if pwd != "" {
					s.cwd = pwd
				}
				if user != "" {
					s.user = user
				}
				s.stateMu.Unlock()
				return ExecResult{
					Stdout: strings.TrimSpace(parsedStdout),
					Code:   code,
					CWD:    pwd,
					User:   user,
				}, nil
			}
		case <-ticker.C:
			if len(recentLines) == 0 {
				continue
			}
			snapshot := strings.Join(recentLines, "\n")
			if isSudoPasswordPrompt(snapshot) {
				if snapshot == lastPasswordPrompt {
					continue
				}
				lastPasswordPrompt = snapshot
				if pwInjectCount == 0 && opts.RootPassword != "" {
					pwInjectCount++
					_, _ = io.WriteString(s.stdin, opts.RootPassword+"\n")
					continue
				}
				if opts.RequestSudoPassword != nil {
					pw, err := opts.RequestSudoPassword("sudo password")
					if err == nil && pw != "" {
						opts.RootPassword = pw
						pwInjectCount++
						_, _ = io.WriteString(s.stdin, pw+"\n")
					}
				}
				continue
			}
			if subQuery == nil || retryCount >= 2 {
				continue
			}
			retryCount++
			systemPrompt := "You are a terminal automation assistant. Analyze the terminal output and provide the NECESSARY input string to proceed with the command. ONLY return the input string itself (e.g., 'y', '1', or ''). Do not add any explanation."
			userPrompt := fmt.Sprintf("Command: %s\nTerminal Output:\n%s\n\nWhat should I type to proceed?", command, snapshot)
			response, err := subQuery(ctx, systemPrompt, userPrompt)
			if err == nil && strings.TrimSpace(response) != "" {
				_, _ = io.WriteString(s.stdin, strings.TrimSpace(response)+"\n")
			}
		}
	}
}

func (s *accountShellSession) Start(ctx context.Context, command string, opts ExecOptions, subQuery ExecuteSubQueryFunc) *ExecHandle {
	streamCh := make(chan string, 512)
	resultCh := make(chan ExecResult, 1)
	runOpts := opts
	runOpts.ChunkCallback = func(chunk string) {
		if opts.ChunkCallback != nil {
			opts.ChunkCallback(chunk)
		}
		select {
		case streamCh <- chunk:
		case <-ctx.Done():
		}
	}
	go func() {
		defer close(streamCh)
		defer close(resultCh)
		res, err := s.Run(ctx, command, runOpts, subQuery)
		if err != nil && strings.TrimSpace(res.Stderr) == "" {
			res.Stderr = err.Error()
			if res.Code == 0 {
				res.Code = 1
			}
		}
		resultCh <- res
	}()
	return &ExecHandle{
		Stream: streamCh,
		Result: resultCh,
		Write: func(input string) error {
			_, err := io.WriteString(s.stdin, input)
			return err
		},
		Kill: func() {
			_ = s.Close()
		},
	}
}

func (s *accountShellSession) Close() error {
	s.closeOnce.Do(func() {
		s.closedFlag.Store(true)
		close(s.closed)
		if s.stdin != nil {
			_ = s.stdin.Close()
		}
		if s.closeFn != nil {
			s.closeErr = s.closeFn()
		}
	})
	return s.closeErr
}

func (s *accountShellSession) currentCWD() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.cwd
}

func (s *accountShellSession) drainOutput() {
	for {
		select {
		case _, ok := <-s.out:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func appendRecentLines(lines []string, chunk string, max int) []string {
	parts := strings.Split(strings.ReplaceAll(chunk, "\r\n", "\n"), "\n")
	lines = append(lines, parts...)
	if max > 0 && len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}

func buildInteractiveBashExecCommand(command, workingDir, metaToken string) string {
	var script strings.Builder
	script.WriteString("set +e\n")
	if strings.TrimSpace(workingDir) != "" {
		script.WriteString(fmt.Sprintf("cd %s >/dev/null 2>&1 || true\n", shellQuote(workingDir)))
	}
	script.WriteString(command)
	if !strings.HasSuffix(command, "\n") {
		script.WriteString("\n")
	}
	script.WriteString("__arg_ec=$?\n")
	script.WriteString(fmt.Sprintf("printf '\\n%s:%%s:%%s:%%s\\n' \"$__arg_ec\" \"$(pwd | base64 | tr -d '\\r\\n')\" \"$(id -un | base64 | tr -d '\\r\\n')\"\n", metaToken))
	// bash -c로 감싸 단일 줄로 stdin에 주입 → account shell PTY의 PS2(>) 프롬프트 억제
	return "bash -c " + shellQuote(script.String())
}

func normalizeReuseSession(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ReuseSessionNever:
		return ReuseSessionNever
	case ReuseSessionRequired:
		return ReuseSessionRequired
	default:
		return ReuseSessionAuto
	}
}

func normalizePrivilegeMethod(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case PrivilegeSU:
		return PrivilegeSU
	case PrivilegeNone:
		return PrivilegeNone
	default:
		return PrivilegeSudo
	}
}

func shouldUseAccountShell(opts ExecOptions) bool {
	if normalizeReuseSession(opts.ReuseSession) == ReuseSessionNever {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(opts.PTYMode), PTYNever) {
		return false
	}
	return strings.TrimSpace(opts.SessionID) != "" ||
		strings.TrimSpace(opts.AsUser) != "" ||
		normalizeReuseSession(opts.ReuseSession) == ReuseSessionRequired
}

func accountShellKey(alias string, opts ExecOptions) string {
	if id := strings.TrimSpace(opts.SessionID); id != "" {
		return strings.TrimSpace(alias) + "|id|" + id
	}
	user := strings.TrimSpace(opts.AsUser)
	if user == "" {
		user = "login"
	}
	shellName := strings.TrimSpace(opts.Shell)
	if shellName == "" {
		shellName = "bash"
	}
	return strings.Join([]string{
		strings.TrimSpace(alias),
		strings.ToLower(strings.TrimSpace(opts.Channel)),
		strings.ToLower(strings.TrimSpace(opts.Role)),
		strings.ToLower(shellName),
		strings.ToLower(user),
		normalizePrivilegeMethod(opts.PrivilegeMethod),
	}, "|")
}

// waitAndHandleSudoPrompt waits up to `wait` for a sudo password prompt to appear in the
// shell's output, injects the password if found, then drains remaining startup output.
// Must be called before writing any command to stdin.
func (s *accountShellSession) waitAndHandleSudoPrompt(ctx context.Context, opts ExecOptions, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()

	var lines []string
	promptDetected := false

waitLoop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-s.out:
			if !ok {
				return fmt.Errorf("account shell closed: %s", s.id)
			}
			lines = appendRecentLines(lines, chunk, 40)
			if isSudoPasswordPrompt(strings.Join(lines, "\n")) {
				promptDetected = true
				break waitLoop
			}
		case <-timer.C:
			break waitLoop
		}
	}

	if !promptDetected {
		return nil
	}

	var pw string
	if opts.RootPassword != "" {
		pw = opts.RootPassword
	} else if opts.RequestSudoPassword != nil {
		var err error
		pw, err = opts.RequestSudoPassword("sudo password for privilege escalation")
		if err != nil {
			return fmt.Errorf("sudo password required: %w", err)
		}
	}
	if strings.TrimSpace(pw) == "" {
		return fmt.Errorf("sudo password required for privilege escalation (hint: provide root_password parameter)")
	}
	_, _ = io.WriteString(s.stdin, pw+"\n")

	// Drain startup output while bash initializes after successful auth (~1.5s).
	drainTimer := time.NewTimer(1500 * time.Millisecond)
	defer drainTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-s.out:
			if !ok {
				return fmt.Errorf("account shell closed after sudo auth: %s", s.id)
			}
		case <-drainTimer.C:
			return nil
		}
	}
}
