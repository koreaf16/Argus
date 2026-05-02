package tui

import (
	"strconv"
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

	title := "Add Server"
	if sf.EditAlias != "" {
		title = "Edit Server"
	}
	lines := []string{titleStyle.Render(title), ""}

	authLabel := [3]string{"SSH Agent", "Identity File", "Password"}[sf.AuthMode]

	sshPwValue := passwordMask(sf.Password)
	if sshPwValue == "" && sf.PasswordRegistered {
		sshPwValue = accentStyle.Render("✓ registered")
	}

	mainFields := []struct {
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
		{sfPassword, "Password     ", sshPwValue, sf.AuthMode == serverFormAuthPassword},
		{sfSavePassword, "Save Password", savePwDisplay(sf.SavePassword), sf.AuthMode == serverFormAuthPassword},
		{sfDefaultCWD, "Default CWD  ", sf.DefaultCWD, true},
	}

	for _, f := range mainFields {
		if !f.shown {
			continue
		}
		lines = append(lines, renderFormField(sf, f.field, f.label, f.value, accentStyle, mutedStyle, errorStyle))
	}

	// ─── Elevation section ───
	lines = append(lines, "", mutedStyle.Render("─── Work Accounts (su / sudo) ───"))

	// Allow elevation toggle
	allowVal := elevAllowDisplay(sf.AllowElevation)
	lines = append(lines, renderFormField(sf, sfAllowElevation, "Allow elevation", allowVal, accentStyle, mutedStyle, errorStyle))

	if sf.AllowElevation {
		// Method radio
		methodVal := elevMethodDisplay(sf.ElevationMethod)
		lines = append(lines, renderFormField(sf, sfElevationMethod, "Method         ", methodVal, accentStyle, mutedStyle, errorStyle))

		// Root Password field
		var rootPwVal string
		if sf.ElevationMethod == "sudo" {
			rootPwVal = "(uses Login Password)"
			lines = append(lines, "  Root Password  : "+mutedStyle.Render(rootPwVal))
		} else {
			if sf.RootPassword != "" {
				rootPwVal = passwordMask(sf.RootPassword)
			} else if sf.RootPwRegistered {
				rootPwVal = accentStyle.Render("✓ registered")
			} else {
				rootPwVal = mutedStyle.Render("(not set)")
			}
			isFocusedPw := sf.FocusIdx == sfRootPassword
			if isFocusedPw {
				lines = append(lines, accentStyle.Bold(true).Render("> Root Password  : ")+rootPwVal)
			} else {
				lines = append(lines, "  Root Password  : "+rootPwVal)
			}
		}

		lines = append(lines, "")

		// Dynamic Targets List
		for i, t := range sf.Targets {
			prefix := "  (" + strconv.Itoa(i+1) + ") "

			// User Name Field
			userField := serverFormField(int(sfTargetBase) + i*3)
			isFocusedUser := sf.FocusIdx == userField
			if isFocusedUser {
				lines = append(lines, accentStyle.Bold(true).Render("> "+prefix+"Work Account: ")+t.User)
			} else {
				lines = append(lines, "  "+prefix+"Work Account: "+t.User)
			}

			// Password Field
			pwField := serverFormField(int(sfTargetBase) + i*3 + 1)
			var tPwVal string
			if sf.ElevationMethod == "sudo" {
				tPwVal = mutedStyle.Render("(uses Login Password)")
			} else {
				if t.Password != "" {
					tPwVal = passwordMask(t.Password)
				} else if t.PwRegistered {
					tPwVal = accentStyle.Render("✓ registered")
				} else {
					tPwVal = mutedStyle.Render("(not set)")
				}
			}

			if sf.ElevationMethod == "su" {
				isFocusedPw := sf.FocusIdx == pwField
				if isFocusedPw {
					lines = append(lines, accentStyle.Bold(true).Render(">       Password   : ")+tPwVal)
				} else {
					lines = append(lines, "        Password   : "+tPwVal)
				}
			} else {
				lines = append(lines, "        Password   : "+tPwVal)
			}

			// Remove Button
			rmField := serverFormField(int(sfTargetBase) + i*3 + 2)
			isFocusedRm := sf.FocusIdx == rmField
			if isFocusedRm {
				lines = append(lines, accentStyle.Bold(true).Render(">       [ Remove ]"))
			} else {
				lines = append(lines, mutedStyle.Render("        [ Remove ]"))
			}
		}

		// Add Target Button
		addField := serverFormField(int(sfTargetBase) + len(sf.Targets)*3)
		if sf.FocusIdx == addField {
			lines = append(lines, "", accentStyle.Bold(true).Render("> [+ Add Work Account]"))
		} else {
			lines = append(lines, "", mutedStyle.Render("  [+ Add Work Account]"))
		}

	} else {
		lines = append(lines, mutedStyle.Render("  Method         : (disabled — toggle ON first)"))
		lines = append(lines, mutedStyle.Render("  Root Password  : (disabled)"))
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

	lines = append(lines, "", mutedStyle.Render("[Tab/Shift+Tab] move  [Space] toggle  [←/→] radio  [Enter] next/submit  [Esc] cancel"))

	return borderStyle.Render(strings.Join(lines, "\n"))
}

// renderFormField renders a single labelled field with focus/error styles.
// The value is treated as plain text (not pre-styled).
func renderFormField(sf *serverFormState, field serverFormField, label, value string, accentStyle, mutedStyle, errorStyle lipgloss.Style) string {
	isFocused := sf.FocusIdx == field
	hasError := sf.ErrorField == field && sf.ErrorMsg != ""
	lbl := label + ": "
	val := value
	if val == "" {
		val = mutedStyle.Render("(empty)")
	}
	switch {
	case isFocused && hasError:
		return errorStyle.Bold(true).Render("> "+lbl) + errorStyle.Render(val)
	case isFocused:
		return accentStyle.Bold(true).Render("> " + lbl + val)
	case hasError:
		return errorStyle.Render("  " + lbl + val)
	default:
		return "  " + lbl + val
	}
}

// renderFormFieldRaw is like renderFormField but the value may already contain
// ANSI styling (e.g. mutedStyle.Render("(any)")), so focus style wraps only the label.
func renderFormFieldRaw(sf *serverFormState, field serverFormField, label, styledValue string, accentStyle, errorStyle lipgloss.Style) string {
	isFocused := sf.FocusIdx == field
	hasError := sf.ErrorField == field && sf.ErrorMsg != ""
	lbl := label + ": "
	switch {
	case isFocused && hasError:
		return errorStyle.Bold(true).Render("> "+lbl) + styledValue
	case isFocused:
		return accentStyle.Bold(true).Render("> "+lbl) + styledValue
	case hasError:
		return errorStyle.Render("  "+lbl) + styledValue
	default:
		return "  " + lbl + styledValue
	}
}

func elevAllowDisplay(allow bool) string {
	if allow {
		return "[x] ON"
	}
	return "[ ] OFF"
}

func elevMethodDisplay(method string) string {
	su := "( ) su"
	sudo := "( ) sudo"
	if method == "su" {
		su = "(●) su"
	} else {
		sudo = "(●) sudo"
	}
	return sudo + "   " + su
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
