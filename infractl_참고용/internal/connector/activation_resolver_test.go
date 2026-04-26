package connector

import (
	"strings"
	"testing"
)

func TestSynthesizeOracleTraceProfileUsesDriverFile(t *testing.T) {
	cmds := synthesizeOracleCommands(ActivationRequest{
		ServerName:  "oracle",
		ServiceName: "AI_DB",
		Host:        "127.0.0.1",
		Port:        1521,
	}, []ActivationEvidence{{Title: "tkprof available", Category: "probe"}})

	trace, ok := cmds["trace_profile"]
	if !ok {
		t.Fatalf("expected trace_profile command")
	}
	if trace.ReadOnly {
		t.Fatalf("trace_profile accepts arbitrary SQL and must not be marked read-only")
	}

	command := trace.Command
	if strings.Contains(command, "@${sql_file}") {
		t.Fatalf("trace_profile must not rely on unexpanded @${sql_file} inside SQL*Plus heredoc:\n%s", command)
	}
	for _, want := range []string{
		`driver_file="$tmp_dir/driver.sql"`,
		`printf '@"%s"\n' "$sql_file"`,
		`TRACE_FILE=`,
		`v\$diag_info`,
		`TRACE_FILE_NOT_READABLE`,
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("trace_profile command missing %q:\n%s", want, command)
		}
	}
}
