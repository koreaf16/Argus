package powershell

import (
	"encoding/json"
	"testing"
)

func TestPowerShellToolIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	tool := &PowerShellTool{}
	tests := []struct {
		name  string
		input json.RawMessage
		want  bool
	}{
		{name: "Get-Content is safe", input: json.RawMessage(`{"command":"Get-Content README.md"}`), want: true},
		{name: "pipeline read-only is safe", input: json.RawMessage(`{"command":"Get-ChildItem | Select-String -Pattern TODO"}`), want: true},
		{name: "Set-Content is unsafe", input: json.RawMessage(`{"command":"Set-Content out.txt hi"}`), want: false},
		{name: "Remove-Item is unsafe", input: json.RawMessage(`{"command":"Remove-Item out.txt -Force"}`), want: false},
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
