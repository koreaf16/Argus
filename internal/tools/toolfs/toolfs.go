package toolfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/koreaf16/argus/internal/constants"
)

func NormalizeSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "default"
	}
	return out
}

func PlansDir() string {
	return filepath.Join(constants.ConfigDir(), "plans")
}

func TodosDir() string {
	return filepath.Join(constants.ConfigDir(), "todos")
}

func PlanPath(sessionID string) string {
	return filepath.Join(PlansDir(), NormalizeSessionID(sessionID)+".md")
}

func TodoPath(sessionID string) string {
	return filepath.Join(TodosDir(), NormalizeSessionID(sessionID)+".json")
}

func EnsureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	if err := EnsureParentDir(path); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func ReadIfExists(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func ReadTextIfExists(path string) (string, error) {
	data, err := ReadIfExists(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func WritePlan(sessionID, content string) (string, error) {
	path := PlanPath(sessionID)
	if err := AtomicWrite(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func LoadPlan(sessionID string) (string, string, error) {
	path := PlanPath(sessionID)
	text, err := ReadTextIfExists(path)
	if err != nil {
		return "", path, err
	}
	return text, path, nil
}

func MustSessionID(sessionID string) string {
	id := NormalizeSessionID(sessionID)
	if id == "" {
		return "default"
	}
	return id
}

func EnsurePlanFile(sessionID string) (string, error) {
	path := PlanPath(sessionID)
	if err := EnsureParentDir(path); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		return "", fmt.Errorf("create plan file: %w", err)
	}
	return path, nil
}
