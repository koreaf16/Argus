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

func TestWorkspaceSystemBlocks_PreservesKoreanConnectionIntent(t *testing.T) {
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
	text := blocks[0].Text
	for _, want := range []string{"접속해", "X로 바꿔", "X 워크스페이스로"} {
		if !strings.Contains(text, want) {
			t.Fatalf("workspace prompt lost UTF-8 text %q: %s", want, text)
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
	if !strings.Contains(text, "Use the 'powershell' tool for LOCAL system operations") {
		t.Fatalf("local Windows prompt should prefer powershell: %s", text)
	}
	if !strings.Contains(text, "Use 'bash' only when the tool call explicitly targets a Unix-like remote workspace") {
		t.Fatalf("local Windows prompt should explain target-scoped bash use: %s", text)
	}
}
