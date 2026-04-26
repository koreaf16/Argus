// Package hooks
// File: backend_command.go
// Description: Shell command hook backend 구현
// Responsibility: HookDef.Command 를 실행하고 stdin/stdout 으로 JSON 입출력

// Ported from: claude_cli/src/utils/hooks/execHttpHook.ts (command 패턴 참조)
// 보안: hook input JSON 은 stdin 으로만 전달 — 쉘 문자열 결합 절대 금지.

package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// commandBackend 는 shell 명령을 실행하는 hook backend 이다.
type commandBackend struct{}

// expandHome 은 경로 문자열 내의 ~ 를 사용자의 홈 디렉토리로 확장한다.
func expandHome(path string) string {
	if !strings.Contains(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	// 1) 시작이 ~ 인 경우
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		return filepath.Join(home, path[2:])
	}
	if path == "~" {
		return home
	}
	// 2) 중간에 포함된 경우 (예: & '~/...') - 단순 치환 시도
	return strings.ReplaceAll(path, "~/", home+"/")
}

// Run 은 HookDef.Command 를 시스템 쉘(sh 또는 cmd)로 실행하고 stdin 에 JSON input 을 전달한다.
func (b *commandBackend) Run(ctx context.Context, def HookDef, input HookInput) (HookOutput, error) {
	if strings.TrimSpace(def.Command) == "" {
		return HookOutput{}, fmt.Errorf("command hook: empty command")
	}

	command := expandHome(def.Command)

	// 2) Windows 특화 처리: .sh -> .ps1 전환 (특히 builtin system_risk 용)
	if runtime.GOOS == "windows" {
		if strings.Contains(command, "system_risk.sh") {
			command = strings.ReplaceAll(command, "system_risk.sh", "system_risk.ps1")
			// PowerShell 에서 스크립트 실행을 위해 & 연산자 보정 (이미 있으면 중복 방지)
			// 단, 이미 & 로 시작하거나 따옴표로 감싸진 경우를 고려하여 단순하게 처리
			if !strings.Contains(command, "&") && strings.HasSuffix(command, ".ps1") {
				command = fmt.Sprintf("& '%s'", command)
			}
		}
	}

	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return HookOutput{}, fmt.Errorf("marshal hook input: %w", err)
	}

	// 보안: json 은 stdin 으로만 전달 — exec.Command 인자에 삽입 금지
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Windows: PowerShell을 기본 쉘로 사용.
		// -NoProfile, -NonInteractive, -ExecutionPolicy Bypass 옵션으로 환경 독립성과 실행 권한 보장.
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command)
	} else {
		// Unix: sh -c 를 기본으로 사용.
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Stdin = bytes.NewReader(jsonBytes)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return HookOutput{}, fmt.Errorf("command hook exit: %w, stderr: %s", err, strings.TrimSpace(stderr.String()))
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		slog.Debug("command hook: empty stdout, treating as allow")
		return HookOutput{Decision: DecisionAllow}, nil
	}

	var result HookOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return HookOutput{}, fmt.Errorf("parse hook output JSON: %w, raw: %s", err, out)
	}
	return result, nil
}
