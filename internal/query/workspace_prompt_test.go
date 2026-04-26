package query

import (
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
