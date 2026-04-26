// Package precheck
// File: checker_file.go
// Description: 원격 호스트의 파일 존재 여부를 사전 검증하는 Checker 구현
// Responsibility: test -f를 사용해 대상 파일이 실제로 존재하는지 확인한다.

package precheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// FileExistsChecker verifies that a file exists on the remote host.
type FileExistsChecker struct {
	path     string
	severity Severity
}

// NewFileExistsChecker constructs a FileExistsChecker for the given remote file path.
func NewFileExistsChecker(path string, sev Severity) *FileExistsChecker {
	return &FileExistsChecker{path: path, severity: sev}
}

func (c *FileExistsChecker) Kind() CheckKind    { return KindFileExists }
func (c *FileExistsChecker) Subject() string    { return c.path }
func (c *FileExistsChecker) Severity() Severity { return c.severity }

// Run executes `test -e <path>` on the remote host (covers files, symlinks, devices).
func (c *FileExistsChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	p := strings.TrimSpace(c.path)
	if p == "" {
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: c.severity}
	}
	cmd := fmt.Sprintf("test -e %s", shellQuote(p))
	result, err := exec.Execute(ctx, cmd)
	ok := err == nil && result.ExitCode == 0
	if ok {
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: c.severity}
	}
	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.path,
		OK:       false,
		Severity: c.severity,
		Message:  fmt.Sprintf("file %q does not exist on remote host", p),
	}
}
