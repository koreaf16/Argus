package workspace

import (
	"fmt"
	"runtime"
	"strings"
)

// ParseEndpointPath parses endpoint expressions like "alias:/path/to/file".
// On Windows, drive-letter absolute paths like "C:\\tmp\\a.txt" are treated as local paths.
func ParseEndpointPath(raw string, activeAlias string) (string, string, error) {
	ep, err := ParseEndpointPathV2(nil, raw, activeAlias)
	if err != nil {
		return "", "", err
	}
	return ep.Alias, ep.RawPath, nil
}

// ParseEndpointPathV2 returns a rich EndpointPath object with OS context.
func ParseEndpointPathV2(m *Manager, raw string, activeAlias string) (EndpointPath, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return EndpointPath{}, fmt.Errorf("endpoint path is empty")
	}

	alias := ""
	path := ""

	// 1. Check for Windows drive letter (only if it's potentially local)
	isWindowsDrive := len(token) >= 3 && token[1] == ':' && ((token[2] == '\\') || (token[2] == '/')) &&
		((token[0] >= 'a' && token[0] <= 'z') || (token[0] >= 'A' && token[0] <= 'Z'))

	if isWindowsDrive {
		// If it has a drive letter, it's definitively local Windows path
		alias = LocalAlias
		path = token
	} else if idx := strings.Index(token, ":"); idx > 0 {
		// 2. Check for alias:path format (but avoid confusing C: with an alias)
		potentialAlias := strings.TrimSpace(token[:idx])
		potentialPath := strings.TrimSpace(token[idx+1:])

		// Valid alias shouldn't be just a drive letter if it's already handled above.
		// Also, if the manager knows this alias, it's an alias.
		if m != nil && m.IsKnownAlias(potentialAlias) {
			alias = potentialAlias
			path = potentialPath
		} else if !isWindowsDrive && len(potentialAlias) > 1 {
			// Heuristic: If it's longer than 1 char and followed by colon, assume it's an alias
			alias = potentialAlias
			path = potentialPath
		}
	}

	// 3. Fallback to active alias
	if alias == "" {
		if strings.TrimSpace(activeAlias) != "" {
			alias = activeAlias
		} else {
			alias = LocalAlias
		}
	}
	if path == "" {
		path = token
	}

	// 4. Resolve OS context and Remote status
	osType := "linux" // Default
	isRemote := false
	if alias == LocalAlias {
		osType = runtime.GOOS
		isRemote = false
	} else if m != nil {
		osType = m.GetOSType(alias)
		isRemote = m.IsRemoteAlias(alias)
	} else {
		// If no manager provided, assume remote Linux if not local
		isRemote = (alias != LocalAlias)
	}

	return EndpointPath{
		Alias:    alias,
		RawPath:  path,
		OSType:   osType,
		IsRemote: isRemote,
	}, nil
}

// IsKnownAlias checks if the alias is local or registered in catalog.
func (m *Manager) IsKnownAlias(alias string) bool {
	if alias == LocalAlias || alias == "local" {
		return true
	}
	_, ok := m.reg.Get(alias)
	return ok
}

// GetOSType returns the OS of the server, defaults to linux for remote.
func (m *Manager) GetOSType(alias string) string {
	if alias == LocalAlias || alias == "local" {
		return runtime.GOOS
	}
	entry, ok := m.reg.Get(alias)
	if ok && entry.Kind == ServerKindAccount {
		if parent, parentOK := m.reg.Get(entry.ParentAlias); parentOK && parent.OSType != "" {
			return strings.ToLower(parent.OSType)
		}
		return "linux"
	}
	if ok && entry.OSType != "" {
		return strings.ToLower(entry.OSType)
	}
	return "linux"
}
