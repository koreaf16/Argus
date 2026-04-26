package query

import (
	"encoding/json"

	"github.com/koreaf16/argus/internal/redact"
	"github.com/koreaf16/argus/internal/services/llm"
)

func sanitizeRequestForTrace(req *llm.Request) any {
	if req == nil {
		return nil
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil
	}
	return sanitizeTraceData(decoded, nil)
}

func sanitizeToolInput(raw json.RawMessage) json.RawMessage {
	secrets := extractToolSecrets(raw)
	return redact.RedactJSONWithSecrets(raw, secrets)
}

func extractToolSecrets(raw json.RawMessage) []string {
	return redact.ExtractSecretsFromJSON(raw)
}

func sanitizeTextWithSecrets(text string, secrets []string) string {
	return redact.RedactTextWithSecrets(text, secrets)
}

func sanitizeTraceData(data any, secrets []string) any {
	switch v := data.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, value := range v {
			out[k] = sanitizeTraceData(value, secrets)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, value := range v {
			out[i] = sanitizeTraceData(value, secrets)
		}
		return out
	case string:
		return sanitizeTextWithSecrets(v, secrets)
	case json.RawMessage:
		if len(v) == 0 {
			return v
		}
		return redact.RedactJSONWithSecrets(v, secrets)
	default:
		return redact.RedactAny(data)
	}
}

func sanitizeToolUseForDisplay(tc llm.ToolUseStart) llm.ToolUseStart {
	tc.Input = sanitizeToolInput(tc.Input)
	return tc
}

func sanitizeToolCallsForStorage(calls []llm.ToolUseStart) []llm.ToolUseStart {
	out := make([]llm.ToolUseStart, len(calls))
	for i := range calls {
		out[i] = sanitizeToolUseForDisplay(calls[i])
	}
	return out
}

// SanitizeMessagesForStorage redacts secrets from tool inputs and text fields
// before persisting session snapshots.
func SanitizeMessagesForStorage(messages []llm.Message) []llm.Message {
	out := cloneMessages(messages)
	secrets := make([]string, 0, 4)
	for i := range out {
		for j := range out[i].Content {
			block := out[i].Content[j]
			if block.Type == llm.ContentToolUse {
				secrets = append(secrets, extractToolSecrets(block.Input)...)
			}
		}
	}
	for i := range out {
		for j := range out[i].Content {
			block := &out[i].Content[j]
			block.Text = sanitizeTextWithSecrets(block.Text, secrets)
			if block.Type == llm.ContentToolUse {
				block.Input = redact.RedactJSONWithSecrets(block.Input, secrets)
			}
		}
	}
	return out
}
