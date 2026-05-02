package tui

import "strings"

type slashSuggestion struct {
	Command     string
	Description string
}

var slashSuggestionCatalog = []slashSuggestion{
	{Command: "help", Description: "명령 목록 표시"},
	{Command: "status", Description: "현재 상태 요약"},
	{Command: "clear", Description: "화면 transcript 초기화"},
	{Command: "model", Description: "모델 조회/전환/등록"},
	{Command: "session", Description: "세션 save/load/list"},
	{Command: "plan", Description: "plan mode on/off"},
	{Command: "memory", Description: "메모리 add/list/search"},
	{Command: "mcp", Description: "MCP list/reload/tools/resources/read"},
	{Command: "server", Description: "원격 워크스페이스 관리"},
	{Command: "skills", Description: "스킬 list/run"},
	{Command: "connector", Description: "MCP 커넥터 수동 search/install/list/remove"},
	{Command: "config", Description: "설정 show/get/set"},
	{Command: "diff", Description: "git diff 보기"},
	{Command: "review", Description: "변경 리뷰 요약"},
	{Command: "commit", Description: "커밋 실행"},
	{Command: "init", Description: "기본 설정 파일 생성"},
	{Command: "keybindings", Description: "키 바인딩 안내"},
	{Command: "quit", Description: "프로그램 종료"},
}

func (m *uiModel) refreshSlashSuggestions() {
	line := strings.TrimLeft(m.input.Value(), " \t")
	if line == "" || !strings.HasPrefix(line, "/") || strings.Contains(line, "\n") {
		m.slashMatches = m.slashMatches[:0]
		m.slashCursor = 0
		return
	}

	query := strings.TrimPrefix(line, "/")
	if strings.Contains(query, " ") {
		m.slashMatches = m.slashMatches[:0]
		m.slashCursor = 0
		return
	}

	prefix := strings.ToLower(strings.TrimSpace(query))
	matches := make([]slashSuggestion, 0, 8)
	for _, cmd := range slashSuggestionCatalog {
		name := strings.ToLower(cmd.Command)
		if prefix == "" || strings.HasPrefix(name, prefix) {
			matches = append(matches, cmd)
		}
		if len(matches) >= 8 {
			break
		}
	}

	m.slashMatches = matches
	if len(m.slashMatches) == 0 {
		m.slashCursor = 0
		return
	}
	if m.slashCursor < 0 || m.slashCursor >= len(m.slashMatches) {
		m.slashCursor = 0
	}
}

func (m *uiModel) showSlashOverlay() bool {
	if m.modal.Kind != modalNone {
		return false
	}
	return len(m.slashMatches) > 0
}

func (m *uiModel) applySlashSuggestion() bool {
	if !m.showSlashOverlay() {
		return false
	}
	if m.slashCursor < 0 || m.slashCursor >= len(m.slashMatches) {
		m.slashCursor = 0
	}
	selected := m.slashMatches[m.slashCursor]
	value := "/" + selected.Command + " "
	m.input.SetValue(value)
	m.input.SetCursor(len(value))
	m.refreshSlashSuggestions()
	return true
}

func (m *uiModel) moveSlashSuggestion(step int) bool {
	if !m.showSlashOverlay() || len(m.slashMatches) < 2 {
		return false
	}
	m.slashCursor = (m.slashCursor + step) % len(m.slashMatches)
	if m.slashCursor < 0 {
		m.slashCursor += len(m.slashMatches)
	}
	return true
}
