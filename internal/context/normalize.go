package context

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	ansiCSIRegex      = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	ansiOSCRegex      = regexp.MustCompile(`\x1b\][^\a]*(?:\a|\x1b\\)`)
	ansiSingleRegex   = regexp.MustCompile(`\x1b[@-Z\\-_]`)
	promptBracketExpr = regexp.MustCompile(`^\[[^\]]+@[^\]]+\]\s*[#$]\s*$`)
	promptBasicExpr   = regexp.MustCompile(`^[\w.\-]+@[\w.\-]+(?::[^\n]*)?[#$]\s*$`)
	promptShortExpr   = regexp.MustCompile(`^[#$]\s*$`)
)

func NormalizeToolResultForContext(toolName, rawOutput string) string {
	trimmed := strings.TrimSpace(rawOutput)
	if trimmed == "" {
		return "(no output)"
	}

	if isShellTool(toolName) {
		if normalized, ok := normalizeShellJSON(trimmed); ok {
			return normalized
		}
		return normalizeShellText(trimmed)
	}
	return normalizeGenericText(trimmed)
}

func isShellTool(toolName string) bool {
	name := strings.ToLower(strings.TrimSpace(toolName))
	return name == "bash" || name == "powershell"
}

func normalizeShellJSON(input string) (string, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return "", false
	}

	stdout, hasStdout := lookupStringCI(payload, "stdout")
	stderr, hasStderr := lookupStringCI(payload, "stderr")
	code, hasCode := lookupIntCI(payload, "code")
	interrupted, _ := lookupBoolCI(payload, "interrupted")
	preSpawn, _ := lookupStringCI(payload, "prespawnerror", "pre_spawn_error")

	if !hasStdout && !hasStderr && !hasCode && strings.TrimSpace(preSpawn) == "" {
		return "", false
	}

	stdout = normalizeShellText(stdout)
	stderr = normalizeShellText(stderr)
	preSpawn = normalizeShellText(preSpawn)

	parts := make([]string, 0, 5)
	if hasCode {
		parts = append(parts, fmt.Sprintf("exit_code: %d", code))
	}
	if interrupted {
		parts = append(parts, "interrupted: true")
	}
	if preSpawn != "" {
		parts = append(parts, "pre_spawn_error:\n"+preSpawn)
	}
	if stdout != "" {
		parts = append(parts, "stdout:\n"+stdout)
	}
	if stderr != "" {
		parts = append(parts, "stderr:\n"+stderr)
	}
	if len(parts) == 0 {
		return "(no output)", true
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n")), true
}

func normalizeShellText(input string) string {
	s := normalizeGenericText(input)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			out = append(out, "")
			continue
		}
		if strings.Contains(trim, "__ARG_B_") || strings.Contains(trim, "__ARG_M_") || strings.Contains(trim, "__ARG_E_") {
			continue
		}
		if promptBracketExpr.MatchString(trim) || promptBasicExpr.MatchString(trim) || promptShortExpr.MatchString(trim) {
			continue
		}
		out = append(out, line)
	}
	return collapseBlankLines(strings.TrimSpace(strings.Join(out, "\n")), 2)
}

func normalizeGenericText(input string) string {
	// Normalize line endings
	s := strings.ReplaceAll(input, "\r\n", "\n")

	// Handle carriage returns by overwriting the current line
	var processed strings.Builder
	var currentLine strings.Builder

	for _, r := range s {
		if r == '\r' {
			currentLine.Reset()
			continue
		}
		if r == '\n' {
			processed.WriteString(currentLine.String())
			processed.WriteRune('\n')
			currentLine.Reset()
			continue
		}
		currentLine.WriteRune(r)
	}
	processed.WriteString(currentLine.String())
	s = processed.String()

	s = ansiOSCRegex.ReplaceAllString(s, "")
	s = ansiCSIRegex.ReplaceAllString(s, "")
	s = ansiSingleRegex.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return collapseBlankLines(strings.TrimSpace(b.String()), 2)
}

func collapseBlankLines(s string, maxBlankRun int) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blankRun := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankRun++
			if blankRun > maxBlankRun {
				continue
			}
		} else {
			blankRun = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func lookupStringCI(m map[string]any, keys ...string) (string, bool) {
	v, ok := lookupCI(m, keys...)
	if !ok {
		return "", false
	}
	if s, ok := v.(string); ok {
		return s, true
	}
	return fmt.Sprintf("%v", v), true
}

func lookupBoolCI(m map[string]any, keys ...string) (bool, bool) {
	v, ok := lookupCI(m, keys...)
	if !ok {
		return false, false
	}
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		lower := strings.ToLower(strings.TrimSpace(b))
		if lower == "true" {
			return true, true
		}
		if lower == "false" {
			return false, true
		}
	}
	return false, false
}

func lookupIntCI(m map[string]any, keys ...string) (int, bool) {
	v, ok := lookupCI(m, keys...)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i, true
		}
	}
	return 0, false
}

func lookupCI(m map[string]any, keys ...string) (any, bool) {
	if len(m) == 0 {
		return nil, false
	}
	for _, key := range keys {
		want := strings.ToLower(strings.TrimSpace(key))
		for k, v := range m {
			if strings.ToLower(strings.TrimSpace(k)) == want {
				return v, true
			}
		}
	}
	return nil, false
}
