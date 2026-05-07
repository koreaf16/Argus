package serverconnect

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

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

func TestSetResultParsesInventoryReady(t *testing.T) {
	m := &ServerConnectInteractiveModel{alias: "sandbox-server", theme: mockTheme{}}
	m.SetResult("[ARGUS_SERVER_CONNECT:connected]\nconnected\n[ARGUS_INVENTORY_HEADER]\nos   Rocky Linux\nuser   sandbox / bash[ARGUS_INVENTORY_READY:326]\nos   Rocky Linux\nuser   sandbox / bash\nservices   35 running / 4 listeners")
	m.SetFinished(true)

	view := m.View()
	if !strings.Contains(view, "inventory ready (0.3s)") {
		t.Fatalf("expected inventory ready in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Rocky Linux") {
		t.Fatalf("expected inventory lines in view, got:\n%s", view)
	}
}

func TestRenderToolUseShowsInventoryReady(t *testing.T) {
	renderer := &ServerConnectRenderer{}
	view := renderer.RenderToolUse(map[string]any{"server": "sandbox-server"},
		"[ARGUS_SERVER_CONNECT:connected]\nconnected\n[ARGUS_INVENTORY_READY:1200]\nos   Rocky Linux\nuser   sandbox / bash\n",
		mockTheme{})

	if !strings.Contains(view, "inventory ready (1.2s)") {
		t.Fatalf("expected inventory ready in rendered use, got:\n%s", view)
	}
	if !strings.Contains(view, "Rocky Linux") {
		t.Fatalf("expected inventory payload in rendered use, got:\n%s", view)
	}
}
