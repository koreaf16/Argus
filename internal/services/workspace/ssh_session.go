package workspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type ExecuteSubQueryFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

type outputChunk struct {
	text  string
	isErr bool
}

type sshTunnel struct {
	id         string
	localAddr  string
	remoteAddr string
	listener   net.Listener
	closed     atomic.Bool
	done       chan struct{}
	closeOnce  sync.Once
}

func (t *sshTunnel) close() error {
	var err error
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		err = t.listener.Close()
	})
	return err
}

type sshSession struct {
	entry  ServerEntry
	client *ssh.Client

	closeOnce sync.Once
	closeErr  error

	stateMu sync.RWMutex
	cwd     string
	user    string

	authClosers []io.Closer

	tunnelsMu sync.RWMutex
	tunnels   map[string]*sshTunnel
	tunnelSeq atomic.Uint64
}

func newSSHSession(entry ServerEntry, managerPrompt PasswordRequestFunc) (*sshSession, error) {
	authMethods, authClosers, err := buildAuthMethods(entry, managerPrompt)
	if err != nil {
		return nil, err
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH auth method is available for %s", entry.Alias)
	}

	hostKeyCB, err := tufoHostKeyCallback(knownHostsPath())
	if err != nil {
		closeAll(authClosers)
		return nil, fmt.Errorf("build host key callback: %w", err)
	}
	config := &ssh.ClientConfig{
		User:            entry.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCB,
		Timeout:         15 * time.Second,
	}

	port := entry.Port
	if port <= 0 {
		port = 22
	}
	addr := net.JoinHostPort(entry.Host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		closeAll(authClosers)
		return nil, fmt.Errorf("ssh dial failed: %w", err)
	}

	initialCWD := strings.TrimSpace(entry.DefaultCWD)
	if initialCWD == "" {
		initialCWD = "~"
	}

	return &sshSession{
		entry:       entry,
		client:      client,
		cwd:         initialCWD,
		user:        entry.User,
		authClosers: authClosers,
		tunnels:     make(map[string]*sshTunnel),
	}, nil
}

func buildAuthMethods(entry ServerEntry, managerPrompt PasswordRequestFunc) ([]ssh.AuthMethod, []io.Closer, error) {
	methods := make([]ssh.AuthMethod, 0, 4)
	closers := make([]io.Closer, 0, 1)

	if entry.Auth.UseAgent {
		if sock := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK")); sock != "" {
			conn, err := net.Dial("unix", sock)
			if err == nil {
				agentClient := agent.NewClient(conn)
				signers, signerErr := agentClient.Signers()
				if signerErr == nil && len(signers) > 0 {
					methods = append(methods, ssh.PublicKeys(signers...))
					closers = append(closers, conn)
				} else {
					_ = conn.Close()
				}
			}
		}
	}

	identityFile := strings.TrimSpace(entry.Auth.IdentityFile)
	if identityFile != "" {
		signer, err := loadSigner(identityFile, entry.Alias, managerPrompt)
		if err != nil {
			closeAll(closers)
			return nil, nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if entry.Auth.AllowPassword && managerPrompt != nil {
		methods = append(methods, ssh.PasswordCallback(func() (string, error) {
			pw, err := managerPrompt(entry.Alias, "ssh", "SSH password:")
			if err == nil {
				fmt.Printf("[DEBUG] Using SSH password for %s: %s\n", entry.Alias, pw)
			}
			return pw, err
		}))
		methods = append(methods, ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i, q := range questions {
				pw, err := managerPrompt(entry.Alias, "ssh", q)
				if err != nil {
					return nil, err
				}
				answers[i] = pw
			}
			return answers, nil
		}))
	}

	return methods, closers, nil
}

