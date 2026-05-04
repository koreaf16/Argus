package tools

import "strings"

const (
	EvidenceLocalCode     = "local_code"
	EvidenceExternalFresh = "external_fresh"
	EvidenceServerState   = "server_state"
	EvidenceUserDecision  = "user_decision"
	EvidenceMutation      = "mutation"
	EvidencePlanning      = "planning"
)

func ToolEvidenceCategories(name string, readOnly bool) []string {
	canonical := CanonicalName(name)
	if strings.HasPrefix(canonical, "mcp__") {
		return nil
	}
	switch canonical {
	case "ask_user":
		return []string{EvidenceUserDecision}
	case "tool_search":
		return []string{EvidencePlanning}
	case "task_plan_init", "enter_plan_mode", "exit_plan_mode":
		return []string{EvidencePlanning}
	case "web_search", "webfetch", "mcp", "list_mcp_resources", "read_mcp_resource":
		return []string{EvidenceExternalFresh}
	case "grep", "glob", "fileread", "fs_list", "lsp":
		return []string{EvidenceLocalCode}
	case "filewrite":
		return []string{EvidenceLocalCode, EvidenceMutation}
	case "bash", "powershell":
		if readOnly {
			return []string{EvidenceLocalCode, EvidenceServerState}
		}
		return []string{EvidenceLocalCode, EvidenceServerState, EvidenceMutation}
	case "server_inspect", "server_metrics":
		return []string{EvidenceServerState}
	case "server_connect":
		return []string{EvidenceServerState, EvidencePlanning}
	case "server_copy", "server_tunnel", "account_shell":
		return []string{EvidenceServerState, EvidenceMutation}
	case "skill", "snip", "task_create", "task_update":
		return []string{EvidencePlanning, EvidenceMutation}
	}
	if readOnly {
		return nil
	}
	return []string{EvidenceMutation}
}

func ToolHasEvidenceCategory(name string, readOnly bool, category string) bool {
	for _, item := range ToolEvidenceCategories(name, readOnly) {
		if item == category {
			return true
		}
	}
	return false
}
