package markdown

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

// Render converts markdown text to ANSI-styled terminal output.
// termWidth is passed to the table renderer for column allocation.
func Render(text string, termWidth int, p Palette) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if p.DisableANSI {
		return text
	}

	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	parts := blockParse(lines, termWidth, p)
	return strings.Join(parts, "\n")
}

func blockParse(lines []string, termWidth int, p Palette) []string {
	var parts []string

	// State
	inCode := false
	fence := ""
	codeLang := ""
	var codeLines []string

	inTable := false
	var tableHeaders []string
	var tableRows [][]string

	addPart := func(s string) { parts = append(parts, s) }

	flushCode := func() {
		addPart(RenderCodeBlock(codeLang, codeLines, p))
		codeLines = codeLines[:0]
		codeLang = ""
	}

	flushTable := func() {
		if len(tableHeaders) > 0 && len(tableRows) > 0 {
			addPart(RenderTable(tableHeaders, tableRows, termWidth, p))
		}
		inTable = false
		tableHeaders = nil
		tableRows = nil
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// ── Inside code block ──────────────────────────────────────────────
		if inCode {
			if f, _, ok := parseFence(trimmed); ok && strings.HasPrefix(trimmed, fence[0:1]) && len(f) >= len(fence) {
				flushCode()
				inCode = false
				fence = ""
			} else {
				codeLines = append(codeLines, line)
			}
			continue
		}

		// ── Detect code fence start ────────────────────────────────────────
		if f, lang, ok := parseFence(trimmed); ok {
			// Close any open table first
			if inTable {
				flushTable()
			}
			inCode = true
			fence = f
			codeLang = lang
			continue
		}

		// ── Table handling ─────────────────────────────────────────────────
		isTableRow := isTableRowLine(trimmed)
		isSep := isTableSepLine(trimmed)

		if isTableRow && !inTable {
			// Check if next line is a separator → start table
			if i+1 < len(lines) && isTableSepLine(strings.TrimSpace(lines[i+1])) {
				inTable = true
				tableHeaders = splitTableRow(trimmed)
				tableRows = nil
			} else {
				addPart(renderInlineLine(line, termWidth, p))
			}
			continue
		}

		if inTable {
			if isSep {
				// Skip separator
				continue
			}
			if isTableRow {
				cells := splitTableRow(trimmed)
				cells = padSlice(cells, len(tableHeaders))
				if len(cells) > len(tableHeaders) {
					cells = cells[:len(tableHeaders)]
				}
				tableRows = append(tableRows, cells)
				continue
			}
			// Non-table line → flush table, then process current line
			flushTable()
			if trimmed != "" {
				addPart(renderInlineLine(line, termWidth, p))
			}
			continue
		}

		// ── Headings ───────────────────────────────────────────────────────
		if level, heading, ok := parseHeading(trimmed); ok {
			addPart(renderHeading(heading, level, termWidth, p))
			continue
		}

		// ── Horizontal rule ────────────────────────────────────────────────
		if isHRule(trimmed) {
			addPart(lipgloss.NewStyle().Foreground(lipgloss.Color(p.Rule)).Render(strings.Repeat("─", 36)))
			continue
		}

		// ── Blockquote ─────────────────────────────────────────────────────
		if strings.HasPrefix(trimmed, "> ") {
			inner := strings.TrimSpace(strings.TrimPrefix(trimmed, "> "))
			addPart(lipgloss.NewStyle().Foreground(lipgloss.Color(p.Quote)).Render("│ " + ParseInline(inner, p)))
			continue
		}

		// ── List items ─────────────────────────────────────────────────────
		if marker, rest, indent, ok := parseListItem(line); ok {
			addPart(renderListItem(marker, rest, indent, termWidth, p))
			continue
		}

		// ── Empty line ─────────────────────────────────────────────────────
		if trimmed == "" {
			addPart("")
			continue
		}

		// ── Regular paragraph line ─────────────────────────────────────────
		addPart(renderInlineLine(line, termWidth, p))
	}

	// Flush any open blocks at EOF
	if inCode {
		flushCode()
	}
	if inTable {
		flushTable()
	}

	return parts
}

// ── Block element helpers ─────────────────────────────────────────────────────

func renderInlineLine(line string, termWidth int, p Palette) string {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Body)).
		Render(ParseInline(line, p))
	return wordwrap.String(styled, termWidth)
}

func renderHeading(text string, level int, termWidth int, p Palette) string {
	var color string
	var bold bool
	switch level {
	case 1:
		color, bold = p.Heading1, true
	case 2:
		color, bold = p.Heading2, true
	case 3:
		color, bold = p.Heading3, true
	default:
		color = p.Heading4
	}
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	if bold {
		s = s.Bold(true)
	} else {
		s = s.Italic(true)
	}
	rendered := s.Render(ParseInline(text, p))
	return wordwrap.String(rendered, termWidth)
}

func renderListItem(marker, text, indent string, termWidth int, p Palette) string {
	prefix := marker + " "
	prefixW := len(prefix)

	bodyW := termWidth - len(indent) - prefixW
	if bodyW < 10 {
		bodyW = 10
	}

	bodyStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Body)).Render(ParseInline(text, p))
	bodyWrapped := wordwrap.String(bodyStyled, bodyW)

	lines := strings.Split(bodyWrapped, "\n")
	if len(lines) == 0 {
		return indent + lipgloss.NewStyle().Foreground(lipgloss.Color(p.Body)).Render(prefix)
	}

	out := indent + lipgloss.NewStyle().Foreground(lipgloss.Color(p.Body)).Render(prefix) + lines[0]
	for i := 1; i < len(lines); i++ {
		out += "\n" + indent + strings.Repeat(" ", prefixW) + lines[i]
	}

	return out
}

