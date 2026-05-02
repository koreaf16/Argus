package query

import (
	"context"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/toolsearch"
	"github.com/koreaf16/argus/internal/types"
)

func TestEvidenceToolExposureFiltersToCoreAndRequiredEvidence(t *testing.T) {
	reg := tool.NewRegistry()
	registerScriptedTool(t, reg, "ask_user", false)
	if err := reg.Register(toolsearch.New()); err != nil {
		t.Fatalf("register tool_search: %v", err)
	}
	registerScriptedTool(t, reg, "task_plan_init", false)
	registerScriptedTool(t, reg, "grep", false)
	registerScriptedTool(t, reg, "filewrite", true)
	registerScriptedTool(t, reg, "web_search", false)

	specs := reg.ToolSpecs(tool.Context{Context: context.Background(), Registry: reg})
	plan := buildEvidencePlan("implement the code fix", nil)
	filtered := filterToolSpecsForEvidence(specs, reg, plan, nil)
	names := specNameSet(filtered)

	for _, want := range []string{"ask_user", "tool_search", "task_plan_init", "grep", "filewrite"} {
		if !names[tool.CanonicalName(want)] {
			t.Fatalf("expected %s to be exposed, got %#v", want, names)
		}
	}
	if names[tool.CanonicalName("web_search")] {
		t.Fatalf("web_search should not be exposed for a local code mutation request")
	}
}

func TestEvidenceToolExposureWorkflowAllowListWins(t *testing.T) {
	reg := tool.NewRegistry()
	if err := reg.Register(toolsearch.New()); err != nil {
		t.Fatalf("register tool_search: %v", err)
	}
	registerScriptedTool(t, reg, "web_search", false)
	registerScriptedTool(t, reg, "filewrite", true)

	st := state.NewAppState()
	st.SetWorkflowCard(&state.WorkflowCard{
		Title: "research",
		Phase: state.WorkflowPhaseResearch,
	})
	specs := filterToolSpecs(reg.ToolSpecs(tool.Context{Registry: reg}), st)
	plan := buildEvidencePlan("implement the latest documented fix", st)
	filtered := filterToolSpecsForEvidence(specs, reg, plan, nil)
	names := specNameSet(filtered)

	if !names[tool.CanonicalName("web_search")] {
		t.Fatalf("web_search should remain available in research phase")
	}
	if names[tool.CanonicalName("filewrite")] {
		t.Fatalf("filewrite should stay hidden because workflow phase filtering wins")
	}
}

func TestEvidenceToolExposureSelectedToolCanBeExposed(t *testing.T) {
	reg := tool.NewRegistry()
	if err := reg.Register(toolsearch.New()); err != nil {
		t.Fatalf("register tool_search: %v", err)
	}
	registerScriptedTool(t, reg, "mcp__docs__lookup", false)

	specs := reg.ToolSpecs(tool.Context{Registry: reg})
	plan := buildEvidencePlan("latest docs", nil)
	filtered := filterToolSpecsForEvidence(specs, reg, plan, nil)
	if specNameSet(filtered)[tool.CanonicalName("mcp__docs__lookup")] {
		t.Fatalf("MCP bridge tool should be hidden until selected")
	}

	selected := map[string]bool{tool.CanonicalName("mcp__docs__lookup"): true}
	filtered = filterToolSpecsForEvidence(specs, reg, plan, selected)
	if !specNameSet(filtered)[tool.CanonicalName("mcp__docs__lookup")] {
		t.Fatalf("selected MCP bridge tool should be exposed")
	}
}

func TestEvidenceGateBlocksHiddenToolCall(t *testing.T) {
	stopToolUse := llm.StopReasonToolUse
	llmScript := &scriptedLLM{
		sequences: [][]llm.Event{
			{
				{Kind: llm.EventToolUseStart, ToolUse: &llm.ToolUseStart{ID: "hidden-1", Name: "web_search", Input: mustRawJSON(t, map[string]any{"query": "not needed"})}},
				{Kind: llm.EventStop, Stop: &stopToolUse},
			},
		},
	}

	reg := tool.NewRegistry()
	if err := reg.Register(toolsearch.New()); err != nil {
		t.Fatalf("register tool_search: %v", err)
	}
	registerScriptedTool(t, reg, "web_search", false)

	engine := NewEngine(llmScript, reg, state.NewAppState(), nil)
	cfg := DefaultConfig()
	cfg.MaxToolIterations = 1
	cfg.PersistenceEnabled = false
	engine.SetConfig(cfg)

	stream, err := engine.SubmitMessage(context.Background(), "implement code change")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	var result string
	for evt := range stream {
		if evt.Kind == UIEventToolResult {
			result = evt.ToolResult
		}
	}
	if !strings.Contains(result, "not available for the current evidence-gated step") {
		t.Fatalf("expected hidden tool block, got %q", result)
	}
}

func TestEvidenceGateBlocksMutationBeforeEvidence(t *testing.T) {
	stopToolUse := llm.StopReasonToolUse
	llmScript := &scriptedLLM{
		sequences: [][]llm.Event{
			{
				{Kind: llm.EventToolUseStart, ToolUse: &llm.ToolUseStart{ID: "write-1", Name: "filewrite", Input: mustRawJSON(t, map[string]any{"path": "x.go", "content": "package x"})}},
				{Kind: llm.EventStop, Stop: &stopToolUse},
			},
		},
	}

	reg := tool.NewRegistry()
	if err := reg.Register(toolsearch.New()); err != nil {
		t.Fatalf("register tool_search: %v", err)
	}
	registerScriptedTool(t, reg, "grep", false)
	registerScriptedTool(t, reg, "filewrite", true)

	engine := NewEngine(llmScript, reg, state.NewAppState(), nil)
	cfg := DefaultConfig()
	cfg.MaxToolIterations = 1
	cfg.PersistenceEnabled = false
	engine.SetConfig(cfg)

	stream, err := engine.SubmitMessage(context.Background(), "implement code change")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	var result string
	for evt := range stream {
		if evt.Kind == UIEventToolResult {
			result = evt.ToolResult
		}
	}
	if !strings.Contains(result, "blocked until required evidence is collected") {
		t.Fatalf("expected evidence prerequisite block, got %q", result)
	}
}

func registerScriptedTool(t *testing.T, reg *tool.Registry, name string, writable bool) {
	t.Helper()
	if err := reg.Register(&scriptedTool{
		name:       name,
		permission: types.BehaviorAllow,
		output:     "ok",
		writable:   writable,
	}); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

func specNameSet(specs []llm.ToolSpec) map[string]bool {
	out := make(map[string]bool, len(specs))
	for _, spec := range specs {
		out[tool.CanonicalName(spec.Name)] = true
	}
	return out
}
