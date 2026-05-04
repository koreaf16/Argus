package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/state"
)

func testRoleContext(t *testing.T) Context {
	t.Helper()
	reg := workspace.NewRegistry("")
	if err := reg.Add(workspace.ServerEntry{Alias: "oracle-server", Kind: workspace.ServerKindSSH, Host: "oracle", User: "oracle"}); err != nil {
		t.Fatalf("add oracle server: %v", err)
	}
	if err := reg.Add(workspace.ServerEntry{Alias: "sandbox-server", Kind: workspace.ServerKindSSH, Host: "sandbox", User: "postgres"}); err != nil {
		t.Fatalf("add sandbox server: %v", err)
	}
	app := state.NewAppState()
	app.SetWorkflowCard(&state.WorkflowCard{
		Title:    "migration",
		Category: state.WorkflowCategoryMigration,
		Phase:    state.WorkflowPhaseExecute,
		WorkspaceRoleProfiles: map[string]state.WorkspaceRoleProfile{
			"source_mysql": {
				Role:           "source_mysql",
				Channel:        "source",
				Server:         "oracle-server",
				AsUser:         "oracle",
				MutationPolicy: "read_only",
			},
			"target_postgres": {
				Role:           "target_postgres",
				Channel:        "target",
				Server:         "sandbox-server",
				AsUser:         "postgres",
				MutationPolicy: "write",
			},
		},
	})
	return Context{
		Context:   context.Background(),
		State:     app,
		Workspace: workspace.NewManager(reg, nil),
	}
}

func TestResolveExecutionRoleDynamicFallback(t *testing.T) {
	ctx := testRoleContext(t)
	// unregistered role + explicit server → dynamic AsUser fallback
	alias, role, err := ResolveExecutionRoleServer(ctx, "oracle-server", "ora19c", "", "bash")
	if err != nil {
		t.Fatalf("resolve role: %v", err)
	}
	if alias != "oracle-server" {
		t.Fatalf("alias = %q, want oracle-server", alias)
	}
	if role.AsUser != "ora19c" {
		t.Fatalf("AsUser = %q, want ora19c", role.AsUser)
	}
}

func TestResolveExecutionRoleRequiresServerForDynamicRole(t *testing.T) {
	ctx := testRoleContext(t)
	// unregistered role without explicit server → "explicit server required"
	_, _, err := ResolveExecutionRoleServer(ctx, "", "ora19c", "", "bash")
	if err == nil {
		t.Fatal("expected explicit server required error")
	}
	if !strings.Contains(err.Error(), "explicit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRoleMutationRejectsReadOnlyWrite(t *testing.T) {
	role := ExecutionRole{Role: "source_mysql", Channel: "source", MutationPolicy: "read_only"}
	err := ValidateRoleMutation(role, "bash", "DROP TABLE customer", false)
	if err == nil {
		t.Fatal("expected read-only role to reject destructive command")
	}
	err = ValidateRoleMutation(role, "bash", "mysqldump dcm customer > customer.sql", false)
	if err != nil {
		t.Fatalf("expected dump command to pass read-only role policy, got %v", err)
	}
}