func loadSigner(identityFile, alias string, managerPrompt PasswordRequestFunc) (ssh.Signer, error) {
	keyBytes, err := os.ReadFile(identityFile)
	if err != nil {
		return nil, fmt.Errorf("read identity file %s for %s: %w", identityFile, alias, err)
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err == nil {
		return signer, nil
	}

	var passphraseErr *ssh.PassphraseMissingError
	if !errors.As(err, &passphraseErr) {
		return nil, fmt.Errorf("parse identity file %s for %s: %w", identityFile, alias, err)
	}
	if managerPrompt == nil {
		return nil, fmt.Errorf("identity %s requires passphrase for %s, but no prompt callback is configured", identityFile, alias)
	}

	passphrase, promptErr := managerPrompt(alias, "ssh", "SSH key passphrase:")
	if promptErr != nil {
		return nil, promptErr
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("parse encrypted identity file %s for %s: %w", identityFile, alias, err)
	}
	return signer, nil
}

func closeAll(closers []io.Closer) {
	for _, c := range closers {
		_ = c.Close()
	}
}

func (s *sshSession) Exec(ctx context.Context, command string, opts ExecOptions, subQuery ExecuteSubQueryFunc) (ExecResult, error) {
	execCWD := strings.TrimSpace(opts.WorkingDir)
	if execCWD == "" {
		execCWD = s.currentCWD()
	}
	if execCWD == "" {
		execCWD = "~"
	}

	session, stdin, stdout, stderr, err := s.openExecSession()
	if err != nil {
		return ExecResult{}, err
	}
	defer func() {
		_ = stdin.Close()
		_ = session.Close()
	}()

	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	metaToken := "__ARG_M_" + id + "__"
	wrapped := buildShellExecCommand(command, execCWD, metaToken, opts.Shell)

	if err := session.Start(wrapped); err != nil {
		return ExecResult{}, fmt.Errorf("failed to start remote command: %w", err)
	}

	chunks := make(chan outputChunk, 128)
	var readWG sync.WaitGroup
	readWG.Add(2)
	go s.pumpOutput(stdout, false, chunks, &readWG)
	go s.pumpOutput(stderr, true, chunks, &readWG)
	go func() {
		defer func() { recover() }() //nolint:errcheck
		readWG.Wait()
		close(chunks)
	}()

	waitCh := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				waitCh <- fmt.Errorf("session.Wait panicked: %v", r)
			}
		}()
		waitCh <- session.Wait()
	}()

	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	recentLines := make([]string, 0, 40)
	maxRetries := 3
	retryCount := 0
	waitDone := false
	chunksDone := false
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for !(waitDone && chunksDone) {
		select {
		case <-ctx.Done():
			_ = session.Close()
			return ExecResult{}, ctx.Err()
		case chunk, ok := <-chunks:
			if !ok {
				chunksDone = true
				continue
			}
			if chunk.isErr {
				stderrBuf.WriteString(chunk.text)
			} else {
				stdoutBuf.WriteString(chunk.text)
				if opts.ChunkCallback != nil {
					opts.ChunkCallback(chunk.text)
				}
			}

			split := strings.Split(strings.ReplaceAll(chunk.text, "\r\n", "\n"), "\n")
			recentLines = append(recentLines, split...)
			if len(recentLines) > 40 {
				recentLines = recentLines[len(recentLines)-40:]
			}
		case waitErr := <-waitCh:
			waitDone = true
			if waitErr != nil {
				var exitErr *ssh.ExitError
				if !errors.As(waitErr, &exitErr) || exitErr.ExitStatus() != 0 {
					return ExecResult{}, fmt.Errorf("remote command failed to complete: %w", waitErr)
				}
			}
		case <-ticker.C:
			if subQuery == nil || retryCount >= maxRetries || len(recentLines) == 0 {
				continue
			}
			retryCount++
			snapshot := strings.Join(recentLines, "\n")
			systemPrompt := "You are a terminal automation assistant. Analyze the terminal output and provide the NECESSARY input string to proceed with the command. ONLY return the input string itself (e.g., 'y', '1', or ''). Do not add any explanation."
			userPrompt := fmt.Sprintf("Command: %s\nTerminal Output:\n%s\n\nWhat should I type to proceed?", command, snapshot)
			response, err := subQuery(ctx, systemPrompt, userPrompt)
			if err != nil {
				continue
			}
			response = strings.TrimSpace(response)
			if response == "" {
				continue
			}
			_, _ = io.WriteString(stdin, response+"\n")
		}
	}

	parsedStdout, code, pwd, user, err := parseExecOutput(stdoutBuf.String(), metaToken)
	if err != nil {
		return ExecResult{
			Stdout: strings.TrimSpace(stdoutBuf.String()),
			Stderr: strings.TrimSpace(stderrBuf.String()),
			Code:   1,
			CWD:    s.currentCWD(),
			User:   s.currentUser(),
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
		Stderr: strings.TrimSpace(stderrBuf.String()),
		Code:   code,
		CWD:    pwd,
		User:   user,
	}, nil
}

