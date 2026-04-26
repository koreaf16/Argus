package serverenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	StateDirName     = ".infractl"
	DefaultRemoteDir = "~/.infractl/workspace"
)

func LocalRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get workspace root: %w", err)
	}
	return filepath.Clean(wd), nil
}

func StateDir() (string, error) {
	root, err := LocalRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, StateDirName), nil
}

func RemoteDirOrDefault(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return DefaultRemoteDir
	}
	return dir
}

// POSIXShellPath returns a shell expression for dir. It keeps leading "~"
// semantics by expanding it through $HOME instead of single-quoting it.
func POSIXShellPath(dir string) string {
	dir = RemoteDirOrDefault(dir)
	switch {
	case dir == "~":
		return "$HOME"
	case strings.HasPrefix(dir, "~/"):
		rest := strings.TrimPrefix(dir, "~/")
		if rest == "" {
			return "$HOME"
		}
		return "$HOME/" + QuotePOSIX(rest)
	default:
		return QuotePOSIX(dir)
	}
}

func QuotePOSIX(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
