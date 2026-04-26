// Package precheck
// File: checker_env_var.go
// Description: 환경변수 설정 여부를 원격 호스트에서 사전 검증하는 Checker 구현
// Responsibility: test -n을 사용해 명령어 경로에 사용된 환경변수가 export되어 있는지 확인한다.

package precheck

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/executor"
)

// EnvVarSetChecker verifies that an environment variable is set (non-empty) on the remote host.
// Used for variables referenced as path prefixes (e.g., $ORACLE_BASE/oradata).
type EnvVarSetChecker struct {
	varName  string
	severity Severity
}

// NewEnvVarSetChecker constructs an EnvVarSetChecker for the given variable name.
func NewEnvVarSetChecker(varName string, sev Severity) *EnvVarSetChecker {
	return &EnvVarSetChecker{varName: varName, severity: sev}
}

func (c *EnvVarSetChecker) Kind() CheckKind    { return KindEnvVarSet }
func (c *EnvVarSetChecker) Subject() string    { return c.varName }
func (c *EnvVarSetChecker) Severity() Severity { return c.severity }

// Run executes `test -n "${VARNAME}"` on the remote host.
func (c *EnvVarSetChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	cmd := fmt.Sprintf(`test -n "${%s}"`, c.varName)
	result, err := exec.Execute(ctx, cmd)
	ok := err == nil && result.ExitCode == 0
	if ok {
		return CheckResult{Kind: c.Kind(), Subject: c.varName, OK: true, Severity: c.severity}
	}
	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.varName,
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"environment variable $%s is not set — export it before running this command",
			c.varName,
		),
	}
}
