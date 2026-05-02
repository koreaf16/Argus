package markdown

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	colPadding        = 2 // 1 space each side of cell content
	tableMargin       = 2 // left indent of the whole table
	minColWidth       = 5 // minimum column content width before aggressive scaling
	tableSafetyMargin = 4 // right-edge safety buffer (mirrors claude_cli SAFETY_MARGIN)
)

// RenderTable renders headers+rows as a box-drawing bordered table string.
func RenderTable(headers []string, rows [][]string, termWidth int, p Palette) string {
	if len(headers) == 0 {
		return ""
	}

	// Normalise column count.
	numCols := len(headers)
	for _, row := range rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	headers = padSlice(headers, numCols)
	for i := range rows {
		rows[i] = padSlice(rows[i], numCols)
	}

	colWidths := allocateColWidths(headers, rows, numCols, termWidth)

	// Wrap cell text to allocated widths, tracking whether each cell was wrapped.
	wrappedHdr := wrapRow(headers, colWidths)
	wrappedRows := make([][]wrappedCell, len(rows))
	maxRowLines := 1
	for i, row := range rows {
		wrappedRows[i] = wrapRow(row, colWidths)
		for _, wc := range wrappedRows[i] {
			if len(wc.lines) > maxRowLines {
				maxRowLines = len(wc.lines)
			}
		}
	}

	// Fall back to vertical key-value layout for very narrow terminals or deeply
	// wrapped rows (mirrors claude_cli useVerticalFormat).
	if termWidth < 80 || maxRowLines > 4 {
		return renderVerticalTable(headers, rows, termWidth, p)
	}

	var parts []string
	indent := strings.Repeat(" ", tableMargin)

	parts = append(parts, indent+renderBorderLine(colWidths, "top", p))
	parts = append(parts, renderDataRows(wrappedHdr, colWidths, true, indent, p)...)
	parts = append(parts, indent+renderBorderLine(colWidths, "mid", p))
	for i, wr := range wrappedRows {
		if i > 0 {
			parts = append(parts, indent+renderBorderLine(colWidths, "mid", p))
		}
		parts = append(parts, renderDataRows(wr, colWidths, false, indent, p)...)
	}
	parts = append(parts, indent+renderBorderLine(colWidths, "bot", p))

	return strings.Join(parts, "\n")
}

// allocateColWidths distributes available terminal width among columns.
// Ported from TableRenderer.tsx:94-174 (Gemini CLI).
func allocateColWidths(headers []string, rows [][]string, numCols, termWidth int) []int {
	type constraint struct{ minW, maxW int }
	cons := make([]constraint, numCols)

	for c := 0; c < numCols; c++ {
		strippedHdr := StripInlineMarkers(headers[c])
		maxContent := VisibleWidth(strippedHdr)
		maxWord := MaxWordWidth(strippedHdr)
		for _, row := range rows {
			cell := StripInlineMarkers(row[c])
			if cw := VisibleWidth(cell); cw > maxContent {
				maxContent = cw
			}
			if mw := MaxWordWidth(cell); mw > maxWord {
				maxWord = mw
			}
		}
		minW := maxWord
		maxW := maxContent
		if maxW < minW {
			maxW = minW
		}
		cons[c] = constraint{minW, maxW}
	}

	// fixedOverhead: border chars (numCols+1) + padding (numCols*colPadding) + tableMargin + safety
	fixedOverhead := (numCols + 1) + numCols*colPadding + tableMargin + tableSafetyMargin
	available := termWidth - fixedOverhead
	if available < 0 {
		available = 0
	}

	totalMin := 0
	for _, c := range cons {
		totalMin += c.minW
	}

	final := make([]int, numCols)

	if totalMin > available {
		// Scale down all columns proportionally, protecting short ones.
		shortTotal := 0
		for _, c := range cons {
			if c.maxW <= minColWidth {
				shortTotal += c.minW
			}
		}
		finalShortTotal := shortTotal
		if finalShortTotal >= available {
			finalShortTotal = 0
		}
		denom := totalMin - finalShortTotal
		scale := 0.0
		if denom > 0 {
			scale = float64(available-finalShortTotal) / float64(denom)
		}
		for c, con := range cons {
			if con.maxW <= minColWidth && finalShortTotal > 0 {
				final[c] = con.minW
			} else {
				w := int(float64(con.minW) * scale)
				if w < 1 {
					w = 1
				}
				final[c] = w
			}
		}
	} else {
		// Distribute surplus space proportionally to growth need.
		surplus := available - totalMin
		totalGrowth := 0
		for _, c := range cons {
			totalGrowth += c.maxW - c.minW
		}
		for c, con := range cons {
			if totalGrowth == 0 {
				final[c] = con.minW
			} else {
				growthNeed := con.maxW - con.minW
				extra := int(float64(surplus) * float64(growthNeed) / float64(totalGrowth))
				w := con.minW + extra
				if w > con.maxW {
					w = con.maxW
				}
				final[c] = w
			}
		}
	}

	return final
}

