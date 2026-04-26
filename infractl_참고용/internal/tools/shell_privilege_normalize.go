package tools

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/yourorg/infractl/internal/privilege"
)

type NormalizedPrivilegeCommand struct {
	Command      string
	BecomeMethod string
	BecomeUser   string
}

type leadingPrivilegeSpec struct {
	Method  string
	User    string
	Payload string
}

type commandSegment struct {
	Raw   string
	Start int
	End   int
}

type embeddedPrivilegeSpec struct {
	Method  string
	User    string
	Segment commandSegment
}

var leadingPrivilegeWrapperRe = regexp.MustCompile(`(?i)^\s*(sudo|su|runuser)\b`)

var errEmbeddedPrivilegeWrapper = errors.New("embedded privilege wrapper detected in command")

func NormalizePrivilegeCommand(command, explicitMethod, explicitUser string) (NormalizedPrivilegeCommand, error) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return NormalizedPrivilegeCommand{}, fmt.Errorf("command must not be empty")
	}

	method := ""
	if strings.TrimSpace(explicitMethod) != "" {
		parsed, ok := privilege.ParseMethod(explicitMethod)
		if !ok || parsed == privilege.MethodNone {
			return NormalizedPrivilegeCommand{}, fmt.Errorf("unsupported become_method: %q", explicitMethod)
		}
		method = string(parsed)
	}
	user := strings.TrimSpace(explicitUser)

	if hasEmbeddedPrivilegeWrapper(cmd) {
		return NormalizedPrivilegeCommand{}, fmt.Errorf("%w", errEmbeddedPrivilegeWrapper)
	}

	inline, hasInline, err := parseLeadingPrivilegeWrapper(cmd)
	if err != nil {
		return NormalizedPrivilegeCommand{}, err
	}
	if !hasInline {
		return NormalizedPrivilegeCommand{
			Command:      command,
			BecomeMethod: method,
			BecomeUser:   user,
		}, nil
	}

	if method != "" && inline.Method != "" && method != inline.Method {
		return NormalizedPrivilegeCommand{}, fmt.Errorf("conflicting privilege method: inline wrapper uses %s but become_method=%s", inline.Method, method)
	}
	if method == "" {
		method = inline.Method
	}
	if user == "" {
		user = inline.User
	}

	return NormalizedPrivilegeCommand{
		Command:      inline.Payload,
		BecomeMethod: method,
		BecomeUser:   user,
	}, nil
}

func hasEmbeddedPrivilegeWrapper(command string) bool {
	segments := splitTopLevelCommandSegments(command)
	if len(segments) == 0 {
		return false
	}
	if len(segments) == 1 {
		return false
	}
	for _, seg := range segments {
		if leadingPrivilegeWrapperRe.MatchString(seg.Raw) {
			return true
		}
	}
	return false
}

func splitTopLevelCommandSegments(command string) []commandSegment {
	var segments []commandSegment
	start := 0
	inSingle := false
	inDouble := false

	flush := func(end int) {
		seg := strings.TrimSpace(command[start:end])
		if seg != "" {
			trimStart := start
			for trimStart < end && isShellWhitespace(command[trimStart]) {
				trimStart++
			}
			trimEnd := end
			for trimEnd > trimStart && isShellWhitespace(command[trimEnd-1]) {
				trimEnd--
			}
			segments = append(segments, commandSegment{
				Raw:   seg,
				Start: trimStart,
				End:   trimEnd,
			})
		}
		start = end
	}

	for i := 0; i < len(command); i++ {
		ch := command[i]
		if inSingle {
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '"' {
				inDouble = false
				continue
			}
			if ch == '\\' && i+1 < len(command) {
				i++
			}
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '\\':
			if i+1 < len(command) {
				i++
			}
		case ';':
			flush(i)
			start = i + 1
		case '|':
			flush(i)
			if i+1 < len(command) && command[i+1] == '|' {
				i++
			}
			start = i + 1
		case '&':
			flush(i)
			if i+1 < len(command) && command[i+1] == '&' {
				i++
			}
			start = i + 1
		}
	}
	flush(len(command))
	return segments
}

