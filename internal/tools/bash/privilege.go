package bash

import (
	"strings"
)

// parseTargetUser extracts the target user and body command from su/sudo invocations.
// Returns ("", "") for commands that are not privilege-escalation wrappers.
func parseTargetUser(cmd string) (user, body string) {
	cmd = strings.TrimSpace(cmd)

	// sudo -u <user> <cmd>
	if strings.HasPrefix(cmd, "sudo -u ") {
		rest := strings.TrimPrefix(cmd, "sudo -u ")
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) == 2 {
			return parts[0], strings.TrimSpace(parts[1])
		}
		return "", ""
	}
	// sudo <cmd>  (default target: root)
	if strings.HasPrefix(cmd, "sudo ") {
		rest := strings.TrimSpace(strings.TrimPrefix(cmd, "sudo "))
		// compound: "sudo su [-] <user> -c ..." — resolve the inner su so the
		// real target user is extracted instead of defaulting to root.
		if innerUser, innerBody := parseTargetUser(rest); innerUser != "" && innerBody != "" {
			return innerUser, innerBody
		}
		return "root", rest
	}

	// su - <user> -c <body>  or  su <user> -c <body>  or  su - -c <body> (root)
	if strings.HasPrefix(cmd, "su ") {
		rest := strings.TrimSpace(strings.TrimPrefix(cmd, "su "))
		// strip optional leading "-"
		targetUser := "root"
		if strings.HasPrefix(rest, "- ") {
			rest = strings.TrimSpace(rest[2:])
		}
		// next token is either a username or "-c"
		if !strings.HasPrefix(rest, "-c ") {
			idx := strings.Index(rest, " ")
			if idx < 0 {
				return "", ""
			}
			targetUser = rest[:idx]
			rest = strings.TrimSpace(rest[idx+1:])
		}
		if !strings.HasPrefix(rest, "-c ") {
			return "", ""
		}
		body = strings.TrimSpace(rest[3:])
		body = unquote(body)
		return targetUser, body
	}

	return "", ""
}

// stripRedundantPrivilegeWrapper removes su/sudo wrappers when the target user
// equals the current user and a -c body is present.
func stripRedundantPrivilegeWrapper(command, currentUser string) string {
	targetUser, body := parseTargetUser(command)
	if targetUser == "" || body == "" {
		return command
	}
	if strings.EqualFold(targetUser, currentUser) {
		return body
	}
	return command
}

// containsSudoToken reports whether cmd contains an unquoted `sudo ` token.
// Mirrors the quote-aware scanning used by rewriteSudoTokens so we don't
// mistakenly rewrite literal "sudo" inside shell strings.
func containsSudoToken(cmd string) bool {
	const sudoPrefix = "sudo "
	if !strings.Contains(cmd, "sudo") {
		return false
	}
	i, n := 0, len(cmd)
	afterSep := true
	for i < n {
		switch {
		case cmd[i] == '\'':
			j := i + 1
			for j < n && cmd[j] != '\'' {
				j++
			}
			if j < n {
				j++
			}
			i, afterSep = j, false
		case cmd[i] == '"':
			j := i + 1
			for j < n {
				if cmd[j] == '\\' {
					j += 2
					continue
				}
				if cmd[j] == '"' {
					j++
					break
				}
				j++
			}
			i, afterSep = j, false
		case afterSep && strings.HasPrefix(cmd[i:], sudoPrefix):
			return true
		case i+1 < n && cmd[i] == '&' && cmd[i+1] == '&':
			i, afterSep = i+2, true
		case i+1 < n && cmd[i] == '|' && cmd[i+1] == '|':
			i, afterSep = i+2, true
		case cmd[i] == '|' || cmd[i] == ';' || cmd[i] == '\n' || cmd[i] == '(' || cmd[i] == '{':
			i, afterSep = i+1, true
		case cmd[i] == ' ' || cmd[i] == '\t':
			i++
		default:
			i, afterSep = i+1, false
		}
	}
	return false
}

