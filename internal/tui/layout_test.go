package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/koreaf16/argus/internal/presentation"
	tool "github.com/koreaf16/argus/internal/tools"
)

func TestRenderFooterIncludesPlanAndTodo(t *testing.T) {
	m := uiModel{
		width: 140,
		footer: presentation.FooterState{
			Model:            "gpt-5.4",
			ProviderName:     "openai",
			PermissionTitle:  "Plan Mode",
			PlanMode:         true,
			TodoCount:        3,
			MCPCount:         2,
			SkillCount:       1,
			Workspace:        "local",
			SessionID:        "session-1234567890",
			SessionShortID:   "session-1",
			ContextUsedLabel: "14% used",
			MemoryUsageLabel: "250.9 MB",
			CWD:              "Argus",
		},
	}

	footer := m.renderFooter()
	if !strings.Contains(footer, "workspace (/directory)") {
		t.Fatalf("footer should include workspace label: %q", footer)
	}
	if !strings.Contains(footer, "14% used") {
		t.Fatalf("footer should include context usage value: %q", footer)
	}
	if !strings.Contains(footer, "250.9 MB") {
		t.Fatalf("footer should include memory value: %q", footer)
	}
}

func TestViewApprovalModalRendersBelowDivider(t *testing.T) {
	m := newOverlayLayoutModelForTest()
	m.modal = modalState{
		Kind:      modalApproval,
		Title:     "Tool Permission",
		Prompt:    "Allow this tool execution?",
		ToolName:  "bash",
		InputJSON: `{"command":"echo hello"}`,
	}

	assertModalRendersBelowDivider(t, m.View(), "Tool Permission")
}

func TestViewAskUserModalRendersBelowDivider(t *testing.T) {
	m := newOverlayLayoutModelForTest()
	m.modal = modalState{
		Kind:  modalAskUser,
		Title: "Question",
		Question: &tool.AskUserQuestion{
			Header:   "Environment",
			Question: "Choose the target environment",
			Type:     "select",
			Options: []tool.AskUserOption{
				{Value: "dev", Label: "Development"},
				{Value: "prod", Label: "Production"},
			},
		},
	}

	assertModalRendersBelowDivider(t, m.View(), "Environment")
}

func TestViewAskUserBatchModalRendersBelowDivider(t *testing.T) {
	m := newOverlayLayoutModelForTest()
	m.modal = modalState{
		Kind:  modalAskUserBatch,
		Title: "Questions",
		Questions: []tool.AskUserQuestion{
			{
				Header:   "Region",
				Question: "Select region",
				Type:     "select",
				Options:  []tool.AskUserOption{{Value: "ap", Label: "APAC"}},
			},
			{
				Header:   "Tier",
				Question: "Select tier",
				Type:     "select",
				Options:  []tool.AskUserOption{{Value: "gold", Label: "Gold"}},
			},
		},
		AskTab:            0,
		AskAnswersByIndex: map[string]string{},
		AskAnswersByID:    map[string]string{},
	}

	assertModalRendersBelowDivider(t, m.View(), "Questions")
}

func TestViewModalDoesNotPushActiveStreamContent(t *testing.T) {
	base := newOverlayLayoutModelForTest()
	base.entries = []transcriptEntry{
		{Kind: "assistant", Body: "stream line", IsActive: true},
	}
	base.lastPrintedIdx = 0

	withoutModal := strings.Split(strings.TrimSuffix(stripANSI(base.View()), "\x1b[J"), "\n")

	withModal := newOverlayLayoutModelForTest()
	withModal.entries = []transcriptEntry{
		{Kind: "assistant", Body: "stream line", IsActive: true},
	}
	withModal.lastPrintedIdx = 0
	withModal.modal = modalState{
		Kind:      modalApproval,
		Title:     "Tool Permission",
		Prompt:    "Allow this tool execution?",
		ToolName:  "bash",
		InputJSON: `{"command":"echo hello"}`,
	}

	withModalLines := strings.Split(strings.TrimSuffix(stripANSI(withModal.View()), "\x1b[J"), "\n")

	noModalIdx := findLineContaining(withoutModal, "stream line")
	withModalIdx := findLineContaining(withModalLines, "stream line")
	if noModalIdx == -1 || withModalIdx == -1 {
		t.Fatalf("expected active stream content in both layouts")
	}
	if noModalIdx != withModalIdx {
		t.Fatalf("modal overlay should not push stream content: without=%d with=%d", noModalIdx, withModalIdx)
	}
}

