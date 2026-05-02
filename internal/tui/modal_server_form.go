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
// visible given the selected auth mode and elevation setting.
// Hidden fields are skipped during Tab navigation.
func activeServerFormFields(sf *serverFormState) []serverFormField {
	base := []serverFormField{sfAlias, sfHost, sfPort, sfUser, sfAuthMode}
	switch sf.AuthMode {
	case serverFormAuthKey:
		base = append(base, sfIdentityFile)
	case serverFormAuthPassword:
		base = append(base, sfPassword, sfSavePassword)
	}
	base = append(base, sfDefaultCWD, sfAllowElevation)

	if sf.AllowElevation {
		base = append(base, sfElevationMethod)
		if sf.ElevationMethod == "su" {
			base = append(base, sfRootPassword)
		}

		for i := range sf.Targets {
			base = append(base, serverFormField(int(sfTargetBase)+i*3)) // User
			if sf.ElevationMethod == "su" {
				base = append(base, serverFormField(int(sfTargetBase)+i*3+1)) // Password
			}
			base = append(base, serverFormField(int(sfTargetBase)+i*3+2)) // Remove
		}
		// Add target button
		base = append(base, serverFormField(int(sfTargetBase)+len(sf.Targets)*3))
	}
	base = append(base, sfSubmit)
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

	case "left", "right":
		if sf.FocusIdx == sfElevationMethod && sf.AllowElevation {
			if sf.ElevationMethod == "su" {
				sf.ElevationMethod = "sudo"
			} else {
				sf.ElevationMethod = "su"
			}
			sf.FocusIdx = sfElevationMethod
		}

	case " ":
		switch sf.FocusIdx {
		case sfAuthMode:
			sf.AuthMode = (sf.AuthMode + 1) % 3
			sf.IdentityFile = ""
			sf.Password = ""
			sf.SavePassword = false
			sf.FocusIdx = sfAuthMode
		case sfSavePassword:
			sf.SavePassword = !sf.SavePassword
		case sfAllowElevation:
			sf.AllowElevation = !sf.AllowElevation
			if !sf.AllowElevation {
				sf.FocusIdx = sfAllowElevation
			}
			if sf.AllowElevation && sf.ElevationMethod == "" {
				sf.ElevationMethod = "sudo"
			}
		case sfElevationMethod:
			if sf.AllowElevation {
				if sf.ElevationMethod == "su" {
					sf.ElevationMethod = "sudo"
				} else {
					sf.ElevationMethod = "su"
				}
				sf.FocusIdx = sfElevationMethod
			}
		default:
			if sf.AllowElevation && sf.FocusIdx >= sfTargetBase {
				if sf.FocusIdx == serverFormField(int(sfTargetBase)+len(sf.Targets)*3) {
					// Add Target Button
					sf.Targets = append(sf.Targets, targetUserState{})
					sf.FocusIdx = serverFormField(int(sfTargetBase) + (len(sf.Targets)-1)*3) // Focus new user field
				} else {
					idx := int(sf.FocusIdx - sfTargetBase)
					targetIdx := idx / 3
					fieldOffset := idx % 3

					// Remove Button
					if fieldOffset == 2 && targetIdx < len(sf.Targets) {
						sf.Targets = append(sf.Targets[:targetIdx], sf.Targets[targetIdx+1:]...)

						// Focus adjustment after removal
						if len(sf.Targets) > 0 {
							if targetIdx > 0 {
								sf.FocusIdx = serverFormField(int(sfTargetBase) + (targetIdx-1)*3)
							} else {
								sf.FocusIdx = serverFormField(int(sfTargetBase))
							}
						} else {
							sf.FocusIdx = serverFormField(int(sfTargetBase)) // This will be the Add button now
						}
					}
				}
			}
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
	case sfRootPassword:
		sf.RootPassword = dropLast(sf.RootPassword)
	default:
		if sf.FocusIdx >= sfTargetBase && sf.AllowElevation {
			idx := int(sf.FocusIdx - sfTargetBase)
			targetIdx := idx / 3
			fieldOffset := idx % 3

			if targetIdx < len(sf.Targets) {
				if fieldOffset == 0 { // User
					sf.Targets[targetIdx].User = dropLast(sf.Targets[targetIdx].User)
				} else if fieldOffset == 1 { // Password
					sf.Targets[targetIdx].Password = dropLast(sf.Targets[targetIdx].Password)
				}
			}
		}
	}
}

