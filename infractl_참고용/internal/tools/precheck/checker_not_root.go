// Package precheck
// File: checker_not_root.go
// Description: Oracle/DB 바이너리를 root로 실행하려는 경우 경고하는 Checker 구현
// Responsibility: id -u로 현재 UID를 확인해 root(0)이면 경고를 반환한다.

package precheck

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

// NotRootChecker warns when a sensitive binary (e.g., sqlplus, rman) is about to be
// executed as root. Oracle and similar tools should run as the dedicated OS user.
type NotRootChecker struct {
	binary   string
	severity Severity
}

// NewNotRootChecker constructs a NotRootChecker for the given binary name.
func NewNotRootChecker(binary string, sev Severity) *NotRootChecker {
	return &NotRootChecker{binary: binary, severity: sev}
}

func (c *NotRootChecker) Kind() CheckKind    { return KindNotRoot }
func (c *NotRootChecker) Subject() string    { return c.binary }
func (c *NotRootChecker) Severity() Severity { return c.severity }

// Run executes `test "$(id -u)" = "0"` to detect root execution.
// Exit code 0 means we ARE root — that is the failure condition.
func (c *NotRootChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	result, err := exec.Execute(ctx, `test "$(id -u)" = "0"`)
	if err != nil {
		// Cannot determine — let it through.
		return CheckResult{Kind: c.Kind(), Subject: c.binary, OK: true, Severity: c.severity}
	}

	if result.ExitCode != 0 {
		// Not root — safe.
		return CheckResult{Kind: c.Kind(), Subject: c.binary, OK: true, Severity: c.severity}
	}

	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.binary,
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"%q should not be run as root — switch to the oracle OS user first",
			c.binary,
		),
	}
}
