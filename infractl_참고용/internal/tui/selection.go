package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type selectionState struct {
	active      bool
	question    string
	options     []SelectOption
	cursor      int
	replyCh     chan SelectResult
	customInput string
	headerLabel string
}

func (s *selectionState) Activate(question string, options []SelectOption, replyCh chan SelectResult) {
	s.active = true
	s.question = question
	s.options = options
	s.cursor = 0
	s.replyCh = replyCh
	s.headerLabel = ""
	s.customInput = ""
}

func (s *selectionState) ActivateWithHeader(question string, options []SelectOption, replyCh chan SelectResult, header string) {
	s.Activate(question, options, replyCh)
	s.headerLabel = header
}

func (s *selectionState) Deactivate() {
	s.active = false
	s.question = ""
	s.options = nil
	s.cursor = 0
	s.replyCh = nil
	s.customInput = ""
	s.headerLabel = ""
}

func (s *selectionState) hasHideOther() bool {
	for _, o := range s.options {
		if o.HideOther {
			return true
		}
	}
	return false
}

func (s *selectionState) hasPreview() bool {
	for _, o := range s.options {
		if o.Preview != "" {
			return true
		}
	}
	return false
}

func (s *selectionState) handleKey(msg tea.KeyMsg) (handled bool, result tea.Msg) {
	if !s.active {
		return false, nil
	}

	if len(s.options) == 0 {
		switch msg.Type {
		case tea.KeyEnter:
			return true, SelectResponseMsg{
				Result:  SelectResult{Index: -1, Label: s.customInput, IsOther: true},
				ReplyCh: s.replyCh,
			}
		case tea.KeyBackspace, tea.KeyDelete:
			if len(s.customInput) > 0 {
				runes := []rune(s.customInput)
				s.customInput = string(runes[:len(runes)-1])
			}
			return true, nil
		case tea.KeyEsc:
			return true, SelectResponseMsg{
				Result:  SelectResult{Index: -1, Label: ""},
				ReplyCh: s.replyCh,
			}
		case tea.KeyRunes:
			s.customInput += string(msg.Runes)
			return true, nil
		}
		return true, nil
	}

	showOther := !s.hasHideOther()
	totalItems := len(s.options)
	if showOther {
		totalItems++
	}

	switch msg.Type {
	case tea.KeyUp:
		if s.cursor > 0 {
			s.cursor--
		}
		return true, nil
	case tea.KeyDown:
		if s.cursor < totalItems-1 {
			s.cursor++
		}
		return true, nil
	case tea.KeyEnter:
		if showOther && s.cursor == len(s.options) {
			return true, SelectResponseMsg{
				Result:  SelectResult{Index: -1, Label: s.customInput, IsOther: true},
				ReplyCh: s.replyCh,
			}
		}
		if s.cursor < len(s.options) {
			return true, SelectResponseMsg{
				Result:  SelectResult{Index: s.cursor, Label: s.options[s.cursor].Label},
				ReplyCh: s.replyCh,
			}
		}
		return true, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if showOther && s.cursor == len(s.options) && len(s.customInput) > 0 {
			runes := []rune(s.customInput)
			s.customInput = string(runes[:len(runes)-1])
		}
		return true, nil
	case tea.KeyEsc:
		return true, SelectResponseMsg{
			Result:  SelectResult{Index: -1, Label: ""},
			ReplyCh: s.replyCh,
		}
	case tea.KeyRunes:
		if showOther && s.cursor == len(s.options) {
			s.customInput += string(msg.Runes)
		}
		return true, nil
	}

	return true, nil
}

func (s *selectionState) View(width int) string {
	if !s.active {
		return ""
	}
	if s.hasPreview() {
		return s.viewRich(width)
	}
	return s.viewSimple(width)
}

func (s *selectionState) viewSimple(width int) string {
	return s.renderGeminiBox(width, false)
}

func (s *selectionState) viewRich(width int) string {
	return s.renderGeminiBox(width, true)
}

