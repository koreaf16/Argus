package agent

import "testing"

func TestDetectIdleInputHintForHereDoc(t *testing.T) {
	cmd := "cat >> ~/.bash_profile << EOF\nexport ORACLE_HOME=/u01/app/oracle\nEOF"
	got := DetectIdleInputHint(cmd)
	if got == "" {
		t.Fatal("expected here-doc hint, got empty string")
	}
}

func TestDetectIdleInputHintWithoutHereDoc(t *testing.T) {
	if got := DetectIdleInputHint("printf '%s\\n' hello"); got != "" {
		t.Fatalf("expected no hint, got %q", got)
	}
}
