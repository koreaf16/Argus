package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m uiModel) renderConnectorSearch(
	titleStyle, mutedStyle lipgloss.Style,
	borderStyle lipgloss.Style,
) string {
	st := m.modal.ConnectorSearch
	if st == nil {
		return borderStyle.Render(titleStyle.Render("⦿ MCP 서버 검색") + "\n(state missing)")
	}

	accentStyle := m.theme.ChevronStyle
	lines := []string{
		titleStyle.Render("⦿ MCP 서버 검색"),
		"",
		mutedStyle.Render("검색어: " + st.Query),
		"",
	}

	if len(st.Results) == 0 {
		lines = append(lines, mutedStyle.Render("검색 결과가 없습니다."))
	} else {
		for i, spec := range st.Results {
			source := fmt.Sprintf("[%-8s]", spec.Source)
			name := spec.Name
			desc := spec.Description
			runtime := string(spec.Runtime)

			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}

			row1 := fmt.Sprintf("%s %s", source, name)
			row2 := fmt.Sprintf("  %s · %s", desc, runtime)

			if i == st.Cursor {
				lines = append(lines,
					accentStyle.Bold(true).Render("› "+row1),
					accentStyle.Render("  "+strings.TrimPrefix(row2, "  ")),
					"",
				)
			} else {
				lines = append(lines,
					"  "+row1,
					mutedStyle.Render(row2),
					"",
				)
			}
		}
	}

	if st.ErrMsg != "" {
		lines = append(lines, m.theme.style(m.theme.ErrorTitleColor).Render("✗ "+st.ErrMsg))
	}

	lines = append(lines, mutedStyle.Render("↑↓ 이동  Enter 설치  Esc 닫기"))
	return borderStyle.Render(strings.Join(lines, "\n"))
}
