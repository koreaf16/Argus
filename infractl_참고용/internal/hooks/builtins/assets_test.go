// Package builtins
// File: assets_test.go
// Description: embed 로드 및 Unbox 단위 테스트

package builtins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellValidatorPrompt_Embedded(t *testing.T) {
	s, err := ShellValidatorPrompt()
	if err != nil {
		t.Fatalf("ShellValidatorPrompt: %v", err)
	}
	if !strings.Contains(s, "$ARGUMENTS") {
		t.Error("shell_validator.md should contain $ARGUMENTS placeholder")
	}
	if !strings.Contains(s, "decision") {
		t.Error("shell_validator.md should document decision JSON schema")
	}
}

func TestLookupPrompt_Known(t *testing.T) {
	s, ok := LookupPrompt("shell_validator")
	if !ok {
		t.Fatal("LookupPrompt('shell_validator') should succeed")
	}
	if s == "" {
		t.Error("shell_validator prompt should not be empty")
	}
}

func TestLookupPrompt_Unknown(t *testing.T) {
	if _, ok := LookupPrompt("does_not_exist"); ok {
		t.Error("unknown prompt should return ok=false")
	}
}

func TestUnbox_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Unbox(dir); err != nil {
		t.Fatalf("Unbox: %v", err)
	}

	// system_risk.sh 확인
	sh := filepath.Join(dir, "system_risk.sh")
	info, err := os.Stat(sh)
	if err != nil {
		t.Fatalf("stat %s: %v", sh, err)
	}
	if info.Size() == 0 {
		t.Error("system_risk.sh should not be empty")
	}

	// shell_validator.md 확인 (agent/ 하위)
	md := filepath.Join(dir, "agent", "shell_validator.md")
	if _, err := os.Stat(md); err != nil {
		t.Fatalf("stat %s: %v", md, err)
	}
}

func TestUnbox_EmptyDst(t *testing.T) {
	if err := Unbox(""); err == nil {
		t.Error("Unbox with empty destination should error")
	}
}
