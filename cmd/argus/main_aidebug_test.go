package main

import "testing"

func TestParseAIDebugAllowDecision(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    bool
		wantErr bool
	}{
		{name: "allow token", raw: "allow", want: true},
		{name: "approve sentence", raw: "decision: approve", want: true},
		{name: "deny token", raw: "deny", want: false},
		{name: "reject sentence", raw: "please reject this", want: false},
		{name: "unknown", raw: "maybe", wantErr: true},
	}

	for _, tc := range tests {
		got, err := parseAIDebugAllowDecision(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestShouldAIDebugAutoInjectPrompt(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   bool
	}{
		{name: "password blocked", prompt: "Password: ", want: false},
		{name: "korean password blocked", prompt: "비밀번호를 입력하세요", want: false},
		{name: "selection allowed", prompt: "run step 1/2? [y/N]", want: true},
		{name: "empty blocked", prompt: "   ", want: false},
	}

	for _, tc := range tests {
		got := shouldAIDebugAutoInjectPrompt(tc.prompt)
		if got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestNormalizeAIDebugInjectedInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain", raw: "y", want: "y"},
		{name: "prefixed", raw: "input: yes", want: "yes"},
		{name: "quoted", raw: "\"n\"", want: "n"},
		{name: "multiline", raw: "\n\nresponse: allow\nextra", want: "allow"},
	}

	for _, tc := range tests {
		got := normalizeAIDebugInjectedInput(tc.raw)
		if got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}
