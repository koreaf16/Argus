package tools

import (
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
)

func TestLocalPathPlatformMismatchWindowsPathOnLinux(t *testing.T) {
	got := localPathPlatformMismatchForPlatform(
		`C:\Users\jhkwa\Downloads\LINUX.X64_193000_db_home.zip`,
		executor.PlatformLinux,
	)
	for _, want := range []string{"Windows-style path", "controller is linux", "local_path"} {
		if !strings.Contains(got, want) {
			t.Fatalf("message = %q, missing %q", got, want)
		}
	}
}

func TestLocalPathPlatformMismatchWindowsPathOnWindowsAllowed(t *testing.T) {
	got := localPathPlatformMismatchForPlatform(
		`C:\Users\jhkwa\Downloads\LINUX.X64_193000_db_home.zip`,
		executor.PlatformWindows,
	)
	if got != "" {
		t.Fatalf("unexpected mismatch: %s", got)
	}
}

func TestLocalPathPlatformMismatchPOSIXPathOnLinuxAllowed(t *testing.T) {
	got := localPathPlatformMismatchForPlatform(
		`/home/sandbox/LINUX.X64_193000_db_home.zip`,
		executor.PlatformLinux,
	)
	if got != "" {
		t.Fatalf("unexpected mismatch: %s", got)
	}
}
