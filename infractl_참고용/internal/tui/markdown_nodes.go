// Package tui
// File: markdown_nodes.go
// Description: 인라인 노드, GFM 테이블, 코드블록 보조 렌더링 함수
// Responsibility: 인라인/테이블 노드 ANSI 출력, 코드블록 줄 수집 유틸

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"
)

// --- inline nodes ---

func (r *ansiRenderer) renderText(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		t := n.(*ast.Text)
		text := string(t.Segment.Value(source))
		if r.inBlockquote {
			dim := lipgloss.NewStyle().Foreground(ColorDim)
			text = dim.Render("│ ") + text
		}
		r.writeStr(text)
		if t.SoftLineBreak() {
			r.writeStr(" ")
		}
		if t.HardLineBreak() {
			r.flushLine()
		}
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderString(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		r.writeStr(string(n.Text(source)))
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderCodeSpan(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		code := string(n.Text(source))
		styled := lipgloss.NewStyle().Foreground(ColorPermission).Render(code)
		r.writeStr(styled)
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderEmphasis(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		em := n.(*ast.Emphasis)
		text := string(n.Text(source))
		var styled string
		if em.Level == 2 {
			styled = lipgloss.NewStyle().Bold(true).Render(text)
		} else {
			styled = lipgloss.NewStyle().Italic(true).Render(text)
		}
		r.writeStr(styled)
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderLink(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		link := n.(*ast.Link)
		text := string(n.Text(source))
		dest := string(link.Destination)
		styled := lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("6")).Render(text)
		r.writeStr(styled + " (" + lipgloss.NewStyle().Foreground(ColorDim).Render(dest) + ")")
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderAutoLink(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		url := string(n.(*ast.AutoLink).URL(source))
		styled := lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("6")).Render(url)
		r.writeStr(styled)
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderImage(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		img := n.(*ast.Image)
		alt := string(n.Text(source))
		dest := string(img.Destination)
		r.writeStr(fmt.Sprintf("[image: %s](%s)", alt, dest))
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

// --- GFM table (box-drawing style) ---

// renderTable handles the outermost Table node.
// entering: pre-scan col widths, draw top border.
// leaving: replace last ├─┤ row with └─┘, add blank line.
func (r *ansiRenderer) renderTable(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		r.flushLine()
		r.tableColWidths = calcTableColWidths(n, source, r.width)
		r.tableCurCol = 0
		r.lines = append(r.lines, buildHorizLine("┌", "─", "┬", "┐", r.tableColWidths))
	} else {
		if len(r.lines) > 0 && strings.HasPrefix(r.lines[len(r.lines)-1], "├") {
			r.lines[len(r.lines)-1] = buildHorizLine("└", "─", "┴", "┘", r.tableColWidths)
		}
		r.tableColWidths = nil
		r.lines = append(r.lines, "")
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderTableHeader(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		r.tableCurCol = 0
		r.tableRowCellTexts = nil
	} else {
		for _, line := range renderWrappedTableRow(r.tableRowCellTexts, r.tableColWidths, true) {
			r.lines = append(r.lines, line)
		}
		r.lines = append(r.lines, buildHorizLine("├", "─", "┼", "┤", r.tableColWidths))
		r.tableCurCol = 0
		r.tableRowCellTexts = nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderTableRow(_ util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		r.tableCurCol = 0
		r.tableRowCellTexts = nil
	} else {
		for _, line := range renderWrappedTableRow(r.tableRowCellTexts, r.tableColWidths, false) {
			r.lines = append(r.lines, line)
		}
		r.lines = append(r.lines, buildHorizLine("├", "─", "┼", "┤", r.tableColWidths))
		r.tableCurCol = 0
		r.tableRowCellTexts = nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderTableCell(_ util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		text := string(n.Text(source))
		r.tableRowCellTexts = append(r.tableRowCellTexts, text)
		r.tableCurCol++
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

// renderWrappedTableRow는 셀 텍스트를 각 열 너비에 맞게 최대 2줄로 래핑하여 행을 렌더링한다.
func renderWrappedTableRow(cells []string, colWidths []int, bold bool) []string {
	if len(cells) == 0 {
		return nil
	}
	wrapped := make([][]string, len(cells))
	rowHeight := 1
	for i, text := range cells {
		colW := 4
		if i < len(colWidths) {
			colW = colWidths[i]
		}
		lines := wrapCellText(text, colW)
		wrapped[i] = lines
		if len(lines) > rowHeight {
			rowHeight = len(lines)
		}
	}

	result := make([]string, rowHeight)
	for lineIdx := 0; lineIdx < rowHeight; lineIdx++ {
		var sb strings.Builder
		for i, wlines := range wrapped {
			colW := 4
			if i < len(colWidths) {
				colW = colWidths[i]
			}
			var cellLine string
			if lineIdx < len(wlines) {
				cellLine = wlines[lineIdx]
			}
			sb.WriteString("│ " + padCell(cellLine, colW) + " ")
		}
		sb.WriteString("│")
		line := sb.String()
		if bold {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		result[lineIdx] = line
	}
	return result
}

// wrapCellText는 텍스트를 maxW 터미널 열 너비에 맞게 최대 2줄로 나눈다.
func wrapCellText(text string, maxW int) []string {
	if lipgloss.Width(text) <= maxW {
		return []string{text}
	}
	runes := []rune(text)
	first, rest := splitRunesByWidth(runes, maxW)
	if len(rest) == 0 {
		return []string{string(first)}
	}
	// 두 번째 줄: 나머지가 maxW를 초과하면 maxW-1에서 자르고 "…" 추가
	second, overflow := splitRunesByWidth(rest, maxW)
	line2 := string(second)
	if len(overflow) > 0 {
		shorter, _ := splitRunesByWidth(rest, maxW-1)
		line2 = string(shorter) + "…"
	}
	return []string{string(first), line2}
}

// splitRunesByWidth는 rune 슬라이스를 maxW 터미널 너비 기준으로 앞/뒤로 분리한다.
func splitRunesByWidth(runes []rune, maxW int) (first, rest []rune) {
	w := 0
	for i, r := range runes {
		rw := lipgloss.Width(string(r))
		if w+rw > maxW {
			return runes[:i], runes[i:]
		}
		w += rw
	}
	return runes, nil
}

// calcTableColWidths는 테이블 AST를 선행 스캔하여 열별 최대 표시 너비를 계산한다.
// maxWidth를 초과하면 가장 넓은 열부터 1씩 줄인다 (최소 4).
func calcTableColWidths(tableNode ast.Node, source []byte, maxWidth int) []int {
	var colWidths []int
	for row := tableNode.FirstChild(); row != nil; row = row.NextSibling() {
		colIdx := 0
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			text := string(cell.Text(source))
			w := cellDisplayWidth(text)
			if w < 4 {
				w = 4
			}
			if colIdx >= len(colWidths) {
				colWidths = append(colWidths, w)
			} else if w > colWidths[colIdx] {
				colWidths[colIdx] = w
			}
			colIdx++
		}
	}
	if len(colWidths) == 0 {
		return colWidths
	}
	// 전체 테이블 폭: 1(좌측 │) + 각 열당 (colW + 2 + 1) = 1 + sum(colW+3)
	tableW := 1
	for _, w := range colWidths {
		tableW += w + 3
	}
	for tableW > maxWidth {
		maxIdx := 0
		for i := range colWidths {
			if colWidths[i] > colWidths[maxIdx] {
				maxIdx = i
			}
		}
		if colWidths[maxIdx] <= 4 {
			break
		}
		colWidths[maxIdx]--
		tableW--
	}
	return colWidths
}

// buildHorizLine은 박스 드로잉 수평 구분선을 생성한다.
// 예: buildHorizLine("┌","─","┬","┐", [4,6]) → "┌──────┬────────┐"
func buildHorizLine(left, fill, mid, right string, colWidths []int) string {
	var sb strings.Builder
	sb.WriteString(left)
	for i, w := range colWidths {
		sb.WriteString(strings.Repeat(fill, w+2))
		if i < len(colWidths)-1 {
			sb.WriteString(mid)
		}
	}
	sb.WriteString(right)
	return sb.String()
}

// padCell은 표시 너비를 기준으로 텍스트를 좌측 정렬 패딩한다.
// lipgloss.Width를 사용해 이모지·CJK 등 와이드 문자를 정확히 처리한다.
func padCell(text string, targetW int) string {
	w := lipgloss.Width(text)
	if w >= targetW {
		return text
	}
	return text + strings.Repeat(" ", targetW-w)
}

// cellDisplayWidth는 터미널 표시 너비를 반환한다 (이모지·CJK 포함).
func cellDisplayWidth(s string) int {
	return lipgloss.Width(s)
}

// collectBlockLines는 코드 블록 노드의 텍스트를 줄 단위로 수집한다.
func collectBlockLines(n ast.Node, source []byte) []string {
	var lines []string
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		line := string(seg.Value(source))
		line = strings.TrimRight(line, "\n\r")
		lines = append(lines, line)
	}
	return lines
}
