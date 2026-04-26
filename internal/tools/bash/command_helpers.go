// Package bash — Bash 명령 권한 검사 헬퍼.
//
// 파일 역할: Checks permission for bash commands with special operators (pipes, redirections).
//
//	Handles pipe segmentation, compound commands, and cross-segment security checks.
//
// 포함 모듈:
//   - CheckCommandOperatorPermissions: Checks pipes, redirections, compound structures
//   - segmentedCommandPermissionResult: Checks each pipe segment independently
//   - buildSegmentWithoutRedirections: Strips output redirections while preserving quotes
//
// 호출/사용 방식:
//   - bash.go calls during permission checking
//
// 연결:
//   - internal/utils/bash: commands, ParsedCommand
//   - internal/utils/permissions: permission checking
//   - 원본: src/tools/BashTool/bashCommandHelpers.ts
package bash

import (
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils/bash"
)

type CommandIdentityCheckers struct {
	IsNormalizedCdCommand  func(string) bool
	IsNormalizedGitCommand func(string) bool
}

// segmentedCommandPermissionResult checks permission for piped commands.
// Validates multiple cd commands and cross-segment cd+git patterns.
func segmentedCommandPermissionResult(
	cmd string,
	segments []string,
	checkers CommandIdentityCheckers,
) types.PermissionResult {
	// Check for multiple cd commands across all segments
	cdCount := 0
	for _, segment := range segments {
		trimmed := segment
		if checkers.IsNormalizedCdCommand(trimmed) {
			cdCount++
		}
	}
	if cdCount > 1 {
		return types.PermissionResult{
			Behavior: types.BehaviorAsk,
			Message:  "Multiple directory changes in one command require approval for clarity",
		}
	}

	// SECURITY: Check for cd+git across pipe segments to prevent bare repo fsmonitor bypass.
	var hasCd, hasGit bool
	for _, segment := range segments {
		// For now, use simple split (TODO: integrate with bash parser)
		subcommands := splitCommandSimple(segment)
		for _, sub := range subcommands {
			if checkers.IsNormalizedCdCommand(sub) {
				hasCd = true
			}
			if checkers.IsNormalizedGitCommand(sub) {
				hasGit = true
			}
		}
	}
	if hasCd && hasGit {
		return types.PermissionResult{
			Behavior: types.BehaviorAsk,
			Message:  "Compound commands with cd and git require approval to prevent bare repository attacks",
		}
	}

	// All segments passed; return allow
	return types.PermissionResult{
		Behavior: types.BehaviorAllow,
		Message:  "All pipe segments approved",
	}
}

// buildSegmentWithoutRedirections strips output redirections from a segment.
func buildSegmentWithoutRedirections(segment string) string {
	// Fast path: no redirections
	if !containsStr(segment, ">") {
		return segment
	}

	// Use ParsedCommand to strip redirections
	parsed := bash.NewRegexParsedCommand(segment)
	return parsed.WithoutOutputRedirections()
}

// CheckCommandOperatorPermissions checks if command uses pipes, redirections, or compound structures.
func CheckCommandOperatorPermissions(
	cmd string,
	checkers CommandIdentityCheckers,
) types.PermissionResult {
	parsed := bash.NewRegexParsedCommand(cmd)
	segments := parsed.GetPipeSegments()

	// Single segment: no special operators
	if len(segments) <= 1 {
		return types.PermissionResult{
			Behavior: types.BehaviorPassthrough,
			Message:  "No pipes found in command",
		}
	}

	// Strip redirections from each segment
	cleanedSegments := make([]string, len(segments))
	for i, seg := range segments {
		cleanedSegments[i] = buildSegmentWithoutRedirections(seg)
	}

	// Check segmented command
	return segmentedCommandPermissionResult(cmd, cleanedSegments, checkers)
}

// Helper: simple command splitter (for deprecated paths)
func splitCommandSimple(cmd string) []string {
	// Phase 1: simple && and ; split
	// TODO: integrate with full bash parser
	return []string{cmd}
}

func containsStr(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
