package tui

import (
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/tui/markdown"
)

func TestRenderTranscriptEntryAt_StreamingAssistantUsesMarkdown(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", false),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "# Heading"},
		},
		assistantOpen:      true,
		assistantStreamIdx: 0,
	}

	got := m.renderTranscriptEntryAt(0, m.entries[0])
	if strings.Contains(got, "# Heading") {
		t.Fatalf("expected markdown heading to be rendered, got raw markdown: %q", got)
	}
	if !strings.Contains(got, "Heading") {
		t.Fatalf("expected rendered output to include heading text: %q", got)
	}
}

func TestScrollbackCmd_FinalizesAssistantBeforeToolUse(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "hello"},
		},
		assistantOpen:      true,
		assistantStreamIdx: 0,
		lastPrintedIdx:     0,
	}
	evt := presentation.Event{
		Kind:     presentation.EventToolUse,
		ToolName: "bash",
		Input:    `{"command":"pwd"}`,
	}

	prevIdx := m.assistantStreamIdx
	m.applyPresentationEvent(evt)
	cmd := m.scrollbackCmd(evt, prevIdx)

	if cmd == nil {
		t.Fatal("expected command to flush assistant and print tool use entry")
	}
	if got := len(m.entries); got != 2 {
		t.Fatalf("expected 2 entries after tool use event, got %d", got)
	}
	if m.entries[0].Kind != "assistant" || m.entries[0].Body != "hello" {
		t.Fatalf("assistant entry was not preserved: %#v", m.entries[0])
	}
	if m.entries[1].Kind != "tool_use" {
		t.Fatalf("expected tool_use entry, got %#v", m.entries[1])
	}
	// 스트리밍 중인 shell tool_use(bash/powershell)는 결과가 도착할 때까지 라이브 뷰에 유지된다.
	// 따라서 assistant(0) 까지만 스크롤백에 출력되고, tool_use(1) 는 lastPrintedIdx 너머에 남는다.
	if m.lastPrintedIdx != 1 {
		t.Fatalf("expected lastPrintedIdx to stop at 1 (tool_use stays in live view), got %d", m.lastPrintedIdx)
	}
}

func TestFinalizeAssistantStream_AvoidsDuplicatePrint(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "hello"},
		},
		lastPrintedIdx: 0,
	}

	first := m.finalizeAssistantStream(0)
	if first == nil {
		t.Fatal("expected first finalization command")
	}
	if m.lastPrintedIdx != 1 {
		t.Fatalf("expected lastPrintedIdx=1 after first finalization, got %d", m.lastPrintedIdx)
	}

	second := m.finalizeAssistantStream(0)
	if second != nil {
		t.Fatal("expected second finalization to be skipped")
	}
}

func TestStableStreamingMarkdownSource_HidesIncompleteTail(t *testing.T) {
	got := stableStreamingMarkdownSource("first line\n**")
	if got != "first line\n" {
		t.Fatalf("expected unstable tail to be removed, got %q", got)
	}
}

func TestStableStreamingPrefix_TrimsOpenTable(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "open table with header+separator+rows is excluded entirely",
			in:   "intro paragraph\n\n| A | B |\n|---|---|\n| 1 | 2 |\n",
			want: "intro paragraph\n\n",
		},
		{
			name: "table-row-only header (no separator yet) is excluded",
			in:   "before\n\n| A | B |\n",
			want: "before\n\n",
		},
		{
			name: "closed table (followed by blank+text) stays in prefix",
			in:   "| A |\n|---|\n| 1 |\n\nafter\n",
			want: "| A |\n|---|\n| 1 |\n\nafter\n",
		},
		{
			name: "open code fence is excluded",
			in:   "intro\n```go\nfunc main() {}\n",
			want: "intro\n",
		},
		{
			name: "no open block returns input unchanged",
			in:   "just a paragraph\n",
			want: "just a paragraph\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := markdown.StableStreamingPrefix(tc.in)
			if got != tc.want {
				t.Fatalf("StableStreamingPrefix(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderTranscriptEntryAt_StreamingAssistantHidesUnstableOnlyLine(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", false),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "**"},
		},
		assistantOpen:      true,
		assistantStreamIdx: 0,
	}

	got := m.renderTranscriptEntryAt(0, m.entries[0])
	if got != "" {
		t.Fatalf("expected unstable markdown-only line to stay hidden, got %q", got)
	}
}

func TestApplyPresentationEvent_ViewClearedResetsRenderState(t *testing.T) {
	m := uiModel{
		entries: []transcriptEntry{
			{Kind: "assistant", Body: "hello"},
			{Kind: "tool_use", Body: "cmd"},
		},
		assistantOpen:         true,
		assistantStreamIdx:    0,
		thinkingOpen:          true,
		thinkingStreamIdx:     1,
		toolUseOpen:           true,
		toolUseStreamIdx:      1,
		assistantFlushedLines: 3,
		thinkingFlushedLines:  2,
		lastPrintedIdx:        9,
		activeTool:            nil,
		toolFocused:           true,
		toolEntryByTaskID: map[string]int{
			"task-1": 1,
		},
	}

	m.applyPresentationEvent(presentation.Event{Kind: presentation.EventViewCleared})

	if len(m.entries) != 0 {
		t.Fatalf("expected entries cleared, got %d", len(m.entries))
	}
	if m.lastPrintedIdx != 0 {
		t.Fatalf("expected lastPrintedIdx reset to 0, got %d", m.lastPrintedIdx)
	}
	if m.assistantOpen || m.assistantStreamIdx != -1 {
		t.Fatalf("assistant stream state not reset: open=%v idx=%d", m.assistantOpen, m.assistantStreamIdx)
	}
	if m.thinkingOpen || m.thinkingStreamIdx != -1 {
		t.Fatalf("thinking stream state not reset: open=%v idx=%d", m.thinkingOpen, m.thinkingStreamIdx)
	}
	if m.toolUseOpen || m.toolUseStreamIdx != -1 {
		t.Fatalf("tool stream state not reset: open=%v idx=%d", m.toolUseOpen, m.toolUseStreamIdx)
	}
	if m.toolFocused {
		t.Fatalf("expected tool focus to reset")
	}
	if len(m.toolEntryByTaskID) != 0 {
		t.Fatalf("expected task index map cleared, got %d entries", len(m.toolEntryByTaskID))
	}
}
