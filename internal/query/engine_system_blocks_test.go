package query

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	tool "github.com/koreaf16/argus/internal/tools"
)

// staticGuideTestTool contributes a static (cache-eligible) guide block.
type staticGuideTestTool struct {
	name string
}

func (t *staticGuideTestTool) Name() string              { return t.name }
func (t *staticGuideTestTool) Description(tool.Context) string { return "test static" }
func (t *staticGuideTestTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{"type": "object"}
}
func (t *staticGuideTestTool) IsReadOnly() bool { return true }
func (t *staticGuideTestTool) MaxResultSizeChars() int { return 0 }
func (t *staticGuideTestTool) CheckPermission(tool.Context, json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}
func (t *staticGuideTestTool) Call(_ tool.Context, _ json.RawMessage) (<-chan tool.ToolEvent, error) {
	ch := make(chan tool.ToolEvent)
	close(ch)
	return ch, nil
}
func (t *staticGuideTestTool) GetStaticSystemPromptGuide() string {
	return "## 언제 이 도구를 쓰는가\n- 정적 가이드 내용"
}

// dynamicGuideTestTool contributes a dynamic (context-dependent) guide block.
type dynamicGuideTestTool struct {
	name string
}

func (t *dynamicGuideTestTool) Name() string              { return t.name }
func (t *dynamicGuideTestTool) Description(tool.Context) string { return "test dynamic" }
func (t *dynamicGuideTestTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{"type": "object"}
}
func (t *dynamicGuideTestTool) IsReadOnly() bool { return false }
func (t *dynamicGuideTestTool) MaxResultSizeChars() int { return 0 }
func (t *dynamicGuideTestTool) CheckPermission(tool.Context, json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}
func (t *dynamicGuideTestTool) Call(_ tool.Context, _ json.RawMessage) (<-chan tool.ToolEvent, error) {
	ch := make(chan tool.ToolEvent)
	close(ch)
	return ch, nil
}
func (t *dynamicGuideTestTool) GetSystemPromptGuide(_ tool.Context) string {
	return "## 언제 이 도구를 쓰는가\n- 동적 가이드 내용"
}

// TestSystemBlockOrdering verifies the system block assembly order:
// base system → workspace → lane → tool-guide-static → tool-guide-dynamic.
func TestSystemBlockOrdering(t *testing.T) {
	reg := tool.NewRegistry()
	if err := reg.Register(&staticGuideTestTool{name: "glob"}); err != nil {
		t.Fatalf("register static tool: %v", err)
	}
	if err := reg.Register(&dynamicGuideTestTool{name: "bash"}); err != nil {
		t.Fatalf("register dynamic tool: %v", err)
	}

	baseBlock := llm.SystemBlock{Type: "text", Text: "BASE_SYSTEM"}
	wsBlock := llm.SystemBlock{Type: "text", Text: "WORKSPACE_INFO"}
	laneBlock := llm.SystemBlock{Type: "text", Text: "LANE_INFO"}

	guideBlocks := reg.SystemGuides(tool.Context{})

	// Assemble in the same order as engine_run.go line 288-289.
	sysBlocks := JoinSystemBlocks(
		[]llm.SystemBlock{baseBlock},
		[]llm.SystemBlock{wsBlock},
		[]llm.SystemBlock{laneBlock},
	)
	sysBlocks = append(sysBlocks, guideBlocks...)

	if len(sysBlocks) < 5 {
		t.Fatalf("expected at least 5 blocks (base+ws+lane+static+dynamic), got %d", len(sysBlocks))
	}

	// Positions: base=0, ws=1, lane=2, static=3, dynamic=4.
	if !strings.Contains(sysBlocks[0].Text, "BASE_SYSTEM") {
		t.Errorf("block[0] must be base system: %s", sysBlocks[0].Text)
	}
	if !strings.Contains(sysBlocks[1].Text, "WORKSPACE_INFO") {
		t.Errorf("block[1] must be workspace: %s", sysBlocks[1].Text)
	}
	if !strings.Contains(sysBlocks[2].Text, "LANE_INFO") {
		t.Errorf("block[2] must be lane: %s", sysBlocks[2].Text)
	}

	// Static guide block must have cache_control:ephemeral and come before dynamic.
	staticIdx := -1
	dynamicIdx := -1
	for i, b := range sysBlocks {
		if strings.Contains(b.Text, "glob") {
			staticIdx = i
		}
		if strings.Contains(b.Text, "bash") {
			dynamicIdx = i
		}
	}
	if staticIdx < 0 {
		t.Fatal("static guide block not found in sysBlocks")
	}
	if dynamicIdx < 0 {
		t.Fatal("dynamic guide block not found in sysBlocks")
	}
	if staticIdx >= dynamicIdx {
		t.Errorf("static guide block (idx=%d) must come before dynamic (idx=%d)", staticIdx, dynamicIdx)
	}
	if sysBlocks[staticIdx].CacheControl == nil || sysBlocks[staticIdx].CacheControl["type"] != "ephemeral" {
		t.Errorf("static guide block must carry cache_control:ephemeral, got %v", sysBlocks[staticIdx].CacheControl)
	}
	if sysBlocks[dynamicIdx].CacheControl != nil {
		t.Errorf("dynamic guide block must NOT have cache_control, got %v", sysBlocks[dynamicIdx].CacheControl)
	}
}
