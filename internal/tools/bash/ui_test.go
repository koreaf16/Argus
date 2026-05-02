package bash

import (
	"strings"
	"testing"
	"github.com/charmbracelet/lipgloss"
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

func (m mockTheme) Style(c string) lipgloss.Style { return lipgloss.NewStyle() }
func (m mockTheme) BaseBodyStyle() lipgloss.Style { return lipgloss.NewStyle() }
func (m mockTheme) BodyColor() string           { return "1" }
func (m mockTheme) MutedColor() string          { return "2" }
func (m mockTheme) Width() int                  { return 100 }

func TestBashViewTruncation(t *testing.T) {
	m := &BashInteractiveModel{
		command: "ls -la",
		output:  "line1\nline2\nline3\nline4\nline5",
		theme:   mockTheme{},
	}

	// 1. 실행 중(isFinished=false), 축약 상태 -> Ctrl+O 힌트 표시
	m.isFinished = false
	m.isExpanded = false
	view := m.View()
	if !strings.Contains(view, "Ctrl+O로 펼치기") {
		t.Errorf("expected view to contain 'Ctrl+O로 펼치기' during execution, got:\n%s", view)
	}

	// 2. 완료됨(isFinished=true), 축약 상태 -> '생략됨' 힌트 표시 (이번 작업 핵심)
	m.isFinished = true
	m.isExpanded = false
	view = m.View()
	if !strings.Contains(view, "생략됨") {
		t.Errorf("expected view to contain '생략됨' after finish, got:\n%s", view)
	}
	if strings.Contains(view, "Ctrl+O로 펼치기") {
		t.Errorf("view should NOT contain 'Ctrl+O로 펼치기' after finish, got:\n%s", view)
	}

	// 3. 완료됨, 확장 상태 -> 힌트 없이 전체 출력
	m.isExpanded = true
	view = m.View()
	if !strings.Contains(view, "line5") {
		t.Errorf("expected expanded view to show all lines, got:\n%s", view)
	}
	if strings.Contains(view, "숨김") {
		t.Errorf("expanded view should not contain '숨김', got:\n%s", view)
	}
}
