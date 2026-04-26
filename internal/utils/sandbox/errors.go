// Package sandbox — Sandbox error types.
//
// 파일 역할: Defines error types for sandbox operations.
// 포함 모듈:
//   - ErrSandboxingNotImplemented: Returned when sandboxing is not available
//
// 호출/사용 방식:
//   - sandbox_adapter.go and other sandbox functions
//
// 연결:
//   - internal/utils/sandbox
//   - 원본: error definitions from src/utils/sandbox/
package sandbox

import "errors"

// ErrSandboxingNotImplemented is returned when sandboxing is requested but not available.
var ErrSandboxingNotImplemented = errors.New("sandboxing not implemented in Phase 1")

// ErrSandboxCreationFailed is returned when sandbox creation fails.
var ErrSandboxCreationFailed = errors.New("failed to create sandbox")

// ErrSandboxExecutionFailed is returned when command execution in sandbox fails.
var ErrSandboxExecutionFailed = errors.New("sandbox execution failed")

// ErrSandboxTimeout is returned when sandbox execution times out.
var ErrSandboxTimeout = errors.New("sandbox execution timeout")
