// Package precheck
// File: checker_binary.go
// Description: non-standard 바이너리의 PATH 존재 여부를 사전 검증하는 Checker 구현
// Responsibility: command -v를 사용해 실행 파일이 원격 호스트의 PATH에 있는지 확인한다.

package precheck

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

// BinaryExistsChecker verifies that a binary is available in PATH on the remote host.
// Uses `command -v` (POSIX) with `type` as a bash built-in fallback.
type BinaryExistsChecker struct {
	binary   string
	severity Severity
}

// NewBinaryExistsChecker constructs a BinaryExistsChecker for the given binary name.
func NewBinaryExistsChecker(binary string, sev Severity) *BinaryExistsChecker {
	return &BinaryExistsChecker{binary: binary, severity: sev}
}

func (c *BinaryExistsChecker) Kind() CheckKind    { return KindBinaryExists }
func (c *BinaryExistsChecker) Subject() string    { return c.binary }
func (c *BinaryExistsChecker) Severity() Severity { return c.severity }

// Run checks whether the binary exists in PATH on the remote executor.
func (c *BinaryExistsChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	q := shellQuote(c.binary)
	cmd := fmt.Sprintf("command -v %s >/dev/null 2>&1 || type %s >/dev/null 2>&1", q, q)
	result, err := exec.Execute(ctx, cmd)
	ok := err == nil && result.ExitCode == 0
	if ok {
		return CheckResult{Kind: c.Kind(), Subject: c.binary, OK: true, Severity: c.severity}
	}
	msg := fmt.Sprintf(
		"binary %q not found in PATH — check that the relevant HOME/bin directory is in $PATH, or use an absolute path",
		c.binary,
	)
	return CheckResult{Kind: c.Kind(), Subject: c.binary, OK: false, Severity: c.severity, Message: msg}
}
