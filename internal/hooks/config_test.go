package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/koreaf16/argus/internal/types"
)

func writeSettingsJSON(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigFromFile_valid(t *testing.T) {
	dir := t.TempDir()
	path := writeSettingsJSON(t, dir, `{
		"hooks": {
			"PreToolUse": [
				{ "matcher": "Bash", "hooks": [{ "type": "command", "command": "echo hello" }] }
			]
		}
	}`)

	cfg, err := loadConfigFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	matchers, ok := cfg[types.HookEventPreToolUse]
	if !ok || len(matchers) != 1 {
		t.Fatalf("expected 1 matcher for PreToolUse, got %+v", cfg)
	}
	if matchers[0].Matcher != "Bash" || len(matchers[0].Hooks) != 1 {
		t.Errorf("unexpected matcher content: %+v", matchers[0])
	}
}

func TestLoadConfigFromFile_noHooksField(t *testing.T) {
	dir := t.TempDir()
	path := writeSettingsJSON(t, dir, `{"model": "claude-opus-4-7"}`)

	cfg, err := loadConfigFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg) != 0 {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestLoadConfigFromFile_unknownEventIgnored(t *testing.T) {
	dir := t.TempDir()
	path := writeSettingsJSON(t, dir, `{
		"hooks": {
			"UnknownEvent": [{ "matcher": "", "hooks": [] }],
			"PostToolUse":  [{ "matcher": "", "hooks": [{ "type": "command", "command": "echo post" }] }]
		}
	}`)

	cfg, err := loadConfigFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, bad := cfg[types.HookEvent("UnknownEvent")]; bad {
		t.Error("unknown event should be silently ignored")
	}
	if _, ok := cfg[types.HookEventPostToolUse]; !ok {
		t.Error("PostToolUse should be present")
	}
}

func TestLoadConfigFromFile_missingFile(t *testing.T) {
	cfg, err := loadConfigFromFile(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("missing file should not error, got: %v", err)
	}
	if len(cfg) != 0 {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestLoadConfigFromFile_invalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := writeSettingsJSON(t, dir, `not valid json`)

	_, err := loadConfigFromFile(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMergeHooksConfig_empty(t *testing.T) {
	result := mergeHooksConfig(HooksConfig{}, HooksConfig{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %+v", result)
	}
}

func TestMergeHooksConfig_baseOnly(t *testing.T) {
	cmd := HookCommand{Type: HookCommandTypeCommand, Command: "echo base"}
	base := HooksConfig{
		types.HookEventSessionStart: {{Matcher: "", Hooks: []HookCommand{cmd}}},
	}
	result := mergeHooksConfig(base, HooksConfig{})
	if len(result[types.HookEventSessionStart]) != 1 {
		t.Errorf("expected 1 matcher, got %d", len(result[types.HookEventSessionStart]))
	}
}

func TestMergeHooksConfig_order(t *testing.T) {
	homeCmd := HookCommand{Type: HookCommandTypeCommand, Command: "echo home"}
	projectCmd := HookCommand{Type: HookCommandTypeCommand, Command: "echo project"}
	base := HooksConfig{
		types.HookEventPreToolUse: {{Matcher: "", Hooks: []HookCommand{homeCmd}}},
	}
	overlay := HooksConfig{
		types.HookEventPreToolUse: {{Matcher: "", Hooks: []HookCommand{projectCmd}}},
	}

	result := mergeHooksConfig(base, overlay)
	matchers := result[types.HookEventPreToolUse]
	if len(matchers) != 2 {
		t.Fatalf("expected 2 matchers, got %d", len(matchers))
	}
	// 홈 훅이 먼저, 프로젝트 훅이 나중
	if matchers[0].Hooks[0].Command != "echo home" {
		t.Errorf("first matcher should be home hook, got %q", matchers[0].Hooks[0].Command)
	}
	if matchers[1].Hooks[0].Command != "echo project" {
		t.Errorf("second matcher should be project hook, got %q", matchers[1].Hooks[0].Command)
	}
}

func TestMergeHooksConfig_differentEvents(t *testing.T) {
	base := HooksConfig{
		types.HookEventSessionStart: {{Matcher: "", Hooks: []HookCommand{
			{Type: HookCommandTypeCommand, Command: "echo start"},
		}}},
	}
	overlay := HooksConfig{
		types.HookEventStop: {{Matcher: "", Hooks: []HookCommand{
			{Type: HookCommandTypeCommand, Command: "echo stop"},
		}}},
	}

	result := mergeHooksConfig(base, overlay)
	if _, ok := result[types.HookEventSessionStart]; !ok {
		t.Error("SessionStart should be in result")
	}
	if _, ok := result[types.HookEventStop]; !ok {
		t.Error("Stop should be in result")
	}
}
