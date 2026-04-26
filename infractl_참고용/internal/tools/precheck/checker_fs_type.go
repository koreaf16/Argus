// Package precheck
// File: checker_fs_type.go
// Description: 원격 호스트의 파일시스템 타입을 사전 검증하는 Checker 구현
// Responsibility: df -PT로 파일시스템 타입을 읽어 Oracle 설치에 부적합한 타입을 감지한다.

package precheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// FsTypeChecker verifies that the filesystem hosting a path is compatible with Oracle installation.
// tmpfs/cifs/smbfs are blocked; nfs/nfs4/fuse trigger a warning.
type FsTypeChecker struct {
	path     string
	severity Severity
}

// NewFsTypeChecker constructs a FsTypeChecker for the given remote path.
func NewFsTypeChecker(p string, sev Severity) *FsTypeChecker {
	return &FsTypeChecker{path: p, severity: sev}
}

func (c *FsTypeChecker) Kind() CheckKind    { return KindFsType }
func (c *FsTypeChecker) Subject() string    { return c.path }
func (c *FsTypeChecker) Severity() Severity { return c.severity }

var blockedFsTypes = map[string]bool{
	"tmpfs": true, "cifs": true, "smbfs": true,
}

var warnFsTypes = map[string]bool{
	"nfs": true, "nfs4": true, "fuse": true,
}

// Run executes `df -PT <path>` and checks the filesystem type.
func (c *FsTypeChecker) Run(ctx context.Context, exec executor.Executor) CheckResult {
	p := strings.TrimSpace(c.path)
	if p == "" {
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: SeverityWarn}
	}

	cmd := fmt.Sprintf("df -PT %s 2>/dev/null | awk 'NR==2{print $2}'", shellQuote(p))
	result, err := exec.Execute(ctx, cmd)
	if err != nil || result.ExitCode != 0 {
		// Cannot determine — skip.
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: SeverityWarn}
	}

	fsType := strings.TrimSpace(result.Stdout)
	if fsType == "" {
		return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: SeverityWarn}
	}

	fsTypeLower := strings.ToLower(fsType)

	if blockedFsTypes[fsTypeLower] {
		return CheckResult{
			Kind:     c.Kind(),
			Subject:  c.path,
			OK:       false,
			Severity: SeverityBlock,
			Message: fmt.Sprintf(
				"filesystem at %q uses type %q which is not supported for Oracle installation",
				p, fsType,
			),
		}
	}

	if warnFsTypes[fsTypeLower] {
		return CheckResult{
			Kind:     c.Kind(),
			Subject:  c.path,
			OK:       false,
			Severity: SeverityWarn,
			Message: fmt.Sprintf(
				"filesystem at %q uses type %q which may cause performance or locking issues",
				p, fsType,
			),
		}
	}

	return CheckResult{Kind: c.Kind(), Subject: c.path, OK: true, Severity: SeverityWarn}
}
