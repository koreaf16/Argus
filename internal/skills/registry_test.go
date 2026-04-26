package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUserSkillsJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "echo.skill.json")
	content := `{"name":"echo-user","description":"test","template":"ECHO {{args}}"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	r := &Registry{entries: make(map[string]skillEntry)}
	if err := r.LoadUserSkills(dir); err != nil {
		t.Fatalf("load skills: %v", err)
	}
	out, err := r.Run("echo-user", []string{"hello"})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	if out != "ECHO hello" {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestLoadUserSkillsMarkdown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "remember.md")
	content := "# skill: note\nhelper\nNote={{args}}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	r := &Registry{entries: make(map[string]skillEntry)}
	if err := r.LoadUserSkills(dir); err != nil {
		t.Fatalf("load skills: %v", err)
	}
	names := r.List()
	if len(names) != 1 || names[0] != "note" {
		t.Fatalf("unexpected names: %v", names)
	}
	out, err := r.Run("note", []string{"abc"})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	if !strings.Contains(out, "abc") {
		t.Fatalf("unexpected output: %s", out)
	}
}
