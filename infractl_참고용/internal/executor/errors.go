// Package executor
// File: errors.go
// Description: 실행 결과의 시맨틱 에러 분석 및 분류
// Responsibility: Exit Code와 Stderr를 분석하여 LLM이 이해하기 쉬운 에러 카테고리 및 힌트 제공

package executor

import (
	"regexp"
	"strings"
)

var (
	permissionFailureRegex = regexp.MustCompile(`(?i)(permission denied|operation not permitted|access denied|access is denied|not writable|current user lacks write access|read-only file system|eacces|cannot create|cannot open|failed to open|허가 거부)`)
)

const (
	CategoryNone       = "none"
	CategoryPermission = "permission"
	CategoryNotFound   = "not_found"
	CategoryTimeout    = "timeout"
	CategoryResource   = "resource"
	CategoryNetwork    = "network"
	CategoryInternal   = "internal_error"
)

// IsPermissionFailure reports whether the result indicates a privilege-related failure.
func IsPermissionFailure(res ExecResult) bool {
	if res.ErrorCategory == CategoryPermission {
		return true
	}
	text := strings.ToLower(res.Stderr + "\n" + res.Stdout)
	return permissionFailureRegex.MatchString(text)
}

// CategorizeResult는 실행 결과를 분석하여 에러 카테고리와 힌트를 설정한다.
func CategorizeResult(res *ExecResult) {
	if res.ExitCode == 0 {
		res.ErrorCategory = CategoryNone
		return
	}

	errText := strings.ToLower(res.Stderr + "\n" + res.Stdout)

	switch {
	case res.ExitCode == 126 || permissionFailureRegex.MatchString(errText):
		res.ErrorCategory = CategoryPermission
		res.ErrorHint = "💡 System Hint: Command failed due to permissions. Consider using 'become_user' or 'become_method: sudo'."

	case res.ExitCode == 127 || strings.Contains(errText, "command not found") || strings.Contains(errText, "no such file"):
		res.ErrorCategory = CategoryNotFound
		res.ErrorHint = "💡 System Hint: Target file or command was not found. Check the path and ensure the software is installed."

	case res.ExitCode == 124 || strings.Contains(errText, "timed out"):
		res.ErrorCategory = CategoryTimeout
		res.ErrorHint = "💡 System Hint: Command timed out. For long tasks, use 'submit_job' or increase the 'timeout' parameter."

	case strings.Contains(errText, "no space left") || strings.Contains(errText, "out of memory"):
		res.ErrorCategory = CategoryResource
		res.ErrorHint = "💡 System Hint: System is low on resources (Disk/Memory). Check 'df -h' or 'free -m'."

	case strings.Contains(errText, "connection refused") || strings.Contains(errText, "network is unreachable"):
		res.ErrorCategory = CategoryNetwork
		res.ErrorHint = "💡 System Hint: Network connectivity issue. Verify remote server availability and firewall settings."

	default:
		res.ErrorCategory = CategoryInternal
		res.ErrorHint = "💡 System Hint: Command failed with an unexpected error. Analyze the stderr above for details."
	}
}
