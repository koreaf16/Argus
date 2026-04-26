// Package tools
// File: install_channel_keys.go
// Description: tools 모듈의 기능 수행.
// Responsibility: tools 관련 로직 처리 및 관리.

package tools

import (
	"fmt"
	"strings"
)

const installChannelManagedKeyPrefix = "infractl-ephemeral:"

// ManagedAuthorizedKeyTag returns the marker token stored on authorized_keys lines
// that were added by infractl for temporary install channels.
func ManagedAuthorizedKeyTag(marker string) string {
	marker = strings.TrimSpace(marker)
	if marker == "" {
		return installChannelManagedKeyPrefix
	}
	return installChannelManagedKeyPrefix + marker
}

// AppendManagedAuthorizedKey appends one managed key line while preserving all
// existing keys/comments as-is. If the same marker already exists, no changes are made.
func AppendManagedAuthorizedKey(existing, publicKey, marker string) (string, bool, error) {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return existing, false, fmt.Errorf("public key is required")
	}
	tag := ManagedAuthorizedKeyTag(marker)
	lines := splitAuthorizedKeysLines(existing)
	for _, line := range lines {
		if lineHasManagedTag(line, tag) {
			return normalizeAuthorizedKeys(lines), false, nil
		}
	}

	managedLine := publicKey
	if !strings.Contains(managedLine, tag) {
		managedLine = managedLine + " " + tag
	}
	lines = append(lines, managedLine)
	return normalizeAuthorizedKeys(lines), true, nil
}

// RemoveManagedAuthorizedKey removes only lines tagged with the provided marker.
// It never deletes unrelated existing keys.
func RemoveManagedAuthorizedKey(existing, marker string) (string, bool) {
	tag := ManagedAuthorizedKeyTag(marker)
	lines := splitAuthorizedKeysLines(existing)
	filtered := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if lineHasManagedTag(line, tag) {
			removed = true
			continue
		}
		filtered = append(filtered, line)
	}
	return normalizeAuthorizedKeys(filtered), removed
}

func splitAuthorizedKeysLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines
}

func normalizeAuthorizedKeys(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func lineHasManagedTag(line, tag string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return false
	}
	for _, token := range strings.Fields(line) {
		if strings.TrimSpace(token) == tag {
			return true
		}
	}
	return false
}
