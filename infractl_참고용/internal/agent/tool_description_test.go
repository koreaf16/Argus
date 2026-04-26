package agent

import (
	"strings"
	"testing"
)

func TestEnsureToolDescriptionInfersShellIntent(t *testing.T) {
	args := map[string]any{
		"command": "/home/oracle/database/runInstaller -silent -responseFile /tmp/db.rsp",
	}

	ensureToolDescription("shell_exec", args)

	desc, _ := args["description"].(string)
	if !strings.Contains(desc, "Oracle") {
		t.Fatalf("description = %q, want Oracle intent", desc)
	}
}

func TestEnsureToolDescriptionPreservesExplicitReason(t *testing.T) {
	args := map[string]any{
		"command":     "dnf install -y oracle-database-preinstall-19c",
		"description": "사용자가 볼 설치 사전 점검",
	}

	ensureToolDescription("shell_exec", args)

	if got := args["description"]; got != "사용자가 볼 설치 사전 점검" {
		t.Fatalf("description = %q, want explicit description preserved", got)
	}
}
