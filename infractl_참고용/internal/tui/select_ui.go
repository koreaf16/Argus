package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SelectOption is one selectable answer in the prompt UI.
type SelectOption struct {
	Label       string
	Description string
	Preview     string
	Tag         string
	HideOther   bool
}

// SelectResult is the user's selection.
type SelectResult struct {
	Index   int
	Label   string
	IsOther bool
	IsBack  bool
}

func geminiBoxWidth(termWidth int) int {
	w := termWidth - 4
	if w < 40 {
		w = 40
	}
	return w
}

func geminiHideOther(options []SelectOption) bool {
	for _, o := range options {
		if o.HideOther {
			return true
		}
	}
	return false
}

func RunSelect(question string, options []SelectOption, width int) SelectResult {
	return RunSelectWithHeader(question, options, width, "")
}

func RunSelectWithHeader(question string, options []SelectOption, width int, header string) SelectResult {
	if len(options) == 0 {
		return readFreeText(question, header, width)
	}

	cursor := 0
	customInput := ""
	hideOther := geminiHideOther(options)
	totalItems := len(options)
	if !hideOther {
		totalItems++
	}

	oldState, err := enableRawMode()
	if err != nil {
		return fallbackSelect(question, options, header)
	}
	defer restoreTermMode(oldState)

	tw := newTermWriter()
	tw.HideCursor()
	defer tw.ShowCursor()

	renderGeminiSelect(tw, header, question, options, cursor, width, hideOther, customInput)

	buf := make([]byte, 16)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		switch {
		case buf[0] == 2: // Ctrl+B
			tw.ClearLive()
			return SelectResult{IsBack: true}

		case buf[0] == 13 || buf[0] == 10:
			tw.ClearLive()
			if !hideOther && cursor == len(options) {
				return SelectResult{Index: -1, Label: customInput, IsOther: true}
			}
			return SelectResult{Index: cursor, Label: options[cursor].Label}

		case buf[0] == 27 && n >= 3:
			if buf[1] == '[' {
				switch buf[2] {
				case 'A':
					if cursor > 0 {
						cursor--
					}
				case 'B':
					if cursor < totalItems-1 {
						cursor++
					}
				}
			}

		case buf[0] == 127 || buf[0] == 8:
			if !hideOther && cursor == len(options) && len(customInput) > 0 {
				runes := []rune(customInput)
				customInput = string(runes[:len(runes)-1])
			}

		case buf[0] == 27 && n == 1:
			tw.ClearLive()
			return SelectResult{Index: -1, Label: ""}

		case buf[0] == 3:
			tw.ClearLive()
			return SelectResult{Index: -1, Label: ""}

		default:
			if !hideOther && cursor == len(options) && buf[0] >= 32 {
				customInput += string(buf[:n])
			}
		}

		tw.ClearLive()
		renderGeminiSelect(tw, header, question, options, cursor, width, hideOther, customInput)
	}
}

func renderGeminiSelect(tw *termWriter, header, question string, options []SelectOption, cursor, width int, hideOther bool, customInput string) {
	innerW := geminiBoxWidth(width)
	hdr := strings.TrimSpace(header)

	var content strings.Builder
	if hdr != "" {
		content.WriteString("  " + StyleGeminiHeader.Render(hdr) + "\n")
		content.WriteString(StyleSubtle.Render("  "+strings.Repeat("\u2500", max(0, innerW-2))) + "\n")
	}

	content.WriteString("\n")
	content.WriteString(wrapText("  "+question, innerW) + "\n")
	content.WriteString("\n")

	for i, opt := range options {
		num := fmt.Sprintf("%d.", i+1)
		if i == cursor {
			bullet := StyleGeminiBullet.Render(">")
			label := StyleGeminiSelected.Width(max(1, innerW-4)).Render(num + " " + opt.Label)
			content.WriteString(fmt.Sprintf("  %s %s", bullet, label))
			if opt.Tag != "" {
				content.WriteString("  " + StyleSelectionTag.Render(opt.Tag))
			}
			content.WriteString("\n")
			if opt.Description != "" {
				content.WriteString(StyleGeminiSubDesc.Render("       "+opt.Description) + "\n")
			}
		} else {
			line := StyleGeminiOption.Render("    " + num + " " + opt.Label)
			content.WriteString(line)
			if opt.Tag != "" {
				content.WriteString("  " + StyleInfoBarDim.Render("["+opt.Tag+"]"))
			}
			content.WriteString("\n")
			if opt.Description != "" {
				content.WriteString(StyleGeminiSubDesc.Render("       "+opt.Description) + "\n")
			}
		}
	}

	if !hideOther {
		otherIdx := len(options)
		labelStr := "Input a custom value"
		if customInput != "" {
			labelStr = customInput
		}

		if cursor == otherIdx {
			bullet := StyleGeminiBullet.Render(">")
			label := StyleGeminiSelected.Width(max(1, innerW-4)).Render(fmt.Sprintf("%d. ", otherIdx+1) + labelStr)
			content.WriteString(fmt.Sprintf("  %s %s\n", bullet, label))
		} else {
			content.WriteString(StyleGeminiOption.Render(fmt.Sprintf("    %d. %s", otherIdx+1, labelStr)) + "\n")
		}
	}

	content.WriteString("\n")
	content.WriteString(StyleGeminiHint.Render("  Enter: select | Ctrl+B: back | Esc: cancel") + "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(ColorGeminiBox).
		PaddingLeft(1).
		PaddingRight(1).
		Width(innerW + 2)

	rendered := boxStyle.Render(content.String())
	tw.prevLines = strings.Count(rendered, "\n") + 1
	tw.Print(rendered + "\n")
}

func buildBoxHeader(title string, totalWidth int) string {
	const (
		topLeft  = "\u256d"
		topRight = "\u256e"
		hLine    = "\u2500"
	)

	visibleTitle := stripANSI(title)
	titleWidth := lipgloss.Width(visibleTitle)
	dashCount := totalWidth - 2 - titleWidth - 2
	if dashCount < 0 {
		dashCount = 0
	}

	borderColor := lipgloss.NewStyle().Foreground(ColorGeminiBox)
	return borderColor.Render(topLeft) +
		borderColor.Render(" ") +
		title +
		borderColor.Render(" ") +
		borderColor.Render(strings.Repeat(hLine, dashCount)) +
		borderColor.Render(topRight)
}

func stripANSI(s string) string {
	var result strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); {
		if runes[i] == '\033' && i+1 < len(runes) && runes[i+1] == '[' {
			i += 2
			for i < len(runes) && runes[i] != 'm' {
				i++
			}
			if i < len(runes) {
				i++
			}
			continue
		}
		result.WriteRune(runes[i])
		i++
	}
	return result.String()
}

