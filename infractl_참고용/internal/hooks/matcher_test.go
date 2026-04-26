// Package hooks
// File: matcher_test.go
// Description: matcher 패턴 파서 및 매칭 단위 테스트
// Responsibility: parseMatcher 와 matchSingle 의 다양한 패턴 케이스 검증

package hooks

import (
	"testing"
)

func TestParseMatcher(t *testing.T) {
	cases := []struct {
		input    string
		wantTool string
		wantGlob string
	}{
		{"Bash", "Bash", ""},
		{"Bash(*)", "Bash", "*"},
		{"Bash(git *)", "Bash", "git *"},
		{"Write(*.yaml)", "Write", "*.yaml"},
		{"Write( *.json )", "Write", "*.json"},
		{"*", "*", ""},
		{"SSH(prod-*)", "SSH", "prod-*"},
		{"Bash(rm -rf /*)", "Bash", "rm -rf /*"},
	}
	for _, c := range cases {
		p := parseMatcher(c.input)
		if p.toolName != c.wantTool {
			t.Errorf("parseMatcher(%q).toolName = %q, want %q", c.input, p.toolName, c.wantTool)
		}
		if p.argGlob != c.wantGlob {
			t.Errorf("parseMatcher(%q).argGlob = %q, want %q", c.input, p.argGlob, c.wantGlob)
		}
	}
}

func makeInput(tool, command string) HookInput {
	return HookInput{
		Tool:  tool,
		Input: map[string]interface{}{"command": command},
	}
}

func makeFileInput(tool, path string) HookInput {
	return HookInput{
		Tool:  tool,
		Input: map[string]interface{}{"file_path": path},
	}
}

func TestMatchSingle(t *testing.T) {
	cases := []struct {
		matcher string
		input   HookInput
		want    bool
	}{
		// 도구 이름만
		{"Bash", makeInput("Bash", "ls -la"), true},
		{"Bash", makeInput("Write", "ls -la"), false},
		// 와일드카드 도구
		{"*", makeInput("Bash", "ls"), true},
		{"*", makeInput("Write", "foo.txt"), true},
		// 명령 glob
		{"Bash(*)", makeInput("Bash", "anything"), true},
		{"Bash(git *)", makeInput("Bash", "git status"), true},
		{"Bash(git *)", makeInput("Bash", "ls -la"), false},
		{"Bash(rm *)", makeInput("Bash", "rm -rf /tmp"), true},
		{"Bash(rm *)", makeInput("Bash", "echo hi"), false},
		// 파일 경로 glob
		{"Write(*.yaml)", makeFileInput("Write", "config.yaml"), true},
		{"Write(*.yaml)", makeFileInput("Write", "config.json"), false},
		{"Write(*.json)", makeFileInput("Write", "schema.json"), true},
		// SSH 패턴
		{"SSH(prod-*)", HookInput{Tool: "SSH", Input: map[string]interface{}{"command": "prod-db"}}, true},
		{"SSH(prod-*)", HookInput{Tool: "SSH", Input: map[string]interface{}{"command": "dev-db"}}, false},
		// 입력 없음 — "*" glob은 빈 문자열도 매칭
		{"Bash(*)", HookInput{Tool: "Bash", Input: nil}, true},
		// 도구 대소문자 비구분
		{"bash", makeInput("Bash", "ls"), true},
	}

	for _, c := range cases {
		got := matchSingle(c.matcher, c.input)
		if got != c.want {
			t.Errorf("matchSingle(%q, tool=%q, cmd=%q) = %v, want %v",
				c.matcher, c.input.Tool, extractMatchTarget(c.input), got, c.want)
		}
	}
}

func TestExtractMatchTarget(t *testing.T) {
	cases := []struct {
		in   HookInput
		want string
	}{
		{HookInput{Input: map[string]interface{}{"command": "ls"}}, "ls"},
		{HookInput{Input: map[string]interface{}{"file_path": "/etc/foo.yaml"}}, "/etc/foo.yaml"},
		{HookInput{Input: map[string]interface{}{"path": "/tmp/x"}}, "/tmp/x"},
		{HookInput{Input: nil}, ""},
		{HookInput{Input: map[string]interface{}{"num": 42}}, ""},
	}
	for _, c := range cases {
		got := extractMatchTarget(c.in)
		if got != c.want {
			t.Errorf("extractMatchTarget(%v) = %q, want %q", c.in.Input, got, c.want)
		}
	}
}
