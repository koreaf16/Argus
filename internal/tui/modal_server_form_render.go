package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderServerForm renders the server-add modal. Called from render.go::renderModal.
func (m uiModel) renderServerForm(
	titleStyle, accentStyle, mutedStyle lipgloss.Style,
	borderStyle lipgloss.Style,
) string {
	sf := m.modal.ServerForm
	if sf == nil {
		return borderStyle.Render(titleStyle.Render("Add Server") + "\n(form state missing)")
	}

	errorStyle := m.theme.style(m.theme.ErrorTitleColor)
	checkStyle := accentStyle.Bold(true)

	lines := []string{titleStyle.Render("Add Server"), ""}

	authLabel := [3]string{"SSH Agent", "Identity File", "Password"}[sf.AuthMode]

	fields := []struct {
		field serverFormField
		label string
		value string
		shown bool
	}{
		{sfAlias, "Alias        ", sf.Alias, true},
		{sfHost, "Host         ", sf.Host, true},
		{sfPort, "Port         ", portDisplay(sf.PortStr), true},
		{sfUser, "User         ", sf.User, true},
		{sfAuthMode, "Auth Method  ", authLabel, true},
		{sfIdentityFile, "Identity File", sf.IdentityFile, sf.AuthMode == serverFormAuthKey},
		{sfPassword, "Password     ", passwordMask(sf.Password), sf.AuthMode == serverFormAuthPassword},
		{sfSavePassword, "Save Password", savePwDisplay(sf.SavePassword), sf.AuthMode == serverFormAuthPassword},
		{sfDefaultCWD, "Default CWD  ", sf.DefaultCWD, true},
	}

	for _, f := range fields {
		if !f.shown {
			continue
		}

		isFocused := sf.FocusIdx == f.field
		hasError := sf.ErrorField == f.field && sf.ErrorMsg != ""

		label := f.label + ": "
		val := f.value
		if val == "" {
			val = mutedStyle.Render("(empty)")
		}

		var line string
		switch {
		case isFocused && hasError:
			line = errorStyle.Bold(true).Render("> "+label) + errorStyle.Render(val)
		case isFocused:
			line = accentStyle.Bold(true).Render("> " + label + val)
		case hasError:
			line = errorStyle.Render("  " + label + val)
		default:
			line = "  " + label + val
		}
		lines = append(lines, line)
	}

	// Submit button
	submitLabel := "[ Submit ]"
	if sf.FocusIdx == sfSubmit {
		lines = append(lines, "", checkStyle.Render("> "+submitLabel))
	} else {
		lines = append(lines, "", mutedStyle.Render("  "+submitLabel))
	}

	if sf.ErrorMsg != "" {
		lines = append(lines, "", errorStyle.Render("✗ "+sf.ErrorMsg))
	}

	lines = append(lines, "", mutedStyle.Render("[Tab/Shift+Tab] move  [Space] toggle  [Enter] next/submit  [Esc] cancel"))

	return borderStyle.Render(strings.Join(lines, "\n"))
}

func portDisplay(s string) string {
	if s == "" {
		return "22"
	}
	return s
}

func passwordMask(pw string) string {
	if pw == "" {
		return ""
	}
	return strings.Repeat("*", len([]rune(pw)))
}

func savePwDisplay(save bool) string {
	if save {
		return "[x] Save (DPAPI-encrypted)"
	}
	return "[ ] Save (Space to toggle)"
}
