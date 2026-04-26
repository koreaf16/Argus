// Package bash — BashTool result message rendering.
//
// 파일 역할: Renders bash tool execution results and error messages.
// 포함 모듈:
//   - BashToolResultMessage: Message formatting for bash results
//
// 호출/사용 방식:
//   - bash.go calls after command execution
//
// 연결:
//   - 원본: src/tools/BashTool/BashToolResultMessage.tsx
package bash

// FormatBashResult formats a bash command result for display.
func FormatBashResult(stdout, stderr string, exitCode int) string {
	// Phase 1: Simple formatting
	return stdout
}