func (s *selectionState) renderGeminiBox(width int, richMode bool) string {
	hdr := strings.TrimSpace(s.headerLabel)

	if len(s.options) == 0 {
		return s.renderTextInputBox(width, hdr)
	}

	showOther := !s.hasHideOther()
	innerW := width - 6
	if innerW < 30 {
		innerW = 30
	}

	var leftSB strings.Builder
	leftSB.WriteString("\n")
	leftSB.WriteString(StyleSelectionQuestion.Render("  "+s.question) + "\n")
	leftSB.WriteString("\n")

	for i, opt := range s.options {
		s.writeGeminiOption(&leftSB, i, opt, richMode)
	}

	if showOther {
		otherIdx := len(s.options)
		labelStr := "Enter a custom value"
		if s.customInput != "" {
			labelStr = s.customInput
		}
		if s.cursor == otherIdx {
			bullet := StyleGeminiBullet.Render(">")
			label := StyleGeminiSelected.Render(fmt.Sprintf("%d. %s", otherIdx+1, labelStr))
			leftSB.WriteString(fmt.Sprintf("  %s %s\n", bullet, label))
		} else {
			leftSB.WriteString(StyleGeminiOption.Render(fmt.Sprintf("    %d. %s", otherIdx+1, labelStr)) + "\n")
		}
	}

	preview := ""
	if richMode && s.cursor < len(s.options) {
		preview = s.options[s.cursor].Preview
	}

	hint := StyleGeminiHint.Render("  Enter to select | Up/Down to navigate | Esc to cancel")

	var bodyContent string
	if preview != "" {
		leftWidth := innerW * 55 / 100
		rightWidth := innerW - leftWidth - 2
		if rightWidth < 20 {
			rightWidth = 20
		}
		innerPreview := rightWidth - 4
		if innerPreview < 10 {
			innerPreview = 10
		}
		previewContent := wrapPreviewLines(preview, innerPreview)
		rightCol := StylePreviewBox.Width(rightWidth).Render(previewContent)
		body := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(leftWidth).Render(leftSB.String()),
			rightCol,
		)
		bodyContent = body + "\n" + hint + "\n"
	} else {
		leftSB.WriteString("\n")
		leftSB.WriteString(hint + "\n")
		bodyContent = leftSB.String()
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorGeminiBox).
		PaddingLeft(1).
		PaddingRight(1).
		Width(innerW + 2)

	renderedBox := boxStyle.Render(bodyContent)
	lines := strings.SplitN(renderedBox, "\n", 2)
	if hdr != "" && len(lines) >= 2 {
		titleLine := " " + StyleGeminiHeader.Render(hdr) + " "
		firstLineW := lipgloss.Width(lines[0])
		headerLine := buildBoxHeader(titleLine, firstLineW)
		return headerLine + "\n" + lines[1]
	}
	return renderedBox
}