func (s *sshSession) StartExec(ctx context.Context, command string, opts ExecOptions, subQuery ExecuteSubQueryFunc) (*ExecHandle, error) {
	_ = subQuery

	execCWD := strings.TrimSpace(opts.WorkingDir)
	if execCWD == "" {
		execCWD = s.currentCWD()
	}
	if execCWD == "" {
		execCWD = "~"
	}

	session, stdin, stdout, stderr, err := s.openExecSession()
	if err != nil {
		return nil, err
	}

	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	metaToken := "__ARG_M_" + id + "__"
	wrapped := buildShellExecCommand(command, execCWD, metaToken, opts.Shell)
	if err := session.Start(wrapped); err != nil {
		_ = stdin.Close()
		_ = session.Close()
		return nil, fmt.Errorf("failed to start remote command: %w", err)
	}

	streamCh := make(chan string, 512)
	resultCh := make(chan ExecResult, 1)
	var killOnce sync.Once
	killFn := func() {
		killOnce.Do(func() {
			_ = session.Close()
		})
	}

	handle := &ExecHandle{
		Stream: streamCh,
		Result: resultCh,
		Write: func(input string) error {
			if stdin == nil {
				return fmt.Errorf("stdin is unavailable")
			}
			_, err := io.WriteString(stdin, input)
			return err
		},
		Kill: killFn,
	}

	go func() {
		defer close(streamCh)
		defer close(resultCh)
		defer func() {
			_ = stdin.Close()
			_ = session.Close()
		}()

		chunks := make(chan outputChunk, 256)
		var readWG sync.WaitGroup
		readWG.Add(2)
		go s.pumpOutput(stdout, false, chunks, &readWG)
		go s.pumpOutput(stderr, true, chunks, &readWG)
		go func() {
			readWG.Wait()
			close(chunks)
		}()

		waitCh := make(chan error, 1)
		go func() { waitCh <- session.Wait() }()

		var stdoutBuf strings.Builder
		var stderrBuf strings.Builder
		waitDone := false
		chunksDone := false

		for !(waitDone && chunksDone) {
			select {
			case <-ctx.Done():
				killFn()
				resultCh <- ExecResult{
					Code:   1,
					CWD:    s.currentCWD(),
					User:   s.currentUser(),
					Stderr: strings.TrimSpace(ctx.Err().Error()),
				}
				return
			case chunk, ok := <-chunks:
				if !ok {
					chunksDone = true
					continue
				}
				if chunk.isErr {
					stderrBuf.WriteString(chunk.text)
				} else {
					stdoutBuf.WriteString(chunk.text)
					if opts.ChunkCallback != nil {
						opts.ChunkCallback(chunk.text)
					}
				}
				streamCh <- chunk.text
			case waitErr := <-waitCh:
				waitDone = true
				if waitErr != nil {
					var exitErr *ssh.ExitError
					if !errors.As(waitErr, &exitErr) || exitErr.ExitStatus() != 0 {
						stderrText := strings.TrimSpace(stderrBuf.String())
						if stderrText == "" {
							stderrText = waitErr.Error()
						}
						resultCh <- ExecResult{
							Stdout: strings.TrimSpace(stdoutBuf.String()),
							Stderr: stderrText,
							Code:   1,
							CWD:    s.currentCWD(),
							User:   s.currentUser(),
						}
						return
					}
				}
			}
		}

		parsedStdout, code, pwd, user, parseErr := parseExecOutput(stdoutBuf.String(), metaToken)
		if parseErr != nil {
			stderrText := strings.TrimSpace(stderrBuf.String())
			if stderrText == "" {
				stderrText = parseErr.Error()
			}
			resultCh <- ExecResult{
				Stdout: strings.TrimSpace(stdoutBuf.String()),
				Stderr: stderrText,
				Code:   1,
				CWD:    s.currentCWD(),
				User:   s.currentUser(),
			}
			return
		}

		s.stateMu.Lock()
		if pwd != "" {
			s.cwd = pwd
		}
		if user != "" {
			s.user = user
		}
		s.stateMu.Unlock()

		resultCh <- ExecResult{
			Stdout: strings.TrimSpace(parsedStdout),
			Stderr: strings.TrimSpace(stderrBuf.String()),
			Code:   code,
			CWD:    pwd,
			User:   user,
		}
	}()

	return handle, nil
}

