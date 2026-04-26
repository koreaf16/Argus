package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourorg/infractl/internal/privilege"
)

// PrivilegePromptRequestMsg asks the TUI to collect a privilege password.
type PrivilegePromptRequestMsg struct {
	Request privilege.PromptRequest
	ReplyCh chan privilege.PromptResponse
}

// PrivilegePromptResponseMsg returns the user's privilege password response.
type PrivilegePromptResponseMsg struct {
	Response privilege.PromptResponse
	ReplyCh  chan privilege.PromptResponse
}

type privilegePromptState struct {
	active  bool
	request privilege.PromptRequest
	replyCh chan privilege.PromptResponse
	input   string
}

func (s *privilegePromptState) render(width int) string {
	if !s.active {
		return ""
	}

	target := s.request.Target
	if strings.TrimSpace(target) == "" {
		target = "localhost"
	}

	innerW := width - 6
	if innerW < 40 {
		innerW = 40
	}

	var content strings.Builder
	content.WriteString("\n")
	content.WriteString(StyleGeminiSubDesc.Render("  Target : ") + StyleGeminiSelected.Render(target) + "\n")
	content.WriteString(StyleGeminiSubDesc.Render("  Method : ") + StyleGeminiOption.Render(string(s.request.Method)) + "\n")
	content.WriteString(StyleGeminiSubDesc.Render("  User   : ") + StyleGeminiOption.Render(s.request.User) + "\n")
	content.WriteString("\n")
	masked := strings.Repeat("*", len(s.input))
	content.WriteString(StyleGeminiSubDesc.Render("  Password: ") + StyleGeminiSelected.Render(masked+"█") + "\n")
	content.WriteString("\n")
	content.WriteString(StyleGeminiHint.Render("  Enter to confirm · Esc to abort") + "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorWarning).
		PaddingLeft(1).
		PaddingRight(1).
		Width(innerW + 2)

	rendered := boxStyle.Render(content.String())

	// 헤더 타이틀 삽입: "🔒 Privilege Password"
	lines := strings.SplitN(rendered, "\n", 2)
	if len(lines) >= 2 {
		titleLine := " " + StyleGeminiHeader.Render("🔒 Privilege Password") + " "
		firstLineW := lipgloss.Width(lines[0])
		// 경고색 테두리로 헤더 생성
		borderColor := lipgloss.NewStyle().Foreground(ColorWarning)
		const hLine = "─"
		visTitle := stripANSI(titleLine)
		dashCount := firstLineW - 2 - len([]rune(visTitle)) - 2
		if dashCount < 0 {
			dashCount = 0
		}
		dashes := strings.Repeat(hLine, dashCount)
		headerLine := borderColor.Render("╭") +
			borderColor.Render(" ") +
			titleLine +
			borderColor.Render(" ") +
			borderColor.Render(dashes) +
			borderColor.Render("╮")
		rendered = headerLine + "\n" + lines[1]
	}

	return rendered
}

func (s *privilegePromptState) handleKey(key tea.KeyMsg) (bool, tea.Msg) {
	if !s.active {
		return false, nil
	}

	switch key.Type {
	case tea.KeyEnter:
		resp := privilege.PromptResponse{Abort: strings.TrimSpace(s.input) == "", Password: s.input}
		s.active = false
		return true, PrivilegePromptResponseMsg{Response: resp, ReplyCh: s.replyCh}
	case tea.KeyBackspace, tea.KeyDelete:
		if len(s.input) > 0 {
			s.input = s.input[:len(s.input)-1]
		}
		return true, nil
	case tea.KeyEsc, tea.KeyCtrlC:
		s.active = false
		return true, PrivilegePromptResponseMsg{
			Response: privilege.PromptResponse{Abort: true},
			ReplyCh:  s.replyCh,
		}
	case tea.KeyRunes:
		s.input += string(key.Runes)
		return true, nil
	}

	return true, nil
}

// PrivilegePromptHandler forwards privilege password prompts into the Bubble Tea program.
type PrivilegePromptHandler struct {
	program *tea.Program
}

// NewPrivilegePromptHandler builds a Bubble Tea-backed privilege prompt handler.
func NewPrivilegePromptHandler(p *tea.Program) *PrivilegePromptHandler {
	return &PrivilegePromptHandler{program: p}
}

// RequestPassword asks the TUI to collect a privilege password and waits for the response.
func (h *PrivilegePromptHandler) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	replyCh := make(chan privilege.PromptResponse, 1)
	h.program.Send(PrivilegePromptRequestMsg{Request: req, ReplyCh: replyCh})

	select {
	case <-ctx.Done():
		return privilege.PromptResponse{Abort: true}, nil
	case resp := <-replyCh:
		return resp, nil
	}
}
