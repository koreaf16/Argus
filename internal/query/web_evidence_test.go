package query

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type scriptedLLM struct {
	mu        sync.Mutex
	sequences [][]llm.Event
	index     int
}

func (s *scriptedLLM) Stream(context.Context, llm.Request) (<-chan llm.Event, error) {
	s.mu.Lock()
	idx := s.index
	s.index++
	s.mu.Unlock()

	events := []llm.Event{}
	if idx < len(s.sequences) {
		events = s.sequences[idx]
	} else {
		stop := llm.StopReasonEndTurn
		events = []llm.Event{{Kind: llm.EventStop, Stop: &stop}}
	}

	ch := make(chan llm.Event, len(events))
	for _, evt := range events {
		ch <- evt
	}
	close(ch)
	return ch, nil
}

func (s *scriptedLLM) CountTokens(context.Context, llm.Request) (int, error) {
	return 30, nil
}

func (s *scriptedLLM) Capabilities() llm.Caps {
	return llm.Caps{Tools: true}
}

func (s *scriptedLLM) Provider() string {
	return "scripted"
}

type scriptedTool struct {
	name       string
	output     string
	permission types.PermissionBehavior
}

func (t *scriptedTool) Name() string { return t.name }
func (t *scriptedTool) Description(tool.Context) string {
	return "scripted tool"
}
func (t *scriptedTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{"type": "object"}
}
func (t *scriptedTool) IsReadOnly() bool { return true }
func (t *scriptedTool) MaxResultSizeChars() int {
	return 0
}
func (t *scriptedTool) CheckPermission(tool.Context, json.RawMessage) (tool.PermissionResult, error) {
	return tool.PermissionResult{Behavior: t.permission}, nil
}
func (t *scriptedTool) Call(_ tool.Context, _ json.RawMessage) (<-chan tool.ToolEvent, error) {
	ch := make(chan tool.ToolEvent, 2)
	ch <- tool.NewOutputEvent(t.output)
	ch <- tool.NewDoneEvent()
	close(ch)
	return ch, nil
}

func TestWebEvidenceGate_RequiresWebFetchBeforeFinalAnswer(t *testing.T) {
	stopToolUse := llm.StopReasonToolUse
	stopEnd := llm.StopReasonEndTurn
	llmScript := &scriptedLLM{
		sequences: [][]llm.Event{
			{
				{Kind: llm.EventToolUseStart, ToolUse: &llm.ToolUseStart{ID: "ws-1", Name: "web_search", Input: mustRawJSON(t, map[string]any{"query": "gemma latest news"})}},
				{Kind: llm.EventStop, Stop: &stopToolUse},
			},
			{
				{Kind: llm.EventTextDelta, Delta: "snippet-only answer"},
				{Kind: llm.EventStop, Stop: &stopEnd},
			},
			{
				{Kind: llm.EventToolUseStart, ToolUse: &llm.ToolUseStart{ID: "wf-1", Name: "webfetch", Input: mustRawJSON(t, map[string]any{"url": "https://ai.google.dev/gemma/docs/releases", "prompt": "extract date and key changes"})}},
				{Kind: llm.EventStop, Stop: &stopToolUse},
			},
			{
				{Kind: llm.EventTextDelta, Delta: "verified answer\nSources:\n- https://ai.google.dev/gemma/docs/releases"},
				{Kind: llm.EventStop, Stop: &stopEnd},
			},
		},
	}

	reg := tool.NewRegistry()
	if err := reg.Register(&scriptedTool{
		name:       "web_search",
		permission: types.BehaviorAllow,
		output:     `{"query":"gemma latest news","results":[{"content":[{"url":"https://ai.google.dev/gemma/docs/releases","title":"Gemma releases","snippet":"release note"}]}]}`,
	}); err != nil {
		t.Fatalf("register web_search: %v", err)
	}
	if err := reg.Register(&scriptedTool{
		name:       "webfetch",
		permission: types.BehaviorAllow,
		output:     `{"code":200,"result":"release date: 2026-04-03","url":"https://ai.google.dev/gemma/docs/releases"}`,
	}); err != nil {
		t.Fatalf("register webfetch: %v", err)
	}

	engine := NewEngine(llmScript, reg, state.NewAppState(), nil)
	cfg := DefaultConfig()
	cfg.MaxToolIterations = 8
	engine.SetConfig(cfg)

	stream, err := engine.SubmitMessage(context.Background(), "today latest gemma news")
	if err != nil {
		t.Fatalf("submit message: %v", err)
	}

	var assistantText strings.Builder
	var sawWebFetch bool
	for evt := range stream {
		if evt.Kind == UIEventAssistantDelta {
			assistantText.WriteString(evt.Delta)
		}
		if evt.Kind == UIEventToolUse && evt.ToolUse != nil && strings.EqualFold(evt.ToolUse.Name, "webfetch") {
			sawWebFetch = true
		}
	}

	out := assistantText.String()
	if strings.Contains(out, "snippet-only answer") {
		t.Fatalf("snippet-only answer should be buffered and hidden, got: %q", out)
	}
	if !strings.Contains(out, "verified answer") {
		t.Fatalf("expected verified final answer, got: %q", out)
	}
	if !sawWebFetch {
		t.Fatalf("expected webfetch call before final answer")
	}
}