// wrappedCell holds the wrap result for a single table cell.
// When wrapped is false (cell fits in one line), source is used for rendering
// so inline markers (**bold**, `code`, etc.) are preserved.
// When wrapped is true, lines contains the plain-text wrap result and inline
// markers are stripped to avoid width mis-calculation across wrap boundaries.
type wrappedCell struct {
	lines   []string // plain text lines (markers stripped), used for padding calc
	source  string   // original cell text (markers intact), used when !wrapped
	wrapped bool     // true when len(lines) > 1
}

// wrapRow wraps each cell to its allocated column width.
func wrapRow(cells []string, colWidths []int) []wrappedCell {
	result := make([]wrappedCell, len(cells))
	for c, cell := range cells {
		w := 0
		if c < len(colWidths) {
			w = colWidths[c]
		}
		plain := StripInlineMarkers(StripANSI(cell))
		lines := WrapToWidth(plain, w)
		result[c] = wrappedCell{
			lines:   lines,
			source:  cell,
			wrapped: len(lines) > 1,
		}
	}
	return result
}

// renderBorderLine renders a top/mid/bot border line.
func renderBorderLine(colWidths []int, kind string, p Palette) string {
	type bc struct{ l, m, r, h string }
	chars := map[string]bc{
		"top": {"┌", "┬", "┐", "─"},
		"mid": {"├", "┼", "┤", "─"},
		"bot": {"└", "┴", "┘", "─"},
	}
	c := chars[kind]
	var sb strings.Builder
	sb.WriteString(c.l)
	for i, w := range colWidths {
		sb.WriteString(strings.Repeat(c.h, w+colPadding))
		if i < len(colWidths)-1 {
			sb.WriteString(c.m)
		}
	}
	sb.WriteString(c.r)
	line := sb.String()
	if p.DisableANSI {
		return line
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.TableBorder)).Render(line)
}

// renderDataRows renders one logical data row (which may span multiple visual lines due to wrapping).
func renderDataRows(wrappedCells []wrappedCell, colWidths []int, isHeader bool, indent string, p Palette) []string {
	maxH := 1
	for _, wc := range wrappedCells {
		if len(wc.lines) > maxH {
			maxH = len(wc.lines)
		}
	}

	base := p.TableRow
	if isHeader {
		base = p.TableHeader
	}

	borderS := lipgloss.NewStyle().Foreground(lipgloss.Color(p.TableBorder))
	pipe := func() string {
		if p.DisableANSI {
			return "|"
		}
		return borderS.Render("│")
	}

	var result []string
	for h := 0; h < maxH; h++ {
		var sb strings.Builder
		sb.WriteString(indent)
		sb.WriteString(pipe())
		for c, wc := range wrappedCells {
			w := 0
			if c < len(colWidths) {
				w = colWidths[c]
			}

			var rendered, plain string
			if wc.wrapped {
				// Multi-line: inline markers are already stripped in wc.lines.
				// Apply base color only to preserve alignment.
				if h < len(wc.lines) {
					plain = wc.lines[h]
				}
				rendered = seg(plain, base, isHeader, false)
			} else if h == 0 {
				// Single line: use original source so inline markers are rendered.
				plain = wc.lines[0]
				rendered = ParseInlineWithBase(wc.source, base, isHeader, p)
			}

			padding := w - VisibleWidth(plain)
			if padding < 0 {
				padding = 0
			}
			sb.WriteString(" " + rendered + strings.Repeat(" ", padding+1))
			sb.WriteString(pipe())
		}
		result = append(result, sb.String())
	}
	return result
}

// renderVerticalTable renders a table in key-value vertical format, used when
// the terminal is too narrow or rows wrap too deeply for horizontal layout.
func renderVerticalTable(headers []string, rows [][]string, termWidth int, p Palette) string {
	if len(headers) == 0 || len(rows) == 0 {
		return ""
	}

	indent := strings.Repeat(" ", tableMargin)
	contentW := termWidth - tableMargin - 2 // "│ " prefix width
	if contentW < 10 {
		contentW = 10
	}

	borderS := lipgloss.NewStyle().Foreground(lipgloss.Color(p.TableBorder))
	pipeS := borderS.Render("│")
	top := indent + borderS.Render("┌"+strings.Repeat("─", termWidth-tableMargin-2)+"┐")
	sep := indent + borderS.Render("├"+strings.Repeat("─", termWidth-tableMargin-2)+"┤")
	bot := indent + borderS.Render("└"+strings.Repeat("─", termWidth-tableMargin-2)+"┘")

	var parts []string
	parts = append(parts, top)
	for i, row := range rows {
		if i > 0 {
			parts = append(parts, sep)
		}
		for c, cell := range row {
			if c >= len(headers) {
				break
			}
			label := ParseInlineWithBase(headers[c], p.TableHeader, true, p) + ": "
			value := ParseInlineWithBase(cell, p.TableRow, false, p)
			combined := label + value
			wrapped := WrapStyled(combined, contentW)
			for j, wl := range wrapped {
				prefix := "   "
				if j == 0 {
					prefix = " "
				}
				parts = append(parts, indent+pipeS+prefix+wl)
			}
		}
	}
	parts = append(parts, bot)
	return strings.Join(parts, "\n")
}

func padSlice(s []string, n int) []string {
	for len(s) < n {
		s = append(s, "")
	}
	return s
}
