package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/tui/markdown"
)

func TestRenderTranscriptEntryAt_StreamingAssistantUsesMarkdown(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", false),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "# Heading\n"},
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
	cmd := m.scrollbackCmd(evt, prevIdx, -1)

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

func TestFinalizeAssistantStream_AddsTwoLeadingBlankLinesWhenUnflushed(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "hello"},
		},
		lastPrintedIdx: 0,
	}

	cmd := m.finalizeAssistantStream(0)
	if cmd == nil {
		t.Fatal("expected finalization command")
	}
	body := teaPrintLineBody(cmd())
	requireStreamingPrefix(t, body)
	if !strings.Contains(body, "hello") {
		t.Fatalf("expected assistant body, got %q", body)
	}
}

func TestFinalizeAssistantStream_DoesNotAddLeadingBlankLinesAfterFlush(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "first\nsecond"},
		},
		assistantFlushedLines: 1,
		assistantFlushedText:  "first",
		lastPrintedIdx:        0,
	}

	cmd := m.finalizeAssistantStream(0)
	if cmd == nil {
		t.Fatal("expected finalization command for unflushed tail")
	}
	body := teaPrintLineBody(cmd())
	if strings.HasPrefix(body, "\n") {
		t.Fatalf("finalization should not add a second leading separator after prior flush: %q", body)
	}
	if strings.Contains(body, "first") || !strings.Contains(body, "second") {
		t.Fatalf("expected only unflushed assistant tail, got %q", body)
	}
}

func TestFinalizeThinkingStream_DoesNotReprintFlushedLines(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "thinking", Title: "Thinking", Body: "first\nsecond"},
		},
		thinkingOpen:                 true,
		thinkingStreamIdx:            0,
		thinkingFlushedLines: 1,
		thinkingFlushedText:  "first",
		lastPrintedIdx:       0,
	}

	m.closeThinkingEntry()
	if m.thinkingFlushedLines != 1 {
		t.Fatalf("expected closeThinkingEntry to preserve flushed line count, got %d", m.thinkingFlushedLines)
	}
	if m.thinkingFlushedText != "first" {
		t.Fatalf("expected closeThinkingEntry to preserve flushed text, got %q", m.thinkingFlushedText)
	}

	cmd := m.finalizeThinkingStream(0)
	if cmd == nil {
		t.Fatal("expected finalization command for unflushed thinking tail")
	}
	body := teaPrintLineBody(cmd())
	if strings.Contains(body, "first") {
		t.Fatalf("finalization reprinted flushed line: %q", body)
	}
	if !strings.Contains(body, "second") {
		t.Fatalf("expected finalization to print unflushed line: %q", body)
	}
	if strings.HasPrefix(body, "\n") {
		t.Fatalf("finalization should not add a second leading separator after prior flush: %q", body)
	}
}

func TestFinalizeThinkingStream_AddsLeadingSeparatorWhenUnflushed(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "thinking", Title: "Thinking", Body: "first\nsecond"},
		},
		lastPrintedIdx: 0,
	}

	cmd := m.finalizeThinkingStream(0)
	if cmd == nil {
		t.Fatal("expected finalization command")
	}
	body := teaPrintLineBody(cmd())
	requireStreamingPrefix(t, body)
	if !strings.Contains(body, "first") || !strings.Contains(body, "second") {
		t.Fatalf("expected full thinking body, got %q", body)
	}
}

func TestFlushStreamingLines_AssistantAddsTwoLeadingBlankLinesOnFirstFlush(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "first\npartial"},
		},
	}
	flushed := 0

	cmd := m.flushStreamingLines(0, &flushed)
	if cmd == nil {
		t.Fatal("expected assistant flush command")
	}
	body := teaPrintLineBody(cmd())
	requireStreamingPrefix(t, body)
	if !strings.Contains(body, "first") || strings.Contains(body, "partial") {
		t.Fatalf("expected only complete assistant line, got %q", body)
	}
	if flushed != 1 {
		t.Fatalf("expected flushed line count 1, got %d", flushed)
	}
	if m.assistantFlushedText == "" {
		t.Fatal("expected assistantFlushedText to be set after flush")
	}
}

func TestFlushStreamingLines_ThinkingAddsLeadingSeparatorOnFirstFlush(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "thinking", Title: "Thinking", Body: "first\npartial"},
		},
	}
	flushed := 0

	cmd := m.flushStreamingLines(0, &flushed)
	if cmd == nil {
		t.Fatal("expected thinking flush command")
	}
	body := teaPrintLineBody(cmd())
	requireStreamingPrefix(t, body)
	if !strings.Contains(body, "first") || strings.Contains(body, "partial") {
		t.Fatalf("expected only complete thinking line, got %q", body)
	}
	if flushed != 1 {
		t.Fatalf("expected flushed line count 1, got %d", flushed)
	}
	if m.thinkingFlushedText == "" {
		t.Fatal("expected thinkingFlushedText to be set after flush")
	}
}

