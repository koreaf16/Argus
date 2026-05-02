package lane

import "strings"

// AccountTransitionKind classifies how a user command affects the account stack.
type AccountTransitionKind int

const (
	// AccountTransitionNone means the command does not change the account stack
	// and runs in the current effective user's shell.
	AccountTransitionNone AccountTransitionKind = iota

	// AccountTransitionEnter means the command opens an interactive shell as a
	// different user (e.g. `su - postgres`, `sudo -i`, `sudo su -`). Subsequent
	// commands until matching exit/logout run inside that shell, so the stack
	// must be pushed.
	AccountTransitionEnter

	// AccountTransitionInline means the command runs a one-shot body as another
	// user (e.g. `sudo -u postgres psql -c "..."`, `su - postgres -c "ls"`).
	// The stack is unchanged; the body is captured by the normal sentinel.
	AccountTransitionInline

	// AccountTransitionExit means the command leaves the current shell layer
	// (e.g. `exit`, `logout`). Stack is popped.
	AccountTransitionExit
)

// AccountTransition describes the effect of a command on a lane's account stack.
type AccountTransition struct {
	Kind   AccountTransitionKind
	User   string
	Method string
}

// ParseAccountTransition classifies a single command. It uses shlex to tokenize
// safely; commands that fail tokenization fall through as None so the caller
// runs them through the normal sentinel protocol.
func ParseAccountTransition(cmd string) AccountTransition {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return AccountTransition{Kind: AccountTransitionNone}
	}

	// Reject obvious multi-statement strings; treat as opaque.
	if strings.ContainsAny(trimmed, ";&|\n") {
		return AccountTransition{Kind: AccountTransitionNone}
	}

	tokens := tokenize(trimmed)
	if len(tokens) == 0 {
		return AccountTransition{Kind: AccountTransitionNone}
	}

	switch tokens[0] {
	case "exit", "logout":
		if len(tokens) == 1 {
			return AccountTransition{Kind: AccountTransitionExit}
		}
		return AccountTransition{Kind: AccountTransitionNone}

	case "su":
		return classifySu(tokens[1:])

	case "sudo":
		return classifySudo(tokens[1:])
	}

	return AccountTransition{Kind: AccountTransitionNone}
}

func classifySu(args []string) AccountTransition {
	user := ""
	hasDash := false
	hasInlineCmd := false

	for i := 0; i < len(args); i++ {
		tok := args[i]
		switch {
		case tok == "-" || tok == "-l" || tok == "--login":
			hasDash = true
		case tok == "-c":
			hasInlineCmd = true
			i = len(args)
		case strings.HasPrefix(tok, "--command="):
			hasInlineCmd = true
		case strings.HasPrefix(tok, "-"):
			// other flags (e.g. -s, -p): keep scanning
		default:
			if user == "" {
				user = tok
			}
		}
	}

	if hasInlineCmd {
		return AccountTransition{Kind: AccountTransitionInline, User: orRoot(user), Method: "su"}
	}
	if hasDash {
		return AccountTransition{Kind: AccountTransitionEnter, User: orRoot(user), Method: "su"}
	}
	if user != "" {
		return AccountTransition{Kind: AccountTransitionEnter, User: user, Method: "su"}
	}
	return AccountTransition{Kind: AccountTransitionEnter, User: "root", Method: "su"}
}

func classifySudo(args []string) AccountTransition {
	user := ""
	loginShell := false
	hasUFlag := false

	for i := 0; i < len(args); i++ {
		tok := args[i]
		switch {
		case tok == "-i" || tok == "--login":
			loginShell = true
		case tok == "-u" || tok == "--user":
			hasUFlag = true
			if i+1 < len(args) {
				user = args[i+1]
				i++
			}
		case strings.HasPrefix(tok, "--user="):
			hasUFlag = true
			user = strings.TrimPrefix(tok, "--user=")
		case strings.HasPrefix(tok, "-"):
			// other flags (e.g. -S, -n): keep scanning
		default:
			// First non-flag positional arg is the body to execute.
			body := args[i:]
			if loginShell && len(body) == 1 && body[0] == "su" {
				return AccountTransition{Kind: AccountTransitionEnter, User: orRoot(user), Method: "sudo"}
			}
			if isSuLoginInvocation(body) {
				target := suTargetFrom(body)
				if target == "" {
					target = orRoot(user)
				}
				return AccountTransition{Kind: AccountTransitionEnter, User: target, Method: "sudo"}
			}
			if hasUFlag {
				return AccountTransition{Kind: AccountTransitionInline, User: user, Method: "sudo"}
			}
			return AccountTransition{Kind: AccountTransitionInline, User: "root", Method: "sudo"}
		}
	}

	// No body: `sudo -i` or `sudo -u <user>` alone => enter shell as that user.
	if loginShell {
		return AccountTransition{Kind: AccountTransitionEnter, User: orRoot(user), Method: "sudo"}
	}
	if hasUFlag {
		return AccountTransition{Kind: AccountTransitionEnter, User: user, Method: "sudo"}
	}
	return AccountTransition{Kind: AccountTransitionEnter, User: "root", Method: "sudo"}
}

func isSuLoginInvocation(body []string) bool {
	if len(body) == 0 || body[0] != "su" {
		return false
	}
	for _, tok := range body[1:] {
		if tok == "-" || tok == "-l" || tok == "--login" {
			return true
		}
	}
	return false
}

func suTargetFrom(body []string) string {
	for i := 1; i < len(body); i++ {
		tok := body[i]
		if tok == "-" || tok == "-l" || tok == "--login" || tok == "-s" {
			continue
		}
		if strings.HasPrefix(tok, "-") {
			continue
		}
		return tok
	}
	return ""
}

func orRoot(user string) string {
	if strings.TrimSpace(user) == "" {
		return "root"
	}
	return user
}

// tokenize splits a single-line command into argv-style tokens, respecting
// single and double quotes and backslash escapes outside single quotes.
// Quotes themselves are stripped; the result is suitable for flag inspection
// but not for re-execution.
func tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	inSingle, inDouble := false, false

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case ch == '\\' && !inSingle && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
		case (ch == ' ' || ch == '\t') && !inSingle && !inDouble:
			flush()
		default:
			cur.WriteByte(ch)
		}
	}
	flush()
	return tokens
}
