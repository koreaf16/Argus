package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/koreaf16/argus/internal/repl/commands"
	"github.com/koreaf16/argus/internal/services/workspace"
)

// activeServerFormFields returns the ordered list of fields that are currently
// visible given the selected auth mode. Hidden fields are skipped during Tab navigation.
func activeServerFormFields(sf *serverFormState) []serverFormField {
	base := []serverFormField{sfAlias, sfHost, sfPort, sfUser, sfAuthMode}
	switch sf.AuthMode {
	case serverFormAuthKey:
		base = append(base, sfIdentityFile)
	case serverFormAuthPassword:
		base = append(base, sfPassword, sfSavePassword)
	}
	base = append(base, sfDefaultCWD, sfSubmit)
	return base
}

func (m *uiModel) handleServerFormKey(msg tea.KeyMsg) {
	sf := m.modal.ServerForm
	if sf == nil {
		m.cancelServerForm(fmt.Errorf("form state missing"))
		return
	}

	fields := activeServerFormFields(sf)
	currentIdx := 0
	for i, f := range fields {
		if f == sf.FocusIdx {
			currentIdx = i
			break
		}
	}

	switch msg.String() {
	case "esc":
		m.cancelServerForm(fmt.Errorf("canceled"))
		return

	case "tab", "down", "ctrl+n":
		next := (currentIdx + 1) % len(fields)
		sf.FocusIdx = fields[next]

	case "shift+tab", "up", "ctrl+p":
		prev := (currentIdx - 1 + len(fields)) % len(fields)
		sf.FocusIdx = fields[prev]

	case "enter":
		if sf.FocusIdx == sfSubmit {
			m.submitServerForm()
			return
		}
		// Move to next field on Enter (like Tab).
		next := (currentIdx + 1) % len(fields)
		sf.FocusIdx = fields[next]

	case " ":
		switch sf.FocusIdx {
		case sfAuthMode:
			sf.AuthMode = (sf.AuthMode + 1) % 3
			// Reset auth-specific fields when mode changes.
			sf.IdentityFile = ""
			sf.Password = ""
			sf.SavePassword = false
			// Snap focus to a valid field after mode change.
			sf.FocusIdx = sfAuthMode
		case sfSavePassword:
			sf.SavePassword = !sf.SavePassword
		}

	case "backspace":
		m.serverFormBackspace(sf)

	default:
		if len(msg.Runes) == 1 {
			m.serverFormAppend(sf, msg.Runes[0])
		}
	}
}

func (m *uiModel) serverFormBackspace(sf *serverFormState) {
	switch sf.FocusIdx {
	case sfAlias:
		sf.Alias = dropLast(sf.Alias)
	case sfHost:
		sf.Host = dropLast(sf.Host)
	case sfPort:
		sf.PortStr = dropLast(sf.PortStr)
	case sfUser:
		sf.User = dropLast(sf.User)
	case sfIdentityFile:
		sf.IdentityFile = dropLast(sf.IdentityFile)
	case sfPassword:
		sf.Password = dropLast(sf.Password)
	case sfDefaultCWD:
		sf.DefaultCWD = dropLast(sf.DefaultCWD)
	}
}

func (m *uiModel) serverFormAppend(sf *serverFormState, r rune) {
	switch sf.FocusIdx {
	case sfAlias:
		// Allow only lowercase letters, digits, and hyphens.
		lr := unicode.ToLower(r)
		if lr >= 'a' && lr <= 'z' || lr >= '0' && lr <= '9' || lr == '-' {
			sf.Alias += string(lr)
		}
	case sfHost:
		sf.Host += string(r)
	case sfPort:
		if r >= '0' && r <= '9' {
			sf.PortStr += string(r)
		}
	case sfUser:
		sf.User += string(r)
	case sfIdentityFile:
		sf.IdentityFile += string(r)
	case sfPassword:
		sf.Password += string(r)
	case sfDefaultCWD:
		sf.DefaultCWD += string(r)
	}
}

func (m *uiModel) submitServerForm() {
	sf := m.modal.ServerForm
	sf.ErrorMsg = ""

	// --- validation ---
	alias := strings.TrimSpace(sf.Alias)
	if alias == "" {
		sf.ErrorMsg = "Alias is required"
		sf.ErrorField = sfAlias
		sf.FocusIdx = sfAlias
		return
	}
	if alias == "local" {
		sf.ErrorMsg = "Alias \"local\" is reserved"
		sf.ErrorField = sfAlias
		sf.FocusIdx = sfAlias
		return
	}
	host := strings.TrimSpace(sf.Host)
	if host == "" {
		sf.ErrorMsg = "Host is required"
		sf.ErrorField = sfHost
		sf.FocusIdx = sfHost
		return
	}
	portStr := sf.PortStr
	if portStr == "" {
		portStr = "22"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		sf.ErrorMsg = "Port must be 1–65535"
		sf.ErrorField = sfPort
		sf.FocusIdx = sfPort
		return
	}
	user := strings.TrimSpace(sf.User)
	if user == "" {
		sf.ErrorMsg = "User is required"
		sf.ErrorField = sfUser
		sf.FocusIdx = sfUser
		return
	}
	if sf.AuthMode == serverFormAuthKey && strings.TrimSpace(sf.IdentityFile) == "" {
		sf.ErrorMsg = "Identity file path is required for key auth"
		sf.ErrorField = sfIdentityFile
		sf.FocusIdx = sfIdentityFile
		return
	}
	if sf.AuthMode == serverFormAuthPassword && strings.TrimSpace(sf.Password) == "" {
		sf.ErrorMsg = "Password is required for password auth"
		sf.ErrorField = sfPassword
		sf.FocusIdx = sfPassword
		return
	}

	// --- registry duplicate check (skip when editing and alias unchanged) ---
	if m.app.cfg.Workspace != nil {
		if _, exists := m.app.cfg.Workspace.Registry().Get(alias); exists && alias != sf.EditAlias {
			sf.ErrorMsg = fmt.Sprintf("Server alias %q already exists", alias)
			sf.ErrorField = sfAlias
			sf.FocusIdx = sfAlias
			return
		}
	}

	auth := workspace.ServerAuth{}
	var plainPw string
	switch sf.AuthMode {
	case serverFormAuthAgent:
		auth.UseAgent = true
		auth.AllowPassword = false
	case serverFormAuthKey:
		auth.IdentityFile = strings.TrimSpace(sf.IdentityFile)
	case serverFormAuthPassword:
		auth.AllowPassword = true
		plainPw = sf.Password
	}

	result := commands.ServerFormResult{
		Entry: workspace.ServerEntry{
			Alias:      alias,
			Kind:       workspace.ServerKindSSH,
			Host:       host,
			Port:       port,
			User:       user,
			DefaultCWD: strings.TrimSpace(sf.DefaultCWD),
			Auth:       auth,
		},
		Password:     plainPw,
		SavePassword: sf.SavePassword && sf.AuthMode == serverFormAuthPassword,
	}

	if m.modal.ServerFormC != nil {
		m.modal.ServerFormC <- serverFormResult{Result: result}
	}
	m.closeModal()
}

func (m *uiModel) cancelServerForm(err error) {
	if m.modal.ServerFormC != nil {
		m.modal.ServerFormC <- serverFormResult{Err: err}
	}
	m.closeModal()
}

func dropLast(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	return string(runes[:len(runes)-1])
}
