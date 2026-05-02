package main

import (
	"testing"

	"github.com/koreaf16/argus/internal/query"
)

func TestParseAIDebugAllowDecision(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    bool
		wantErr bool
	}{
		{name: "allow token", raw: "allow", want: true},
		{name: "approve sentence", raw: "decision: approve", want: true},
		{name: "deny token", raw: "deny", want: false},
		{name: "reject sentence", raw: "please reject this", want: false},
		{name: "unknown", raw: "maybe", wantErr: true},
	}

	for _, tc := range tests {
		got, err := parseAIDebugAllowDecision(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestShouldAIDebugAutoInjectPrompt(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   bool
	}{
		{name: "password blocked", prompt: "Password: ", want: false},
		{name: "korean password blocked", prompt: "비밀번호를 입력하세요", want: false},
		{name: "selection allowed", prompt: "run step 1/2? [y/N]", want: true},
		{name: "empty blocked", prompt: "   ", want: false},
	}

	for _, tc := range tests {
		got := shouldAIDebugAutoInjectPrompt(tc.prompt)
		if got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestNormalizeAIDebugInjectedInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain", raw: "y", want: "y"},
		{name: "prefixed", raw: "input: yes", want: "yes"},
		{name: "quoted", raw: "\"n\"", want: "n"},
		{name: "multiline", raw: "\n\nresponse: allow\nextra", want: "allow"},
	}

	for _, tc := range tests {
		got := normalizeAIDebugInjectedInput(tc.raw)
		if got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

// TestUIEventKindAIDebugCoverage는 모든 UIEventKind가 aidebug trace 채널에서
// 어떻게 커버되는지 명시적으로 문서화하고, 새 enum 추가 시 반드시 이 맵을
// 업데이트하도록 강제합니다. 새 UIEventKind 추가 후 이 테스트가 실패하면
// traceType 또는 covTUIOnly 등록이 필요합니다.
func TestUIEventKindAIDebugCoverage(t *testing.T) {
	type coverageStatus int
	const (
		covOK        coverageStatus = iota // aidebug trace 발행됨
		covPartial                         // 일부 경우만 trace됨 (향후 개선 대상)
		covTUIOnly                         // TUI 렌더링 전용, 별도 trace 없음
		covViaParent                       // 관련 상위/하위 trace로 커버됨
	)

	type coverage struct {
		status    coverageStatus
		traceType string // 발행되는 trace type (covOK/covPartial 시)
	}

	// uiEventCoverage: UIEventKind → aidebug trace 커버리지 매핑.
	// stream.go에 새 UIEventKind 추가 시 반드시 여기에도 등록해야 합니다.
	uiEventCoverage := map[query.UIEventKind]coverage{
		query.UIEventAssistantDelta:     {covOK, "llm.provider_chunk"},
		query.UIEventThinkingDelta:      {covPartial, "llm.thinking (done만 trace됨, delta는 미발행)"},
		query.UIEventThinkingDone:       {covOK, "llm.thinking"},
		query.UIEventToolUse:            {covOK, "tool.call.start"},
		query.UIEventToolResult:         {covOK, "tool.call.finish + tool.call.output"},
		query.UIEventPlanExecutionReady: {covTUIOnly, "plan.step trace로 대체 (aidebug 전용 루프)"},
		query.UIEventNotice:             {covOK, "notice"},
		query.UIEventError:              {covOK, "error"},
		query.UIEventDone:               {covOK, "turn.finish (stop_reason 포함)"},
		query.UIEventToolDelta:          {covViaParent, "tool.call.output (최종 결과 시 발행)"},
		query.UIEventPasswordPrompt:     {covOK, "aidebug.decision"},
		query.UIEventAskUserPrompt:      {covOK, "aidebug.decision"},
		query.UIEventAskUserBatchPrompt: {covOK, "aidebug.decision"},
		query.UIEventParallelBatch:      {covTUIOnly, "parallel tool grouping annotation"},
	}

	// allUIEventKinds: stream.go const 블록의 모든 값.
	// 새 UIEventKind 추가 시 여기도 함께 추가해야 합니다.
	allUIEventKinds := []query.UIEventKind{
		query.UIEventAssistantDelta,
		query.UIEventThinkingDelta,
		query.UIEventThinkingDone,
		query.UIEventToolUse,
		query.UIEventToolResult,
		query.UIEventPlanExecutionReady,
		query.UIEventNotice,
		query.UIEventError,
		query.UIEventDone,
		query.UIEventToolDelta,
		query.UIEventPasswordPrompt,
		query.UIEventAskUserPrompt,
		query.UIEventAskUserBatchPrompt,
		query.UIEventParallelBatch,
	}

	// 모든 알려진 UIEventKind가 커버리지 맵에 등록되어 있는지 검증
	for _, kind := range allUIEventKinds {
		if _, ok := uiEventCoverage[kind]; !ok {
			t.Errorf("UIEventKind %q 가 커버리지 맵에 없습니다 — traceType을 등록하거나 covTUIOnly로 표시하세요", kind)
		}
	}

	// 커버리지 맵에 더 이상 존재하지 않는 UIEventKind가 있으면 경고
	allKindSet := make(map[query.UIEventKind]struct{}, len(allUIEventKinds))
	for _, k := range allUIEventKinds {
		allKindSet[k] = struct{}{}
	}
	for kind := range uiEventCoverage {
		if _, ok := allKindSet[kind]; !ok {
			t.Errorf("커버리지 맵의 UIEventKind %q 가 allUIEventKinds에 없습니다 — 삭제된 enum이면 맵에서도 제거하세요", kind)
		}
	}
}
