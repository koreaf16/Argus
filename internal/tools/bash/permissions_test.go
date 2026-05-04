package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils/permissions"
)

func rawCommandInput(t *testing.T, command string) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"command": command})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return payload
}

func TestCheckBashPermissionsFallbackAllowsReadOnlyCommand(t *testing.T) {
	permCtx := types.ToolPermissionContext{
		Mode: types.PermissionModeDefault,
	}

	result, err := CheckBashPermissions(tool.Context{}, permCtx, rawCommandInput(t, "ls -la"))
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if result.Behavior != types.BehaviorAllow {
		t.Fatalf("expected allow for read-only command, got %s", result.Behavior)
	}
}

func TestCheckBashPermissionsFallbackAsksForMutatingCommand(t *testing.T) {
	permCtx := types.ToolPermissionContext{
		Mode: types.PermissionModeDefault,
	}

	result, err := CheckBashPermissions(tool.Context{}, permCtx, rawCommandInput(t, "rm -rf tmp"))
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if result.Behavior != types.BehaviorAsk {
		t.Fatalf("expected ask for mutating command, got %s", result.Behavior)
	}
}

func TestCheckBashPermissionsRespectsExplicitDenyRule(t *testing.T) {
	permCtx := types.ToolPermissionContext{
		Mode: types.PermissionModeDefault,
		AlwaysDenyRules: types.ToolPermissionRulesBySource{
			types.RuleSourceSession: {"bash(rm:*)"},
		},
	}

	result, err := CheckBashPermissions(tool.Context{}, permCtx, rawCommandInput(t, "rm -rf tmp"))
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if result.Behavior != types.BehaviorDeny {
		t.Fatalf("expected deny from explicit rule, got %s", result.Behavior)
	}
}

func TestCheckBashPermissionsExportAndMonitoring(t *testing.T) {
	permCtx := types.ToolPermissionContext{Mode: types.PermissionModeDefault}

	cases := []string{
		"export ORACLE_SID=AI26",
		"export ORACLE_HOME=/u01/app/oracle/product/19.0.0/dbhome_1",
		"export ORACLE_SID=AI26; export ORACLE_HOME=/u01/app/oracle/product/19.0.0/dbhome_1",
		"ps aux",
		"ps -ef",
		"df -h",
		"free -m",
		"id",
		"whoami",
		"uname -a",
		"date",
		"hostname",
		"env",
		"printenv PATH",
		"uptime",
	}
	for _, cmd := range cases {
		result, err := CheckBashPermissions(tool.Context{}, permCtx, rawCommandInput(t, cmd))
		if err != nil {
			t.Fatalf("check permission for %q: %v", cmd, err)
		}
		if result.Behavior != types.BehaviorAllow {
			t.Errorf("expected allow for %q (whitelist), got %s", cmd, result.Behavior)
		}
	}
}

func TestClassifyBashCommandWithMockLLM(t *testing.T) {
	cases := []struct {
		name        string
		llmResp     string
		wantAllow   bool
		wantUnavail bool
	}{
		{
			name:      "high confidence read-only → allow",
			llmResp:   `{"read_only": true, "confidence": "high", "reason": "ps only reads process info"}`,
			wantAllow: true,
		},
		{
			name:      "high confidence mutating → ask",
			llmResp:   `{"read_only": false, "confidence": "high", "reason": "rm deletes files"}`,
			wantAllow: false,
		},
		{
			name:      "low confidence → ask regardless",
			llmResp:   `{"read_only": true, "confidence": "low", "reason": "not sure"}`,
			wantAllow: false,
		},
		{
			name:      "medium confidence → ask regardless",
			llmResp:   `{"read_only": true, "confidence": "medium", "reason": "probably safe"}`,
			wantAllow: false,
		},
		{
			name:        "invalid JSON → unavailable",
			llmResp:     `not json`,
			wantAllow:   false,
			wantUnavail: true,
		},
		{
			name:      "JSON wrapped in text → extracted and parsed",
			llmResp:   `Here is the answer: {"read_only": true, "confidence": "high", "reason": "safe"}`,
			wantAllow: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockSubQuery := func(_ context.Context, _, _ string) (string, error) {
				return tc.llmResp, nil
			}
			result := permissions.ClassifyBashCommand(
				context.Background(), "some-unique-command-"+tc.name,
				permissions.NewDenialTrackingState(),
				permissions.SubQueryFunc(mockSubQuery),
				nil,
				nil,
			)
			if tc.wantUnavail && !result.Unavailable {
				t.Errorf("expected Unavailable=true, got false (behavior=%s)", result.Behavior)
			}
			gotAllow := result.Behavior == types.ClassifierBehaviorAllow
			if gotAllow != tc.wantAllow {
				t.Errorf("wantAllow=%v, got behavior=%s (unavailable=%v)", tc.wantAllow, result.Behavior, result.Unavailable)
			}
		})
	}
}

func TestClassifyBashCommandCache(t *testing.T) {
	var callCount atomic.Int32
	mockSubQuery := func(_ context.Context, _, _ string) (string, error) {
		callCount.Add(1)
		return `{"read_only": true, "confidence": "high", "reason": "cached test"}`, nil
	}

	cmd := fmt.Sprintf("cache-test-command-%d", callCount.Load())
	for i := 0; i < 3; i++ {
		permissions.ClassifyBashCommand(
			context.Background(), cmd,
			permissions.NewDenialTrackingState(),
			permissions.SubQueryFunc(mockSubQuery),
			nil,
			nil,
		)
	}
	if callCount.Load() != 1 {
		t.Errorf("expected LLM called once, got %d times", callCount.Load())
	}
}

func TestClassifyBashCommandNilSubQuery(t *testing.T) {
	result := permissions.ClassifyBashCommand(
		context.Background(), "ps aux",
		permissions.NewDenialTrackingState(),
		nil,
		nil,
		nil,
	)
	if !result.Unavailable {
		t.Error("expected Unavailable=true when subQuery is nil")
	}
	if result.Behavior != types.ClassifierBehaviorAsk {
		t.Errorf("expected Ask behavior, got %s", result.Behavior)
	}
}
