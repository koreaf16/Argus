package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	liveAreaHeight  = 13
	liveAreaContent = 10
)

type liveArea struct {
	height     int
	width      int
	title      string
	header     string
	info       string
	lines      []string
	totalLines int
	drawn      int
}

func newLiveArea(width int) *liveArea {
	return &liveArea{height: liveAreaHeight, width: width}
}

func (la *liveArea) SetTitle(s string)  { la.title = s }
func (la *liveArea) SetHeader(s string) { la.header = s }
func (la *liveArea) SetInfo(s string)   { la.info = s }

func (la *liveArea) Append(line string) {
	la.totalLines++
	la.lines = append(la.lines, truncateLiveLine(line, la.contentWidth()-2))
	if len(la.lines) > liveAreaContent {
		la.lines = la.lines[len(la.lines)-liveAreaContent:]
	}
}

func (la *liveArea) SetContent(lines []string) {
	la.SetContentWithTotal(lines, len(lines))
}

func (la *liveArea) SetContentWithTotal(lines []string, total int) {
	if total < len(lines) {
		total = len(lines)
	}
	la.totalLines = total
	maxW := la.contentWidth() - 2
	if len(lines) > liveAreaContent {
		lines = lines[len(lines)-liveAreaContent:]
	}
	la.lines = make([]string, len(lines))
	for i, l := range lines {
		la.lines[i] = truncateLiveLine(l, maxW)
	}
}

func (la *liveArea) Reset() {
	la.title = ""
	la.header = ""
	la.info = ""
	la.lines = nil
	la.totalLines = 0
}

func (la *liveArea) Redraw() {
	la.clearDrawn()
	la.draw()
}

func (la *liveArea) PrintPermanent(text string) {
	la.clearDrawn()
	if text != "" {
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		fmt.Fprint(os.Stdout, text)
	}
	la.draw()
}

func (la *liveArea) PrintPermanentBatch(texts ...string) {
	la.clearDrawn()
	for _, text := range texts {
		if text == "" {
			continue
		}
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		fmt.Fprint(os.Stdout, text)
	}
	la.draw()
}

func (la *liveArea) Clear() {
	la.clearDrawn()
}

func (la *liveArea) SetWidth(w int) {
	la.width = w
}

func (la *liveArea) contentWidth() int {
	return max(20, la.width-4)
}

func (la *liveArea) clearDrawn() {
	if la.drawn > 0 {
		// ANSI 위로 이동 시 실제 터미널 물리 줄 수를 고려해야 하지만,
		// lipgloss.Height로 계산된 값을 기반으로 안전하게 이동합니다.
		fmt.Fprintf(os.Stdout, ansiCursorUp, la.drawn)
		fmt.Fprint(os.Stdout, ansiClearToEnd)
		la.drawn = 0
	}
}

func (la *liveArea) draw() {
	rendered := la.render()
	if rendered == "" {
		la.drawn = 0
		return
	}
	// lipgloss.Height는 ANSI 이스케이프와 줄 바꿈을 포함한 실제 높이를 반환합니다.
	la.drawn = lipgloss.Height(rendered)
	fmt.Fprint(os.Stdout, rendered)
	if !strings.HasSuffix(rendered, "\n") {
		fmt.Fprint(os.Stdout, "\n")
		la.drawn++
	}
}

func (la *liveArea) render() string {
	if len(la.lines) > 0 {
		boxW := la.contentWidth()
		numContent := la.height - 2
		parts := make([]string, numContent)
		parts[0] = la.info

		for i, line := range la.lines {
			if 1+i < numContent {
				parts[1+i] = line
			}
		}

		content := strings.Join(parts, "\n")
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSubtle).
			Width(boxW).
			Render(content)

		if la.title != "" {
			lines := strings.Split(box, "\n")
			if len(lines) > 0 {
				lines[0] = buildRoundedBorderTitleLine(lipgloss.Width(lines[0]), la.title)
				box = strings.Join(lines, "\n")
			}
		}

		if la.header != "" {
			return la.header + "\n" + box
		}
		return box
	}

	if la.header != "" {
		return la.header
	}

	return ""
}

func truncateLiveLine(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	w := 0
	for i, r := range s {
		rw := 1
		if r > 0x2E7F {
			rw = 2
		}
		if w+rw > maxW {
			return s[:i] + "..."
		}
		w += rw
	}
	return s
}
