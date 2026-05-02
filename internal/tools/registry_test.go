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
