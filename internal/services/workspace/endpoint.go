package workspace

import (
	"fmt"
	"strings"
)

// ParseEndpointPath parses endpoint expressions like "alias:/path/to/file".
// On Windows, drive-letter absolute paths like "C:\\tmp\\a.txt" are treated as local paths.
func ParseEndpointPath(raw string, activeAlias string) (string, string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", "", fmt.Errorf("endpoint path is empty")
	}
	if len(token) >= 3 && token[1] == ':' && ((token[2] == '\\') || (token[2] == '/')) &&
		((token[0] >= 'a' && token[0] <= 'z') || (token[0] >= 'A' && token[0] <= 'Z')) {
		return LocalAlias, token, nil
	}
	if idx := strings.Index(token, ":"); idx > 0 {
		alias := strings.TrimSpace(token[:idx])
		path := strings.TrimSpace(token[idx+1:])
		if alias != "" && path != "" {
			return alias, path, nil
		}
	}
	if strings.TrimSpace(activeAlias) == "" {
		activeAlias = LocalAlias
	}
	return activeAlias, token, nil
}
