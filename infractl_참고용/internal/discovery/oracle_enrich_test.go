package discovery

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
)

type mockExec struct {
	executor.Executor
	outputs map[string]executor.ExecResult
}

func (m *mockExec) Execute(ctx context.Context, cmd string) (executor.ExecResult, error) {
	for k, v := range m.outputs {
		if k == cmd {
			return v, nil
		}
	}
	// Regex match for complex commands
	if strings.Contains(cmd, "lsnrctl status") {
		return m.outputs["lsnrctl status"], nil
	}
	if strings.Contains(cmd, "sqlplus") {
		return m.outputs["sqlplus"], nil
	}
	if strings.Contains(cmd, "readlink") {
		return m.outputs["readlink"], nil
	}
	return executor.ExecResult{ExitCode: 1}, nil
}

func (m *mockExec) Target() string { return "test-server" }
func (m *mockExec) Host() string   { return "localhost" }

func TestEnrichViaSQLPlus(t *testing.T) {
	mock := &mockExec{
		outputs: map[string]executor.ExecResult{
			"sqlplus": {
				ExitCode: 0,
				Stdout: `
CDB:YES
PDB:HRPDB:READ WRITE
PDB:SALESPDB:MOUNTED
`,
			},
		},
	}

	svc := &DiscoveredService{
		Type:    ServiceOracle,
		Name:    "ORCL",
		User:    "oracle",
		Details: make(map[string]string),
	}

	ok := enrichViaSQLPlus(context.Background(), mock, svc, "/u01/app/oracle")
	if !ok {
		t.Fatal("enrichViaSQLPlus failed")
	}

	if svc.Details["cdb"] != "yes" {
		t.Errorf("expected cdb=yes, got %s", svc.Details["cdb"])
	}

	if svc.Details["pdbs"] != "HRPDB:READ WRITE,SALESPDB:MOUNTED" {
		t.Errorf("expected pdbs list, got %s", svc.Details["pdbs"])
	}
}

func TestEnrichViaListener(t *testing.T) {
	mock := &mockExec{
		outputs: map[string]executor.ExecResult{
			"lsnrctl status": {
				ExitCode: 0,
				Stdout: `
Services Summary...
Service "HRPDB" has 1 instance(s).
Service "ORCL" has 1 instance(s).
Service "ORCLXDB" has 1 instance(s).
`,
			},
		},
	}

	svc := &DiscoveredService{
		Type:    ServiceOracle,
		Name:    "ORCL",
		User:    "oracle",
		Details: make(map[string]string),
	}

	ok := enrichViaListener(context.Background(), mock, svc, "/u01/app/oracle")
	if !ok {
		t.Fatal("enrichViaListener failed")
	}

	if svc.Details["pdbs"] != "HRPDB" {
		t.Errorf("expected pdbs=HRPDB, got %s", svc.Details["pdbs"])
	}
}
