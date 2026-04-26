package executor

import (
	"regexp"
	"strings"
)

var (
	windowsDrivePathRe = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9])([A-Z]:\\[^\t\r\n "'<>|]*)`)
	windowsUNCPathRe   = regexp.MustCompile(`\\\\[^\\/\t\r\n "'<>|]+\\[^\t\r\n "'<>|]+`)
)

// FirstWindowsPath returns the first Windows-style absolute path found in s.
// It detects drive-letter paths (C:\...) and UNC paths (\\server\share\...).
func FirstWindowsPath(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}

	if match := windowsDrivePathRe.FindStringSubmatch(s); len(match) >= 3 {
		return match[2], true
	}
	if match := windowsUNCPathRe.FindString(s); match != "" {
		return match, true
	}
	return "", false
}

// ContainsWindowsPath reports whether s contains a Windows-style absolute path.
func ContainsWindowsPath(s string) bool {
	_, ok := FirstWindowsPath(s)
	return ok
}
