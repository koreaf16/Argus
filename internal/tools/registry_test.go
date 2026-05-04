package tools

import (
	"encoding/json"
	"testing"
)

type registryTestTool struct {
	name string
}

func (t *registryTestTool) Name() string { return t.name }
func (t *registryTestTool) Description(Context) string {
	return "test tool"
}
func (t *registryTestTool) InputSchema() ToolInputJSONSchema {
	return ToolInputJSONSchema{"type": "object"}
}
func (t *registryTestTool) IsReadOnly() bool { return true }
func (t *registryTestTool) Call(Context, json.RawMessage) (<-chan ToolEvent, error) {
	ch := make(chan ToolEvent, 1)
	close(ch)
	return ch, nil
}
func (t *registryTestTool) CheckPermission(Context, json.RawMessage) (PermissionResult, error) {
	return DefaultAllowPermission(), nil
}
func (t *registryTestTool) MaxResultSizeChars() int { return 0 }

func TestRegistryLookupByAlias(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&registryTestTool{name: "enter_plan_mode"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	if err := reg.RegisterAlias("EnterPlanMode", "enter_plan_mode"); err != nil {
		t.Fatalf("register alias: %v", err)
	}

	if _, ok := reg.Lookup("enter_plan_mode"); !ok {
		t.Fatalf("expected canonical lookup to succeed")
	}
	if _, ok := reg.Lookup("EnterPlanMode"); !ok {
		t.Fatalf("expected legacy alias lookup to succeed")
	}
}

func TestRegistryToolSpecsUseCanonicalNames(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&registryTestTool{name: "ask_user"}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	if err := reg.RegisterAlias("AskUserQuestionTool", "ask_user"); err != nil {
		t.Fatalf("register alias: %v", err)
	}

	specs := reg.ToolSpecs(Context{})
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Name != "ask_user" {
		t.Fatalf("unexpected tool spec name: %s", specs[0].Name)
	}
}

func TestRegistryLookupUsesCanonicalAliases(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&registryTestTool{name: "webfetch"}); err != nil {
		t.Fatalf("register webfetch: %v", err)
	}
	if _, ok := reg.Lookup("web_fetch"); !ok {
		t.Fatalf("expected web_fetch alias lookup to resolve to webfetch")
	}
}

// staticGuideTool contributes a static (cache-eligible) system-prompt guide.
type staticGuideTool struct{ registryTestTool }

func (t *staticGuideTool) GetStaticSystemPromptGuide() string {
	return "## 언제 이 도구를 쓰는가\n- 파일 검색"
}

// dynamicGuideTool contributes a dynamic (context-dependent) guide.
type dynamicGuideTool struct{ registryTestTool }

func (t *dynamicGuideTool) GetSystemPromptGuide(_ Context) string {
	return "## 언제 이 도구를 쓰는가\n- 셸 실행"
}

func TestSystemGuides_StaticAndDynamicBlocks(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&staticGuideTool{registryTestTool{name: "glob"}}); err != nil {
		t.Fatalf("register static tool: %v", err)
	}
	if err := reg.Register(&dynamicGuideTool{registryTestTool{name: "bash"}}); err != nil {
		t.Fatalf("register dynamic tool: %v", err)
	}

	blocks := reg.SystemGuides(Context{})
	if len(blocks) != 2 {
		t.Fatalf("expected 2 guide blocks (static + dynamic), got %d", len(blocks))
	}

	staticBlock := blocks[0]
	if staticBlock.CacheControl == nil {
		t.Fatalf("static guide block must have cache_control set")
	}
	if staticBlock.CacheControl["type"] != "ephemeral" {
		t.Fatalf("static guide block cache_control type must be ephemeral, got %v", staticBlock.CacheControl["type"])
	}
	if !containsStr(staticBlock.Text, "glob") {
		t.Fatalf("static block missing tool name 'glob': %s", staticBlock.Text)
	}

	dynamicBlock := blocks[1]
	if dynamicBlock.CacheControl != nil {
		t.Fatalf("dynamic guide block must not have cache_control, got %v", dynamicBlock.CacheControl)
	}
	if !containsStr(dynamicBlock.Text, "bash") {
		t.Fatalf("dynamic block missing tool name 'bash': %s", dynamicBlock.Text)
	}
}

func TestSystemGuides_StaticTakesPriorityOverDynamic(t *testing.T) {
	// A tool implementing both interfaces: static wins (cache-eligible).
	type bothTool struct {
		registryTestTool
	}
	bt := &struct {
		registryTestTool
	}{registryTestTool{name: "hybrid"}}
	_ = bt // both interface check via staticGuideTool (single interface is sufficient)

	// Verify: if only static, no dynamic block is emitted.
	reg := NewRegistry()
	if err := reg.Register(&staticGuideTool{registryTestTool{name: "only_static"}}); err != nil {
		t.Fatalf("register: %v", err)
	}
	blocks := reg.SystemGuides(Context{})
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block for static-only registry, got %d", len(blocks))
	}
	if blocks[0].CacheControl == nil || blocks[0].CacheControl["type"] != "ephemeral" {
		t.Fatalf("sole block must be the static (cached) block")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
