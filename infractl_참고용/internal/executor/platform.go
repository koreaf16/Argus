// Package executor
// File: platform.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package executor

import (
	"runtime"
	"strings"
)

// Platform identifies the target operating system family for an executor.
type Platform string

const (
	PlatformUnknown Platform = "unknown"
	PlatformLinux   Platform = "linux"
	PlatformDarwin  Platform = "darwin"
	PlatformWindows Platform = "windows"
)

// PlatformProvider exposes target platform and shell metadata.
type PlatformProvider interface {
	Platform() Platform
	ShellName() string
}

// NormalizePlatform converts a runtime GOOS or OS description into a platform family.
func NormalizePlatform(raw string) Platform {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case value == "", value == string(PlatformUnknown):
		return PlatformUnknown
	case strings.Contains(value, "windows"):
		return PlatformWindows
	case strings.Contains(value, "darwin"), strings.Contains(value, "mac"), strings.Contains(value, "macos"), strings.Contains(value, "os x"):
		return PlatformDarwin
	case strings.Contains(value, "linux"),
		strings.Contains(value, "ubuntu"),
		strings.Contains(value, "debian"),
		strings.Contains(value, "centos"),
		strings.Contains(value, "red hat"),
		strings.Contains(value, "redhat"),
		strings.Contains(value, "rhel"),
		strings.Contains(value, "suse"),
		strings.Contains(value, "oracle linux"),
		strings.Contains(value, "amazon linux"),
		strings.Contains(value, "alpine"):
		return PlatformLinux
	default:
		return PlatformUnknown
	}
}

// PlatformFromExecutor returns the target platform for an executor.
func PlatformFromExecutor(exec Executor) Platform {
	if provider, ok := exec.(PlatformProvider); ok {
		if platform := provider.Platform(); platform != PlatformUnknown {
			return platform
		}
	}
	if exec == nil || IsLocalTarget(exec.Target()) {
		return NormalizePlatform(runtime.GOOS)
	}
	return PlatformUnknown
}

// CommandPlatform returns the platform to use for command construction.
// Remote executors without explicit metadata fall back to Linux instead of the controller OS.
func CommandPlatform(exec Executor) Platform {
	platform := PlatformFromExecutor(exec)
	if platform != PlatformUnknown {
		return platform
	}
	return PlatformLinux
}

// ShellNameForExecutor returns the preferred shell label for an executor.
func ShellNameForExecutor(exec Executor) string {
	if provider, ok := exec.(PlatformProvider); ok {
		if shell := strings.TrimSpace(provider.ShellName()); shell != "" {
			return shell
		}
	}

	switch PlatformFromExecutor(exec) {
	case PlatformWindows:
		return "PowerShell"
	case PlatformLinux, PlatformDarwin:
		return "bash"
	default:
		if exec != nil && !IsLocalTarget(exec.Target()) {
			return "ssh"
		}
		return LocalShellName()
	}
}

// IsLocalTarget returns true when the target resolves to the local controller.
func IsLocalTarget(target string) bool {
	target = strings.TrimSpace(target)
	return target == "" || strings.EqualFold(target, "localhost") || strings.EqualFold(target, "local")
}

