package tools

import (
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/state"
)

type ExecutionRole struct {
	Role            string
	Channel         string
	Server          string
	DefaultUser     string
	AsUser          string
	PrivilegeMethod string
	MutationPolicy  string
	AllowedTools    []string
}

func ResolveExecutionRole(ctx Context, requestedServer, requestedRole, requestedChannel, toolName string) (ExecutionRole, error) {
	roleName := strings.TrimSpace(requestedRole)
	channel := strings.ToLower(strings.TrimSpace(requestedChannel))
	profile, hasProfile, err := lookupWorkspaceRoleProfile(ctx, roleName, channel)
	if err != nil {
		return ExecutionRole{}, err
	}

	resolved := ExecutionRole{
		Role:            strings.TrimSpace(profile.Role),
		Channel:         strings.ToLower(strings.TrimSpace(profile.Channel)),
		Server:          strings.TrimSpace(profile.Server),
		DefaultUser:     strings.TrimSpace(profile.DefaultUser),
		AsUser:          strings.TrimSpace(profile.AsUser),
		PrivilegeMethod: strings.ToLower(strings.TrimSpace(profile.PrivilegeMethod)),
		MutationPolicy:  strings.ToLower(strings.TrimSpace(profile.MutationPolicy)),
		AllowedTools:    normalizeAllowedToolNames(profile.AllowedTools),
	}
	if resolved.Role == "" {
		resolved.Role = roleName
	}
	if resolved.Channel == "" {
		resolved.Channel = channel
	}
	if roleName != "" && !hasProfile {
		return ExecutionRole{}, fmt.Errorf("unknown workspace role: %s", roleName)
	}
	if channel != "" && resolved.Channel != "" && channel != resolved.Channel {
		return ExecutionRole{}, fmt.Errorf("workspace role %s is bound to channel %s, not %s", resolved.Role, resolved.Channel, channel)
	}
	if err := validateRoleTool(resolved, toolName); err != nil {
		return ExecutionRole{}, err
	}

	requestedServer = strings.TrimSpace(requestedServer)
	if requestedServer != "" && resolved.Server != "" {
		reqAlias := ResolveWorkspaceAlias(ctx, requestedServer)
		roleAlias := ResolveWorkspaceAlias(ctx, resolved.Server)
		if reqAlias != roleAlias {
			return ExecutionRole{}, fmt.Errorf("workspace role %s is bound to server %s, but tool requested server %s", resolved.Role, roleAlias, reqAlias)
		}
	}
	if requestedServer != "" {
		resolved.Server = requestedServer
	}

	if strings.TrimSpace(resolved.Server) == "" {
		if CanonicalName(toolName) != "server_copy" {
			if err := RequireExplicitServerAlias(ctx, "", toolName); err != nil {
				return ExecutionRole{}, err
			}
		}
		resolved.Server = requestedServer
	}
	if strings.TrimSpace(resolved.Server) != "" {
		alias, err := ResolveValidatedWorkspaceAlias(ctx, resolved.Server)
		if err != nil {
			return ExecutionRole{}, err
		}
		resolved.Server = alias
	}
	if resolved.MutationPolicy == "" && hasProfile {
		resolved.MutationPolicy = inferRoleMutationPolicy(resolved.Role, resolved.Channel)
	}
	return resolved, nil
}

func ResolveExecutionRoleServer(ctx Context, requestedServer, requestedRole, requestedChannel, toolName string) (string, ExecutionRole, error) {
	role, err := ResolveExecutionRole(ctx, requestedServer, requestedRole, requestedChannel, toolName)
	if err != nil {
		return "", ExecutionRole{}, err
	}
	if strings.TrimSpace(role.Server) != "" {
		return role.Server, role, nil
	}
	alias, err := ResolveValidatedWorkspaceAlias(ctx, requestedServer)
	if err != nil {
		return "", ExecutionRole{}, err
	}
	role.Server = alias
	return alias, role, nil
}

func ValidateRoleMutation(role ExecutionRole, toolName, command string, mutates bool) error {
	policy := strings.ToLower(strings.TrimSpace(role.MutationPolicy))
	if policy == "" || policy == "unrestricted" || strings.TrimSpace(role.Role) == "" && strings.TrimSpace(role.Channel) == "" {
		return nil
	}
	canonicalTool := CanonicalName(toolName)
	if policy == "transfer" && canonicalTool != "server_copy" {
		return fmt.Errorf("role %s/channel %s is transfer-only; use server_copy", displayRole(role), role.Channel)
	}
	if policy != "read_only" {
		return nil
	}
	if mutates || commandViolatesReadOnlyRole(command) {
		return fmt.Errorf("role %s/channel %s is read-only; refusing mutating %s operation", displayRole(role), role.Channel, canonicalTool)
	}
	return nil
}

func CredentialTargetUser(asUser, privilegeMethod string) string {
	user := strings.TrimSpace(asUser)
	if user != "" {
		return user
	}
	if method := strings.ToLower(strings.TrimSpace(privilegeMethod)); method == "sudo" || method == "su" {
		return "root"
	}
	return ""
}

