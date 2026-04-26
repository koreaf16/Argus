// Package precheck
// File: checker_disk_space.go
// Description: 원격 호스트의 파일시스템 여유 공간을 사전 검증하는 Checker 구현
// Responsibility: df -B1로 여유 바이트를 읽어 최소 요구 공간을 만족하는지 확인한다.

package precheck

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// DiskSpaceChecker verifies that a filesystem has enough free space before extraction.
// Parsing failures are treated as OK (check-unavailable = pass through).
type DiskSpaceChecker struct {
	path      string
	minBytes  int64
	severity  Severity
}

// NewDiskSpaceChecker constructs a DiskSpaceChecker for the given path and minimum bytes.
func NewDiskSpaceChecker(p string, minBytes int64, sev Severity) *DiskSpaceChecker {
	return &DiskSpaceChecker{path: p, minBytes: minBytes, severity: sev}
}

func (c *DiskSpaceChecker) Kind() CheckKind    { return KindDiskSpace }
func (c *DiskSpaceChecker) Subject() string    { return c.path }
func (c *DiskSpaceChecker) Severity() Severity { return c.severity }

// Run executes `df -B1 <path>` and checks available bytes against minBytes.
func (c *DiskSpaceChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	p := strings.TrimSpace(c.path)
	if p == "" {
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: c.severity}
	}

	cmd := fmt.Sprintf("df -B1 %s 2>/dev/null | awk 'NR==2 {print $4}'", shellQuote(p))
	result, err := exec.Execute(ctx, cmd)
	if err != nil || result.ExitCode != 0 {
		// Cannot determine — skip.
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: c.severity}
	}

	freeStr := strings.TrimSpace(result.Stdout)
	freeBytes, parseErr := strconv.ParseInt(freeStr, 10, 64)
	if parseErr != nil || freeBytes < 0 {
		// Unparseable output — skip.
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: c.severity}
	}

	if freeBytes >= c.minBytes {
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: c.severity}
	}

	return CheckResult{
		Kind:     c.Kind(),
		Subject:  c.path,
		OK:       false,
		Severity: c.severity,
		Message: fmt.Sprintf(
			"filesystem at %q has only %s free, at least %s required",
			p, formatBytes(freeBytes), formatBytes(c.minBytes),
		),
	}
}

// formatBytes converts a byte count into a human-readable GB/MB/KB string.
func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
