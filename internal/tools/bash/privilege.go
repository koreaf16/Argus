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
		return "root", strings.TrimSpace(strings.TrimPrefix(cmd, "sudo "))
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
