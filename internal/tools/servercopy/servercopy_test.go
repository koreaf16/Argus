package servercopy

import (
	"context"
	"testing"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
)

func newTestContext(t *testing.T) tool.Context {
	t.Helper()
	reg := workspace.NewRegistry("")
	for _, alias := range []string{"oracle-server", "sandbox-server"} {
		if err := reg.Add(workspace.ServerEntry{
			Alias: alias,
			Kind:  workspace.ServerKindSSH,
			Host:  alias + ".example.com",
			Port:  22,
			User:  "oracle",
		}); err != nil {
			t.Fatalf("add server %s: %v", alias, err)
		}
	}
	return tool.Context{
		Context:   context.Background(),
		Workspace: workspace.NewManager(reg, nil),
	}
}

func TestHasExplicitEndpointAlias(t *testing.T) {
	ctx := newTestContext(t)

	if !hasExplicitEndpointAlias(ctx, "oracle-server:/tmp/a.sql", "") {
		t.Fatal("expected alias:path to be explicit")
	}
	if !hasExplicitEndpointAlias(ctx, "/tmp/a.sql", "oracle-server") {
		t.Fatal("expected src_server/dst_server to be explicit")
	}
	if hasExplicitEndpointAlias(ctx, "/tmp/a.sql", "") {
		t.Fatal("plain path should not be explicit")
	}
	if hasExplicitEndpointAlias(ctx, `C:\tmp\a.sql`, "") {
		t.Fatal("windows drive path should not be treated as alias:path")
	}
}

func TestRequireExplicitCopyEndpoints_MultiRemote(t *testing.T) {
	ctx := newTestContext(t)

	err := requireExplicitCopyEndpoints(ctx, copyRequest{
		Src: "oracle-server:/tmp/a.sql",
		Dst: "/tmp/a.sql",
	})
	if err == nil {
		t.Fatal("expected explicit destination validation error")
	}

	if err := requireExplicitCopyEndpoints(ctx, copyRequest{
		Src: "oracle-server:/tmp/a.sql",
		Dst: "sandbox-server:/tmp/a.sql",
	}); err != nil {
		t.Fatalf("unexpected explicit endpoint validation error: %v", err)
	}
}
