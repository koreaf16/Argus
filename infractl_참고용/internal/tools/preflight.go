// Package tools
// File: preflight.go
// Description: 원격 파일 작업 전 권한, 디스크 공간, 파일시스템 안전성 사전 검사
// Responsibility: 대용량 파일 전송/생성 전 대상 경로의 쓰기 권한·잔여 용량·파일시스템 타입을 병렬 확인한다.

package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yourorg/infractl/internal/executor"
)

// RunReadPreflightChecks runs filesystem safety and readability checks.
// Use this for download operations where write permission is not required.
func RunReadPreflightChecks(ctx context.Context, exec executor.Executor, dir string) error {
	if executor.CommandPlatform(exec) == executor.PlatformWindows {
		return nil
	}
	type checkFn func() error
	checks := []checkFn{
		func() error { return checkFilesystemSafe(ctx, exec, dir) },
		func() error { return checkPathReadable(ctx, exec, dir) },
	}
	errs := make(chan error, len(checks))
	var wg sync.WaitGroup
	for _, fn := range checks {
		wg.Add(1)
		go func(f checkFn) {
			defer wg.Done()
			if err := f(); err != nil {
				errs <- err
			}
		}(fn)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// checkPathReadable verifies that the target path exists and is readable.
func checkPathReadable(ctx context.Context, exec executor.Executor, dir string) error {
	quoted := executor.QuotePOSIX(dir)
	script := fmt.Sprintf(`[ -r %s ] && echo "readable" || echo "notreadable"`, quoted)
	result, err := exec.Execute(ctx, script)
	if err != nil {
		return nil // Best effort
	}
	if strings.TrimSpace(result.Stdout) == "notreadable" {
		return fmt.Errorf("no read permission on %q", dir)
	}
	return nil
}

// RunPreflightChecks runs writable, disk space, and filesystem safety checks in parallel.
// dir is the target directory (or its ancestor if it does not yet exist).
// requiredBytes is the minimum free space required; pass 0 to skip the space check.
// On Windows executors the filesystem and writable checks are skipped (POSIX-only).
func RunPreflightChecks(ctx context.Context, exec executor.Executor, dir string, requiredBytes int64) error {
	if executor.CommandPlatform(exec) == executor.PlatformWindows {
		return nil
	}

	// becomeUser context check: if the executor implies privilege, skip the plain write test.
	// We check if the command would start with sudo/su/runuser.
	if CommandImpliesPrivilege(dir) { // This is a heuristic, better to check executor state if available.
		return RunFilesystemOnlyChecks(ctx, exec, dir, requiredBytes)
	}

	type checkFn func() error
	checks := []checkFn{
		func() error { return checkAncestorWritable(ctx, exec, dir) },
		func() error { return checkFilesystemSafe(ctx, exec, dir) },
	}
	if requiredBytes > 0 {
		checks = append(checks, func() error { return checkRemoteDiskSpace(ctx, exec, dir, requiredBytes) })
	}

	errs := make(chan error, len(checks))
	var wg sync.WaitGroup
	for _, fn := range checks {
		wg.Add(1)
		go func(f checkFn) {
			defer wg.Done()
			if err := f(); err != nil {
				errs <- err
			}
		}(fn)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// checkAncestorWritable verifies that the nearest existing ancestor of dir is writable.
// This correctly handles the case where dir does not yet exist (e.g. a new subdirectory
// to be created during an upload). It walks up the path until it finds an existing entry,
// then tests write permission there.
func checkAncestorWritable(ctx context.Context, exec executor.Executor, dir string) error {
	quoted := executor.QuotePOSIX(dir)
	// Walk up until we find an existing path, then test -w on it.
	script := fmt.Sprintf(
		`d=%s; while [ ! -e "$d" ] && [ "$d" != "/" ]; do d=$(dirname "$d"); done; test -w "$d" && printf "writable:%%s" "$d" || printf "notwritable:%%s" "$d"`,
		quoted,
	)
	result, err := exec.Execute(ctx, script)
	if err != nil {
		return fmt.Errorf("write permission check on %q: %w", dir, err)
	}
	combined := strings.TrimSpace(result.Stdout + result.Stderr)
	if strings.HasPrefix(combined, "notwritable:") {
		checkedPath := strings.TrimPrefix(combined, "notwritable:")
		if checkedPath == "" {
			checkedPath = dir
		}
		return fmt.Errorf("no write permission on %q (checked ancestor %q)", dir, checkedPath)
	}
	if !strings.HasPrefix(combined, "writable:") {
		return fmt.Errorf("write permission check on %q returned unexpected output: %q", dir, combined)
	}
	return nil
}

// checkFilesystemSafe verifies the filesystem hosting dir is not read-only and is not
// a virtual/immutable filesystem (proc, sysfs, squashfs, etc.).
func checkFilesystemSafe(ctx context.Context, exec executor.Executor, dir string) error {
	quoted := executor.QuotePOSIX(dir)
	// Walk to nearest existing ancestor for df.
	// "|| true" ensures the pipeline always exits 0 even when the login shell has
	// set -o pipefail and df -PT is unavailable or fails on the remote host.
	script := fmt.Sprintf(
		`d=%s; while [ ! -e "$d" ] && [ "$d" != "/" ]; do d=$(dirname "$d"); done; df -PT "$d" 2>/dev/null | awk 'NR==2{print $2}' || true`,
		quoted,
	)
	result, err := exec.Execute(ctx, script)
	if err != nil {
		// df -PT may not be supported on this host, or the login-shell profile
		// (set -e / set -o pipefail) caused a non-zero exit before we could reach
		// the "|| true" guard. Either way the check is best-effort — skip silently.
		return nil
	}
	fsType := strings.TrimSpace(result.Stdout + result.Stderr)
	if fsType == "" {
		// df -PT not available on all systems; skip silently.
		return nil
	}

	blockedTypes := map[string]bool{
		"proc":     true,
		"sysfs":    true,
		"devtmpfs": true,
		"devpts":   true,
		"squashfs": true,
		"overlay":  false, // overlay is writable (Docker containers), allow
	}
	if blockedTypes[fsType] {
		return fmt.Errorf("target path %q is on a virtual/immutable filesystem (%s): writes are not allowed", dir, fsType)
	}

	// Check for read-only mount option.
	mountScript := fmt.Sprintf(
		`d=%s; while [ ! -e "$d" ] && [ "$d" != "/" ]; do d=$(dirname "$d"); done; findmnt -n -o OPTIONS --target "$d" 2>/dev/null || mount | grep " $(df --output=target "$d" 2>/dev/null | tail -1) " 2>/dev/null | head -1`,
		quoted,
	)
	mountResult, mountErr := exec.Execute(ctx, mountScript)
	if mountErr != nil {
		return nil // mount check is best-effort; don't block on error
	}
	opts := strings.TrimSpace(mountResult.Stdout + mountResult.Stderr)
	for _, opt := range strings.Split(opts, ",") {
		if strings.TrimSpace(opt) == "ro" {
			return fmt.Errorf("target path %q is on a read-only mount", dir)
		}
	}
	return nil
}

// checkRemoteDiskSpace verifies that the filesystem hosting dir has at least requiredBytes free.
// Uses POSIX `df -P` on the nearest existing ancestor directory.
func checkRemoteDiskSpace(ctx context.Context, exec executor.Executor, dir string, requiredBytes int64) error {
	if requiredBytes <= 0 {
		return nil
	}
	quoted := executor.QuotePOSIX(dir)
	// Walk to nearest existing ancestor for df.
	script := fmt.Sprintf(
		`d=%s; while [ ! -d "$d" ] && [ "$d" != "/" ]; do d=$(dirname "$d"); done; df -P "$d"`,
		quoted,
	)
	result, err := exec.Execute(ctx, script)
	if err != nil {
		return fmt.Errorf("disk space check on %q: %w", dir, err)
	}
	available, err := parseDFAvailableBytes(result.Stdout)
	if err != nil {
		return fmt.Errorf("disk space check: %w", err)
	}
	if available < requiredBytes {
		return fmt.Errorf("insufficient disk space: %.1f MB available, %.1f MB required on %q",
			float64(available)/(1<<20), float64(requiredBytes)/(1<<20), dir)
	}
	return nil
}

// RunFilesystemOnlyChecks runs filesystem safety and optional disk space checks,
// skipping the writable check. Used when privilege escalation is implied (e.g. sudo/su)
// so the plain write-permission test would give a false negative.
func RunFilesystemOnlyChecks(ctx context.Context, exec executor.Executor, dir string, requiredBytes int64) error {
	if executor.CommandPlatform(exec) == executor.PlatformWindows {
		return nil
	}
	type checkFn func() error
	checks := []checkFn{
		func() error { return checkFilesystemSafe(ctx, exec, dir) },
	}
	if requiredBytes > 0 {
		checks = append(checks, func() error { return checkRemoteDiskSpace(ctx, exec, dir, requiredBytes) })
	}
	errs := make(chan error, len(checks))
	var wg sync.WaitGroup
	for _, fn := range checks {
		wg.Add(1)
		go func(f checkFn) {
			defer wg.Done()
			if err := f(); err != nil {
				errs <- err
			}
		}(fn)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// parseDFAvailableBytes parses `df -P` stdout and returns the available space in bytes.
// Format: Filesystem  1K-blocks  Used  Available  Use%  Mounted-on
func parseDFAvailableBytes(output string) (int64, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected df output: %q", output)
	}
	// The data line is the last non-empty line (handles continuation for long device names).
	dataLine := ""
	for i := len(lines) - 1; i >= 1; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			dataLine = lines[i]
			break
		}
	}
	if dataLine == "" {
		return 0, fmt.Errorf("no data row in df output: %q", output)
	}
	fields := strings.Fields(dataLine)
	// df -P normal row:       Filesystem  1K-blocks  Used  Available  Use%  MountedOn  (6 fields)
	// df -P continuation row:            1K-blocks  Used  Available  Use%  MountedOn  (5 fields)
	// Available is always the 3rd-last field before Use% and MountedOn.
	var availableField string
	switch len(fields) {
	case 6:
		availableField = fields[3]
	case 5:
		availableField = fields[2]
	default:
		return 0, fmt.Errorf("unexpected df data row: %q", dataLine)
	}
	var kBlocks int64
	if _, scanErr := fmt.Sscanf(availableField, "%d", &kBlocks); scanErr != nil {
		return 0, fmt.Errorf("parse available blocks from %q: %w", availableField, scanErr)
	}
	return kBlocks * 1024, nil // df -P reports in 1 K-block units
}
