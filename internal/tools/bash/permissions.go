// Package bash — Bash permission checks and rule matching.
package bash

import (
	"encoding/json"
	"regexp"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils/permissions"
)

var (
	envVarPattern       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	safeEnvVars         = map[string]bool{"NODE_ENV": true, "CI": true, "FORCE_COLOR": true}
	safeWrapperCommands = map[string]bool{"time": true, "timeout": true, "nohup": true, "env": true}
	bareShellPrefixes   = map[string]bool{"sudo": true, "su": true, "doas": true, "pkexec": true, "runuser": true}
)

// CheckBashPermissions checks if a bash command is allowed based on user rules.
func CheckBashPermissions(ctx tool.Context, permCtx types.ToolPermissionContext, input json.RawMessage) (tool.PermissionResult, error) {
	command := tool.ExtractStringInput(input, "command")
	if strings.TrimSpace(command) == "" {
		return tool.PermissionResult{Behavior: types.BehaviorAsk, Message: "empty command requires confirmation"}, nil
	}

	strippedCmd := stripSafeWrappers(command)
	firstWord := strings.Fields(strippedCmd)
	if len(firstWord) > 0 {
		if bareShellPrefixes[firstWord[0]] {
			// DO NOT block sudo/su! The new plan supports interactive PTY sudo prompts.
			// Just pass it to the normal rule matcher.
		}
	}

	// 1. Check tool-specific Deny Rules
	for _, rule := range permissions.GetDenyRules(permCtx) {
		if rule.RuleValue.ToolName == "bash" && rule.RuleValue.RuleContent != "" {
			parsed := permissions.ParsePermissionRule(rule.RuleValue.RuleContent)
			if permissions.MatchesPermissionRule(parsed, strippedCmd, false) {
				return tool.PermissionResult{
					Behavior: types.BehaviorDeny,
					Message:  "Command matches a configured deny rule.",
					DecisionReason: &types.PermissionDecisionReason{
						Type: types.DecisionReasonRule,
						Rule: &rule,
					},
				}, nil
			}
		}
	}

	// 2. Check tool-specific Ask Rules
	for _, rule := range permissions.GetAskRules(permCtx) {
		if rule.RuleValue.ToolName == "bash" && rule.RuleValue.RuleContent != "" {
			parsed := permissions.ParsePermissionRule(rule.RuleValue.RuleContent)
			if permissions.MatchesPermissionRule(parsed, strippedCmd, false) {
				return tool.PermissionResult{
					Behavior: types.BehaviorAsk,
					Message:  "Command matches a configured ask rule.",
					DecisionReason: &types.PermissionDecisionReason{
						Type: types.DecisionReasonRule,
						Rule: &rule,
					},
				}, nil
			}
		}
	}

	// 3. Check tool-specific Allow Rules
	for _, rule := range permissions.GetAllowRules(permCtx) {
		if rule.RuleValue.ToolName == "bash" && rule.RuleValue.RuleContent != "" {
			parsed := permissions.ParsePermissionRule(rule.RuleValue.RuleContent)
			if permissions.MatchesPermissionRule(parsed, strippedCmd, false) {
				return tool.PermissionResult{
					Behavior:       types.BehaviorAllow,
					DecisionReason: &types.PermissionDecisionReason{Type: types.DecisionReasonRule, Rule: &rule},
				}, nil
			}
		}
	}

	// 4. Default: fallback to shared shell policy (mode-aware allow/ask/deny)
	// Suggest prefix for future cache
	suggestedPrefix := getSimpleCommandPrefix(strippedCmd)
	var suggestions []types.PermissionUpdate
	if suggestedPrefix != "" && !bareShellPrefixes[suggestedPrefix] {
		suggestions = permissions.SuggestionForPrefix("bash", suggestedPrefix)
	}

	result := tool.EvaluateShellCommandPermission(ctx, strippedCmd, "bash")
	if len(suggestions) > 0 {
		result.Suggestions = append(result.Suggestions, suggestions...)
	}
	return result, nil
}

// stripSafeWrappers removes leading env vars and safe wrapper commands like "time".
func stripSafeWrappers(cmd string) string {
	trimmed := strings.TrimSpace(cmd)
	fields := strings.Fields(trimmed)

	var result []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]

		// Remove safe env vars
		if envVarPattern.MatchString(f) {
			idx := strings.Index(f, "=")
			if idx > 0 && safeEnvVars[f[:idx]] {
				continue
			}
			// Unsafe env var stops stripping
			result = append(result, fields[i:]...)
			break
		}

		// Remove safe wrappers
		if safeWrapperCommands[f] {
			continue
		}

		// Not a safe wrapper or env var, stop stripping
		result = append(result, fields[i:]...)
		break
	}

	if len(result) == 0 {
		return trimmed
	}
	return strings.Join(result, " ")
}

func getSimpleCommandPrefix(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}

	first := fields[0]
	// Subcommand-based CLIs
	subcommandCLIs := map[string]bool{
		"git": true, "npm": true, "yarn": true, "pnpm": true, "cargo": true,
		"go": true, "docker": true, "kubectl": true, "apt": true, "apt-get": true,
		"brew": true, "python": true, "python3": true, "pip": true, "pip3": true,
		"sudo": true, "su": true,
	}

	if subcommandCLIs[first] && len(fields) >= 2 {
		return first + " " + fields[1]
	}

	return first
}