func (s *sshSession) pumpOutput(r io.Reader, isErr bool, out chan<- outputChunk, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 8192)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out <- outputChunk{text: string(buf[:n]), isErr: isErr}
		}
		if err != nil {
			return
		}
	}
}

func buildShellExecCommand(command, workingDir, metaToken, shellKind string) string {
	switch strings.ToLower(strings.TrimSpace(shellKind)) {
	case "powershell":
		return buildPowerShellExecCommand(command, workingDir, metaToken)
	default:
		return buildBashExecCommand(command, workingDir, metaToken)
	}
}

func buildBashExecCommand(command, workingDir, metaToken string) string {
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
	script.WriteString("exit 0\n")
	return "bash -lc " + shellQuote(script.String())
}

func buildPowerShellExecCommand(command, workingDir, metaToken string) string {
	var script strings.Builder
	script.WriteString("$ErrorActionPreference = 'Continue'\n")
	if strings.TrimSpace(workingDir) != "" {
		script.WriteString(fmt.Sprintf("try { Set-Location -LiteralPath %s } catch {}\n", powerShellQuote(workingDir)))
	}
	script.WriteString(command)
	if !strings.HasSuffix(command, "\n") {
		script.WriteString("\n")
	}
	script.WriteString("$__arg_ec = $LASTEXITCODE\n")
	script.WriteString("if ($null -eq $__arg_ec) { if ($?) { $__arg_ec = 0 } else { $__arg_ec = 1 } }\n")
	script.WriteString("$__arg_pwd = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes((Get-Location).Path))\n")
	script.WriteString("$__arg_user = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes((whoami)))\n")
	script.WriteString(fmt.Sprintf("Write-Output \"`n%s:$($__arg_ec):$($__arg_pwd):$($__arg_user)\"\n", metaToken))
	script.WriteString("exit 0\n")
	return "powershell -NoProfile -NonInteractive -EncodedCommand " + encodePowerShellEncodedCommand(script.String())
}

func powerShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func encodePowerShellEncodedCommand(script string) string {
	runes := utf16.Encode([]rune(script))
	utf16le := make([]byte, 0, len(runes)*2)
	for _, r := range runes {
		utf16le = append(utf16le, byte(r), byte(r>>8))
	}
	return base64.StdEncoding.EncodeToString(utf16le)
}

