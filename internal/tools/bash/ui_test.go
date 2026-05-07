package bash

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/tools/shellsignal"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

func TestResolveTargetAlias(t *testing.T) {
	if got := resolveTargetAlias(map[string]any{"server": "a100-server"}); got != "a100-server" {
		t.Fatalf("resolveTargetAlias(server) = %q", got)
	}
	if got := resolveTargetAlias(map[string]any{"_active_workspace": "remote"}); got != "remote" {
		t.Fatalf("resolveTargetAlias(active) = %q", got)
	}
	if got := resolveTargetAlias(map[string]any{}); got != "local" {
		t.Fatalf("resolveTargetAlias(default) = %q", got)
	}
}

func TestOnStreamDeltaCapsBuffer(t *testing.T) {
	m := &BashInteractiveModel{}
	large := make([]byte, maxShellOutputBufferChars+1024)
	for i := range large {
		large[i] = 'a'
	}
	m.OnStreamDelta(string(large))
	if len(m.output) > maxShellOutputBufferChars {
		t.Fatalf("output len = %d, want <= %d", len(m.output), maxShellOutputBufferChars)
	}
}

type mockTheme struct {
	toolui.ThemeContext
}

func (m mockTheme) Style(string) lipgloss.Style   { return lipgloss.NewStyle() }
func (m mockTheme) BaseBodyStyle() lipgloss.Style { return lipgloss.NewStyle() }
func (m mockTheme) Width() int                    { return 100 }
func (m mockTheme) BodyColor() string             { return "body" }
func (m mockTheme) MutedColor() string            { return "muted" }
func (m mockTheme) ToolUseColor() string          { return "tool" }
func (m mockTheme) ToolResultColor() string       { return "result" }
func (m mockTheme) StatusSuccessColor() string    { return "success" }
func (m mockTheme) StatusWarningColor() string    { return "warning" }
func (m mockTheme) StatusErrorColor() string      { return "error" }
func (m mockTheme) ClaudeAccentColor() string     { return "accent" }
func (m mockTheme) BorderColor() string           { return "border" }
func (m mockTheme) AnimFrame() int                { return 0 }

func TestBashViewShellGuardAllowPrelude(t *testing.T) {
	m := &BashInteractiveModel{
		toolName:      "bash",
		targetAlias:   "local",
		showTarget:    true,
		command:       "pwd",
		output:        "/workspace",
		theme:         mockTheme{},
		hasShellGuard: true,
		shellGuard: toolui.ShellGuardInfo{
			Decision:      "allow",
			Risk:          "low",
			ChannelAction: "create",
			ChannelKey:    "local|jhkwa|exec",
			ExecLabel:     "local/jhkwa",
		},
	}

	view := m.View()
	if !strings.Contains(view, "guard: allow - low - create local|jhkwa|exec") {
		t.Fatalf("expected guard prelude, got:\n%s", view)
	}
	if !strings.Contains(view, "exec: local/jhkwa - pwd") {
		t.Fatalf("expected exec prelude, got:\n%s", view)
	}
	if !strings.Contains(view, "/workspace") {
		t.Fatalf("expected shell output after guard prelude, got:\n%s", view)
	}
}

func TestBashViewShellGuardSurfacesEffectiveShell(t *testing.T) {
	m := &BashInteractiveModel{
		toolName:      "bash",
		targetAlias:   "local",
		showTarget:    true,
		command:       `dir "C:\imsi\"`,
		output:        "",
		theme:         mockTheme{},
		hasShellGuard: true,
		shellGuard: toolui.ShellGuardInfo{
			Decision:      "allow",
			Risk:          "low",
			ChannelAction: "local",
			ChannelKey:    "local|jhkwa|exec",
			ExecLabel:     "local/jhkwa",
			Shell:         "powershell",
		},
	}

	view := m.View()
	if !strings.Contains(view, "exec: local/jhkwa (powershell) - dir") {
		t.Fatalf("expected exec line to expose effective shell, got:\n%s", view)
	}
}

