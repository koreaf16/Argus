package query

import (
	"runtime"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/workspace"
)

func TestWorkspaceSystemBlocks_IncludesAliasesWithoutSecrets(t *testing.T) {
	reg := workspace.NewRegistry("")
	if err := reg.Add(workspace.ServerEntry{
		Alias: "oracle-server",
		Kind:  workspace.ServerKindSSH,
		Host:  "10.0.0.8",
		Port:  22,
		User:  "oracle",
	}); err != nil {
		t.Fatalf("add workspace entry: %v", err)
	}
	mgr := workspace.NewManager(reg, nil)
	blocks := workspaceSystemBlocks(mgr)
	if len(blocks) == 0 {
		t.Fatalf("expected workspace system block")
	}
	text := blocks[0].Text
	if !strings.Contains(text, "oracle-server") || !strings.Contains(text, "10.0.0.8") || !strings.Contains(text, "oracle") {
		t.Fatalf("workspace alias details missing: %s", text)
	}
	if strings.Contains(strings.ToLower(text), "root_password") {
		t.Fatalf("workspace block should not expose password fields: %s", text)
	}
}

func TestWorkspaceSystemBlocks_IncludesElevationPolicy(t *testing.T) {
	reg := workspace.NewRegistry("")
	if err := reg.Add(workspace.ServerEntry{
		Alias: "sandbox-server",
		Kind:  workspace.ServerKindSSH,
		Host:  "10.0.0.8",
		Port:  22,
		User:  "sandbox",
	}); err != nil {
		t.Fatalf("add workspace entry: %v", err)
	}
	mgr := workspace.NewManager(reg, nil)
	blocks := workspaceSystemBlocks(mgr)
	if len(blocks) == 0 {
		t.Fatalf("expected workspace system block")
	}
	combined := ""
	for _, b := range blocks {
		combined += b.Text
	}
	// Elevation policy must be present for remote workspace safety.
	for _, want := range []string{"권한 상승", "ELEVATION POLICY", "DISABLED"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("workspace prompt missing elevation content %q: %s", want, combined)
		}
	}
}

func TestWorkspaceSystemBlocks_LocalWindowsDoesNotClaimBashHidden(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("local Windows prompt only")
	}
	reg := workspace.NewRegistry("")
	mgr := workspace.NewManager(reg, nil)
	blocks := workspaceSystemBlocks(mgr)
	if len(blocks) < 2 {
		t.Fatalf("expected active environment block, got %d blocks", len(blocks))
	}

	text := blocks[len(blocks)-1].Text
	if strings.Contains(text, "hidden and unavailable") {
		t.Fatalf("local Windows prompt should not contradict visible bash tools: %s", text)
	}
	// Checks for Korean: "로컬 시스템 작업에는 'powershell' 도구를 사용하십시오."
	if !strings.Contains(text, "powershell") {
		t.Fatalf("local Windows prompt should prefer powershell: %s", text)
	}
	// Checks for Korean: "'bash'는 도구 호출이 `server` 또는 `role`을 통해 유닉스 계열 원격 워크스페이스를 명시적으로 대상으로 하는 경우에만 사용하십시오."
	if !strings.Contains(text, "bash") || !strings.Contains(text, "server") {
		t.Fatalf("local Windows prompt should explain target-scoped bash use: %s", text)
	}
}