func findEmbeddedPrivilegeSpec(command string) (*embeddedPrivilegeSpec, error) {
	segments := splitTopLevelCommandSegments(command)
	for _, seg := range segments {
		if !leadingPrivilegeWrapperRe.MatchString(seg.Raw) {
			continue
		}
		spec, hasInline, err := parseLeadingPrivilegeWrapper(seg.Raw)
		if err != nil {
			return nil, err
		}
		if !hasInline {
			continue
		}
		return &embeddedPrivilegeSpec{
			Method:  spec.Method,
			User:    spec.User,
			Segment: seg,
		}, nil
	}
	return nil, nil
}

type shellWord struct {
	Raw   string
	Start int
	End   int
}

func parseShellWords(command string) []shellWord {
	words := make([]shellWord, 0, 8)
	i := 0
	for i < len(command) {
		for i < len(command) && isShellWhitespace(command[i]) {
			i++
		}
		if i >= len(command) {
			break
		}
		start := i
		inSingle := false
		inDouble := false
		for i < len(command) {
			ch := command[i]
			if inSingle {
				if ch == '\'' {
					inSingle = false
				}
				i++
				continue
			}
			if inDouble {
				if ch == '"' {
					inDouble = false
					i++
					continue
				}
				if ch == '\\' && i+1 < len(command) {
					i += 2
					continue
				}
				i++
				continue
			}
			if isShellWhitespace(ch) {
				break
			}
			if ch == '\'' {
				inSingle = true
				i++
				continue
			}
			if ch == '"' {
				inDouble = true
				i++
				continue
			}
			if ch == '\\' && i+1 < len(command) {
				i += 2
				continue
			}
			i++
		}
		words = append(words, shellWord{
			Raw:   command[start:i],
			Start: start,
			End:   i,
		})
	}
	return words
}

func isShellWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func shellWordValue(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) >= 2 {
		if (v[0] == '\'' && v[len(v)-1] == '\'') || (v[0] == '"' && v[len(v)-1] == '"') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func parseLeadingPrivilegeWrapper(command string) (leadingPrivilegeSpec, bool, error) {
	match := leadingPrivilegeWrapperRe.FindStringSubmatch(command)
	if match == nil {
		return leadingPrivilegeSpec{}, false, nil
	}
	wrapper := strings.ToLower(match[1])
	switch wrapper {
	case "sudo":
		spec, err := parseLeadingSudo(command)
		return spec, true, err
	case "su":
		spec, err := parseLeadingSU(command)
		return spec, true, err
	case "runuser":
		spec, err := parseLeadingRunuser(command)
		return spec, true, err
	default:
		return leadingPrivilegeSpec{}, true, fmt.Errorf("unsupported inline privilege wrapper: %s", wrapper)
	}
}

func parseLeadingSudo(command string) (leadingPrivilegeSpec, error) {
	words := parseShellWords(command)
	if len(words) == 0 || !strings.EqualFold(shellWordValue(words[0].Raw), "sudo") {
		return leadingPrivilegeSpec{}, fmt.Errorf("invalid inline sudo command")
	}
	user := "root"
	i := 1
	for i < len(words) {
		tok := shellWordValue(words[i].Raw)
		if tok == "--" {
			i++
			break
		}
		if !strings.HasPrefix(tok, "-") {
			break
		}
		switch {
		case tok == "-u" || tok == "--user":
			i++
			if i >= len(words) {
				return leadingPrivilegeSpec{}, fmt.Errorf("inline sudo missing user after %q", tok)
			}
			user = shellWordValue(words[i].Raw)
			i++
		case strings.HasPrefix(tok, "-u") && len(tok) > 2:
			user = tok[2:]
			i++
		case strings.HasPrefix(tok, "--user="):
			user = strings.TrimPrefix(tok, "--user=")
			i++
		default:
			i++
		}
	}
	if i >= len(words) {
		return leadingPrivilegeSpec{}, fmt.Errorf("inline sudo command must include a payload command")
	}
	payload := strings.TrimSpace(command[words[i].Start:])
	return leadingPrivilegeSpec{
		Method:  string(privilege.MethodSudo),
		User:    privilege.NormalizeUser(user),
		Payload: payload,
	}, nil
}

func parseLeadingSU(command string) (leadingPrivilegeSpec, error) {
	words := parseShellWords(command)
	if len(words) == 0 || !strings.EqualFold(shellWordValue(words[0].Raw), "su") {
		return leadingPrivilegeSpec{}, fmt.Errorf("invalid inline su command")
	}
	user := "root"
	payload := ""
	for i := 1; i < len(words); i++ {
		tok := shellWordValue(words[i].Raw)
		switch {
		case tok == "-" || tok == "-l" || tok == "--login" || tok == "-m" || tok == "-p":
			continue
		case tok == "-s" || tok == "--shell":
			i++
		case tok == "-c":
			i++
			if i >= len(words) {
				return leadingPrivilegeSpec{}, fmt.Errorf("inline su missing payload after -c")
			}
			payload = shellWordValue(words[i].Raw)
		case strings.HasPrefix(tok, "-c") && len(tok) > 2:
			payload = strings.TrimPrefix(tok, "-c")
		case strings.HasPrefix(tok, "-"):
			continue
		default:
			if user == "root" {
				user = tok
				continue
			}
			if payload == "" {
				payload = strings.TrimSpace(command[words[i].Start:])
			}
		}
	}
	if strings.TrimSpace(payload) == "" {
		return leadingPrivilegeSpec{}, fmt.Errorf("inline su without -c payload is not supported; use become_method=\"su\" with a normal command")
	}
	return leadingPrivilegeSpec{
		Method:  string(privilege.MethodSU),
		User:    privilege.NormalizeUser(user),
		Payload: strings.TrimSpace(payload),
	}, nil
}

func parseLeadingRunuser(command string) (leadingPrivilegeSpec, error) {
	words := parseShellWords(command)
	if len(words) == 0 || !strings.EqualFold(shellWordValue(words[0].Raw), "runuser") {
		return leadingPrivilegeSpec{}, fmt.Errorf("invalid inline runuser command")
	}
	user := "root"
	payload := ""
	for i := 1; i < len(words); i++ {
		tok := shellWordValue(words[i].Raw)
		switch {
		case tok == "-u" || tok == "-l" || tok == "--user":
			i++
			if i >= len(words) {
				return leadingPrivilegeSpec{}, fmt.Errorf("inline runuser missing user after %q", tok)
			}
			user = shellWordValue(words[i].Raw)
		case strings.HasPrefix(tok, "--user="):
			user = strings.TrimPrefix(tok, "--user=")
		case tok == "-c":
			i++
			if i >= len(words) {
				return leadingPrivilegeSpec{}, fmt.Errorf("inline runuser missing payload after -c")
			}
			payload = shellWordValue(words[i].Raw)
		case strings.HasPrefix(tok, "-c") && len(tok) > 2:
			payload = strings.TrimPrefix(tok, "-c")
		case strings.HasPrefix(tok, "-"):
			continue
		default:
			if payload == "" {
				payload = strings.TrimSpace(command[words[i].Start:])
			}
		}
	}
	if strings.TrimSpace(payload) == "" {
		return leadingPrivilegeSpec{}, fmt.Errorf("inline runuser without -c payload is not supported; use become_method=\"su\" with a normal command")
	}
	return leadingPrivilegeSpec{
		Method:  string(privilege.MethodSU),
		User:    privilege.NormalizeUser(user),
		Payload: strings.TrimSpace(payload),
	}, nil
}
