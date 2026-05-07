// Package permissions defines always-allow tools for auto mode.
package permissions

import "github.com/koreaf16/argus/internal/constants"

// safeAutoModeTools are tool names that can be used in auto mode without
// invoking the classifier.
var safeAutoModeTools = map[string]bool{
	"fileread":           true,
	"grep":               true,
	"glob":               true,
	"lsp":                true,
	"tool_search":        true,
	"list_mcp_resources": true,
	"read_mcp_resource":  true,
	"task_create":        true,
	"task_update":        true,
	"server_inventory":   true,

	// Plan-mode / ask-user UX tools.
	constants.AskUserToolName:             true,
	constants.LegacyAskUserToolName:       true,
	constants.LegacyAskUserAliasName:      true,
	constants.EnterPlanModeToolName:       true,
	constants.ExitPlanModeToolName:        true,
	constants.LegacyEnterPlanModeToolName: true,
	constants.LegacyExitPlanModeToolName:  true,
}

// IsAutoModeAllowlistedTool reports whether a tool is in the auto-mode allowlist.
func IsAutoModeAllowlistedTool(toolName string) bool {
	return safeAutoModeTools[toolName]
}