// rewriteSudoTokens rewrites every unquoted "sudo " occurrence in cmd to inject
// the cached password via stdin, so password-less sudo works regardless of where
// sudo appears in a compound command (e.g. "cmd && sudo subcmd").
// quotedPassword must already be POSIX-shell-quoted (e.g. from POSIXShellQuote).
// Returns cmd unchanged if no unquoted sudo tokens are found or all already have -S.
func rewriteSudoTokens(cmd, quotedPassword string) string {
	const sudoPrefix = "sudo "
	if !strings.Contains(cmd, "sudo") {
		return cmd
	}

	var result strings.Builder
	result.Grow(len(cmd) + 64)
	i, n := 0, len(cmd)
	afterSep := true // command start counts as after a separator
	prevPipe := false // most recent separator was a single `|` (stdin already fed)

	for i < n {
		switch {
		case cmd[i] == '\'':
			// Single-quoted string: copy verbatim until closing '.
			j := i + 1
			for j < n && cmd[j] != '\'' {
				j++
			}
			if j < n {
				j++
			}
			result.WriteString(cmd[i:j])
			i, afterSep, prevPipe = j, false, false

		case cmd[i] == '"':
			// Double-quoted string: respect backslash escapes.
			j := i + 1
			for j < n {
				if cmd[j] == '\\' {
					j += 2
					continue
				}
				if cmd[j] == '"' {
					j++
					break
				}
				j++
			}
			result.WriteString(cmd[i:j])
			i, afterSep, prevPipe = j, false, false

		case afterSep && strings.HasPrefix(cmd[i:], sudoPrefix):
			// Check if -S flag is already present on this sudo invocation.
			rest := strings.TrimLeft(cmd[i+len(sudoPrefix):], " \t")
			alreadyS := rest == "-S" ||
				strings.HasPrefix(rest, "-S ") ||
				strings.HasPrefix(rest, "-S\t") ||
				strings.HasPrefix(rest, "-S\n")
			switch {
			case alreadyS && prevPipe:
				// 앞 명령이 stdin을 이미 공급 (e.g. `printf pw | sudo -S ls`).
				// 그대로 둔다.
				result.WriteString(sudoPrefix)
			case alreadyS && !prevPipe:
				// `-S` 만 붙어있고 stdin 공급원이 없으면 sudo 가 stdin에서 비밀번호를
				// 못 읽어 lane PTY 의 다음 라인을 비밀번호로 오인하게 된다. 항상
				// printf | 를 prepend 해서 비밀번호를 stdin으로 강제 주입한다.
				result.WriteString(`printf "%s\n" `)
				result.WriteString(quotedPassword)
				result.WriteString(" | sudo ")
			default:
				result.WriteString(`printf "%s\n" `)
				result.WriteString(quotedPassword)
				result.WriteString(" | sudo -S ")
			}
			i, afterSep, prevPipe = i+len(sudoPrefix), false, false

		case i+1 < n && cmd[i] == '&' && cmd[i+1] == '&':
			result.WriteString("&&")
			i, afterSep, prevPipe = i+2, true, false

		case i+1 < n && cmd[i] == '|' && cmd[i+1] == '|':
			result.WriteString("||")
			i, afterSep, prevPipe = i+2, true, false

		case cmd[i] == '|':
			result.WriteByte(cmd[i])
			i, afterSep, prevPipe = i+1, true, true

		case cmd[i] == ';' || cmd[i] == '\n' || cmd[i] == '(' || cmd[i] == '{':
			result.WriteByte(cmd[i])
			i, afterSep, prevPipe = i+1, true, false

		case cmd[i] == ' ' || cmd[i] == '\t':
			result.WriteByte(cmd[i])
			i++ // preserve afterSep / prevPipe across whitespace

		default:
			result.WriteByte(cmd[i])
			i, afterSep, prevPipe = i+1, false, false
		}
	}

	return result.String()
}

// unquote strips a single surrounding quote pair (' or ") from s, if present.
func unquote(s string) string {
	if len(s) < 2 {
		return s
	}
	if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}
