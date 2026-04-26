package powershell

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

func TestInputSchemaIncludesServer(t *testing.T) {
	schema := NewPowerShellTool().InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing")
	}
	if _, ok := props["server"]; !ok {
		t.Fatalf("expected server property in powershell input schema")
	}
}

func TestCheckPermissionDeniesUnixTarget(t *testing.T) {
	reg := workspace.NewRegistry("")
	if err := reg.Add(workspace.ServerEntry{
		Alias: "linux-box",
		Kind:  workspace.ServerKindSSH,
		Host:  "10.0.0.8",
		Port:  22,
		User:  "ubuntu",
	}); err != nil {
		t.Fatalf("add server: %v", err)
	}
	mgr := workspace.NewManager(reg, nil)
	mgr.SetInspectSnapshot("linux-box", workspace.InspectSnapshot{
		Alias: "linux-box",
		OS:    "Linux 6.8.0",
		Shell: "/bin/bash",
	})

	input, _ := json.Marshal(map[string]any{
		"command": "Get-Process",
		"server":  "linux-box",
	})
	ctx := tool.Context{
		Context:   context.Background(),
		Workspace: mgr,
	}
	got, err := NewPowerShellTool().CheckPermission(ctx, input)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if got.Behavior != types.BehaviorDeny {
		t.Fatalf("behavior = %s, want %s", got.Behavior, types.BehaviorDeny)
	}
}
