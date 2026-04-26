package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactJSON_ReplacesSensitiveKeys(t *testing.T) {
	raw := json.RawMessage(`{"command":"echo hi","password":"s3cr3t","nested":{"root_password":"pw2"}}`)
	out := RedactJSON(raw)
	got := string(out)
	if strings.Contains(got, "s3cr3t") || strings.Contains(got, "pw2") {
		t.Fatalf("secret leaked in redacted json: %s", got)
	}
	if !strings.Contains(got, Placeholder) {
		t.Fatalf("expected placeholder in redacted json: %s", got)
	}
}

func TestExtractSecretsFromJSON(t *testing.T) {
	raw := json.RawMessage(`{"password":"abc123","root_password":"xyz789","noop":"ok"}`)
	secrets := ExtractSecretsFromJSON(raw)
	joined := strings.Join(secrets, ",")
	if !strings.Contains(joined, "abc123") || !strings.Contains(joined, "xyz789") {
		t.Fatalf("expected extracted secrets, got: %v", secrets)
	}
}

func TestRedactTextWithSecrets(t *testing.T) {
	in := `password=abc123 token:xyz789`
	out := RedactTextWithSecrets(in, []string{"abc123", "xyz789"})
	if strings.Contains(out, "abc123") || strings.Contains(out, "xyz789") {
		t.Fatalf("secret leaked in text: %s", out)
	}
	if strings.Count(out, Placeholder) < 2 {
		t.Fatalf("expected placeholder replacements, got: %s", out)
	}
}
