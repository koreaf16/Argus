package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func allowedRoots(ctx Context) ([]string, error) {
	roots := make([]string, 0, 2)

	wd := strings.TrimSpace(ctx.WorkingDir)
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	wdAbs, err := filepath.Abs(wd)
	if err != nil {
		return nil, err
	}
	roots = append(roots, filepath.Clean(wdAbs))

	if ctx.State != nil {
		for _, extra := range ctx.State.AdditionalWorkingDirectories() {
			extra = strings.TrimSpace(extra)
			if extra == "" {
				continue
			}
			abs, err := filepath.Abs(extra)
			if err != nil {
				continue
			}
			clean := filepath.Clean(abs)
			dup := false
			for _, existing := range roots {
				if samePath(existing, clean) {
					dup = true
					break
				}
			}
			if !dup {
				roots = append(roots, clean)
			}
		}
	}

	return roots, nil
}

func resolvePathUnderRoots(ctx Context, rawPath string) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	path = expandHome(path)

	// 절대 경로인 경우, 로컬에서는 보안 루트 체크를 생략하고 그대로 허용 (단, 민감한 경로는 Write 시 ResolvePathForWrite에서 별도 체크)
	if filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		return filepath.Clean(abs), nil
	}

	// 상대 경로인 경우 기존처럼 WorkingDir 기준으로 해결 및 루트 체크
	base := strings.TrimSpace(ctx.WorkingDir)
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	path = filepath.Join(base, path)

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)

	roots, err := allowedRoots(ctx)
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		if isWithinRoot(abs, root) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("path is outside allowed roots: %s", rawPath)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func isWithinRoot(path, root string) bool {
	if samePath(path, root) {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