func (s *selectionState) renderTextInputBox(width int, header string) string {
	innerW := width - 6
	if innerW < 30 {
		innerW = 30
	}

	input := s.customInput
	if input == "" {
		input = "(type your answer)"
	}

	var body strings.Builder
	body.WriteString("\n")
	body.WriteString(StyleSelectionQuestion.Render("  "+s.question) + "\n")
	body.WriteString("\n")
	body.WriteString(StyleGeminiSubDesc.Render("  Answer: ") + StyleGeminiSelected.Render(input) + "\n")
	body.WriteString("\n")
	body.WriteString(StyleGeminiHint.Render("  Type and press Enter | Esc to cancel") + "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorGeminiBox).
		PaddingLeft(1).
		PaddingRight(1).
		Width(innerW + 2)

	renderedBox := boxStyle.Render(body.String())
	lines := strings.SplitN(renderedBox, "\n", 2)
	hdr := strings.TrimSpace(header)
	if hdr != "" && len(lines) >= 2 {
		titleLine := " " + StyleGeminiHeader.Render(hdr) + " "
		firstLineW := lipgloss.Width(lines[0])
		headerLine := buildBoxHeader(titleLine, firstLineW)
		return headerLine + "\n" + lines[1]
	}
	return renderedBox
}

func (s *selectionState) writeGeminiOption(sb *strings.Builder, i int, opt SelectOption, richMode bool) {
	selected := i == s.cursor
	num := fmt.Sprintf("%d.", i+1)

	if selected {
		bullet := StyleGeminiBullet.Render(">")
		label := StyleGeminiSelected.Render(num + " " + opt.Label)
		line := fmt.Sprintf("  %s %s", bullet, label)
		if richMode && opt.Tag != "" {
			line += "  " + StyleSelectionTag.Render(opt.Tag)
		}
		sb.WriteString(line + "\n")
		if opt.Description != "" {
			sb.WriteString(StyleGeminiSubDesc.Render("       "+opt.Description) + "\n")
		}
	} else {
		line := StyleGeminiOption.Render("    " + num + " " + opt.Label)
		if richMode && opt.Tag != "" {
			line += "  " + StyleInfoBarDim.Render("["+opt.Tag+"]")
		}
		sb.WriteString(line + "\n")
		if opt.Description != "" {
			sb.WriteString(StyleGeminiSubDesc.Render("       "+opt.Description) + "\n")
		}
	}
}

func (s *selectionState) writeOption(sb *strings.Builder, i int, opt SelectOption, richMode bool) {
	s.writeGeminiOption(sb, i, opt, richMode)
}

func wrapPreviewLines(text string, maxWidth int) string {
	var result strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= maxWidth {
			result.WriteString(line + "\n")
			continue
		}
		for len(line) > maxWidth {
			result.WriteString(line[:maxWidth] + "\n")
			line = line[maxWidth:]
		}
		if line != "" {
			result.WriteString(line + "\n")
		}
	}
	return strings.TrimRight(result.String(), "\n")
}

func (s *selectionState) Height() int {
	if !s.active {
		return 0
	}
	if len(s.options) == 0 {
		return 6
	}
	showOther := !s.hasHideOther()
	h := 3
	for _, opt := range s.options {
		h++
		if opt.Description != "" {
			h++
		}
	}
	if showOther {
		h++
	}
	return h
}

type TUISelectHandler struct {
	program *tea.Program
}

func NewTUISelectHandler(p *tea.Program) *TUISelectHandler {
	return &TUISelectHandler{program: p}
}

func (h *TUISelectHandler) SetProgram(p *tea.Program) { h.program = p }

func (h *TUISelectHandler) RequestSelect(question string, options []SelectOption) (SelectResult, error) {
	return h.RequestSelectWithHeader(question, options, "")
}

func (h *TUISelectHandler) RequestSelectWithHeader(question string, options []SelectOption, header string) (SelectResult, error) {
	if h.program == nil {
		return SelectResult{Index: -1}, nil
	}
	replyCh := make(chan SelectResult, 1)
	h.program.Send(SelectRequestMsg{
		Question: question,
		Header:   header,
		Options:  options,
		ReplyCh:  replyCh,
	})
	result := <-replyCh
	return result, nil
}

func (h *TUISelectHandler) RequestSelectCtx(ctx context.Context, question string, options []SelectOption) (SelectResult, error) {
	return h.RequestSelectCtxWithHeader(ctx, question, options, "")
}

func (h *TUISelectHandler) RequestSelectCtxWithHeader(ctx context.Context, question string, options []SelectOption, header string) (SelectResult, error) {
	if h.program == nil {
		return SelectResult{Index: -1}, nil
	}
	replyCh := make(chan SelectResult, 1)
	h.program.Send(SelectRequestMsg{
		Question: question,
		Header:   header,
		Options:  options,
		ReplyCh:  replyCh,
	})
	select {
	case <-ctx.Done():
		return SelectResult{Index: -1}, nil
	case result := <-replyCh:
		return result, nil
	}
}
