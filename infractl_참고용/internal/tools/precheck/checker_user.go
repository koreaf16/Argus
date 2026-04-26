// Package precheck
// File: checker_user.go
// Description: OS 계정 존재 여부를 원격 서버에서 사전 검증하는 Checker 구현
// Responsibility: id(1) 명령을 사용해 대상 유저가 실제로 존재하는지 확인한다.

package precheck

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

// UserExistsChecker verifies that an OS user account exists on the remote host.
// Uses POSIX id(1) which queries NSS — works for local, LDAP, and AD-backed users.
type UserExistsChecker struct {
	user     string
	severity Severity
}

// NewUserExistsChecker constructs a UserExistsChecker for the given OS username.
func NewUserExistsChecker(user string, sev Severity) *UserExistsChecker {
	return &UserExistsChecker{user: user, severity: sev}
}

func (c *UserExistsChecker) Kind() CheckKind  { return KindUserExists }
func (c *UserExistsChecker) Subject() string  { return c.user }
func (c *UserExistsChecker) Severity() Severity { return c.severity }

// Run executes `id <user>` on the remote host and reports whether the account exists.
func (c *UserExistsChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	cmd := fmt.Sprintf("id %s >/dev/null 2>&1", shellQuote(c.user))
	result, err := exec.Execute(ctx, cmd)
	ok := err == nil && result.ExitCode == 0
	if ok {
		return CheckResult{Kind: c.Kind(), Subject: c.user, OK: true, Severity: c.severity}
	}
	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.user,
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"OS user %q does not exist on this host — verify with 'getent passwd %s' or check /etc/passwd",
			c.user, c.user,
		),
	}
}
