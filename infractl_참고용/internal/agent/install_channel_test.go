// Package agent
// File: install_channel_test.go
// Description: Install channel policy enforcement unit tests.
// Responsibility: Validate install/migrate channel blocks and bypass behavior.

package agent

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

type installPolicyMutationTool struct{}

func (installPolicyMutationTool) Name() string        { return "shell_exec" }
func (installPolicyMutationTool) Description() string { return "mutation" }
func (installPolicyMutationTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (installPolicyMutationTool) IsReadOnly() bool { return false }
func (installPolicyMutationTool) IsEnabled() bool  { return true }
func (installPolicyMutationTool) Execute(context.Context, map[string]interface{}, executor.Executor) (tools.ToolOutcome, error) {
	return tools.ToolOutcome{Content: "ok", Success: true}, nil
}

func makeInstallPolicyAgent(t *testing.T, kind taskctx.TaskKind, servers []store.Server) *Agent {
	t.Helper()
	mgr := taskctx.NewManager()
	_, err := mgr.Declare(taskctx.DeclareRequest{
		Title:        "policy-test",
		Kind:         kind,
		TargetServer: "target",
	})
	if err != nil {
		t.Fatalf("declare task: %v", err)
	}
	return &Agent{
		taskMgr: mgr,
		store:   fakeServerStore{servers: servers},
	}
}

func TestEnforceInstallChannelPolicyBlocksLocalhostInstall(t *testing.T) {
	a := makeInstallPolicyAgent(t, taskctx.KindInstall, nil)
	msg, blocked := a.enforceInstallChannelPolicy(context.Background(), installPolicyMutationTool{}, "shell_exec", "localhost")
	if !blocked {
		t.Fatalf("expected localhost to be blocked for install task")
	}
	if msg == "" {
		t.Fatal("expected block message")
	}
}

func TestEnforceInstallChannelPolicyBlocksNonEphemeralServer(t *testing.T) {
	a := makeInstallPolicyAgent(t, taskctx.KindInstall, []store.Server{
		{Name: "prod-web", Ephemeral: false, Purpose: "install_channel"},
	})
	msg, blocked := a.enforceInstallChannelPolicy(context.Background(), installPolicyMutationTool{}, "shell_exec", "prod-web")
	if !blocked {
		t.Fatalf("expected non-ephemeral target to be blocked")
	}
	if msg == "" {
		t.Fatal("expected block message")
	}
}

func TestEnforceInstallChannelPolicyPassesNonInstallTask(t *testing.T) {
	a := makeInstallPolicyAgent(t, taskctx.KindInspect, nil)
	msg, blocked := a.enforceInstallChannelPolicy(context.Background(), installPolicyMutationTool{}, "shell_exec", "localhost")
	if blocked {
		t.Fatalf("expected non-install task to pass, got msg=%q", msg)
	}
}

func TestEnforceInstallChannelPolicyBypassTools(t *testing.T) {
	a := makeInstallPolicyAgent(t, taskctx.KindInstall, nil)
	for name := range installChannelBypassTools {
		msg, blocked := a.enforceInstallChannelPolicy(context.Background(), installPolicyMutationTool{}, name, "localhost")
		if blocked {
			t.Fatalf("expected bypass tool %q to pass, msg=%q", name, msg)
		}
	}
}
