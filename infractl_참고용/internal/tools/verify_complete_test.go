package tools

import (
	"context"
	"strings"
	"testing"
)

func TestVerifyCompleteToolExecuteValidation(t *testing.T) {
	tool := &VerifyCompleteTool{}
	ctx := context.Background()

	tests := []struct {
		name     string
		args     map[string]interface{}
		wantText string
	}{
		{
			name:     "missing summary and evidence",
			args:     map[string]interface{}{},
			wantText: "Error: 'summary' and 'verification_evidence' are required",
		},
		{
			name: "accepts tool id",
			args: map[string]interface{}{
				"tool_id":               "call_shell_1",
				"summary":               "Oracle preinstall package is present",
				"verification_evidence": "rpm -q oracle-database-preinstall-19c returned installed",
			},
			wantText: "Verification recorded for call_shell_1",
		},
		{
			name: "falls back to task key",
			args: map[string]interface{}{
				"task_key":              "oracle-preinstall",
				"summary":               "Oracle preinstall package is present",
				"verification_evidence": "rpm -q oracle-database-preinstall-19c returned installed",
			},
			wantText: "Verification recorded for oracle-preinstall",
		},
		{
			name: "auto chained header",
			args: map[string]interface{}{
				"task_key":              "job:42",
				"summary":               "Background job completed",
				"verification_evidence": "job manager reported success",
				"auto_chained":          true,
			},
			wantText: "[자동 후속 검증]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tool.Execute(ctx, tt.args, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(got.Content, tt.wantText) {
				t.Fatalf("content = %q, want substring %q", got.Content, tt.wantText)
			}
		})
	}
}

func TestVerifyCompleteToolIsMutation(t *testing.T) {
	if (&VerifyCompleteTool{}).IsReadOnly() {
		t.Fatal("verify_complete should run as a mutation tool so verification state is serialized")
	}
}
