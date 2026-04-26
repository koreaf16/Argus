package tools

import (
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

type shellSegmentInfo struct {
	Operator   string
	Raw        string
	Normalized string
	Tokens     []string
}

func shellExecSucceeded(command string, result executor.ExecResult) (bool, string) {
	if result.ExitCode == 0 {
		return true, ""
	}
	if ok, note := allowExpectedServiceVerificationSuccess(command, result); ok {
		return true, note
	}
	return false, ""
}

func allowExpectedServiceVerificationSuccess(command string, result executor.ExecResult) (bool, string) {
	segments := parseShellSegmentInfo(command)
	if len(segments) < 2 {
		return false, ""
	}

	last := segments[len(segments)-1]
	if last.Operator != "&&" {
		return false, ""
	}

	action := serviceActionFromTokens(last.Tokens)
	subject := serviceSubjectFromTokens(last.Tokens)
	if action == "" || subject == "" {
		return false, ""
	}

	switch action {
	case "status", "is-active":
		if !hasPriorServiceAction(segments[:len(segments)-1], subject, "stop", "disable") {
			return false, ""
		}
		if !resultShowsInactiveService(result) {
			return false, ""
		}
		return true, fmt.Sprintf(
			"[Verification Note] final service check returned exit %d because %s is inactive, which matches the requested stop/disable state.",
			result.ExitCode,
			subject,
		)
	case "is-enabled":
		if !hasPriorServiceAction(segments[:len(segments)-1], subject, "disable", "mask") {
			return false, ""
		}
		if !resultShowsDisabledService(result) {
			return false, ""
		}
		return true, fmt.Sprintf(
			"[Verification Note] final enablement check returned exit %d because %s is disabled, which matches the requested state.",
			result.ExitCode,
			subject,
		)
	default:
		return false, ""
	}
}

func parseShellSegmentInfo(command string) []shellSegmentInfo {
	rawSegments := splitTopLevelCommandSegments(command)
	out := make([]shellSegmentInfo, 0, len(rawSegments))
	for _, seg := range rawSegments {
		normalized := normalizeShellSegment(seg.Raw)
		out = append(out, shellSegmentInfo{
			Operator:   shellSegmentOperator(command, seg),
			Raw:        strings.TrimSpace(seg.Raw),
			Normalized: normalized,
			Tokens:     shellTokens(normalized),
		})
	}
	return out
}

func normalizeShellSegment(segment string) string {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return ""
	}
	if spec, hasInline, err := parseLeadingPrivilegeWrapper(segment); err == nil && hasInline {
		return strings.TrimSpace(spec.Payload)
	}
	return segment
}

func shellSegmentOperator(command string, seg commandSegment) string {
	i := seg.Start - 1
	for i >= 0 && isShellWhitespace(command[i]) {
		i--
	}
	if i < 0 {
		return ""
	}

	switch command[i] {
	case ';':
		return ";"
	case '|':
		if i-1 >= 0 && command[i-1] == '|' {
			return "||"
		}
		return "|"
	case '&':
		if i-1 >= 0 && command[i-1] == '&' {
			return "&&"
		}
		return "&"
	default:
		return ""
	}
}

func hasPriorServiceAction(segments []shellSegmentInfo, subject string, actions ...string) bool {
	want := strings.ToLower(strings.TrimSpace(subject))
	for _, seg := range segments {
		if !strings.EqualFold(serviceSubjectFromTokens(seg.Tokens), want) {
			continue
		}
		action := serviceActionFromTokens(seg.Tokens)
		for _, candidate := range actions {
			if strings.EqualFold(action, candidate) {
				return true
			}
		}
	}
	return false
}

func resultShowsInactiveService(result executor.ExecResult) bool {
	output := strings.ToLower(strings.TrimSpace(result.Stdout + "\n" + result.Stderr))
	if output == "" {
		return false
	}
	return strings.Contains(output, "active: inactive") ||
		strings.Contains(output, "inactive (dead)") ||
		strings.HasSuffix(output, "inactive") ||
		strings.Contains(output, "\ninactive\n")
}

func resultShowsDisabledService(result executor.ExecResult) bool {
	output := strings.ToLower(strings.TrimSpace(result.Stdout + "\n" + result.Stderr))
	if output == "" {
		return false
	}
	return strings.Contains(output, "\ndisabled\n") ||
		strings.HasSuffix(output, "disabled") ||
		strings.Contains(output, "disabled;")
}
