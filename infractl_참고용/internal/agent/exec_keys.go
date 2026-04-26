package agent

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	wordRe       = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}_\-./]{1,}`)
	numberRe     = regexp.MustCompile(`\b\d+\b`)
	pathLikeRe   = regexp.MustCompile(`([a-z]:\\[^\s]+|/[^\s]+)`)
	uuidLikeRe   = regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f-]{27,}\b`)
	multiSpaceRe = regexp.MustCompile(`\s+`)
)

var stopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "that": {}, "this": {},
	"from": {}, "into": {}, "about": {}, "show": {}, "please": {}, "just": {},
	"계속": {}, "진행": {}, "해줘": {}, "해": {}, "다시": {}, "좀": {},
}

func buildTaskKey(prompt string) string {
	prompt = strings.ToLower(strings.TrimSpace(prompt))
	if prompt == "" {
		return ""
	}
	tokens := wordRe.FindAllString(prompt, -1)
	seen := make(map[string]struct{}, len(tokens))
	out := make([]string, 0, 8)
	for _, t := range tokens {
		if _, blocked := stopWords[t]; blocked {
			continue
		}
		if _, exists := seen[t]; exists {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
		if len(out) >= 8 {
			break
		}
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "|")
}

func buildCommandKey(toolName string, args map[string]interface{}) string {
	tool := strings.TrimSpace(strings.ToLower(toolName))
	if tool == "" {
		return ""
	}
	if tool == "shell_exec" {
		cmd, _ := args["command"].(string)
		return shellCommandKey(cmd)
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, strings.ToLower(strings.TrimSpace(k)))
	}
	sort.Strings(keys)
	if len(keys) > 6 {
		keys = keys[:6]
	}
	return fmt.Sprintf("%s|%s", tool, strings.Join(keys, "|"))
}

func shellCommandKey(cmd string) string {
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	if cmd == "" {
		return "shell_exec"
	}
	cmd = pathLikeRe.ReplaceAllString(cmd, "<path>")
	cmd = numberRe.ReplaceAllString(cmd, "<num>")
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "shell_exec"
	}
	out := make([]string, 0, 5)
	out = append(out, parts[0])
	for _, p := range parts[1:] {
		if strings.HasPrefix(p, "-") || wordRe.MatchString(p) {
			out = append(out, p)
		}
		if len(out) >= 5 {
			break
		}
	}
	return strings.Join(out, "|")
}

func buildFailureSignature(errMsg, output string, exitCode int) string {
	base := strings.TrimSpace(errMsg)
	if base == "" {
		base = strings.TrimSpace(output)
	}
	base = strings.ToLower(base)
	base = uuidLikeRe.ReplaceAllString(base, "<uuid>")
	base = pathLikeRe.ReplaceAllString(base, "<path>")
	base = numberRe.ReplaceAllString(base, "<num>")
	base = multiSpaceRe.ReplaceAllString(base, " ")
	base = strings.TrimPrefix(base, "error:")
	base = strings.TrimSpace(base)
	if len(base) > 160 {
		base = base[:160]
	}
	if base == "" {
		return fmt.Sprintf("exit_code_%d", exitCode)
	}
	return base
}