func TestRenderTranscriptEntryAt_StreamingThinkingSkipsOnlyFlushedLines(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		ui: UISettings{
			ViewThinking: true,
		},
		width: 80,
		entries: []transcriptEntry{
			{Kind: "thinking", Title: "Thinking", Body: "first\nsecond\npartial"},
		},
		thinkingOpen:         true,
		thinkingStreamIdx:    0,
		thinkingFlushedLines: 1,
	}

	got := stripANSI(m.renderTranscriptEntryAt(0, m.entries[0]))
	if strings.Contains(got, "first") {
		t.Fatalf("rendered already-flushed thinking line: %q", got)
	}
	if !strings.Contains(got, "second") {
		t.Fatalf("expected unflushed thinking tail, got %q", got)
	}
	if strings.Contains(got, "partial") {
		t.Fatalf("expected incomplete thinking line to stay hidden, got %q", got)
	}
}

func TestRenderTranscriptEntryAt_StreamingThinkingHidesSinglePartialLine(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", true),
		ui: UISettings{
			ViewThinking: true,
		},
		width: 80,
		entries: []transcriptEntry{
			{Kind: "thinking", Title: "Thinking", Body: "partial"},
		},
		thinkingOpen:      true,
		thinkingStreamIdx: 0,
	}

	got := m.renderTranscriptEntryAt(0, m.entries[0])
	if got != "" {
		t.Fatalf("expected incomplete thinking line to stay hidden, got %q", got)
	}
}

func TestRenderTranscriptEntryAt_StreamingThinkingUsesMarkdown(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", false),
		ui: UISettings{
			ViewThinking: true,
		},
		width: 80,
		entries: []transcriptEntry{
			{Kind: "thinking", Title: "Thinking", Body: "# Heading\n"},
		},
		thinkingOpen:      true,
		thinkingStreamIdx: 0,
	}

	got := m.renderTranscriptEntryAt(0, m.entries[0])
	if strings.Contains(got, "# Heading") {
		t.Fatalf("expected markdown heading to be rendered, got raw markdown: %q", got)
	}
	if !strings.Contains(got, "Heading") {
		t.Fatalf("expected rendered output to include heading text: %q", got)
	}
}

func TestAssistantStreamSource_LineStableHidesSinglePartialLine(t *testing.T) {
	m := uiModel{ui: DefaultUISettings()}

	if got := m.assistantStreamSource("partial"); got != "" {
		t.Fatalf("expected single partial line to stay hidden, got %q", got)
	}
	if got := m.assistantStreamSource("first\npartial"); got != "first\n" {
		t.Fatalf("expected only complete line prefix, got %q", got)
	}
}

func TestAssistantStreamSource_TokenLiveStillHoldsOpenTable(t *testing.T) {
	m := uiModel{ui: DefaultUISettings()}
	m.ui.Streaming.Mode = "token-live"
	m.ui.Streaming.HideUnstableMarkdown = false

	got := m.assistantStreamSource("| A | B |\n|---|---|\n| 1 | 2 |\n")
	if got != "" {
		t.Fatalf("expected token-live mode to hold open markdown table, got %q", got)
	}
}

func TestRenderFrame_LiveAssistantKeepsTwoLeadingBlankLines(t *testing.T) {
	input := textarea.New()
	m := uiModel{
		theme:  resolveUITheme("default", true),
		ui:     DefaultUISettings(),
		width:  80,
		height: 20,
		input:  input,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "first\n", IsActive: true},
		},
		assistantOpen:      true,
		assistantStreamIdx: 0,
		lastPrintedIdx:     0,
	}

	body := stripANSI(m.renderFrame().body)
	requireLeadingVisualBlankLines(t, body, streamingLeadingBlankLines)
	if !strings.Contains(body, "first") {
		t.Fatalf("expected live assistant body, got %q", body)
	}
}

func TestRenderFrame_LiveAssistantContinuationDoesNotAddLeadingBlankLines(t *testing.T) {
	input := textarea.New()
	m := uiModel{
		theme:  resolveUITheme("default", true),
		ui:     DefaultUISettings(),
		width:  80,
		height: 20,
		input:  input,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "first\nsecond\n", IsActive: true},
		},
		assistantOpen:                 true,
		assistantStreamIdx:            0,
		assistantFlushedLines: 1,
		assistantFlushedText:  "first",
		lastPrintedIdx:        0,
	}

	body := stripANSI(m.renderFrame().body)
	if strings.HasPrefix(body, "\n") {
		t.Fatalf("continuation should not add leading blank lines, got %q", body)
	}
	if strings.Contains(body, "first") || !strings.Contains(body, "second") {
		t.Fatalf("expected only live assistant continuation, got %q", body)
	}
}