func lookupWorkspaceRoleProfile(ctx Context, roleName, channel string) (state.WorkspaceRoleProfile, bool, error) {
	if ctx.State == nil {
		return state.WorkspaceRoleProfile{}, false, nil
	}
	card := ctx.State.WorkflowCard()
	if card == nil {
		return state.WorkspaceRoleProfile{}, false, nil
	}
	if strings.TrimSpace(roleName) != "" {
		if profile, ok := card.WorkspaceRoleProfiles[roleName]; ok {
			return profile, true, nil
		}
		if server, ok := card.WorkspaceRoles[roleName]; ok {
			return state.WorkspaceRoleProfile{
				Role:           roleName,
				Channel:        inferRoleChannel(roleName),
				Server:         server,
				MutationPolicy: inferRoleMutationPolicy(roleName, ""),
			}, true, nil
		}
		return state.WorkspaceRoleProfile{}, false, nil
	}
	if strings.TrimSpace(channel) == "" {
		return state.WorkspaceRoleProfile{}, false, nil
	}

	var matched state.WorkspaceRoleProfile
	matchCount := 0
	for key, profile := range card.WorkspaceRoleProfiles {
		profileChannel := strings.ToLower(strings.TrimSpace(profile.Channel))
		if profileChannel == "" {
			profileChannel = inferRoleChannel(key)
		}
		if profileChannel != channel {
			continue
		}
		if strings.TrimSpace(profile.Role) == "" {
			profile.Role = key
		}
		matched = profile
		matchCount++
	}
	for key, server := range card.WorkspaceRoles {
		if _, exists := card.WorkspaceRoleProfiles[key]; exists {
			continue
		}
		if inferRoleChannel(key) != channel {
			continue
		}
		matched = state.WorkspaceRoleProfile{
			Role:           key,
			Channel:        channel,
			Server:         server,
			MutationPolicy: inferRoleMutationPolicy(key, channel),
		}
		matchCount++
	}
	if matchCount == 0 {
		return state.WorkspaceRoleProfile{}, false, nil
	}
	if matchCount > 1 {
		return state.WorkspaceRoleProfile{}, false, fmt.Errorf("channel %s matches multiple workspace roles; specify `role`", channel)
	}
	return matched, true, nil
}

func validateRoleTool(role ExecutionRole, toolName string) error {
	if len(role.AllowedTools) == 0 {
		return nil
	}
	canonicalTool := CanonicalName(toolName)
	for _, allowed := range role.AllowedTools {
		if CanonicalName(allowed) == canonicalTool {
			return nil
		}
	}
	return fmt.Errorf("tool %s is not allowed for workspace role %s", canonicalTool, displayRole(role))
}

func normalizeAllowedToolNames(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		if name := CanonicalName(item); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func inferRoleChannel(role string) string {
	lower := strings.ToLower(strings.TrimSpace(role))
	switch {
	case strings.Contains(lower, "source"):
		return "source"
	case strings.Contains(lower, "target"):
		return "target"
	case strings.Contains(lower, "transfer"), strings.Contains(lower, "copy"):
		return "transfer"
	case strings.Contains(lower, "verify"), strings.Contains(lower, "check"):
		return "verify"
	default:
		return ""
	}
}

func inferRoleMutationPolicy(role, channel string) string {
	switch firstNonEmptyLower(channel, inferRoleChannel(role)) {
	case "source", "verify":
		return "read_only"
	case "target":
		return "write"
	case "transfer":
		return "transfer"
	default:
		return ""
	}
}

func commandViolatesReadOnlyRole(command string) bool {
	lower := " " + strings.ToLower(strings.Join(strings.Fields(command), " ")) + " "
	if strings.TrimSpace(lower) == "" {
		return false
	}
	blocked := []string{
		" rm ", " mv ", " chmod ", " chown ", " chgrp ", " kill ", " pkill ", " killall ",
		" systemctl ", " service ", " shutdown ", " reboot ", " poweroff ",
		" drop database ", " drop table ", " delete from ", " update ", " insert into ",
		" alter table ", " truncate ", " create database ", " create table ",
	}
	for _, token := range blocked {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func displayRole(role ExecutionRole) string {
	if strings.TrimSpace(role.Role) != "" {
		return strings.TrimSpace(role.Role)
	}
	if strings.TrimSpace(role.Channel) != "" {
		return strings.TrimSpace(role.Channel)
	}
	return "(unnamed)"
}

func firstNonEmptyLower(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	return ""
}

func ApplyRoleDefaultsToExecOptions(execOpts *workspace.ExecOptions, role ExecutionRole) {
	if execOpts == nil {
		return
	}
	if strings.TrimSpace(execOpts.AsUser) == "" {
		execOpts.AsUser = firstNonEmptyLowerPreserve(role.AsUser, role.DefaultUser)
	}
	if strings.TrimSpace(execOpts.PrivilegeMethod) == "" {
		execOpts.PrivilegeMethod = strings.TrimSpace(role.PrivilegeMethod)
	}
	if strings.TrimSpace(execOpts.Role) == "" {
		execOpts.Role = strings.TrimSpace(role.Role)
	}
	if strings.TrimSpace(execOpts.Channel) == "" {
		execOpts.Channel = strings.TrimSpace(role.Channel)
	}
}

func firstNonEmptyLowerPreserve(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
