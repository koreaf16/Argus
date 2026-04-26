// Package precheck
// File: checker_dir.go
// Description: 원격 호스트의 디렉토리 존재 여부를 사전 검증하는 Checker 구현
// Responsibility: test -d를 사용해 대상 디렉토리가 실제로 존재하는지 확인한다.

package precheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// DirExistsChecker verifies that a directory exists on the remote host.
// Reports a clear error with mkdir hint when the directory is missing.
type DirExistsChecker struct {
	dir      string
	severity Severity
}

// NewDirExistsChecker constructs a DirExistsChecker for the given remote directory path.
func NewDirExistsChecker(dir string, sev Severity) *DirExistsChecker {
	return &DirExistsChecker{dir: dir, severity: sev}
}

func (c *DirExistsChecker) Kind() CheckKind    { return KindDirExists }
func (c *DirExistsChecker) Subject() string    { return c.dir }
func (c *DirExistsChecker) Severity() Severity { return c.severity }

// Run executes `test -d <dir>` on the remote host.
func (c *DirExistsChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	dir := strings.TrimSpace(c.dir)
	if dir == "" || dir == "." || dir == "/" {
		// Root and current-dir always exist; skip check.
		return CheckResult{Kind: c.Kind(), Subject: c.dir, OK: true, Severity: c.severity}
	}
	cmd := fmt.Sprintf("test -d %s", shellQuote(dir))
	result, err := exec.Execute(ctx, cmd)
	ok := err == nil && result.ExitCode == 0
	if ok {
		return CheckResult{Kind: c.Kind(), Subject: c.dir, OK: true, Severity: c.severity}
	}
	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.dir,
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"directory %q does not exist on remote host — create it first: mkdir -p %s",
			dir, shellQuote(dir),
		),
	}
}

// shellQuote wraps a string in single quotes, escaping any existing single quotes.
// Defined here as the shared helper for the precheck package.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
