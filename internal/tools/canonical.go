package tools

import "strings"

var canonicalToolNameAliases = map[string]string{
	"web_fetch":         "webfetch",
	"websearch":         "web_search",
	"file_read":         "fileread",
	"read":              "fileread",
	"file_write":        "filewrite",
	"mcp_tool":          "mcp",
	"askuser":           "ask_user",
	"ask_user_tool":     "ask_user",
	"ask_user_batch":    "ask_user",
	"toolsearch":        "tool_search",
	"taskcreate":        "task_create",
	"taskupdate":        "task_update",
	"enterworktreetool": "enter_worktree",
	"exitworktreetool":  "exit_worktree",
}

// CanonicalName returns the stable lookup key used across registry, workflow,
// web evidence, and UI routing.
func CanonicalName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if alias, ok := canonicalToolNameAliases[n]; ok {
		return alias
	}
	return n
}
