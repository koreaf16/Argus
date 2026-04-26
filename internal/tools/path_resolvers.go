package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolvePathForRead(ctx Context, rawPath string) (string, error) {
	return resolvePathUnderRoots(ctx, rawPath)
}

func ResolvePathForWrite(ctx Context, rawPath string) (string, error) {
	path, err := resolvePathUnderRoots(ctx, rawPath)
	if err != nil {
		return "", err
	}
	lower := strings.ToLower(path)
	if strings.Contains(lower, string(filepath.Separator)+".ssh"+string(filepath.Separator)) ||
		strings.HasSuffix(lower, string(filepath.Separator)+".ssh") ||
		strings.Contains(lower, "id_rsa") ||
		strings.Contains(lower, "id_ed25519") ||
		strings.Contains(lower, ".bashrc") ||
		strings.Contains(lower, ".zshrc") {
		return "", fmt.Errorf("write to sensitive path is denied: %s", rawPath)
	}
	return path, nil
}

func ResolveGlobPattern(ctx Context, pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	pattern = expandHome(pattern)
	if !filepath.IsAbs(pattern) {
		base := strings.TrimSpace(ctx.WorkingDir)
		if base == "" {
			var err error
			base, err = os.Getwd()
			if err != nil {
				return "", err
			}
		}
		pattern = filepath.Join(base, pattern)
	}
	pattern = filepath.Clean(pattern)

	baseDir := globBaseDir(pattern)
	roots, err := allowedRoots(ctx)
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		if isWithinRoot(baseDir, root) {
			return pattern, nil
		}
	}
	return "", fmt.Errorf("glob pattern is outside allowed roots")
}

func globBaseDir(pattern string) string {
	lastSep := strings.LastIndexAny(pattern, `\/`)
	if lastSep < 0 {
		return pattern
	}
	firstWildcard := strings.IndexAny(pattern, "*?[")
	if firstWildcard < 0 {
		return pattern
	}
	prefix := pattern[:firstWildcard]
	idx := strings.LastIndexAny(prefix, `\/`)
	if idx < 0 {
		return prefix
	}
	if idx == 0 {
		return prefix[:1]
	}
	return prefix[:idx]
}

func ResolveWorkingDirectory(ctx Context, rawPath string) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		path = strings.TrimSpace(ctx.WorkingDir)
		if path == "" {
			var err error
			path, err = os.Getwd()
			if err != nil {
				return "", err
			}
		}
	}

	resolved, err := resolvePathUnderRoots(ctx, path)
	if err != nil {
		return "", err
	}
	roots, err := allowedRoots(ctx)
	if err != nil {
		return "", err
	}
	allowed := false
	for _, root := range roots {
		if isWithinRoot(resolved, root) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("working directory is outside allowed roots: %s", rawPath)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", rawPath)
	}
	return resolved, nil
}