func parseExecOutput(output, metaToken string) (string, int, string, string, error) {
	metaPrefix := metaToken + ":"
	metaIdx := strings.LastIndex(output, metaPrefix)
	if metaIdx < 0 {
		return "", 0, "", "", fmt.Errorf("remote execution metadata missing")
	}

	metaPart := output[metaIdx+len(metaPrefix):]
	metaLine := metaPart
	if nl := strings.IndexAny(metaLine, "\r\n"); nl >= 0 {
		metaLine = metaLine[:nl]
	}

	parts := strings.SplitN(strings.TrimSpace(metaLine), ":", 3)
	if len(parts) != 3 {
		return "", 0, "", "", fmt.Errorf("invalid remote execution metadata")
	}

	code, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", 0, "", "", fmt.Errorf("invalid remote exit code: %w", err)
	}
	pwd, err := decodeBase64(parts[1])
	if err != nil {
		return "", 0, "", "", fmt.Errorf("decode remote cwd: %w", err)
	}
	pwd = strings.TrimSpace(pwd)
	user, err := decodeBase64(parts[2])
	if err != nil {
		return "", 0, "", "", fmt.Errorf("decode remote user: %w", err)
	}
	user = strings.TrimSpace(user)

	cleaned := strings.TrimRight(output[:metaIdx], "\r\n")
	return cleaned, code, pwd, user, nil
}

func (s *sshSession) openExecSession() (*ssh.Session, io.WriteCloser, io.Reader, io.Reader, error) {
	session, err := s.client.NewSession()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create SSH session channel: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 80, 40, modes); err != nil {
		_ = session.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to request PTY: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to open stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to open stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to open stderr pipe: %w", err)
	}
	return session, stdin, stdout, stderr, nil
}

