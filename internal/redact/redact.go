package redact

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

const Placeholder = "[REDACTED]"

var (
	// JSON 키-값 패턴: "password": "..."
	jsonKeyValueRegex = regexp.MustCompile(`(?i)("(?:password|root_password|passphrase|secret|token|api_key|apikey|authorization|private_key|client_secret|access_key|secret_key)"\s*:\s*)"([^"]*)"`)
	// 평문 키=값 패턴: password=..., API_KEY=...
	kvPairRegex = regexp.MustCompile(`(?i)\b(password|root_password|passphrase|api[_-]?key|secret[_-]?key|access[_-]?key|client[_-]?secret|token|secret|private[_-]?key)\b(\s*[:=]\s*)([^\s,;"'\r\n]{4,})`)
	// JWT (header.payload.signature)
	jwtRegex = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{2,}(?:\.[A-Za-z0-9_-]{2,}){1,2}`)
	// HTTP Authorization: Bearer <token>
	bearerRegex = regexp.MustCompile(`(?i)\bBearer\s+([A-Za-z0-9\-._~+/]{8,}=*)`)
	// AWS Access Key ID (AKIA...)
	awsKeyRegex = regexp.MustCompile(`\b(AKIA[A-Z0-9]{16})\b`)
)

var sensitiveKeys = map[string]struct{}{
	"password":      {},
	"root_password": {},
	"passphrase":    {},
	"secret":        {},
	"token":         {},
	"api_key":       {},
	"apikey":        {},
	"authorization": {},
	"private_key":   {},
	"secret_key":    {},
	"access_key":    {},
	"client_secret": {},
}

func RedactJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return json.RawMessage([]byte(RedactText(string(raw))))
	}
	redacted := redactAny(payload)
	out, err := json.Marshal(redacted)
	if err != nil {
		return json.RawMessage([]byte(RedactText(string(raw))))
	}
	return out
}

func RedactJSONWithSecrets(raw json.RawMessage, secrets []string) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	redacted := RedactJSON(raw)
	return json.RawMessage([]byte(RedactTextWithSecrets(string(redacted), secrets)))
}

func ExtractSecretsFromJSON(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	out := make([]string, 0, 4)
	collectSecrets(payload, &out)
	return uniqueSecrets(out)
}

func RedactText(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	out := jsonKeyValueRegex.ReplaceAllString(s, `${1}"`+Placeholder+`"`)
	out = kvPairRegex.ReplaceAllString(out, `${1}${2}`+Placeholder)
	out = jwtRegex.ReplaceAllString(out, Placeholder)
	out = bearerRegex.ReplaceAllString(out, "Bearer "+Placeholder)
	out = awsKeyRegex.ReplaceAllString(out, Placeholder)
	return out
}

func RedactTextWithSecrets(s string, secrets []string) string {
	out := RedactText(s)
	uniq := uniqueSecrets(secrets)
	sort.SliceStable(uniq, func(i, j int) bool {
		return len(uniq[i]) > len(uniq[j])
	})
	for _, sec := range uniq {
		if len(sec) < 3 {
			continue
		}
		out = strings.ReplaceAll(out, sec, Placeholder)
	}
	return out
}

func RedactAny(v any) any {
	return redactAny(v)
}

func redactAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			key := strings.ToLower(strings.TrimSpace(k))
			if _, ok := sensitiveKeys[key]; ok {
				out[k] = Placeholder
				continue
			}
			out[k] = redactAny(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = redactAny(x[i])
		}
		return out
	case string:
		return RedactText(x)
	default:
		return v
	}
}

func collectSecrets(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			key := strings.ToLower(strings.TrimSpace(k))
			if _, ok := sensitiveKeys[key]; ok {
				if s, ok := val.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" && s != Placeholder {
						*out = append(*out, s)
					}
				}
			}
			collectSecrets(val, out)
		}
	case []any:
		for _, item := range x {
			collectSecrets(item, out)
		}
	}
}

func uniqueSecrets(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || s == Placeholder {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
