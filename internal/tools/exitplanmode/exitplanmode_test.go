package exitplanmode

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/toolfs"
	"github.com/koreaf16/argus/internal/types"
)

func TestExitPlanModeNormalizesAllowedPromptsAndWritesPlanSteps(t *testing.T) {
	wd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	app := state.NewAppState()
	app.SetSessionID("s1")
	app.SetMode("plan")
	app.SetPermissionMode(types.PermissionModePlan)
	app.SetPrePlanMode(types.PermissionModeDontAsk)

	if _, err := toolfs.WritePlan("s1", "draft"); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	raw, err := json.Marshal(map[string]any{
		"plan": "draft submitted through tool input",
		"allowedPrompts": []map[string]any{
			{"tool": " Bash ", "prompt": " ls -la ", "server": " oracle-server ", "role": " source_mysql ", "channel": " source ", "as_user": " oracle ", "privilege_method": " sudo "},
			{"tool": "python", "prompt": "print(1)"},
			{"tool": "powershell", "prompt": "  Get-Process  "},
			{"tool": "bash", "prompt": "   "},
		},
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	ch, err := NewExitPlanModeTool().Call(tool.Context{
		Context: context.Background(),
		State:   app,
	}, raw)
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	var payload struct {
		RestoredMode   string `json:"restored_mode"`
		Plan           string `json:"plan"`
		PlanFile       string `json:"plan_file"`
		AllowedPrompts []struct {
			Tool            string `json:"tool"`
			Prompt          string `json:"prompt"`
			Server          string `json:"server"`
			Role            string `json:"role"`
			Channel         string `json:"channel"`
			AsUser          string `json:"as_user"`
			PrivilegeMethod string `json:"privilege_method"`
		} `json:"allowed_prompts"`
		AllowedPromptCount int `json:"allowed_prompts_count"`
	}
	for ev := range ch {
		if ev.Kind != tool.ToolEventOutput {
			continue
		}
		if err := json.Unmarshal([]byte(ev.Output), &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
	}

	if payload.RestoredMode != string(types.PermissionModeDontAsk) {
		t.Fatalf("expected restored mode %q, got %q", types.PermissionModeDontAsk, payload.RestoredMode)
	}
	if payload.AllowedPromptCount != 2 || len(payload.AllowedPrompts) != 2 {
		t.Fatalf("expected 2 allowed prompts, got count=%d len=%d", payload.AllowedPromptCount, len(payload.AllowedPrompts))
	}
	if payload.AllowedPrompts[0].Tool != "bash" || payload.AllowedPrompts[0].Prompt != "ls -la" {
		t.Fatalf("unexpected first allowed prompt: %+v", payload.AllowedPrompts[0])
	}
	if payload.AllowedPrompts[0].Server != "oracle-server" {
		t.Fatalf("unexpected first allowed prompt server: %+v", payload.AllowedPrompts[0])
	}
	if payload.AllowedPrompts[0].Role != "source_mysql" || payload.AllowedPrompts[0].Channel != "source" || payload.AllowedPrompts[0].AsUser != "oracle" || payload.AllowedPrompts[0].PrivilegeMethod != "sudo" {
		t.Fatalf("unexpected first allowed prompt context: %+v", payload.AllowedPrompts[0])
	}
	if payload.AllowedPrompts[1].Tool != "powershell" || payload.AllowedPrompts[1].Prompt != "Get-Process" {
		t.Fatalf("unexpected second allowed prompt: %+v", payload.AllowedPrompts[1])
	}
	if !strings.Contains(payload.Plan, "1. [bash@oracle-server,role=source_mysql,channel=source,as=oracle,via=sudo] ls -la") || !strings.Contains(payload.Plan, "2. [powershell] Get-Process") {
		t.Fatalf("plan text missing numbered steps: %q", payload.Plan)
	}
	if !strings.Contains(payload.Plan, "draft submitted through tool input") {
		t.Fatalf("plan text missing submitted plan: %q", payload.Plan)
	}
	if payload.PlanFile == "" {
		t.Fatal("expected plan_file path")
	}

	planText, _, err := toolfs.LoadPlan("s1")
	if err != nil {
		t.Fatalf("load plan: %v", err)
	}
	if !strings.Contains(planText, "1. [bash@oracle-server,role=source_mysql,channel=source,as=oracle,via=sudo] ls -la") {
		t.Fatalf("plan file was not updated: %q", planText)
	}
	if app.InPlanMode() {
		t.Fatal("expected app state to exit plan mode")
	}
}