func (s *sshSession) readFileSFTP(path string) ([]byte, error) {
	file, err := s.openFileForReadSFTP(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func (s *sshSession) writeFileSFTP(path string, content []byte, overwrite bool) error {
	file, err := s.openFileForWriteSFTP(path, overwrite)
	if err != nil {
		return err
	}
	
	// 스트리밍 전송
	_, err = io.Copy(file, bytes.NewReader(content))
	file.Close() // 먼저 닫아야 디스크에 완전히 쓰임
	if err != nil {
		return fmt.Errorf("write copy failed: %w", err)
	}

	// [보강] sftp 클라이언트를 직접 사용하여 파일 크기 검증
	client, err := sftp.NewClient(s.client)
	if err != nil {
		return nil // 클라이언트 생성 실패는 검증만 포기 (데이터는 이미 보냄)
	}
	defer client.Close()

	info, err := client.Stat(path)
	if err == nil {
		if info.Size() != int64(len(content)) {
			return fmt.Errorf("size mismatch: wrote %d, expected %d", info.Size(), len(content))
		}
	}

	return nil
}


func (s *sshSession) openFileForReadSFTP(path string) (io.ReadCloser, error) {
	client, err := sftp.NewClient(s.client)
	if err != nil {
		return nil, fmt.Errorf("open sftp client: %w", err)
	}

	file, err := client.Open(path)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &sftpReadCloser{file: file, client: client}, nil
}

func (s *sshSession) openFileForWriteSFTP(path string, overwrite bool) (io.WriteCloser, error) {
	client, err := sftp.NewClient(s.client)
	if err != nil {
		return nil, fmt.Errorf("open sftp client: %w", err)
	}

	if !overwrite {
		if _, statErr := client.Stat(path); statErr == nil {
			_ = client.Close()
			return nil, fmt.Errorf("destination already exists: %s", path)
		}
	}

	dir := pathpkgDir(path)
	if dir != "" && dir != "." {
		if err := client.MkdirAll(dir); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("create remote directory %s: %w", dir, err)
		}
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	file, err := client.OpenFile(path, flags)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &sftpWriteCloser{file: file, client: client}, nil
}

type sftpReadCloser struct {
	file   *sftp.File
	client *sftp.Client
}

func (c *sftpReadCloser) Read(p []byte) (int, error) {
	return c.file.Read(p)
}

func (c *sftpReadCloser) Close() error {
	fileErr := c.file.Close()
	clientErr := c.client.Close()
	if fileErr != nil {
		return fileErr
	}
	return clientErr
}

type sftpWriteCloser struct {
	file   *sftp.File
	client *sftp.Client
}

func (c *sftpWriteCloser) Write(p []byte) (int, error) {
	return c.file.Write(p)
}

func (c *sftpWriteCloser) Close() error {
	fileErr := c.file.Close()
	clientErr := c.client.Close()
	if fileErr != nil {
		return fileErr
	}
	return clientErr
}

func (s *sshSession) uploadFileSFTP(localPath, remotePath string, overwrite bool) error {
	src, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := s.openFileForWriteSFTP(remotePath, overwrite)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func (s *sshSession) downloadFileSFTP(remotePath, localPath string, overwrite bool) error {
	src, err := s.openFileForReadSFTP(remotePath)
	if err != nil {
		return err
	}
	defer src.Close()

	if !overwrite {
		if _, err := os.Stat(localPath); err == nil {
			return fmt.Errorf("destination already exists: %s", localPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if !overwrite {
		flags = os.O_CREATE | os.O_WRONLY | os.O_EXCL
	}
	dst, err := os.OpenFile(localPath, flags, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func (s *sshSession) listDirSFTP(root string, recursive bool, depth int) ([]FileEntry, error) {
	client, err := sftp.NewClient(s.client)
	if err != nil {
		return nil, fmt.Errorf("open sftp client: %w", err)
	}
	defer client.Close()

	base := strings.TrimSpace(root)
	if base == "" {
		base = "."
	}

	out := make([]FileEntry, 0, 64)
	var walk func(current string, currentDepth int) error
	walk = func(current string, currentDepth int) error {
		items, err := client.ReadDir(current)
		if err != nil {
			return err
		}
		for _, item := range items {
			itemPath := path.Join(current, item.Name())
			out = append(out, FileEntry{
				Name:    item.Name(),
				Path:    itemPath,
				IsDir:   item.IsDir(),
				Size:    item.Size(),
				Mode:    item.Mode().String(),
				ModTime: item.ModTime(),
			})
			if !recursive || !item.IsDir() {
				continue
			}
			nextDepth := currentDepth + 1
			if depth > 0 && nextDepth >= depth {
				continue
			}
			if err := walk(itemPath, nextDepth); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(base, 0); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func pathpkgDir(p string) string {
	cleaned := strings.TrimSpace(p)
	if cleaned == "" {
		return ""
	}
	return path.Dir(cleaned)
}

func (s *sshSession) openTunnel(localAddr, remoteAddr string) (TunnelInfo, error) {
	if strings.TrimSpace(localAddr) == "" {
		localAddr = "127.0.0.1:0"
	}
	if strings.TrimSpace(remoteAddr) == "" {
		return TunnelInfo{}, fmt.Errorf("remote address is required")
	}

	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return TunnelInfo{}, err
	}

	id := strconv.FormatUint(s.tunnelSeq.Add(1), 36)
	tunnel := &sshTunnel{
		id:         id,
		localAddr:  listener.Addr().String(),
		remoteAddr: remoteAddr,
		listener:   listener,
		done:       make(chan struct{}),
	}

	s.tunnelsMu.Lock()
	s.tunnels[id] = tunnel
	s.tunnelsMu.Unlock()

	go s.runTunnel(tunnel)

	return TunnelInfo{
		ID:         id,
		Alias:      s.entry.Alias,
		LocalAddr:  tunnel.localAddr,
		RemoteAddr: tunnel.remoteAddr,
		Active:     true,
	}, nil
}

func (s *sshSession) runTunnel(tunnel *sshTunnel) {
	defer close(tunnel.done)
	for {
		conn, err := tunnel.listener.Accept()
		if err != nil {
			if tunnel.closed.Load() {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Temporary() {
				continue
			}
			return
		}
		go s.proxyConnection(conn, tunnel.remoteAddr)
	}
}

func (s *sshSession) proxyConnection(localConn net.Conn, remoteAddr string) {
	remoteConn, err := s.client.Dial("tcp", remoteAddr)
	if err != nil {
		_ = localConn.Close()
		return
	}

	go func() {
		defer func() { recover() }() //nolint:errcheck
		_, _ = io.Copy(remoteConn, localConn)
		_ = remoteConn.Close()
	}()
	go func() {
		defer func() { recover() }() //nolint:errcheck
		_, _ = io.Copy(localConn, remoteConn)
		_ = localConn.Close()
	}()
}

func (s *sshSession) closeTunnel(id string) error {
	s.tunnelsMu.Lock()
	tunnel, ok := s.tunnels[id]
	if ok {
		delete(s.tunnels, id)
	}
	s.tunnelsMu.Unlock()
	if !ok {
		return fmt.Errorf("unknown tunnel id: %s", id)
	}
	return tunnel.close()
}

func (s *sshSession) listTunnels() []TunnelInfo {
	s.tunnelsMu.RLock()
	defer s.tunnelsMu.RUnlock()
	out := make([]TunnelInfo, 0, len(s.tunnels))
	for _, t := range s.tunnels {
		out = append(out, TunnelInfo{
			ID:         t.id,
			Alias:      s.entry.Alias,
			LocalAddr:  t.localAddr,
			RemoteAddr: t.remoteAddr,
			Active:     !t.closed.Load(),
		})
	}
	return out
}

func (s *sshSession) closeTunnels() {
	s.tunnelsMu.Lock()
	tunnels := make([]*sshTunnel, 0, len(s.tunnels))
	for _, t := range s.tunnels {
		tunnels = append(tunnels, t)
	}
	s.tunnels = make(map[string]*sshTunnel)
	s.tunnelsMu.Unlock()

	for _, t := range tunnels {
		_ = t.close()
		select {
		case <-t.done:
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *sshSession) Close() error {
	s.closeOnce.Do(func() {
		s.closeTunnels()
		if s.client != nil {
			s.closeErr = s.client.Close()
		}
		closeAll(s.authClosers)
	})
	return s.closeErr
}

func (s *sshSession) currentCWD() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.cwd
}

func (s *sshSession) currentUser() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.user
}

func decodeBase64(raw string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// knownHostsPath 는 argus 전용 SSH known_hosts 파일 경로를 반환한다.
func knownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// 홈 디렉터리를 알 수 없으면 현재 디렉터리 아래 .Argus 사용
		return filepath.Join(".Argus", "ssh_known_hosts")
	}
	return filepath.Join(home, ".Argus", "ssh_known_hosts")
}

// tufoHostKeyCallback 은 TOFU(Trust On First Use) 방식의 SSH 호스트 키 콜백을 반환한다.
// - 최초 접속: 호스트 키를 knownHostsPath 에 저장하고 허용.
// - 재접속: 저장된 키와 일치하면 허용, 변경되었으면 거부(MITM 경고).
func tufoHostKeyCallback(knownHostsFile string) (ssh.HostKeyCallback, error) {
	if err := os.MkdirAll(filepath.Dir(knownHostsFile), 0o700); err != nil {
		return nil, fmt.Errorf("create known_hosts dir: %w", err)
	}
	// 파일이 없으면 생성
	f, err := os.OpenFile(knownHostsFile, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open known_hosts: %w", err)
	}
	f.Close()

	cb, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("parse known_hosts: %w", err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := cb(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			// 저장된 키와 다름 — MITM 공격 가능성
			return fmt.Errorf(
				"SSH 호스트 키가 변경되었습니다 (%s). MITM 공격 가능성. %s 에서 이전 항목을 삭제 후 재시도하세요",
				hostname, knownHostsFile,
			)
		}
		// 알 수 없는 호스트 — TOFU: 저장 후 허용
		wf, werr := os.OpenFile(knownHostsFile, os.O_APPEND|os.O_WRONLY, 0o600)
		if werr != nil {
			return fmt.Errorf("호스트 키 저장 실패 (%s): %w", hostname, werr)
		}
		defer wf.Close()
		line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
		if _, werr := fmt.Fprintln(wf, line); werr != nil {
			return fmt.Errorf("호스트 키 기록 실패 (%s): %w", hostname, werr)
		}
		return nil
	}, nil
}
