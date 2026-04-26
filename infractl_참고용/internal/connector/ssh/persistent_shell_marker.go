// Package ssh
// File: persistent_shell_marker.go
// Description: Delimiter protocol for persistent shell command boundary detection
// Responsibility: Generate unique tokens and build/parse delimiter markers
//                 that wrap each command sent to a persistent bash session.

package ssh

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const (
	// markerPrefix / markerSuffix bracket all delimiter markers.
	markerPrefix = "<<<INFRACTL_"
	markerSuffix = ">>>"

	// maxRecentLines is the number of recent output lines retained for idle-prompt detection.
	maxRecentLines = 10

	// shellLineChanBuf is the line channel buffer size for the persistent shell reader goroutine.
	// Large enough to absorb command output bursts without dropping markers.
	shellLineChanBuf = 1024
)

// generateToken returns a cryptographically random 16-hex-character token.
// The token is unique per command invocation so delimiter markers cannot
// collide with actual command output.
func generateToken() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// beginMarker returns the BEGIN marker for token.
func beginMarker(token string) string { return markerPrefix + "B_" + token + markerSuffix }

// endMarker returns the END marker for token.
func endMarker(token string) string { return markerPrefix + "E_" + token + markerSuffix }

// rcPrefix returns the prefix of the RC marker for token.
func rcPrefix(token string) string { return markerPrefix + "RC_" + token + ":" }

// wrapCommand builds the stdin block sent to the persistent bash session.
// The block uses single-quoted echo for BEGIN/END markers (no expansion needed)
// and double-quoted echo for the RC marker (requires ${variable} expansion).
//
// Protocol (sent as stdin lines):
//
//	echo '<<<INFRACTL_B_TOKEN>>>'
//	<cmd>
//	__ic_rc=$?; __ic_u=$(whoami); __ic_d=$(pwd)
//	echo '<<<INFRACTL_E_TOKEN>>>'
//	echo "<<<INFRACTL_RC_TOKEN:${__ic_rc}:${__ic_u}:${__ic_d}>>>"
func wrapCommand(token, cmd string) string {
	bm := beginMarker(token)
	em := endMarker(token)
	rcp := rcPrefix(token)
	return "echo '" + bm + "'\n" +
		cmd + "\n" +
		"__ic_rc=$?; __ic_u=$(whoami); __ic_d=$(pwd)\n" +
		"echo '" + em + "'\n" +
		"echo \"" + rcp + "${__ic_rc}:${__ic_u}:${__ic_d}" + markerSuffix + "\"\n"
}

// parseRCLine extracts exit code, current user, and current directory from an RC marker line.
// Expected format: <<<INFRACTL_RC_TOKEN:exitCode:user:dir>>>
// Returns ok=false if the line does not match or cannot be parsed.
func parseRCLine(line, token string) (exitCode int, user, dir string, ok bool) {
	pfx := rcPrefix(token)
	if !strings.HasPrefix(line, pfx) || !strings.HasSuffix(line, markerSuffix) {
		return 0, "", "", false
	}
	inner := line[len(pfx) : len(line)-len(markerSuffix)]
	// inner: "0:oracle:/u01/app/oracle" — SplitN(3) keeps colons inside dir intact
	parts := strings.SplitN(inner, ":", 3)
	if len(parts) != 3 {
		return 0, "", "", false
	}
	var code int
	if _, err := fmt.Sscanf(parts[0], "%d", &code); err != nil {
		return 0, "", "", false
	}
	return code, parts[1], parts[2], true
}

// passwordPromptRE matches common password prompt patterns in English, Korean, and Japanese.
var passwordPromptRE = regexp.MustCompile(
	`(?i)(password:|passphrase for|암호:|パスワード:|\[sudo\] password for)`,
)

// isPasswordPrompt reports whether any of the recent lines looks like a password prompt.
func isPasswordPrompt(lines []string) bool {
	for _, l := range lines {
		if passwordPromptRE.MatchString(l) {
			return true
		}
	}
	return false
}

// appendRecent appends line to lines and trims to at most max entries.
func appendRecent(lines []string, line string, max int) []string {
	lines = append(lines, line)
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return lines
}