func TestWebEvidenceGate_FailsClosedWhenWebFetchNotCompleted(t *testing.T) {
	stopToolUse := llm.StopReasonToolUse
	stopEnd := llm.StopReasonEndTurn
	llmScript := &scriptedLLM{
		sequences: [][]llm.Event{
			{
				{Kind: llm.EventToolUseStart, ToolUse: &llm.ToolUseStart{ID: "ws-1", Name: "web_search", Input: mustRawJSON(t, map[string]any{"query": "gemma latest"})}},
				{Kind: llm.EventStop, Stop: &stopToolUse},
			},
			{
				{Kind: llm.EventTextDelta, Delta: "attempt one"},
				{Kind: llm.EventStop, Stop: &stopEnd},
			},
			{
				{Kind: llm.EventTextDelta, Delta: "attempt two"},
				{Kind: llm.EventStop, Stop: &stopEnd},
			},
			{
				{Kind: llm.EventTextDelta, Delta: "attempt three"},
				{Kind: llm.EventStop, Stop: &stopEnd},
			},
		},
	}

	reg := tool.NewRegistry()
	if err := reg.Register(&scriptedTool{
		name:       "web_search",
		permission: types.BehaviorAllow,
		output:     `{"query":"gemma latest","results":[{"content":[{"url":"https://example.com/post","title":"post","snippet":"snippet"}]}]}`,
	}); err != nil {
		t.Fatalf("register web_search: %v", err)
	}

	engine := NewEngine(llmScript, reg, state.NewAppState(), nil)
	cfg := DefaultConfig()
	cfg.MaxToolIterations = 10
	engine.SetConfig(cfg)

	stream, err := engine.SubmitMessage(context.Background(), "latest gemma news")
	if err != nil {
		t.Fatalf("submit message: %v", err)
	}

	var assistantText strings.Builder
	for evt := range stream {
		if evt.Kind == UIEventAssistantDelta {
			assistantText.WriteString(evt.Delta)
		}
	}

	out := assistantText.String()
	if strings.Contains(out, "attempt one") || strings.Contains(out, "attempt two") || strings.Contains(out, "attempt three") {
		t.Fatalf("buffered premature summaries must be hidden, got: %q", out)
	}
	if !strings.Contains(out, "Web verification was required but no successful webfetch was completed.") {
		t.Fatalf("expected fail-closed message, got: %q", out)
	}
}

func TestClassifyWebEvidencePolicy_StrongRules(t *testing.T) {
	p1 := classifyWebEvidencePolicy(context.Background(), nil, "오늘 gemma 최신 뉴스 찾아봐", "")
	if !p1.Enabled {
		t.Fatalf("expected strong-enable policy for latest/news query")
	}

	p2 := classifyWebEvidencePolicy(context.Background(), nil, "이 코드 리팩터링 해줘", "")
	if p2.Enabled {
		t.Fatalf("expected strong-disable policy for code refactor query")
	}
}

func TestClassifyWebEvidencePolicy_SiteHintDomains(t *testing.T) {
	p := classifyWebEvidencePolicy(context.Background(), nil, "qwen 27b 모델 Hugging Face에서 찾아봐", "")
	if !p.Enabled {
		t.Fatalf("expected web policy enabled for explicit site lookup")
	}
	if len(p.PreferredDomains) == 0 || !strings.EqualFold(p.PreferredDomains[0], "huggingface.co") {
		t.Fatalf("expected huggingface preferred domain, got: %#v", p.PreferredDomains)
	}
}

func TestClassifyWebEvidencePolicy_PackageRegistryDomains(t *testing.T) {
	p := classifyWebEvidencePolicy(context.Background(), nil, "requests package pypi에서 찾아줘", "")
	if !p.Enabled {
		t.Fatalf("expected web policy enabled for explicit pypi lookup")
	}
	if len(p.PreferredDomains) == 0 || !strings.EqualFold(p.PreferredDomains[0], "pypi.org") {
		t.Fatalf("expected pypi preferred domain, got: %#v", p.PreferredDomains)
	}
}

func TestClassifyWebEvidencePolicy_NuGetAndArtifactHubDomains(t *testing.T) {
	p1 := classifyWebEvidencePolicy(context.Background(), nil, "Serilog package nuget.org에서 확인해", "")
	if !p1.Enabled {
		t.Fatalf("expected web policy enabled for explicit nuget lookup")
	}
	if len(p1.PreferredDomains) == 0 || !strings.EqualFold(p1.PreferredDomains[0], "nuget.org") {
		t.Fatalf("expected nuget preferred domain, got: %#v", p1.PreferredDomains)
	}

	p2 := classifyWebEvidencePolicy(context.Background(), nil, "nginx helm chart artifacthub에서 찾아줘", "")
	if !p2.Enabled {
		t.Fatalf("expected web policy enabled for explicit artifacthub lookup")
	}
	if len(p2.PreferredDomains) == 0 || !strings.EqualFold(p2.PreferredDomains[0], "artifacthub.io") {
		t.Fatalf("expected artifacthub preferred domain, got: %#v", p2.PreferredDomains)
	}
}

func TestClassifyWebEvidencePolicy_LLMFallbackFailOpen(t *testing.T) {
	exec := func(context.Context, string, string) (string, error) {
		return "unexpected text", nil
	}
	p := classifyWebEvidencePolicy(context.Background(), exec, "gemma release policy changes", "")
	if !p.Enabled {
		t.Fatalf("expected fail-open enable when classifier output is invalid")
	}
}

func mustRawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return b
}
