//go:build tools
// +build tools

// Package executor_test
// File: linux_scenario_test.go
// Description: Linux shell execution scenarios virtual test
// Responsibility: Simulate sudo, env vars, and stateful shell sessions for Linux

package executor_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
)

// mockLinuxSession simulates a stateful Linux shell session.
type mockLinuxSession struct {
	user    string
	cwd     string
	env     map[string]string
	history []string
}

func newMockLinuxSession() *mockLinuxSession {
	return &mockLinuxSession{
		user: "user",
		cwd:  "/home/user",
		env:  make(map[string]string),
	}
}

func (s *mockLinuxSession) execute(command string) (executor.ShellRunResult, error) {
	s.history = append(s.history, command)
	result := executor.ShellRunResult{
		CurrentUser: s.user,
		CurrentDir:  s.cwd,
		ExitCode:    0,
	}

	cmd := strings.TrimSpace(command)
	switch {
	case cmd == "whoami":
		result.Stdout = s.user
	case cmd == "pwd":
		result.Stdout = s.cwd
	case strings.HasPrefix(cmd, "cd "):
		newDir := strings.TrimPrefix(cmd, "cd ")
		if strings.HasPrefix(newDir, "/") {
			s.cwd = newDir
		} else {
			s.cwd = s.cwd + "/" + newDir
		}
		result.CurrentDir = s.cwd
	case strings.HasPrefix(cmd, "export "):
		parts := strings.SplitN(strings.TrimPrefix(cmd, "export "), "=", 2)
		if len(parts) == 2 {
			s.env[parts[0]] = parts[1]
		}
	case strings.HasPrefix(cmd, "echo $"):
		varName := strings.TrimPrefix(cmd, "echo $")
		result.Stdout = s.env[varName]
	case cmd == "cat /etc/shadow":
		if s.user != "root" {
			result.ExitCode = 1
			result.Stdout = "cat: /etc/shadow: Permission denied"
		} else {
			result.Stdout = "root:$6$randomsalt$encryptedpassword:12345:0:99999:7:::"
		}
	case cmd == "sudo -i":
		s.user = "root"
		s.cwd = "/root"
		result.CurrentUser = s.user
		result.CurrentDir = s.cwd
	case cmd == "apt install git":
		// 대화형 시뮬레이션을 위해 onIdle/onChunk 같은 메커니즘이 필요하지만
		// 여기서는 간단히 history에 "Y"가 있는지 확인하여 결과 반환
		result.Stdout = "Reading package lists... Done\nBuilding dependency tree\nNeed to get 10MB of archives. After this operation, 30MB of additional disk space will be used. Do you want to continue? [Y/n]"
		// 실제 PersistentSessionExecutor는 여기서 실행을 멈추고 입력을 기다림.
		// 테스트를 위해 일단 history만 기록
	case cmd == "Y":
		result.Stdout = "Selecting previously unselected package git.\nSetting up git (1:2.34.1-1ubuntu1) ...\nDone."
	default:
		result.Stdout = fmt.Sprintf("executed: %s", cmd)
	}

	return result, nil
}

func TestLinuxScenarios(t *testing.T) {
	session := newMockLinuxSession()

	t.Run("StandardCommand", func(t *testing.T) {
		res, _ := session.execute("whoami")
		if res.Stdout != "user" {
			t.Errorf("expected user, got %s", res.Stdout)
		}
	})

	t.Run("InteractivePrompt", func(t *testing.T) {
		// 1. 명령 실행
		res, _ := session.execute("apt install git")
		if !strings.Contains(res.Stdout, "Do you want to continue?") {
			t.Errorf("expected interactive prompt, got %s", res.Stdout)
		}

		// 2. 입력 주입 (Y)
		res, _ = session.execute("Y")
		if !strings.Contains(res.Stdout, "Setting up git") {
			t.Errorf("expected success after Y, got %s", res.Stdout)
		}
	})

	t.Run("PermissionDenied", func(t *testing.T) {
		res, _ := session.execute("cat /etc/shadow")
		if res.ExitCode == 0 {
			t.Error("expected failure for non-root user")
		}
		if !strings.Contains(res.Stdout, "Permission denied") {
			t.Errorf("expected permission denied message, got %s", res.Stdout)
		}
	})

	t.Run("PrivilegeEscalation", func(t *testing.T) {
		// sudo -i 시뮬레이션
		res, _ := session.execute("sudo -i")
		if res.CurrentUser != "root" {
			t.Errorf("expected user to be root, got %s", res.CurrentUser)
		}
		if res.CurrentDir != "/root" {
			t.Errorf("expected dir to be /root, got %s", res.CurrentDir)
		}

		// 이제 shadow 파일 읽기 성공해야 함
		res, _ = session.execute("cat /etc/shadow")
		if res.ExitCode != 0 {
			t.Errorf("expected success for root user, got exit code %d", res.ExitCode)
		}
		if !strings.Contains(res.Stdout, "root:$6$") {
			t.Errorf("expected shadow content, got %s", res.Stdout)
		}
	})

	t.Run("StatePersistence", func(t *testing.T) {
		// 디렉토리 이동
		session.execute("cd /tmp/test")
		res, _ := session.execute("pwd")
		if res.Stdout != "/tmp/test" {
			t.Errorf("expected /tmp/test, got %s", res.Stdout)
		}

		// 환경 변수 유지
		session.execute("export FOO=bar")
		res, _ = session.execute("echo $FOO")
		if res.Stdout != "bar" {
			t.Errorf("expected bar, got %s", res.Stdout)
		}
	})
}
