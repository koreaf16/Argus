// Package tui
// File: markdown_render.go
// Description: goldmark AST 노드를 ANSI 터미널 출력으로 변환하는 커스텀 렌더러
// Responsibility: 각 마크다운 노드 타입별 ANSI 스타일링 및 줄 단위 출력 생성

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// ansiRenderer는 goldmark AST를 ANSI 터미널 줄 슬라이스로 변환한다.
type ansiRenderer struct {
	width int
	lines []string
	cur   strings.Builder // 현재 줄 버퍼

	listDepth    int
	listOrdered  bool
	listCounter  int
	inBlockquote bool

	tableColWidths    []int    // 테이블 열별 최대 표시 너비 (pre-scan)
	tableCurCol       int      // 현재 행의 열 인덱스
	tableRowCellTexts []string // 현재 행의 셀 텍스트 (wrapping 처리용)
}

// newAnsiRenderer는 지정된 터미널 폭의 렌더러를 생성한다.
func newAnsiRenderer(width int) *ansiRenderer {
	if width < 20 {
		width = 80
	}
	return &ansiRenderer{width: width}
}

// RegisterFuncs는 goldmark 렌더러에 노드별 핸들러를 등록한다.
func (r *ansiRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// block nodes
	reg.Register(ast.KindDocument, r.renderDocument)
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)
	// inline nodes
	reg.Register(ast.KindText, r.renderText)
	reg.Register(ast.KindString, r.renderString)
	reg.Register(ast.KindCodeSpan, r.renderCodeSpan)
	reg.Register(ast.KindEmphasis, r.renderEmphasis)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
	reg.Register(ast.KindImage, r.renderImage)
	// GFM table
	reg.Register(east.KindTable, r.renderTable)
	reg.Register(east.KindTableHeader, r.renderTableHeader)
	reg.Register(east.KindTableRow, r.renderTableRow)
	reg.Register(east.KindTableCell, r.renderTableCell)
}

// flushLine은 현재 줄 버퍼를 lines에 추가하고 초기화한다.
func (r *ansiRenderer) flushLine() {
	r.lines = append(r.lines, r.cur.String())
	r.cur.Reset()
}

// writeStr은 현재 줄 버퍼에 텍스트를 추가한다.
func (r *ansiRenderer) writeStr(s string) {
	r.cur.WriteString(s)
}

func (r *ansiRenderer) renderDocument(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderHeading(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		h := n.(*ast.Heading)
		text := stripEmoji(string(n.Text(source)))
		var styled string
		switch h.Level {
		case 1:
			styled = lipgloss.NewStyle().Bold(true).Foreground(ColorClaude).Render(text)
		case 2:
			styled = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(text)
		default:
			styled = lipgloss.NewStyle().Bold(true).Foreground(ColorDim).Render(text)
		}
		r.flushLine()
		r.writeStr(styled)
		r.flushLine()
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderParagraph(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		line := r.cur.String()
		r.cur.Reset()
		wrapped := wordwrap.String(line, r.width)
		for _, wl := range strings.Split(wrapped, "\n") {
			r.lines = append(r.lines, wl)
		}
		r.lines = append(r.lines, "")
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderFencedCodeBlock(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		cb := n.(*ast.FencedCodeBlock)
		lang := ""
		if cb.Language(source) != nil {
			lang = string(cb.Language(source))
		}

		codeLines := collectBlockLines(cb, source)

		var inner strings.Builder
		if lang != "" {
			inner.WriteString(lipgloss.NewStyle().Foreground(ColorDim).Render(lang))
			inner.WriteString("\n")
		}
		inner.WriteString(strings.Join(codeLines, "\n"))

		// 내부 content의 실제 visible 너비를 측정하여 박스 너비 결정
		// Width() 고정 시 유니코드 테이블 문자가 잘리는 버그 방지
		actualW := 0
		for _, line := range strings.Split(inner.String(), "\n") {
			w := lipgloss.Width(line)
			if w > actualW {
				actualW = w
			}
		}
		boxW := actualW
		if boxW > r.width-4 {
			boxW = r.width - 4
		}
		if boxW < 10 {
			boxW = 10
		}

		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSubtle).
			Width(boxW).
			Render(inner.String())

		r.flushLine()
		for _, bl := range strings.Split(box, "\n") {
			r.lines = append(r.lines, bl)
		}
		r.lines = append(r.lines, "")
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderCodeBlock(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		codeLines := collectBlockLines(n, source)
		content := strings.Join(codeLines, "\n")
		actualW := 0
		for _, line := range codeLines {
			w := lipgloss.Width(line)
			if w > actualW {
				actualW = w
			}
		}
		boxW := actualW
		if boxW > r.width-4 {
			boxW = r.width - 4
		}
		if boxW < 10 {
			boxW = 10
		}
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSubtle).
			Width(boxW).
			Render(content)

		r.flushLine()
		for _, bl := range strings.Split(box, "\n") {
			r.lines = append(r.lines, bl)
		}
		r.lines = append(r.lines, "")
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderBlockquote(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	r.inBlockquote = entering
	if !entering {
		r.lines = append(r.lines, "")
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderList(_ util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		l := n.(*ast.List)
		r.listDepth++
		r.listOrdered = l.IsOrdered()
		r.listCounter = l.Start
		if r.listCounter == 0 {
			r.listCounter = 1
		}
	} else {
		r.listDepth--
		if r.listDepth == 0 {
			r.lines = append(r.lines, "")
		}
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderListItem(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		indent := strings.Repeat("  ", r.listDepth-1)
		if r.listOrdered {
			r.writeStr(fmt.Sprintf("%s%d. ", indent, r.listCounter))
			r.listCounter++
		} else {
			r.writeStr(indent + "• ")
		}
	} else {
		line := r.cur.String()
		r.cur.Reset()
		if r.inBlockquote {
			dim := lipgloss.NewStyle().Foreground(ColorDim)
			line = dim.Render("│ ") + line
		}
		wrapped := wordwrap.String(line, r.width-r.listDepth*2)
		for _, wl := range strings.Split(wrapped, "\n") {
			r.lines = append(r.lines, wl)
		}
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderThematicBreak(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		hr := lipgloss.NewStyle().Foreground(ColorSubtle).Render(strings.Repeat("─", r.width))
		r.flushLine()
		r.lines = append(r.lines, hr)
		r.lines = append(r.lines, "")
	}
	return ast.WalkContinue, nil
}

// stripEmoji는 텍스트에서 이모지 문자를 제거한다.
// Windows 터미널에서 이모지가 깨지는 문제를 방지한다.
func stripEmoji(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isEmoji(r) {
			continue
		}
		b.WriteRune(r)
	}
	// 이모지 제거 후 연속 공백 정리
	result := strings.TrimSpace(b.String())
	return result
}

// isEmoji는 rune이 이모지 범위에 속하는지 확인한다.
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F600 && r <= 0x1F64F: // Emoticons
		return true
	case r >= 0x1F300 && r <= 0x1F5FF: // Misc Symbols and Pictographs
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport and Map
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols
		return true
	case r >= 0x1FA00 && r <= 0x1FAFF: // Extended Symbols
		return true
	case r >= 0x2600 && r <= 0x27BF: // Misc symbols, Dingbats
		return true
	case r >= 0x1F1E0 && r <= 0x1F1FF: // Flags
		return true
	case r == 0xFE0F || r == 0x200D: // Variation Selector, ZWJ
		return true
	}
	return false
}
