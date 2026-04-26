// Package precheck
// File: checker_rm_safety.go
// Description: 위험한 rm 명령(rm -rf / 등)을 감지해 실행을 차단하는 Checker 구현
// Responsibility: 감지된 패턴 자체가 위험 신호이므로 Run()은 항상 Block 실패를 반환한다.

package precheck

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

// RmSafetyChecker blocks commands that match dangerous rm patterns (e.g., rm -rf /).
// The checker's existence already implies the pattern was matched; Run always fails.
type RmSafetyChecker struct {
	pattern  string
	severity Severity
}

// NewRmSafetyChecker constructs an RmSafetyChecker for the detected dangerous pattern.
func NewRmSafetyChecker(pattern string, sev Severity) *RmSafetyChecker {
	return &RmSafetyChecker{pattern: pattern, severity: sev}
}

func (c *RmSafetyChecker) Kind() CheckKind    { return KindRmSafety }
func (c *RmSafetyChecker) Subject() string    { return c.pattern }
func (c *RmSafetyChecker) Severity() Severity { return c.severity }

// Run always returns a failing result because the checker is instantiated only when
// a dangerous rm pattern has already been detected.
func (c *RmSafetyChecker) Run(_ context.Context, _ executor.Executor) CheckResult {
	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.pattern,
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"dangerous rm command detected: %q — this will permanently delete files",
			c.pattern,
		),
	}
}
