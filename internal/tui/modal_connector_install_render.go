package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m uiModel) renderConnectorInstall(
	titleStyle, mutedStyle lipgloss.Style,
	borderStyle lipgloss.Style,
) string {
	st := m.modal.ConnectorInstall
	if st == nil {
		return borderStyle.Render(titleStyle.Render("⦿ MCP 서버 설치") + "\n(state missing)")
	}

	accentStyle := m.theme.ChevronStyle
	errorStyle := m.theme.style(m.theme.ErrorTitleColor)
	checkStyle := m.theme.style(m.theme.UserTitleColor)

	lines := []string{
		titleStyle.Render("⦿ MCP 서버 설치"),
		"",
		m.theme.style(m.theme.BodyColor).Bold(true).Render(st.Spec.Name),
		mutedStyle.Render(fmt.Sprintf("%s · %s", st.Spec.Runtime, st.Spec.Source)),
		"",
	}

	if st.Installing {
		lines = append(lines, accentStyle.Render("⟳ 도구 발견 중..."))
		return borderStyle.Render(strings.Join(lines, "\n"))
	}

	if len(st.Spec.EnvPrompts) == 0 {
		lines = append(lines, mutedStyle.Render("환경변수가 필요하지 않습니다."), "")
	} else {
		for i, ep := range st.Spec.EnvPrompts {
			isFocused := i == st.FocusIdx
			hasError := st.ErrMsg != "" && i == st.FocusIdx

			required := "(선택)"
			if ep.Required {
				required = "(필수)"
			}

			val := st.EnvValues[i]
			// Mask secret values
			display := val
			if ep.Secret && len(val) > 0 {
				display = strings.Repeat("*", len(val))
			}

			label := fmt.Sprintf("%-14s: %s█", ep.Key, display)
			hint := fmt.Sprintf("  %s %s", required, ep.Description)

			switch {
			case isFocused && hasError:
				lines = append(lines,
					errorStyle.Bold(true).Render("> "+label),
					errorStyle.Render(hint),
					"",
				)
			case isFocused:
				lines = append(lines,
					accentStyle.Bold(true).Render("> "+label),
					mutedStyle.Render(hint),
					"",
				)
			default:
				lines = append(lines,
					"  "+label,
					mutedStyle.Render(hint),
					"",
				)
			}
		}
	}

	// Submit button
	submitIdx := len(st.Spec.EnvPrompts)
	if st.FocusIdx == submitIdx {
		lines = append(lines, checkStyle.Bold(true).Render("› [ 설치 ]"), "")
	} else {
		lines = append(lines, mutedStyle.Render("  [ 설치 ]"), "")
	}

	if st.ErrMsg != "" {
		lines = append(lines, errorStyle.Render("✗ "+st.ErrMsg), "")
	}

	if len(st.Spec.EnvPrompts) > 0 {
		lines = append(lines, mutedStyle.Render("Tab/↑↓ 이동  Enter 다음/설치  Esc 취소"))
	} else {
		lines = append(lines, mutedStyle.Render("Enter 설치  Esc 취소"))
	}

	return borderStyle.Render(strings.Join(lines, "\n"))
}
