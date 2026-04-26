// Package precheck
// File: checker_sudo_allowed.go
// Description: 현재 계정의 sudo 권한을 원격 호스트에서 사전 검증하는 Checker 구현
// Responsibility: sudo -l로 대상 유저에 대한 sudo 허용 여부를 확인한다.

package precheck

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

// SudoAllowedChecker verifies that the current user has sudo permission for a target user.
// Severity is always Warn — sudo configuration is complex and a false negative should not block.
type SudoAllowedChecker struct {
	targetUser string
	severity   Severity
}

// NewSudoAllowedChecker constructs a SudoAllowedChecker for the given target user.
func NewSudoAllowedChecker(targetUser string, sev Severity) *SudoAllowedChecker {
	return &SudoAllowedChecker{targetUser: targetUser, severity: sev}
}

func (c *SudoAllowedChecker) Kind() CheckKind    { return KindSudoAllowed }
func (c *SudoAllowedChecker) Subject() string    { return c.targetUser }
func (c *SudoAllowedChecker) Severity() Severity { return c.severity }

// Run checks whether the current user may sudo to targetUser via `sudo -l`.
func (c *SudoAllowedChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	cmd := fmt.Sprintf(
		`sudo -l 2>/dev/null | grep -qE "(ALL|\b%s\b)"`,
		c.targetUser,
	)
	result, err := exec.Execute(ctx, cmd)
	ok := err == nil && result.ExitCode == 0
	if ok {
		return CheckResult{Kind: c.Kind(), Subject: c.targetUser, OK: true, Severity: c.severity}
	}
	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.targetUser,
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"current user may not have sudo permission for user %q — verify sudoers configuration",
			c.targetUser,
		),
	}
}
