// Package tools
// File: preflight_critical_path.go
// Description: 위험 시스템 경로 탐지 및 경고/차단 분류
// Responsibility: 대상 경로가 /proc, /sys 같은 가상 파일시스템이나 /etc, /boot 같은
//                 핵심 시스템 디렉토리에 속하는지 판별하고 경고 문자열을 반환한다.

package tools

import (
	"fmt"
	"strings"
)

// criticalEntry defines a system path prefix and its risk classification.
type criticalEntry struct {
	prefix   string
	critical bool   // true = block-level (virtual/immutable FS); false = warn-level
	label    string // human-readable label shown in warnings
}

// criticalPaths is ordered from most-specific to least-specific.
var criticalPaths = []criticalEntry{
	{prefix: "/proc", critical: true, label: "virtual process filesystem"},
	{prefix: "/sys", critical: true, label: "virtual kernel filesystem"},
	{prefix: "/dev", critical: true, label: "device filesystem"},
	{prefix: "/etc/passwd", critical: true, label: "system account database"},
	{prefix: "/etc/shadow", critical: true, label: "system credential database"},
	{prefix: "/etc/sudoers", critical: true, label: "privilege policy file"},
	{prefix: "/boot", critical: false, label: "bootloader directory"},
	{prefix: "/etc", critical: false, label: "system configuration directory"},
	{prefix: "/usr/local", critical: false, label: "local system install directory"},
	{prefix: "/usr/bin", critical: false, label: "system binary directory"},
	{prefix: "/usr/sbin", critical: false, label: "system admin binary directory"},
	{prefix: "/usr/lib", critical: false, label: "system library directory"},
	{prefix: "/usr/lib64", critical: false, label: "system library directory"},
	{prefix: "/sbin", critical: false, label: "system admin binary directory"},
	{prefix: "/lib", critical: false, label: "system library directory"},
	{prefix: "/lib64", critical: false, label: "system library directory"},
	{prefix: "/var/run", critical: false, label: "runtime state directory"},
}

// devNullDevices are virtual character devices that are safe write targets.
// They are commonly used as discard sinks (2>/dev/null, >/dev/null 2>&1) and
// must not be blocked even though /dev is otherwise treated as a virtual FS.
var devNullDevices = map[string]bool{
	"/dev/null":   true,
	"/dev/zero":   true,
	"/dev/stdin":  true,
	"/dev/stdout": true,
	"/dev/stderr": true,
}

// CheckCriticalPath checks whether targetPath falls under a known critical system directory.
//
// Returns:
//   - warning: a non-empty human-readable warning string when the path is sensitive.
//   - isCritical: true when the path is a virtual/immutable filesystem where writes
//     should be blocked outright (/proc, /sys, /dev); false for warn-only paths.
//
// This is a pure function (no I/O) and is safe to call from any goroutine.
func CheckCriticalPath(targetPath string) (warning string, isCritical bool) {
	// Normalise: ensure consistent slash prefix for prefix matching.
	p := targetPath
	if !strings.HasPrefix(p, "/") {
		// Relative paths or Windows paths are not checked.
		return "", false
	}

	// Allow writes to standard null/discard devices — these are universally
	// used as discard sinks in shell redirections and pose no real risk.
	if devNullDevices[p] {
		return "", false
	}

	for _, entry := range criticalPaths {
		if pathHasPrefix(p, entry.prefix) {
			if entry.critical {
				return fmt.Sprintf(
					"[BLOCKED] target path %q is inside %s (%s): writes to this location are not allowed",
					targetPath, entry.prefix, entry.label,
				), true
			}
			return fmt.Sprintf(
				"[WARNING] target path %q is inside %s (%s): confirm this change is intentional",
				targetPath, entry.prefix, entry.label,
			), false
		}
	}
	return "", false
}

// pathHasPrefix reports whether p equals prefix or starts with prefix followed by a slash.
// This prevents false matches like /etcfoo matching /etc.
func pathHasPrefix(p, prefix string) bool {
	if p == prefix {
		return true
	}
	return strings.HasPrefix(p, prefix+"/")
}
