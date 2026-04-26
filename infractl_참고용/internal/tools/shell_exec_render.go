// Package tools
// File: shell_exec_render.go
// Description: shell_exec 결과 렌더링 및 명령어 패턴 판별 헬퍼
// Responsibility: 실행 결과 포맷팅 + SCP/파일전송 명령 탐지

package tools

import (
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// renderExecResult는 실행 결과를 사람이 읽기 쉬운 형태로 포맷한다.
func renderExecResult(exec executor.Executor, result executor.ExecResult) string {
	lines := []string{
		fmt.Sprintf("Execution Context: %s", executor.ExecutionContextLabel(exec)),
		fmt.Sprintf("[Exit Code: %d]", result.ExitCode),
	}
	stdout := strings.TrimSpace(result.Stdout)
	stderr := strings.TrimSpace(result.Stderr)
	if stdout != "" {
		lines = append(lines, stdout)
	}
	if stderr != "" {
		lines = append(lines, "[stderr]", stderr)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// isLocalSCPCommand는 cmd가 scp 호출인지 확인한다.
// localhost에서 실행되는 scp는 SSH 비밀번호 프롬프트가 /dev/tty로 출력돼
// idle handler가 감지하지 못하고 무한루프에 빠진다.
func isLocalSCPCommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	return trimmed == "scp" ||
		strings.HasPrefix(trimmed, "scp ") ||
		strings.HasPrefix(trimmed, "scp\t")
}

// isFileTransferCommand는 명령에 파일 작업 바이너리 호출이 포함되어 있는지 확인한다.
func isFileTransferCommand(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, token := range []string{"scp ", "scp\t", "scp\r", "scp\n", "scp{", "unzip ", "cp ", "mv ", "rm "} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	if strings.Contains(lower, "scp.exe") {
		return true
	}
	return false
}
