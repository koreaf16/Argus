package servertunnel

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

func init() {
	toolui.Register("server_tunnel", &ServerTunnelRenderer{})
}

type ServerTunnelRenderer struct{}

func (r *ServerTunnelRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	action, _ := args["action"].(string)
	server, _ := args["server"].(string)
	action = strings.TrimSpace(action)
	server = strings.TrimSpace(server)

	var label string
	switch strings.ToLower(action) {
	case "open":
		localAddr, _ := args["local_addr"].(string)
		remoteAddr, _ := args["remote_addr"].(string)
		localAddr = strings.TrimSpace(localAddr)
		remoteAddr = strings.TrimSpace(remoteAddr)
		if localAddr != "" && remoteAddr != "" {
			label = fmt.Sprintf("open  %s → %s", localAddr, remoteAddr)
		} else if server != "" {
			label = "open  " + server
		} else {
			label = "open"
		}
	case "close":
		tunnelID, _ := args["tunnel_id"].(string)
		if id := strings.TrimSpace(tunnelID); id != "" {
			label = "close  " + id
		} else {
			label = "close"
		}
	case "list":
		if server != "" {
			label = "list  " + server
		} else {
			label = "list"
		}
	default:
		if action != "" {
			label = action
		}
	}
	return toolui.FormatToolCall("ServerTunnel", label, 120, theme)
}

func (r *ServerTunnelRenderer) RenderToolResult(resultText string, _ int64, theme toolui.ThemeContext) string {
	text := strings.TrimSpace(resultText)
	if text == "" {
		return ""
	}

	// close 결과: "closed tunnel <id>" 평문
	if strings.HasPrefix(strings.ToLower(text), "closed tunnel") {
		successStyle := theme.Style(theme.StatusSuccessColor())
		line := successStyle.Render("✔ " + text)
		return toolui.FormatResultLines([]string{line}, true, false, theme)
	}

	// open 결과: JSON 객체 → workspace.TunnelInfo
	if strings.HasPrefix(text, "{") {
		var info workspace.TunnelInfo
		if err := json.Unmarshal([]byte(text), &info); err == nil {
			return renderTunnelOpen(info, theme)
		}
	}

	// list 결과: JSON 배열 → []workspace.TunnelInfo
	if strings.HasPrefix(text, "[") {
		var items []workspace.TunnelInfo
		if err := json.Unmarshal([]byte(text), &items); err == nil {
			return renderTunnelList(items, theme)
		}
	}

	// 폴백: 평문
	lines := splitNonEmpty(text)
	if len(lines) == 0 {
		return ""
	}
	return toolui.FormatResultLines(lines, true, false, theme)
}

func (r *ServerTunnelRenderer) CreateInteractiveModel(_ map[string]any, _ toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func renderTunnelOpen(info workspace.TunnelInfo, theme toolui.ThemeContext) string {
	successStyle := theme.Style(theme.StatusSuccessColor())
	mutedStyle := theme.Style(theme.MutedColor())

	lines := []string{
		successStyle.Render("✔ Tunnel opened"),
		fmt.Sprintf("%s  →  %s  %s",
			info.LocalAddr,
			info.RemoteAddr,
			mutedStyle.Render("(ID: "+info.ID+")"),
		),
	}
	return toolui.FormatResultLines(lines, true, false, theme)
}

func renderTunnelList(items []workspace.TunnelInfo, theme toolui.ThemeContext) string {
	mutedStyle := theme.Style(theme.MutedColor())

	if len(items) == 0 {
		return toolui.FormatStatusLine("no active tunnels", theme)
	}

	noun := "tunnels"
	if len(items) == 1 {
		noun = "tunnel"
	}
	lines := []string{fmt.Sprintf("%d %s active", len(items), noun)}
	for _, item := range items {
		line := fmt.Sprintf("·  %s → %s  %s",
			item.LocalAddr,
			item.RemoteAddr,
			mutedStyle.Render("("+item.ID+")"),
		)
		lines = append(lines, line)
	}
	return toolui.FormatResultLines(lines, true, false, theme)
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if l := strings.TrimSpace(line); l != "" {
			out = append(out, l)
		}
	}
	return out
}
