package hooks

import (
	"encoding/json"
	"testing"

	"github.com/koreaf16/argus/internal/types"
)

func Test_matchesTool(t *testing.T) {
	cases := []struct {
		matcher, toolName string
		want              bool
	}{
		{"", "Bash", true},
		{"", "", true},
		{"Bash", "Bash", true},
		{"bash", "Bash", true},
		{"BASH", "bash", true},
		{"Bash", "PowerShell", false},
		{"Bash", "", false},
	}
	for _, c := range cases {
		got := matchesTool(c.matcher, c.toolName)
		if got != c.want {
			t.Errorf("matchesTool(%q, %q) = %v, want %v", c.matcher, c.toolName, got, c.want)
		}
	}
}

func Test_resolveCommands(t *testing.T) {
	cmd1 := HookCommand{Type: HookCommandTypeCommand, Command: "echo cmd1"}
	cmd2 := HookCommand{Type: HookCommandTypeCommand, Command: "echo cmd2"}
	cmd3 := HookCommand{Type: HookCommandTypeCommand, Command: "echo cmd3"}

	cfg := HooksConfig{
		types.HookEventPreToolUse: {
			{Matcher: "Bash", Hooks: []HookCommand{cmd1}},
			{Matcher: "", Hooks: []HookCommand{cmd2}},
			{Matcher: "PowerShell", Hooks: []HookCommand{cmd3}},
		},
	}

	t.Run("tool-specific and wildcard both match", func(t *testing.T) {
		cmds := resolveCommands(cfg, types.HookEventPreToolUse, "Bash")
		if len(cmds) != 2 {
			t.Fatalf("expected 2 commands, got %d", len(cmds))
		}
		if cmds[0].Command != cmd1.Command || cmds[1].Command != cmd2.Command {
			t.Errorf("unexpected commands: %+v", cmds)
		}
	})

	t.Run("wildcard matches all tools", func(t *testing.T) {
		cmds := resolveCommands(cfg, types.HookEventPreToolUse, "FileRead")
		if len(cmds) != 1 || cmds[0].Command != cmd2.Command {
			t.Errorf("expected wildcard only, got %+v", cmds)
		}
	})

	t.Run("event not found returns nil", func(t *testing.T) {
		cmds := resolveCommands(cfg, types.HookEventPostToolUse, "Bash")
		if cmds != nil {
			t.Errorf("expected nil, got %+v", cmds)
		}
	})
}

func makeInputWithCommand(toolName, command string) HookInput {
	raw, _ := json.Marshal(map[string]string{"command": command})
	return HookInput{
		ToolName:      toolName,
		ToolInputJSON: string(raw),
	}
}

func Test_matchesIfCondition(t *testing.T) {
	cases := []struct {
		name   string
		ifCond string
		input  HookInput
		want   bool
	}{
		{"empty condition always matches", "", makeInputWithCommand("Bash", "git status"), true},
		{"non-tool event always matches", "Bash", HookInput{ToolName: ""}, true},
		{"tool name matches, no content", "Bash", makeInputWithCommand("Bash", "ls"), true},
		{"tool name case-insensitive", "bash", makeInputWithCommand("Bash", "ls"), true},
		{"tool name mismatch", "PowerShell", makeInputWithCommand("Bash", "git status"), false},
		{"tool name + command wildcard matches", "Bash(git *)", makeInputWithCommand("Bash", "git status"), true},
		{"tool name + command wildcard no match", "Bash(git *)", makeInputWithCommand("Bash", "ls -la"), false},
		{"tool name + exact command", "Bash(npm)", makeInputWithCommand("Bash", "npm"), true},
		{"tool name + exact command mismatch", "Bash(npm)", makeInputWithCommand("Bash", "npm install"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := matchesIfCondition(c.ifCond, c.input)
			if got != c.want {
				t.Errorf("matchesIfCondition(%q, %+v) = %v, want %v", c.ifCond, c.input, got, c.want)
			}
		})
	}
}

// 타입 임포트 사용 확인
var _ = types.HookEventPreToolUse
