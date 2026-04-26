// Package tools
// File: shell_exec_defensive.go
// Description: Shell 명령 실행 전 방어 로직(Defensive Logic) 구현
// Responsibility: 대량 출력 방지, 사전 권한 체크, 비동기 실행 래핑

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/executor"
)

// DefensiveTactic 정의
type DefensiveTactic struct {
	OriginalCommand string
	ModifiedCommand string
	IsBackground    bool
	LogPath         string
	Warnings        []string
}

// ApplyDefensiveTactics는 명령어를 분석하여 안전한 형태로 변경한다.
func ApplyDefensiveTactics(cmd string, isBackground bool) *DefensiveTactic {
	tactic := &DefensiveTactic{
		OriginalCommand: cmd,
		ModifiedCommand: cmd,
		IsBackground:    isBackground,
	}

	// 1. 대량 출력 방지 (Anti-Flood)
	tactic.applyAntiFlood()

	// 2. 비동기 실행 래핑 (Background)
	if isBackground {
		tactic.applyBackgroundWrapper()
	}

	return tactic
}

// applyAntiFlood는 대량 출력이 예상되는 명령어에 quiet 플래그를 추가한다.
func (t *DefensiveTactic) applyAntiFlood() {
	cmd := t.ModifiedCommand
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	baseCmd := filepath.Base(parts[0])
	switch baseCmd {
	case "unzip":
		if !strings.Contains(cmd, " -q") {
			t.ModifiedCommand = strings.Replace(cmd, "unzip", "unzip -q", 1)
			t.Warnings = append(t.Warnings, "Added '-q' to unzip to prevent session flooding.")
		}
	case "tar":
		if strings.Contains(cmd, "-v") || strings.Contains(cmd, "v") {
			// v 옵션 제거 시도
			t.ModifiedCommand = strings.Replace(cmd, "v", "", 1)
			t.Warnings = append(t.Warnings, "Removed 'v' flag from tar to prevent session flooding.")
		}
	case "cp", "mv":
		if strings.Contains(cmd, " -v") {
			t.ModifiedCommand = strings.Replace(cmd, " -v", "", -1)
			t.Warnings = append(t.Warnings, "Removed '-v' flag from copy/move command.")
		}
	}
}

// applyBackgroundWrapper는 nohup과 리다이렉션을 사용하여 비동기 실행을 구성한다.
func (t *DefensiveTactic) applyBackgroundWrapper() {
	logFile := fmt.Sprintf("/tmp/infractl_job_%d.log", time.Now().Unix())
	t.LogPath = logFile

	// nohup bash -c <cmd> > <log> 2>&1 & echo $!
	// 명령어 이스케이핑을 고려하여 구성
	t.ModifiedCommand = fmt.Sprintf("nohup bash -c %q > %s 2>&1 & echo $!", t.ModifiedCommand, logFile)
	t.Warnings = append(t.Warnings, fmt.Sprintf("Command wrapped with nohup. Logs: %s", logFile))
}

// PerformPreflightChecks는 실행 전 권한/경로 유효성을 검사한다.
func PerformPreflightChecks(ctx context.Context, exec executor.Executor, cmd string, becomeMethod string, user string) error {
	// 1. 계정 유효성 체크
	if user != "" && user != "root" {
		checkCmd := fmt.Sprintf("id %s", user)
		res, err := exec.Execute(ctx, checkCmd)
		if err != nil || res.ExitCode != 0 {
			return fmt.Errorf("pre-flight check failed: target user %q does not exist or is inaccessible", user)
		}
	}

	// 2. 쓰기 권한 체크 (현재 작업 디렉터리)
	checkWriteCmd := "[ -w . ] || echo 'NO_WRITE_PERMISSION'"
	res, err := exec.Execute(ctx, checkWriteCmd)
	if err == nil && strings.Contains(res.Stdout, "NO_WRITE_PERMISSION") {
		return fmt.Errorf("pre-flight check failed: no write permission in current directory")
	}

	// 3. command-level mutation path owner/mode preflight
	if err := ValidateCommandMutationPathAccess(ctx, exec, cmd, becomeMethod, user); err != nil {
		return fmt.Errorf("pre-flight check failed: %w", err)
	}

	// 4. critical path block + warn pass-through
	scan := ScanCommand(cmd)
	for _, targetPath := range scan.TargetPaths {
		warning, isCritical := CheckCriticalPath(targetPath)
		if warning == "" {
			continue
		}
		if isCritical {
			return fmt.Errorf("pre-flight check failed: %s", warning)
		}
		slog.Warn("pre-flight sensitive path warning", "path", targetPath, "warning", warning)
	}

	return nil
}