// ── Parsing helpers ───────────────────────────────────────────────────────────

func parseFence(trimmed string) (fence, lang string, ok bool) {
	for _, f := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, f) {
			return f, strings.TrimSpace(strings.TrimPrefix(trimmed, f)), true
		}
	}
	return "", "", false
}

func parseHeading(trimmed string) (level int, text string, ok bool) {
	i := 0
	for i < len(trimmed) && trimmed[i] == '#' {
		i++
	}
	if i == 0 || i > 4 || i >= len(trimmed) || trimmed[i] != ' ' {
		return 0, "", false
	}
	return i, strings.TrimSpace(trimmed[i+1:]), true
}

func parseListItem(line string) (marker, text, indent string, ok bool) {
	trimLeft := strings.TrimLeft(line, " \t")
	if trimLeft == "" {
		return "", "", "", false
	}
	ind := line[:len(line)-len(trimLeft)]
	if len(trimLeft) >= 2 && (trimLeft[0] == '-' || trimLeft[0] == '*' || trimLeft[0] == '+') && trimLeft[1] == ' ' {
		return string(trimLeft[0]), strings.TrimSpace(trimLeft[2:]), ind, true
	}
	digits := 0
	for digits < len(trimLeft) && trimLeft[digits] >= '0' && trimLeft[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits+1 < len(trimLeft) && trimLeft[digits] == '.' && trimLeft[digits+1] == ' ' {
		return trimLeft[:digits+1], strings.TrimSpace(trimLeft[digits+2:]), ind, true
	}
	return "", "", "", false
}

func isHRule(trimmed string) bool {
	clean := strings.ReplaceAll(trimmed, " ", "")
	return len(clean) >= 3 &&
		(strings.Trim(clean, "-") == "" || strings.Trim(clean, "_") == "" || strings.Trim(clean, "*") == "")
}

func isTableRowLine(trimmed string) bool {
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
}

var tableSepChars = strings.NewReplacer("-", "", ":", "", "|", "", " ", "")

func isTableSepLine(trimmed string) bool {
	if !strings.Contains(trimmed, "-") {
		return false
	}
	rest := tableSepChars.Replace(trimmed)
	return rest == ""
}

func splitTableRow(trimmed string) []string {
	// Trim leading/trailing pipes then split on |
	inner := strings.TrimPrefix(trimmed, "|")
	inner = strings.TrimSuffix(inner, "|")
	parts := strings.Split(inner, "|")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// StableStreamingPrefix returns the longest prefix of text whose rendered markdown
// output is guaranteed not to shift as more text is appended at the tail. Lines
// belonging to a still-open multi-line block (open code fence, open or in-progress
// table) are excluded, because their rendered structure can change retroactively
// as new tokens arrive (e.g. a table grows downward and re-positions its bottom
// border, breaking line-index-based diff flushing).
func StableStreamingPrefix(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	// 스트리밍 본문이 "\n"으로 끝나면 split이 만든 마지막 빈 문자열은 단순 아티팩트다.
	// 진짜 빈 줄(블록 종료)이 아니므로 분석 길이에서 제외한다.
	n := len(lines)
	if n > 0 && lines[n-1] == "" {
		n--
	}

	inCode := false
	fence := ""
	codeStart := -1

	inTable := false
	tableStart := -1

	for i := 0; i < n; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if inCode {
			if f, _, ok := parseFence(trimmed); ok && strings.HasPrefix(trimmed, fence[0:1]) && len(f) >= len(fence) {
				inCode = false
				fence = ""
				codeStart = -1
			}
			continue
		}

		if f, _, ok := parseFence(trimmed); ok {
			inCode = true
			fence = f
			codeStart = i
			continue
		}

		if inTable {
			if isTableSepLine(trimmed) || isTableRowLine(trimmed) {
				continue
			}
			inTable = false
			tableStart = -1
		}

		if !inTable && isTableRowLine(trimmed) {
			if i+1 < n && isTableSepLine(strings.TrimSpace(lines[i+1])) {
				inTable = true
				tableStart = i
				continue
			}
			// 분석 범위(n) 내에 후속 비-공백 라인이 없으면, 표 헤더가 형성 중일 수
			// 있으므로 보수적으로 표가 진행 중인 것으로 간주한다.
			if onlyEmptyLinesAfter(lines, i, n) {
				inTable = true
				tableStart = i
				continue
			}
		}
	}

	cutoff := len(lines)
	if inCode && codeStart >= 0 && codeStart < cutoff {
		cutoff = codeStart
	}
	if inTable && tableStart >= 0 && tableStart < cutoff {
		cutoff = tableStart
	}

	if cutoff >= len(lines) {
		return text
	}
	if cutoff <= 0 {
		return ""
	}
	return strings.Join(lines[:cutoff], "\n") + "\n"
}

func onlyEmptyLinesAfter(lines []string, idx, end int) bool {
	for j := idx + 1; j < end; j++ {
		if strings.TrimSpace(lines[j]) != "" {
			return false
		}
	}
	return true
}