func (m *uiModel) serverFormAppend(sf *serverFormState, r rune) {
	switch sf.FocusIdx {
	case sfAlias:
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
	case sfRootPassword:
		sf.RootPassword += string(r)
	default:
		if sf.FocusIdx >= sfTargetBase && sf.AllowElevation {
			idx := int(sf.FocusIdx - sfTargetBase)
			targetIdx := idx / 3
			fieldOffset := idx % 3

			if targetIdx < len(sf.Targets) {
				if fieldOffset == 0 { // User
					sf.Targets[targetIdx].User += string(r)
				} else if fieldOffset == 1 { // Password
					sf.Targets[targetIdx].Password += string(r)
				}
			}
		}
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
	if sf.AuthMode == serverFormAuthPassword && strings.TrimSpace(sf.Password) == "" && !sf.PasswordRegistered {
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

	// Build elevation policy from form state.
	elev := workspace.Elevation{}
	var elevPw string
	targetPwMap := make(map[string]string)
	workAccounts := make([]workspace.ServerEntry, 0, len(sf.Targets))

	if sf.AllowElevation {
		elev.Allowed = true

		// Map Method back to the stored Mode ("password", "reuse_login")
		if sf.ElevationMethod == "sudo" {
			elev.Mode = "reuse_login"
		} else {
			elev.Mode = "password"
		}

		elev.TargetUsers = append(elev.TargetUsers, "root")
		if sf.ElevationMethod == "su" {
			if sf.RootPassword != "" {
				targetPwMap["root"] = sf.RootPassword
			}
			// Using the root password as the primary elevPw for legacy fallback/compatibility
			elevPw = sf.RootPassword
		}

		for i, t := range sf.Targets {
			u := strings.TrimSpace(t.User)
			if u == "" {
				sf.ErrorMsg = "Work account cannot be empty"
				sf.ErrorField = serverFormField(int(sfTargetBase) + i*3)
				sf.FocusIdx = sf.ErrorField
				return
			}
			// Avoid duplicate targets
			duplicate := false
			for _, existing := range elev.TargetUsers {
				if existing == u {
					duplicate = true
					break
				}
			}
			if !duplicate {
				elev.TargetUsers = append(elev.TargetUsers, u)
			}
			workAccounts = append(workAccounts, workspace.ServerEntry{
				Alias:        serverFormAccountAlias(alias, u),
				Kind:         workspace.ServerKindAccount,
				ParentAlias:  alias,
				User:         u,
				SwitchMethod: sf.ElevationMethod,
			})

			if sf.ElevationMethod == "su" && t.Password != "" {
				targetPwMap[u] = t.Password
				if elevPw == "" {
					// Grab the first available target password as legacy fallback if root wasn't set
					elevPw = t.Password
				}
			}
		}
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
			Elevation:  elev,
		},
		WorkAccounts:      workAccounts,
		Password:          plainPw,
		SavePassword:      sf.SavePassword && sf.AuthMode == serverFormAuthPassword,
		ElevationPassword: elevPw,
		TargetPasswords:   targetPwMap, // New field to map user -> password
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

func serverFormAccountAlias(parentAlias, user string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(user)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		} else if r == '_' || r == '.' {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return strings.ToLower(strings.TrimSpace(parentAlias)) + "-account"
	}
	return strings.ToLower(strings.TrimSpace(parentAlias)) + "-" + b.String()
}