func teaPrintLineBody(msg any) string {
	v := reflect.ValueOf(msg)
	if v.IsValid() {
		if v.Kind() == reflect.Struct {
			if field := v.FieldByName("messageBody"); field.IsValid() && field.Kind() == reflect.String {
				return field.String()
			}
		}
	}
	return ""
}

func streamingPrefix() string {
	return strings.Repeat("\n", streamingLeadingBlankLines)
}

func requireStreamingPrefix(t *testing.T, body string) {
	t.Helper()
	if !strings.HasPrefix(body, streamingPrefix()) {
		t.Fatalf("expected %d leading blank lines, got %q", streamingLeadingBlankLines, body)
	}
}

func requireLeadingVisualBlankLines(t *testing.T, body string, want int) {
	t.Helper()
	lines := strings.Split(body, "\n")
	if len(lines) < want {
		t.Fatalf("expected at least %d lines, got %q", want, body)
	}
	for i := 0; i < want; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			t.Fatalf("expected visual line %d to be blank, got %q in %q", i, lines[i], body)
		}
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
			name: "closed table (followed by blank line) stays in prefix",
			in:   "| A |\n|---|\n| 1 |\n\n",
			want: "| A |\n|---|\n| 1 |\n\n",
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

func TestFlushStreamingLines_AssistantHoldsOpenMarkdownTable(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", false),
		ui:    DefaultUISettings(),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "| A | B |\n|---|---|\n| 1 | 2 |\n", IsActive: true},
		},
		assistantOpen:      true,
		assistantStreamIdx: 0,
	}
	flushed := 0

	if cmd := m.flushStreamingLines(0, &flushed); cmd != nil {
		t.Fatal("expected open markdown table to stay out of scrollback")
	}
	if flushed != 0 {
		t.Fatalf("expected no raw lines flushed for open table, got %d", flushed)
	}
	if got := m.renderTranscriptEntryAt(0, m.entries[0]); got != "" {
		t.Fatalf("expected open markdown table to stay out of live render, got %q", got)
	}
}

func TestFlushStreamingLines_AssistantRendersMarkdownTableAfterBlank(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", false),
		ui:    DefaultUISettings(),
		width: 100,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "| Short | Long |\n|---|---|\n| 1 | a much longer cell |\n\n", IsActive: true},
		},
		assistantOpen:      true,
		assistantStreamIdx: 0,
	}
	flushed := 0

	cmd := m.flushStreamingLines(0, &flushed)
	if cmd == nil {
		t.Fatal("expected closed markdown table to flush")
	}
	body := stripANSI(teaPrintLineBody(cmd()))
	if strings.Contains(body, "|---|") {
		t.Fatalf("expected rendered table, got raw markdown separator in %q", body)
	}
	if !strings.Contains(body, "a much longer cell") {
		t.Fatalf("expected rendered table content, got %q", body)
	}
	if flushed != 4 {
		t.Fatalf("expected all table lines plus blank terminator flushed, got %d", flushed)
	}
	if m.assistantFlushedText == "" {
		t.Fatal("expected assistantFlushedText to be set after table flush")
	}
}

func TestAssistantMarkdownDoesNotRenderInlineCodeBackground(t *testing.T) {
	m := uiModel{
		theme: resolveUITheme("default", false),
		ui:    DefaultUISettings(),
		width: 80,
		entries: []transcriptEntry{
			{Kind: "assistant", Title: "Assistant", Body: "Use `code` here."},
		},
	}
	if m.themeToPalette().InlineCodeBg == "" {
		t.Fatal("expected base markdown palette to keep inline code background for non-LLM surfaces")
	}
	if got := m.llmOutputPalette().InlineCodeBg; got != "" {
		t.Fatalf("expected assistant palette to clear inline code background, got %q", got)
	}
	if got := m.thinkingPalette().InlineCodeBg; got != "" {
		t.Fatalf("expected thinking palette to clear inline code background, got %q", got)
	}

	rendered := m.renderTranscriptEntryAt(-1, m.entries[0])
	if strings.Contains(rendered, "\x1b[48;") {
		t.Fatalf("expected assistant output without ANSI background color, got %q", rendered)
	}
	if !strings.Contains(stripANSI(rendered), "code") {
		t.Fatalf("expected inline code text to remain visible, got %q", rendered)
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
	if m.thinkingFlushedLines != 0 || m.thinkingFlushedText != "" {
		t.Fatalf("thinking flush state not reset: raw=%d text=%q", m.thinkingFlushedLines, m.thinkingFlushedText)
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
