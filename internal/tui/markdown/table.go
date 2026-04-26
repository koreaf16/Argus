package markdown

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	colPadding  = 2 // 1 space each side of cell content
	tableMargin = 2 // left indent of the whole table
	minColWidth = 5 // minimum column content width before aggressive scaling
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

	// Wrap cell text to allocated widths.
	wrappedHdr := wrapRow(headers, colWidths)
	wrappedRows := make([][][]string, len(rows))
	for i, row := range rows {
		wrappedRows[i] = wrapRow(row, colWidths)
	}

	var parts []string
	indent := strings.Repeat(" ", tableMargin)

	parts = append(parts, indent+renderBorderLine(colWidths, "top", p))
	parts = append(parts, renderDataRows(wrappedHdr, colWidths, true, indent, p)...)
	parts = append(parts, indent+renderBorderLine(colWidths, "mid", p))
	for _, wr := range wrappedRows {
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

	// fixedOverhead: border chars (numCols+1) + padding (numCols*colPadding) + tableMargin
	fixedOverhead := (numCols + 1) + numCols*colPadding + tableMargin
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

// wrapRow wraps each cell string to its allocated column width.
func wrapRow(cells []string, colWidths []int) [][]string {
	result := make([][]string, len(cells))
	for c, cell := range cells {
		w := 0
		if c < len(colWidths) {
			w = colWidths[c]
		}
		result[c] = WrapToWidth(StripInlineMarkers(StripANSI(cell)), w)
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
func renderDataRows(wrappedCells [][]string, colWidths []int, isHeader bool, indent string, p Palette) []string {
	maxH := 1
	for _, lines := range wrappedCells {
		if len(lines) > maxH {
			maxH = len(lines)
		}
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
		for c, lines := range wrappedCells {
			w := 0
			if c < len(colWidths) {
				w = colWidths[c]
			}
			cell := ""
			if h < len(lines) {
				cell = lines[h]
			}

			cellW := VisibleWidth(cell)
			padding := w - cellW
			if padding < 0 {
				padding = 0
			}

			var rendered string
			if isHeader {
				rendered = ParseInlineWithBase(cell, p.TableHeader, true, p)
			} else {
				rendered = ParseInlineWithBase(cell, p.TableRow, false, p)
			}

			// 1 space padding on each side + right-fill
			sb.WriteString(" " + rendered + strings.Repeat(" ", padding+1))
			sb.WriteString(pipe())
		}
		result = append(result, sb.String())
	}
	return result
}

func padSlice(s []string, n int) []string {
	for len(s) < n {
		s = append(s, "")
	}
	return s
}