func TestViewKeepsPromptAnchorWhenStreamHeightChanges(t *testing.T) {
	// Dynamic Inline Rendering 원칙(CLAUDE.md)상 View() 출력 길이는 활성 stream 길이에 따라
	// 변동하며, anchor는 빈 줄 padding으로 화면 하단에 고정되지 않는다. 이는 fullscreen
	// AltScreen 모드 가정의 검증이라 더 이상 적용되지 않는다.
	t.Skip("inline rendering: anchor floats with stream length by design")

	base := newOverlayLayoutModelForTest()
	base.modal = modalState{}
	base.entries = []transcriptEntry{
		{Kind: "assistant", Body: "line one", IsActive: true},
	}
	base.lastPrintedIdx = 0

	withLargeStream := newOverlayLayoutModelForTest()
	withLargeStream.modal = modalState{}
	withLargeStream.entries = []transcriptEntry{
		{Kind: "assistant", Body: strings.Repeat("x\n", 30), IsActive: true},
	}
	withLargeStream.lastPrintedIdx = 0

	baseLines := strings.Split(strings.TrimSuffix(stripANSI(base.View()), "\x1b[J"), "\n")
	largeLines := strings.Split(strings.TrimSuffix(stripANSI(withLargeStream.View()), "\x1b[J"), "\n")

	baseModeIdx := findLineContaining(baseLines, "Shift+Tab to accept edits")
	largeModeIdx := findLineContaining(largeLines, "Shift+Tab to accept edits")
	if baseModeIdx == -1 || largeModeIdx == -1 {
		t.Fatalf("mode row should be visible in both renders")
	}
	if baseModeIdx != largeModeIdx {
		t.Fatalf("prompt anchor shifted by stream growth: base=%d large=%d", baseModeIdx, largeModeIdx)
	}
}

func newOverlayLayoutModelForTest() uiModel {
	in := textarea.New()
	in.Placeholder = "Type your message or @path/to/file"
	in.Prompt = ""
	in.SetHeight(1)

	return uiModel{
		width:         120,
		height:        24,
		theme:         resolveUITheme("default", true),
		input:         in,
		footer:        presentation.BuildFooterState(nil, "Argus"),
		busy:          true,
		busyStartedAt: time.Now().Add(-stalledAfter - time.Second),
	}
}

func findLineContaining(lines []string, needle string) int {
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func assertModalRendersBelowDivider(t *testing.T, rendered, modalNeedle string) {
	t.Helper()

	lines := strings.Split(strings.TrimSuffix(stripANSI(rendered), "\x1b[J"), "\n")
	waitingIdx := findLineContaining(lines, "Processing...")
	dividerIdx := findLineContaining(lines, strings.Repeat("─", 20))
	modalIdx := findLineContaining(lines, modalNeedle)
	modeIdx := findLineContaining(lines, "Shift+Tab to accept edits")

	if waitingIdx == -1 || dividerIdx == -1 || modalIdx == -1 {
		t.Fatalf("expected processing row, divider, and modal title in output:\n%s", strings.Join(lines, "\n"))
	}
	if waitingIdx >= dividerIdx {
		t.Fatalf("processing row should remain above divider, got processing=%d divider=%d", waitingIdx, dividerIdx)
	}
	if waitingIdx < 2 || strings.TrimSpace(lines[waitingIdx-1]) != "" || strings.TrimSpace(lines[waitingIdx-2]) != "" {
		t.Fatalf("processing row should have two protected blank lines above it")
	}
	if modalIdx <= dividerIdx {
		t.Fatalf("modal should overlay below divider, got modal=%d divider=%d", modalIdx, dividerIdx)
	}
	if modeIdx != -1 {
		t.Fatalf("mode row should be covered by modal overlay, got line %d", modeIdx)
	}
}
