// Package precheck
// File: checker_file_owned.go
// Description: 원격 호스트의 파일 소유권을 사전 검증하는 Checker 구현
// Responsibility: test -O를 사용해 대상 파일이 현재 실행 계정 소유인지 확인한다.

package precheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// FileOwnedChecker verifies that the current user owns the target file.
// If the file does not exist, the check is skipped (OK=true).
// chmod/chown operations fail silently when the caller does not own the file.
type FileOwnedChecker struct {
	path     string
	severity Severity
}

// NewFileOwnedChecker constructs a FileOwnedChecker for the given remote path.
func NewFileOwnedChecker(path string, sev Severity) *FileOwnedChecker {
	return &FileOwnedChecker{path: path, severity: sev}
}

func (c *FileOwnedChecker) Kind() CheckKind    { return KindFileOwned }
func (c *FileOwnedChecker) Subject() string    { return c.path }
func (c *FileOwnedChecker) Severity() Severity { return c.severity }

// Run executes `test -e <path> && test -O <path>` on the remote host.
// A missing file is treated as OK — FileExistsChecker handles that separately.
func (c *FileOwnedChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	p := strings.TrimSpace(c.path)
	if p == "" {
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: c.severity}
	}

	// If the file does not exist, skip this check.
	existCmd := fmt.Sprintf("test -e %s", shellQuote(p))
	existResult, err := exec.Execute(ctx, existCmd)
	if err != nil || existResult.ExitCode != 0 {
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: c.severity}
	}

	// File exists — verify ownership.
	ownCmd := fmt.Sprintf("test -O %s", shellQuote(p))
	ownResult, err := exec.Execute(ctx, ownCmd)
	ok := err == nil && ownResult.ExitCode == 0
	if ok {
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: c.severity}
	}

	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.path,
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"file %q is not owned by current user — re-run as the file owner or root",
			p,
		),
	}
}
