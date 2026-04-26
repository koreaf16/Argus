// Package precheck
// File: checker_process_exists.go
// Description: kill/pkill 실행 전 대상 프로세스(PID 또는 이름)의 존재 여부를 검증하는 Checker 구현
// Responsibility: kill -0 또는 pgrep -x를 사용해 프로세스 존재를 확인한다.

package precheck

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

// ProcessExistsChecker verifies that a target process exists before kill/pkill runs.
// For numeric PIDs it uses `kill -0`; for names it uses `pgrep -x`.
type ProcessExistsChecker struct {
	target   string
	isPid    bool
	severity Severity
}

// NewProcessExistsChecker constructs a ProcessExistsChecker.
// isPid=true for numeric PIDs, false for process names.
func NewProcessExistsChecker(target string, isPid bool, sev Severity) *ProcessExistsChecker {
	return &ProcessExistsChecker{target: target, isPid: isPid, severity: sev}
}

func (c *ProcessExistsChecker) Kind() CheckKind    { return KindProcessExists }
func (c *ProcessExistsChecker) Subject() string    { return c.target }
func (c *ProcessExistsChecker) Severity() Severity { return c.severity }

// Run checks whether the target process exists using kill -0 (PID) or pgrep -x (name).
func (c *ProcessExistsChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	var cmd string
	if c.isPid {
		cmd = fmt.Sprintf("kill -0 %s 2>/dev/null", shellQuote(c.target))
	} else {
		cmd = fmt.Sprintf("pgrep -x %s > /dev/null 2>&1", shellQuote(c.target))
	}

	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		// Cannot determine — let it through.
		return CheckResult{Kind: c.Kind(), Subject: c.target, OK: true, Severity: c.severity}
	}

	if result.ExitCode == 0 {
		return CheckResult{Kind: c.Kind(), Subject: c.target, OK: true, Severity: c.severity}
	}

	var msg string
	if c.isPid {
		msg = fmt.Sprintf("process with PID %q does not exist", c.target)
	} else {
		msg = fmt.Sprintf("no process named %q is currently running", c.target)
	}

	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.target,
		OK:       false,
		Severity: c.severity,
		Message:  msg,
	}
}