func TestBashViewShellGuardDeniedStopsAtGuard(t *testing.T) {
	m := &BashInteractiveModel{
		toolName:      "bash",
		targetAlias:   "prod-api",
		showTarget:    true,
		command:       "blocked",
		theme:         mockTheme{},
		hasShellGuard: true,
		shellGuard: toolui.ShellGuardInfo{
			Decision: "denied",
			Risk:     "high",
			Reason:   "mixed privilege command must be split before execution",
		},
	}

	view := m.View()
	if !strings.Contains(view, "guard: denied - high - mixed privilege command must be split before execution") {
		t.Fatalf("expected denied guard line, got:\n%s", view)
	}
	if strings.Contains(view, "exec:") {
		t.Fatalf("denied guard should not render exec line, got:\n%s", view)
	}
	if strings.Contains(view, "waiting for output") || strings.Contains(view, "TAB focus") {
		t.Fatalf("denied guard should not render live shell body, got:\n%s", view)
	}
}

func TestBashViewFinishedSummaryKeepsShellGuard(t *testing.T) {
	m := &BashInteractiveModel{
		toolName:      "bash",
		targetAlias:   "local",
		showTarget:    true,
		command:       "npm run build",
		output:        "building\nsuccessfully bundled",
		theme:         mockTheme{},
		hasShellGuard: true,
		shellGuard: toolui.ShellGuardInfo{
			Decision:      "allow",
			Risk:          "medium",
			ChannelAction: "local",
			ChannelKey:    "local|jhkwa|exec",
			ExecLabel:     "local/jhkwa",
		},
	}

	m.SetResult(`{"stdout":"building\nsuccessfully bundled","stderr":"","code":0}`)
	m.SetFinished(true)

	view := m.View()
	if !strings.Contains(view, "guard: allow - medium - local local|jhkwa|exec") {
		t.Fatalf("expected guard prelude to remain after finish, got:\n%s", view)
	}
	if !strings.Contains(view, "done | exit 0 | successfully bundled | 2 lines") {
		t.Fatalf("expected compact summary after guard prelude, got:\n%s", view)
	}
}

func TestPowerShellRendererUsesShellGuardPrelude(t *testing.T) {
	renderer := &BashRenderer{}
	view := renderer.RenderToolUse(map[string]any{
		"_tool_name": "powershell",
		"command":    "Get-Process",
		"server":     "local",
		"_shell_guard": map[string]any{
			"decision":       "allow",
			"risk":           "low",
			"channel_action": "local",
			"channel_key":    "local|jhkwa|exec",
			"exec_label":     "local/jhkwa",
		},
	}, "pwsh", mockTheme{})

	if !strings.Contains(view, "PowerShell(Get-Process [local])") {
		t.Fatalf("expected powershell headline, got:\n%s", view)
	}
	if !strings.Contains(view, "guard: allow - low - local local|jhkwa|exec") {
		t.Fatalf("expected powershell guard prelude, got:\n%s", view)
	}
	if !strings.Contains(view, "exec: local/jhkwa - Get-Process") {
		t.Fatalf("expected powershell exec prelude, got:\n%s", view)
	}
}

func TestBashViewRunningTailLimit(t *testing.T) {
	m := &BashInteractiveModel{
		toolName:    "bash",
		targetAlias: "local",
		showTarget:  true,
		command:     "tail -f app.log",
		output:      strings.Join([]string{"line1", "line2", "line3", "line4", "line5", "line6", "line7", "line8", "line9", "line10", "line11", "line12"}, "\n"),
		theme:       mockTheme{},
	}

	view := m.View()
	if strings.Contains(view, "\n  │ line1\n") || strings.Contains(view, "\n  │ line2\n") {
		t.Fatalf("expected view to keep only the latest tail lines, got:\n%s", view)
	}
	if !strings.Contains(view, "line3") {
		t.Fatalf("expected oldest visible tail line to remain visible, got:\n%s", view)
	}
	if !strings.Contains(view, "line12") {
		t.Fatalf("expected last output line to remain visible, got:\n%s", view)
	}
	if !strings.Contains(view, "tail 10/12") {
		t.Fatalf("expected tail counter in running shell box, got:\n%s", view)
	}
	if !strings.Contains(view, "Ctrl+O expand") {
		t.Fatalf("expected expand hint in running shell box, got:\n%s", view)
	}
}

