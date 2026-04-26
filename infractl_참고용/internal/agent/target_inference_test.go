package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

type targetInferenceExecutor struct {
	target   string
	platform executor.Platform
}

func (e targetInferenceExecutor) Execute(context.Context, string) (executor.ExecResult, error) {
	return executor.ExecResult{ExitCode: 0}, nil
}

func (e targetInferenceExecutor) Target() string { return e.target }
func (e targetInferenceExecutor) Host() string   { return e.target }
func (e targetInferenceExecutor) Platform() executor.Platform {
	return e.platform
}
func (e targetInferenceExecutor) ShellName() string {
	if e.platform == executor.PlatformWindows {
		return "PowerShell"
	}
	return "bash"
}

func TestInferTargetWindowsPathLocalWindowsActiveLinuxRoutesLocalhost(t *testing.T) {
	mgr := executor.NewManager(targetInferenceExecutor{target: "localhost", platform: executor.PlatformWindows})
	mgr.Register("sandbox", targetInferenceExecutor{target: "sandbox", platform: executor.PlatformLinux})

	target, diag := inferTargetFromPathSyntax(
		"shell_exec",
		map[string]any{"command": `Get-ChildItem -LiteralPath 'C:\Users\jhkwa\Downloads'`},
		&store.Server{Name: "sandbox", OS: "Rocky Linux 9.7"},
		mgr,
		executor.PlatformWindows,
	)

	if diag != "" {
		t.Fatalf("unexpected diagnostic: %s", diag)
	}
	if target != "localhost" {
		t.Fatalf("target = %q, want localhost", target)
	}
}

func TestInferTargetWindowsPathLocalLinuxActiveWindowsKeepsActiveFallback(t *testing.T) {
	mgr := executor.NewManager(targetInferenceExecutor{target: "localhost", platform: executor.PlatformLinux})
	mgr.Register("win-app", targetInferenceExecutor{target: "win-app", platform: executor.PlatformWindows})

	target, diag := inferTargetFromPathSyntax(
		"file_read",
		map[string]any{"path": `C:\Temp\notes.txt`},
		&store.Server{Name: "win-app", OS: "Windows Server 2022"},
		mgr,
		executor.PlatformLinux,
	)

	if diag != "" {
		t.Fatalf("unexpected diagnostic: %s", diag)
	}
	if target != "" {
		t.Fatalf("target = %q, want empty so active workspace fallback is used", target)
	}
}

func TestInferTargetWindowsPathLocalLinuxActiveLinuxReturnsDiagnostic(t *testing.T) {
	mgr := executor.NewManager(targetInferenceExecutor{target: "localhost", platform: executor.PlatformLinux})
	mgr.Register("sandbox", targetInferenceExecutor{target: "sandbox", platform: executor.PlatformLinux})

	target, diag := inferTargetFromPathSyntax(
		"shell_exec",
		map[string]any{"command": `Get-ChildItem -LiteralPath 'C:\Users\jhkwa\Downloads'`},
		&store.Server{Name: "sandbox", OS: "Rocky Linux 9.7"},
		mgr,
		executor.PlatformLinux,
	)

	if target != "" {
		t.Fatalf("target = %q, want empty", target)
	}
	for _, want := range []string{"Windows-style path", "localhost is linux", "sandbox"} {
		if !strings.Contains(diag, want) {
			t.Fatalf("diagnostic = %q, missing %q", diag, want)
		}
	}
}

func TestInferTargetWindowsPathBothWindowsReturnsAmbiguousDiagnostic(t *testing.T) {
	mgr := executor.NewManager(targetInferenceExecutor{target: "localhost", platform: executor.PlatformWindows})
	mgr.Register("win-app", targetInferenceExecutor{target: "win-app", platform: executor.PlatformWindows})

	target, diag := inferTargetFromPathSyntax(
		"file_read",
		map[string]any{"path": `C:\Temp\notes.txt`},
		&store.Server{Name: "win-app", OS: "Windows Server 2022"},
		mgr,
		executor.PlatformWindows,
	)

	if target != "" {
		t.Fatalf("target = %q, want empty", target)
	}
	for _, want := range []string{"ambiguous", `target:"localhost"`, `target:"win-app"`} {
		if !strings.Contains(diag, want) {
			t.Fatalf("diagnostic = %q, missing %q", diag, want)
		}
	}
}

func TestInferTargetFileTransferDoesNotUseLocalPathForTarget(t *testing.T) {
	target, diag := inferTargetFromPathSyntax(
		"file_transfer",
		map[string]any{
			"action":      "upload",
			"local_path":  `C:\Users\jhkwa\Downloads\LINUX.X64_193000_db_home.zip`,
			"remote_path": "/tmp/LINUX.X64_193000_db_home.zip",
		},
		&store.Server{Name: "sandbox", OS: "Rocky Linux 9.7"},
		nil,
		executor.PlatformWindows,
	)

	if target != "" || diag != "" {
		t.Fatalf("target=%q diag=%q, want both empty", target, diag)
	}
}
