// Package tui
// File: cmdbox_labels.go
// Description: 도구 표시명, 인자 요약, shimmer 레이블 등 cmdBox 레이블링 함수 모음
// Responsibility: toolDisplayName / toolDisplayArg / toolShimmerLabel 등 텍스트 생성 로직

package tui

import (
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

func toolDisplayName(toolName string) string {
	names := map[string]string{
		"shell_exec":       "Shell",
		"file_read":        "Read",
		"file_write":       "Write",
		"server_add":       "Workspace Add",
		"server_remove":    "Workspace Remove",
		"server_list":      "Workspaces",
		"server_focus":     "Workspace Focus",
		"workspace_add":    "Workspace Add",
		"workspace_remove": "Workspace Remove",
		"workspace_list":   "Workspaces",
		"workspace_focus":  "Workspace Focus",
		"process_list":     "Processes",
		"network_info":     "Network",
		"disk_usage":       "Disk Usage",
		"service_status":   "Services",
		"system_info":      "System Info",
		"k8s_query":        "Kubernetes",
		"web_fetch":        "Web Fetch",
		"web_search":       "Search",
		"knowledge_search": "Knowledge",
		"knowledge_add":    "Learn",
	}
	if n, ok := names[toolName]; ok {
		return n
	}
	return toolName
}

func toolTargetLabel(toolName, target string) string {
	if toolName != "shell_exec" {
		if target != "" && target != "localhost" {
			return "workspace -> " + target
		}
		return ""
	}

	if target == "" || target == "localhost" {
		return "local / " + executor.LocalShellName()
	}

	return "workspace -> " + target
}

func toolDisplayArg(toolName string, args map[string]any) string {
	switch toolName {
	case "shell_exec":
		if label := shellTaskLabel(toolName, args); label != "" {
			return truncateStr(label, 70)
		}
	case "file_read", "file_write":
		if p, ok := args["path"].(string); ok {
			return p
		}
	case "server_add", "server_remove", "workspace_add", "workspace_remove":
		if n, ok := args["name"].(string); ok {
			return n
		}
	case "web_fetch":
		if u, ok := args["url"].(string); ok && strings.TrimSpace(u) != "" {
			return truncateStr(u, 50)
		}
	case "k8s_query":
		action, _ := args["action"].(string)
		if strings.TrimSpace(action) == "" {
			action = "get"
		}
		resources := make([]string, 0, 4)
		if resource, ok := args["resource"].(string); ok && strings.TrimSpace(resource) != "" {
			resources = append(resources, strings.TrimSpace(resource))
		}
		resources = append(resources, argStringList(args, "resources")...)
		resources = dedupeStrings(resources)
		if len(resources) > 0 {
			if len(resources) == 1 {
				return fmt.Sprintf("%s %s", action, truncateStr(resources[0], 40))
			}
			return fmt.Sprintf("%s %s (+%d)", action, truncateStr(resources[0], 28), len(resources)-1)
		}
		return action
	case "web_search":
		if q, ok := args["query"].(string); ok {
			return `"` + truncateStr(q, 40) + `"`
		}
	case "knowledge_search":
		if q, ok := args["query"].(string); ok {
			return truncateStr(q, 40)
		}
	}
	return ""
}

func argStringList(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	out := []string{}
	add := func(raw string) {
		for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t'
		}) {
			s := strings.TrimSpace(token)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
	}
	switch vv := v.(type) {
	case string:
		add(vv)
	case []string:
		for _, item := range vv {
			add(item)
		}
	case []interface{}:
		for _, item := range vv {
			if s, ok := item.(string); ok {
				add(s)
			}
		}
	}
	return out
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, strings.TrimSpace(item))
	}
	return out
}

// toolShimmerLabel은 도구 실행 중 shimmer에 표시할 레이블을 생성한다.
func toolShimmerLabel(name string, args map[string]any) string {
	displayName := toolDisplayName(name)
	switch name {
	case "web_fetch":
		if u, ok := args["url"].(string); ok && u != "" {
			if domain := extractURLDomain(u); domain != "" {
				return displayName + " " + domain
			}
		}
	case "web_search":
		if q, ok := args["query"].(string); ok && q != "" {
			return displayName + ` "` + truncateStr(q, 30) + `"`
		}
	case "shell_exec":
		if label := shellTaskLabel(name, args); label != "" {
			return displayName + " " + truncateStr(label, 40)
		}
	case "file_read", "file_write":
		if p, ok := args["path"].(string); ok && p != "" {
			return displayName + " " + truncateStr(p, 40)
		}
	}
	return displayName + "..."
}

// extractURLDomain은 URL에서 호스트명(www. 제거)을 추출한다.
func extractURLDomain(rawURL string) string {
	s := rawURL
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}
	if idx := strings.IndexByte(s, '/'); idx >= 0 {
		s = s[:idx]
	}
	if idx := strings.IndexByte(s, '?'); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimPrefix(s, "www.")
}

func previewCmd(toolName string, args map[string]interface{}) string {
	switch toolName {
	case "shell_exec":
		if cmd, ok := args["command"].(string); ok {
			runes := []rune(cmd)
			if len(runes) > 60 {
				return string(runes[:57]) + "..."
			}
			return cmd
		}
	case "file_read", "file_write":
		if p, ok := args["path"].(string); ok {
			return p
		}
	}
	return "running..."
}
