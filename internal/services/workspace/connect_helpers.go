package workspace

import (
	"context"
	"fmt"
	"strings"
)

// ConnectAndActivate establishes a session (if needed) and switches active workspace.
func (m *Manager) ConnectAndActivate(alias string) (string, error) {
	resolved := m.ResolveAlias(alias)
	if err := m.Connect(resolved); err != nil {
		return "", err
	}
	if err := m.SetActive(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// ConnectActivateInspect connects, switches active workspace, and collects inspect data.
func (m *Manager) ConnectActivateInspect(ctx context.Context, alias string) (InspectSnapshot, error) {
	resolved, err := m.ConnectAndActivate(alias)
	if err != nil {
		return InspectSnapshot{}, err
	}
	return m.RunInspect(ctx, resolved)
}

// FormatPasswordPrompt builds a consistent credential prompt that includes alias and target account.
func FormatPasswordPrompt(reg *Registry, alias, kind, prompt string) string {
	displayAlias := normalizeAlias(alias)
	if displayAlias == "" {
		displayAlias = LocalAlias
	}

	label := strings.TrimSpace(prompt)
	if label == "" {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			label = "password:"
		} else {
			label = fmt.Sprintf("%s password:", strings.ReplaceAll(kind, "/", " as "))
		}
	}

	target := workspaceTargetLabel(reg, displayAlias)
	if target == "" || target == displayAlias {
		return fmt.Sprintf("[%s] %s", displayAlias, label)
	}
	return fmt.Sprintf("[%s] %s - %s", displayAlias, target, label)
}

func workspaceTargetLabel(reg *Registry, alias string) string {
	if reg == nil {
		return ""
	}
	entry, ok := reg.Get(alias)
	if !ok {
		return ""
	}
	if entry.Kind == ServerKindAccount {
		parent, ok := reg.Get(entry.ParentAlias)
		if !ok {
			return fmt.Sprintf("%s via %s", entry.User, entry.ParentAlias)
		}
		host := strings.TrimSpace(parent.Host)
		port := parent.Port
		if port <= 0 {
			port = 22
		}
		return fmt.Sprintf("%s via %s@%s:%d", entry.User, parent.User, host, port)
	}
	if entry.Kind != ServerKindSSH {
		return LocalAlias
	}

	host := strings.TrimSpace(entry.Host)
	if host == "" {
		return alias
	}
	port := entry.Port
	if port <= 0 {
		port = 22
	}
	user := strings.TrimSpace(entry.User)
	if user == "" {
		return fmt.Sprintf("%s:%d", host, port)
	}
	return fmt.Sprintf("%s@%s:%d", user, host, port)
}

func FormatInspectSummary(snap InspectSnapshot) string {
	var parts []string
	if snap.OS != "" {
		line := snap.OS
		if idx := strings.IndexByte(line, '\n'); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		parts = append(parts, "OS: "+line)
	}
	if snap.User != "" {
		parts = append(parts, "user: "+snap.User)
	}
	if snap.Shell != "" {
		shell := snap.Shell
		if idx := strings.IndexByte(shell, '\n'); idx >= 0 {
			shell = strings.TrimSpace(shell[:idx])
		}
		parts = append(parts, "shell: "+shell)
	}
	if snap.Uptime != "" {
		parts = append(parts, "uptime: "+strings.TrimSpace(snap.Uptime))
	}
	if snap.Services != "" {
		lines := strings.Split(strings.TrimRight(snap.Services, "\n"), "\n")
		parts = append(parts, fmt.Sprintf("services: %d running", len(lines)))
	}
	if snap.Listeners != "" {
		lines := strings.Split(strings.TrimRight(snap.Listeners, "\n"), "\n")
		parts = append(parts, fmt.Sprintf("listeners: %d ports", len(lines)))
	}
	if snap.Docker != "" {
		lines := strings.Split(strings.TrimRight(snap.Docker, "\n"), "\n")
		parts = append(parts, fmt.Sprintf("docker: %d containers", len(lines)))
	}
	if len(parts) == 0 {
		return "inspect: no data collected\n"
	}
	return strings.Join(parts, " / ") + "\n"
}
