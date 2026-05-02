package bash

import (
	"encoding/json"
	"testing"
)

func TestBashToolIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	tool := &BashTool{}
	tests := []struct {
		name  string
		input json.RawMessage
		want  bool
	}{
		{name: "ripgrep is safe", input: json.RawMessage(`{"command":"rg TODO ."}`), want: true},
		{name: "git show is safe", input: json.RawMessage(`{"command":"git show HEAD~1"}`), want: true},
		{name: "python can mutate so unsafe", input: json.RawMessage(`{"command":"python -c \"open('x','w').write('a')\""}`), want: false},
		{name: "find delete is unsafe", input: json.RawMessage(`{"command":"find . -name '*.tmp' -delete"}`), want: false},
		{name: "gh repo edit is unsafe", input: json.RawMessage(`{"command":"gh repo edit --description hi"}`), want: false},
		{name: "gh repo view is safe", input: json.RawMessage(`{"command":"gh repo view owner/repo"}`), want: true},
		{name: "redirect write is unsafe", input: json.RawMessage(`{"command":"echo hi > out.txt"}`), want: false},
		{name: "install command is unsafe", input: json.RawMessage(`{"command":"npm install left-pad"}`), want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tool.IsConcurrencySafe(tc.input)
			if got != tc.want {
				t.Fatalf("IsConcurrencySafe() = %v, want %v", got, tc.want)
			}
		})
	}
}
