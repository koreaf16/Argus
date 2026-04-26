package oracle

import (
	"strings"
	"testing"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/tools"
)

func TestBuildSQLPlusCmdAddsContainerSwitchForOSAuthPDB(t *testing.T) {
	cmd := buildSQLPlusCmd("AI26", "/ as sysdba", "SELECT SYS_CONTEXT('USERENV','CON_NAME') FROM dual", "AI_DB")

	if !strings.Contains(cmd, `ALTER SESSION SET CONTAINER = "AI_DB";`) {
		t.Fatalf("expected container switch in command:\n%s", cmd)
	}
	if strings.Index(cmd, "ALTER SESSION SET CONTAINER") > strings.Index(cmd, "SELECT SYS_CONTEXT") {
		t.Fatalf("container switch must run before user SQL:\n%s", cmd)
	}
}

func TestBuildSQLPlusCmdQuotesPDBIdentifier(t *testing.T) {
	cmd := buildSQLPlusCmd("AI26", "/ as sysdba", "SELECT 1 FROM dual", `A"I_DB`)
	if !strings.Contains(cmd, `ALTER SESSION SET CONTAINER = "A""I_DB";`) {
		t.Fatalf("expected quoted PDB identifier, got:\n%s", cmd)
	}
}

func TestGenerateToolsAddsAdvancedOracleTools(t *testing.T) {
	cdb := New()
	rootTools := cdb.GenerateTools(conn.ServiceInfo{
		Name:    "AI26",
		Details: map[string]string{"cdb": "yes"},
	}, conn.Credentials{Username: "/", Role: "sysdba", OSAuth: true})

	for _, name := range []string{
		"oracle_ai26.top_sql",
		"oracle_ai26.trace_profile",
		"oracle_ai26.sql_profile",
		"oracle_ai26.health_check",
		"oracle_ai26.awr_report",
		"oracle_ai26.ash_report",
		"oracle_ai26.tuning_recommend",
		"oracle_ai26.tuning_apply",
		"oracle_ai26.list_pdbs",
	} {
		if !hasTool(rootTools, name) {
			t.Fatalf("expected tool %s, got %v", name, cdb.ToolNames())
		}
	}

	pdb := New()
	pdbTools := pdb.GenerateTools(conn.ServiceInfo{
		Name:        "AI26",
		SubInstance: "AI_DB",
		Details:     map[string]string{"cdb": "yes"},
	}, conn.Credentials{Username: "/", Role: "sysdba", OSAuth: true})

	if hasTool(pdbTools, "oracle_ai26_ai_db.list_pdbs") {
		t.Fatalf("PDB connector should not expose list_pdbs, got %v", pdb.ToolNames())
	}
	for _, name := range []string{
		"oracle_ai26_ai_db.trace_profile",
		"oracle_ai26_ai_db.sql_profile",
		"oracle_ai26_ai_db.health_check",
		"oracle_ai26_ai_db.tuning_recommend",
		"oracle_ai26_ai_db.tuning_apply",
	} {
		if !hasTool(pdbTools, name) {
			t.Fatalf("expected PDB tool %s, got %v", name, pdb.ToolNames())
		}
	}
}

func TestBuildTopSQLQuery(t *testing.T) {
	query := buildTopSQLQuery("cpu", 10, 5, "HR")
	if !strings.Contains(query, "s.cpu_time DESC") {
		t.Fatalf("expected cpu sort, got query:\n%s", query)
	}
	if !strings.Contains(query, "ROWNUM <= 10") {
		t.Fatalf("expected limit, got query:\n%s", query)
	}
	if !strings.Contains(query, "executions >= 5") {
		t.Fatalf("expected min executions, got query:\n%s", query)
	}
	if !strings.Contains(query, "UPPER(s.parsing_schema_name) = UPPER('HR')") {
		t.Fatalf("expected schema filter, got query:\n%s", query)
	}
}

func TestBuildSQLProfileSummaryQueryIncludeTextToggle(t *testing.T) {
	withText := buildSQLProfileSummaryQuery("abc123", -1, true)
	withoutText := buildSQLProfileSummaryQuery("abc123", -1, false)
	if !strings.Contains(withText, "sql_text") {
		t.Fatalf("expected sql_text column when include_text=true:\n%s", withText)
	}
	if strings.Contains(withoutText, "sql_text") {
		t.Fatalf("did not expect sql_text column when include_text=false:\n%s", withoutText)
	}
}

func TestBuildTraceProfileShellCmdUsesDriverFile(t *testing.T) {
	cmd := buildTraceProfileShellCmd("AI26", "/ as sysdba", "AI_DB", "select 1 from dual;", "TRACE_A", "exeela,fchela")
	for _, want := range []string{
		`driver_file="$tmp_dir/driver.sql"`,
		`printf '@"%s"\n' "$sql_file"`,
		`TRACE_FILE=`,
		`TKPROF_FILE=`,
		`ALTER SESSION SET CONTAINER = "AI_DB";`,
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("trace profile command missing %q:\n%s", want, cmd)
		}
	}
}

func hasTool(toolList []tools.Tool, name string) bool {
	for _, t := range toolList {
		if t.Name() == name {
			return true
		}
	}
	return false
}
