package oracle

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
)

type schemaGuardTestExecutor struct{}

func (schemaGuardTestExecutor) Execute(_ context.Context, cmd string) (executor.ExecResult, error) {
	switch {
	case strings.Contains(cmd, "SELECT cdb FROM v$database;"):
		return executor.ExecResult{Stdout: "YES", ExitCode: 0}, nil
	case strings.Contains(cmd, "FROM cdb_tables t"):
		return executor.ExecResult{Stdout: "AI_DB (Owner: APP)", ExitCode: 0}, nil
	default:
		return executor.ExecResult{Stdout: "", ExitCode: 0}, nil
	}
}

func (schemaGuardTestExecutor) Target() string { return "localhost" }
func (schemaGuardTestExecutor) Host() string   { return "localhost" }

func TestSearchTableInCDBGuidancePrefersPDBConnectorTool(t *testing.T) {
	out := searchTableInCDB(context.Background(), "AI26", "/ as sysdba", "", "APP_USERS", schemaGuardTestExecutor{})

	if !strings.Contains(out, "oracle_<sid>_<pdb>.query") {
		t.Fatalf("expected pdb-scoped connector guidance, got: %q", out)
	}
	if strings.Contains(strings.ToUpper(out), "ALTER SESSION SET CONTAINER") {
		t.Fatalf("expected ALTER SESSION guidance to be removed, got: %q", out)
	}
}
