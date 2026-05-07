package toolui

import (
	"encoding/json"
	"strings"
)

type ShellGuardInfo struct {
	Decision      string
	Risk          string
	ChannelAction string
	ChannelKey    string
	Reason        string
	ExecLabel     string
	// Shell is set only when the effective shell differs from the tool name
	// (e.g. bash tool on Windows local runs under powershell). Empty otherwise.
	Shell string
}

func ParseShellGuard(args map[string]any) (ShellGuardInfo, bool) {
	raw, ok := args["_shell_guard"]
	if !ok || raw == nil {
		return ShellGuardInfo{}, false
	}

	var data map[string]any
	switch v := raw.(type) {
	case map[string]any:
		data = v
	case string:
		_ = json.Unmarshal([]byte(v), &data)
	default:
		b, _ := json.Marshal(v)
		_ = json.Unmarshal(b, &data)
	}
	if len(data) == 0 {
		return ShellGuardInfo{}, false
	}

	info := ShellGuardInfo{
		Decision:      shellGuardString(data, "decision"),
		Risk:          shellGuardString(data, "risk"),
		ChannelAction: shellGuardString(data, "channel_action"),
		ChannelKey:    shellGuardString(data, "channel_key"),
		Reason:        shellGuardString(data, "reason"),
		ExecLabel:     shellGuardString(data, "exec_label"),
		Shell:         shellGuardString(data, "shell"),
	}
	return info, info.Summary() != "" || info.ExecLabel != ""
}

func (g ShellGuardInfo) Summary() string {
	parts := []string{}
	if g.Decision != "" {
		parts = append(parts, g.Decision)
	}
	if g.Risk != "" {
		parts = append(parts, g.Risk)
	}
	if g.BlocksExecution() {
		if g.Reason != "" {
			parts = append(parts, g.Reason)
		}
		return strings.Join(parts, " - ")
	}
	if channelPart := strings.TrimSpace(g.ChannelAction + " " + g.ChannelKey); channelPart != "" {
		parts = append(parts, channelPart)
	}
	return strings.Join(parts, " - ")
}

func (g ShellGuardInfo) BlocksExecution() bool {
	switch strings.ToLower(strings.TrimSpace(g.Decision)) {
	case "denied", "deny", "blocked", "ask":
		return true
	default:
		return false
	}
}

func (g ShellGuardInfo) IsDenied() bool {
	switch strings.ToLower(strings.TrimSpace(g.Decision)) {
	case "denied", "deny", "blocked":
		return true
	default:
		return false
	}
}

func FormatShellGuardPrelude(info ShellGuardInfo, command, fallbackExecLabel string, theme ThemeContext) string {
	summary := info.Summary()
	if summary == "" {
		return ""
	}

	lines := []string{"guard: " + summary}
	if !info.BlocksExecution() {
		execLabel := strings.TrimSpace(info.ExecLabel)
		if execLabel == "" {
			execLabel = strings.TrimSpace(fallbackExecLabel)
		}
		if execLabel == "" {
			execLabel = "local"
		}
		if shell := strings.TrimSpace(info.Shell); shell != "" {
			execLabel = execLabel + " (" + shell + ")"
		}
		command = collapseShellGuardCommand(command)
		if command == "" {
			lines = append(lines, "exec: "+execLabel)
		} else {
			lines = append(lines, "exec: "+execLabel+" - "+command)
		}
	}

	return FormatResultLines(lines, false, info.IsDenied(), theme)
}

func shellGuardString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func collapseShellGuardCommand(command string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(command, "\n", " ")), " ")
}