func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 || len([]rune(text)) <= maxWidth {
		return text
	}

	var result strings.Builder
	runes := []rune(text)
	for len(runes) > maxWidth {
		result.WriteString(string(runes[:maxWidth]) + "\n")
		runes = runes[maxWidth:]
	}
	if len(runes) > 0 {
		result.WriteString(string(runes))
	}
	return result.String()
}

func readFreeText(question, header string, width int) SelectResult {
	oldState, err := enableRawMode()
	if err != nil {
		return fallbackSelect(question, nil, header)
	}
	defer restoreTermMode(oldState)

	tw := newTermWriter()
	tw.HideCursor()
	defer tw.ShowCursor()

	input := ""
	for {
		renderFreeText(tw, header, question, input, width)

		buf := make([]byte, 16)
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		switch {
		case buf[0] == 2: // Ctrl+B
			tw.ClearLive()
			return SelectResult{IsBack: true}
		case buf[0] == 13 || buf[0] == 10:
			tw.ClearLive()
			return SelectResult{Index: -1, Label: input, IsOther: true}
		case buf[0] == 127 || buf[0] == 8:
			if len(input) > 0 {
				runes := []rune(input)
				input = string(runes[:len(runes)-1])
			}
		case buf[0] == 27 || buf[0] == 3: // Esc or Ctrl+C
			tw.ClearLive()
			return SelectResult{Index: -1, Label: ""}
		default:
			if buf[0] >= 32 {
				input += string(buf[:n])
			}
		}
		tw.ClearLive()
	}
}

func renderFreeText(tw *termWriter, header, question, input string, width int) {
	innerW := geminiBoxWidth(width)
	hdr := strings.TrimSpace(header)

	var content strings.Builder
	if hdr != "" {
		content.WriteString("  " + StyleGeminiHeader.Render(hdr) + "\n")
		content.WriteString(StyleSubtle.Render("  "+strings.Repeat("\u2500", max(0, innerW-2))) + "\n")
	}

	content.WriteString("\n")
	content.WriteString(wrapText("  "+question, innerW) + "\n")
	content.WriteString("\n")
	content.WriteString(StyleGeminiSelected.Render("  > "+input) + "\n")
	content.WriteString("\n")
	content.WriteString(StyleGeminiHint.Render("  Enter: confirm | Ctrl+B: back | Esc: cancel") + "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(ColorGeminiBox).
		PaddingLeft(1).
		PaddingRight(1).
		Width(innerW + 2)

	rendered := boxStyle.Render(content.String())
	tw.prevLines = strings.Count(rendered, "\n") + 1
	tw.Print(rendered + "\n")
}

func fallbackSelect(question string, options []SelectOption, header string) SelectResult {
	hdr := strings.TrimSpace(header)
	if hdr != "" {
		fmt.Println(StyleGeminiHeader.Render("[" + hdr + "]"))
	}
	fmt.Println(StyleGeminiSelected.Render(question))
	for i, opt := range options {
		desc := ""
		if opt.Description != "" {
			desc = "\n    " + StyleGeminiSubDesc.Render(opt.Description)
		}
		fmt.Printf("  %d. %s%s\n", i+1, opt.Label, desc)
	}
	fmt.Printf("  %d. Input a custom value\n", len(options)+1)
	fmt.Print(StyleGeminiHint.Render("Select a number: "))

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	text := strings.TrimSpace(line)

	for i, opt := range options {
		if text == fmt.Sprintf("%d", i+1) {
			return SelectResult{Index: i, Label: opt.Label}
		}
	}
	if text == fmt.Sprintf("%d", len(options)+1) {
		return readFreeText(question, header, 80)
	}
	return SelectResult{Index: -1, Label: text, IsOther: true}
}

func descPart(desc string) string {
	if desc == "" {
		return ""
	}
	return " | " + desc
}