func TestBashViewFinishedSummaryKeepsHeaderAndUsesLowercaseResult(t *testing.T) {
	m := &BashInteractiveModel{
		toolName:    "bash",
		targetAlias: "local",
		showTarget:  true,
		command:     "npm run build",
		output:      "building\nsuccessfully bundled",
		theme:       mockTheme{},
	}

	m.SetResult(`{"stdout":"building\nsuccessfully bundled","stderr":"","code":0}`)
	m.SetFinished(true)

	view := m.View()
	if !strings.Contains(view, "Bash(npm run build [local])") {
		t.Fatalf("expected header to stay visible after finish, got:\n%s", view)
	}
	if !strings.Contains(view, "done | exit 0 | successfully bundled | 2 lines") {
		t.Fatalf("expected compact summary on branch line, got:\n%s", view)
	}
}

func TestBashViewBackgroundSummary(t *testing.T) {
	m := &BashInteractiveModel{
		toolName:    "powershell",
		targetAlias: "local",
		showTarget:  true,
		command:     "Start-Job",
		theme:       mockTheme{},
	}

	m.SetResult(`{"stdout":"Local: http://127.0.0.1:5173","stderr":"","code":0,"background_task_id":"job-29f7","backgrounded_by_user":true}`)
	m.SetFinished(true)

	view := m.View()
	if !strings.Contains(view, "background | job-29f7 | last: Local: http://127.0.0.1:5173") {
		t.Fatalf("expected background summary, got:\n%s", view)
	}
}

func TestCtrlDSendsEOFWhenFocused(t *testing.T) {
	input := make(chan string, 1)
	m := &BashInteractiveModel{
		inputChan:  input,
		isFocused:  true,
		isFinished: false,
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if next == nil {
		t.Fatal("expected interactive model to remain available")
	}

	select {
	case got := <-input:
		if got != "\x04" {
			t.Fatalf("expected EOF byte, got %q", got)
		}
	default:
		t.Fatal("expected Ctrl+D to forward an EOF byte while focused")
	}
}

func TestCtrlDRequestsBackgroundWhenUnfocused(t *testing.T) {
	input := make(chan string, 1)
	m := &BashInteractiveModel{
		inputChan:  input,
		isFocused:  false,
		isFinished: false,
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if next == nil {
		t.Fatal("expected interactive model to remain available")
	}

	select {
	case got := <-input:
		if got != shellsignal.BackgroundRequest {
			t.Fatalf("expected background request, got %q", got)
		}
	default:
		t.Fatal("expected Ctrl+D to forward a background request when unfocused")
	}
}

func TestTabSendsTabByteWhenFocused(t *testing.T) {
	input := make(chan string, 1)
	m := &BashInteractiveModel{
		inputChan:  input,
		isFocused:  true,
		isFinished: false,
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if next == nil {
		t.Fatal("expected interactive model to remain available")
	}

	select {
	case got := <-input:
		if got != "\t" {
			t.Fatalf("expected tab byte, got %q", got)
		}
	default:
		t.Fatal("expected Tab to forward a tab byte while focused")
	}
}

func TestTabIgnoredWhenUnfocused(t *testing.T) {
	input := make(chan string, 1)
	m := &BashInteractiveModel{
		inputChan:  input,
		isFocused:  false,
		isFinished: false,
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	select {
	case got := <-input:
		t.Fatalf("expected Tab to be consumed by toolui focus toggle while unfocused, got %q", got)
	default:
	}
}

func TestLiveHintLineShowsEOFWhenFocused(t *testing.T) {
	m := &BashInteractiveModel{
		toolName:    "bash",
		targetAlias: "local",
		showTarget:  true,
		command:     "cat",
		theme:       mockTheme{},
		isFocused:   true,
	}

	view := m.View()
	if !strings.Contains(view, "Ctrl+D EOF") {
		t.Fatalf("expected focused hint to show 'Ctrl+D EOF', got:\n%s", view)
	}
	if strings.Contains(view, "Ctrl+D background") {
		t.Fatalf("focused hint should not show background label, got:\n%s", view)
	}
}
