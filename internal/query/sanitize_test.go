package query

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
)

func TestSanitizeMessagesForStorage_RedactsSecrets(t *testing.T) {
	secret := "pw123456"
	msgs := []llm.Message{
		{
			Role: llm.RoleAssistant,
			Content: []llm.ContentBlock{
				{
					Type:  llm.ContentToolUse,
					Name:  "bash",
					Input: json.RawMessage(`{"command":"id","password":"` + secret + `"}`),
				},
			},
		},
		{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{
				{
					Type: llm.ContentToolResult,
					Name: "bash",
					Text: "echo " + secret,
				},
			},
		},
	}

	out := SanitizeMessagesForStorage(msgs)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal sanitized messages: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, secret) {
		t.Fatalf("secret leaked after sanitization: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction placeholder, got: %s", got)
	}
}

func TestSanitizeTraceData_RedactsRawMessageAndText(t *testing.T) {
	secret := "root_pw_77"
	data := map[string]any{
		"input":  json.RawMessage(`{"root_password":"` + secret + `"}`),
		"output": "password=" + secret,
	}
	safe := sanitizeTraceData(data, []string{secret})
	raw, err := json.Marshal(safe)
	if err != nil {
		t.Fatalf("marshal sanitized trace data: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("secret leaked from trace data: %s", string(raw))
	}
}
