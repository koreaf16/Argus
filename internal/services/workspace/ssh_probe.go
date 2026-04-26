package workspace

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/ssh"
)

func (s *sshSession) runProbeCommand(ctx context.Context, command string) (string, int, error) {
	session, err := s.client.NewSession()
	if err != nil {
		return "", 1, err
	}
	defer session.Close()

	type probeResult struct {
		out []byte
		err error
	}

	done := make(chan probeResult, 1)
	go func() {
		out, runErr := session.CombinedOutput(command)
		done <- probeResult{out: out, err: runErr}
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return "", 1, ctx.Err()
	case res := <-done:
		code := 0
		if res.err != nil {
			var exitErr *ssh.ExitError
			if errors.As(res.err, &exitErr) {
				code = exitErr.ExitStatus()
			} else {
				return string(res.out), 1, res.err
			}
		}
		return string(res.out), code, nil
	}
}

func (s *sshSession) detectPlatform(ctx context.Context) (platform string, evidence string, err error) {
	if out, _, probeErr := s.runProbeCommand(ctx, "cmd /c ver"); probeErr == nil {
		if p := classifyPlatformText(out); p != PlatformUnknown {
			return p, strings.TrimSpace(out), nil
		}
	}

	if out, code, probeErr := s.runProbeCommand(ctx, "uname -s"); probeErr == nil && code == 0 {
		if p := classifyPlatformText(out); p != PlatformUnknown {
			return p, strings.TrimSpace(out), nil
		}
	}

	if out, _, probeErr := s.runProbeCommand(ctx, "powershell -NoProfile -NonInteractive -Command \"$env:OS\""); probeErr == nil {
		if p := classifyPlatformText(out); p != PlatformUnknown {
			return p, strings.TrimSpace(out), nil
		}
	}

	return PlatformUnknown, "", nil
}

func (s *sshSession) detectBashAvailability(ctx context.Context) (bool, string, error) {
	out, code, err := s.runProbeCommand(ctx, "bash --version")
	if err != nil {
		return false, "", err
	}
	lower := strings.ToLower(out)
	if code == 0 && strings.Contains(lower, "bash") {
		return true, strings.TrimSpace(out), nil
	}
	return false, strings.TrimSpace(out), nil
}
