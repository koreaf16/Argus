package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const requestLogSeparator = "##############################"

var requestLogMu sync.Mutex

func appendRequestLog(provider, model string, req Request) {
	entry := formatRequestLogEntry(provider, model, req, time.Now())

	requestLogMu.Lock()
	defer requestLogMu.Unlock()

	f, err := os.OpenFile("llm.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(entry)
}

func formatRequestLogEntry(provider, model string, req Request, now time.Time) string {
	var sb strings.Builder
	sb.WriteString(requestLogSeparator)
	sb.WriteString("\n")
	sb.WriteString("LLM REQUEST\n")
	sb.WriteString("Time: ")
	sb.WriteString(now.Format(time.RFC3339))
	sb.WriteString("\n")
	if strings.TrimSpace(provider) != "" {
		sb.WriteString("Provider: ")
		sb.WriteString(strings.TrimSpace(provider))
		sb.WriteString("\n")
	}
	if strings.TrimSpace(model) != "" {
		sb.WriteString("Model: ")
		sb.WriteString(strings.TrimSpace(model))
		sb.WriteString("\n")
	}
	if req.MaxTokens > 0 {
		sb.WriteString(fmt.Sprintf("Max tokens: %d\n", req.MaxTokens))
	}
	if req.Thinking != nil {
		sb.WriteString("Thinking:\n")
		if strings.TrimSpace(req.Thinking.Type) != "" {
			sb.WriteString("  type: ")
			sb.WriteString(strings.TrimSpace(req.Thinking.Type))
			sb.WriteString("\n")
		}
		if req.Thinking.BudgetTokens > 0 {
			sb.WriteString(fmt.Sprintf("  budget tokens: %d\n", req.Thinking.BudgetTokens))
		}
	}
	sb.WriteString("\n")

	renderSystemBlocks(&sb, req.System)
	renderMessages(&sb, req.Messages)
	renderToolSpecs(&sb, req.Tools)

	sb.WriteString("\n")
	return sb.String()
}

func renderSystemBlocks(sb *strings.Builder, blocks []SystemBlock) {
	sb.WriteString("SYSTEM PROMPT\n")
	if len(blocks) == 0 {
		sb.WriteString("(none)\n\n")
		return
	}
	for i, block := range blocks {
		sb.WriteString(fmt.Sprintf("System block %d", i+1))
		if strings.TrimSpace(block.Type) != "" {
			sb.WriteString(" (")
			sb.WriteString(strings.TrimSpace(block.Type))
			sb.WriteString(")")
		}
		sb.WriteString("\n")
		writeTextBlock(sb, block.Text, "  ")
		if len(block.CacheControl) > 0 {
			sb.WriteString("  Cache control:\n")
			renderAny(sb, block.CacheControl, "    ")
		}
		sb.WriteString("\n")
	}
}

func renderMessages(sb *strings.Builder, messages []Message) {
	sb.WriteString("CONVERSATION MESSAGES\n")
	if len(messages) == 0 {
		sb.WriteString("(none)\n\n")
		return
	}
	for i, msg := range messages {
		sb.WriteString(fmt.Sprintf("Message %d: %s\n", i+1, msg.Role))
		if len(msg.Content) == 0 {
			sb.WriteString("  (empty)\n\n")
			continue
		}
		for _, block := range msg.Content {
			renderContentBlock(sb, block, "  ")
		}
		sb.WriteString("\n")
	}
}

func renderContentBlock(sb *strings.Builder, block ContentBlock, indent string) {
	switch block.Type {
	case ContentText:
		sb.WriteString(indent)
		sb.WriteString("Text:\n")
		writeTextBlock(sb, block.Text, indent+"  ")
	case ContentThinking:
		sb.WriteString(indent)
		sb.WriteString("Thinking:\n")
		writeTextBlock(sb, block.Text, indent+"  ")
	case ContentToolUse:
		sb.WriteString(indent)
		sb.WriteString("Tool call:\n")
		if strings.TrimSpace(block.ID) != "" {
			sb.WriteString(indent)
			sb.WriteString("  id: ")
			sb.WriteString(strings.TrimSpace(block.ID))
			sb.WriteString("\n")
		}
		if strings.TrimSpace(block.Name) != "" {
			sb.WriteString(indent)
			sb.WriteString("  name: ")
			sb.WriteString(strings.TrimSpace(block.Name))
			sb.WriteString("\n")
		}
		sb.WriteString(indent)
		sb.WriteString("  input:\n")
		renderRawJSON(sb, block.Input, indent+"    ")
	case ContentToolResult:
		sb.WriteString(indent)
		sb.WriteString("Tool result:\n")
		if strings.TrimSpace(block.ToolUseID) != "" {
			sb.WriteString(indent)
			sb.WriteString("  tool use id: ")
			sb.WriteString(strings.TrimSpace(block.ToolUseID))
			sb.WriteString("\n")
		}
		if strings.TrimSpace(block.Name) != "" {
			sb.WriteString(indent)
			sb.WriteString("  name: ")
			sb.WriteString(strings.TrimSpace(block.Name))
			sb.WriteString("\n")
		}
		sb.WriteString(indent)
		sb.WriteString(fmt.Sprintf("  error: %t\n", block.IsError))
		sb.WriteString(indent)
		sb.WriteString("  output:\n")
		writeTextBlock(sb, block.Text, indent+"    ")
	default:
		sb.WriteString(indent)
		sb.WriteString("Content block: ")
		sb.WriteString(string(block.Type))
		sb.WriteString("\n")
		writeTextBlock(sb, block.Text, indent+"  ")
		if len(block.Input) > 0 {
			sb.WriteString(indent)
			sb.WriteString("  input:\n")
			renderRawJSON(sb, block.Input, indent+"    ")
		}
	}
}

func renderToolSpecs(sb *strings.Builder, tools []ToolSpec) {
	sb.WriteString("AVAILABLE TOOLS\n")
	if len(tools) == 0 {
		sb.WriteString("(none)\n")
		return
	}
	for i, spec := range tools {
		sb.WriteString(fmt.Sprintf("Tool %d: %s\n", i+1, strings.TrimSpace(spec.Name)))
		if strings.TrimSpace(spec.Type) != "" {
			sb.WriteString("  type: ")
			sb.WriteString(strings.TrimSpace(spec.Type))
			sb.WriteString("\n")
		}
		if strings.TrimSpace(spec.Description) != "" {
			sb.WriteString("  description:\n")
			writeTextBlock(sb, spec.Description, "    ")
		}
		if len(spec.InputSchema) > 0 {
			sb.WriteString("  input schema:\n")
			renderAny(sb, spec.InputSchema, "    ")
		}
		sb.WriteString("\n")
	}
}

func renderRawJSON(sb *strings.Builder, raw json.RawMessage, indent string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		sb.WriteString(indent)
		sb.WriteString("(empty)\n")
		return
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		writeTextBlock(sb, string(trimmed), indent)
		return
	}
	renderAny(sb, v, indent)
}

func renderAny(sb *strings.Builder, v any, indent string) {
	switch val := v.(type) {
	case map[string]any:
		if len(val) == 0 {
			sb.WriteString(indent)
			sb.WriteString("(empty object)\n")
			return
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := val[key]
			if isScalar(child) {
				sb.WriteString(indent)
				sb.WriteString(key)
				sb.WriteString(": ")
				sb.WriteString(formatScalar(child))
				sb.WriteString("\n")
				continue
			}
			sb.WriteString(indent)
			sb.WriteString(key)
			sb.WriteString(":\n")
			renderAny(sb, child, indent+"  ")
		}
	case []any:
		if len(val) == 0 {
			sb.WriteString(indent)
			sb.WriteString("(empty list)\n")
			return
		}
		for _, item := range val {
			if isScalar(item) {
				sb.WriteString(indent)
				sb.WriteString("- ")
				sb.WriteString(formatScalar(item))
				sb.WriteString("\n")
				continue
			}
			sb.WriteString(indent)
			sb.WriteString("-\n")
			renderAny(sb, item, indent+"  ")
		}
	case []string:
		if len(val) == 0 {
			sb.WriteString(indent)
			sb.WriteString("(empty list)\n")
			return
		}
		for _, item := range val {
			sb.WriteString(indent)
			sb.WriteString("- ")
			sb.WriteString(item)
			sb.WriteString("\n")
		}
	case map[string]string:
		converted := make(map[string]any, len(val))
		for k, v := range val {
			converted[k] = v
		}
		renderAny(sb, converted, indent)
	default:
		if isScalar(val) {
			sb.WriteString(indent)
			sb.WriteString(formatScalar(val))
			sb.WriteString("\n")
			return
		}
		sb.WriteString(indent)
		sb.WriteString(fmt.Sprint(val))
		sb.WriteString("\n")
	}
}

func isScalar(v any) bool {
	switch v.(type) {
	case nil, string, bool, json.Number, float64, float32, int, int64, int32, uint, uint64, uint32:
		return true
	default:
		return false
	}
}

func formatScalar(v any) string {
	switch val := v.(type) {
	case nil:
		return "(null)"
	case string:
		if strings.TrimSpace(val) == "" {
			return "(empty)"
		}
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case json.Number:
		return val.String()
	default:
		return fmt.Sprint(val)
	}
}

func writeTextBlock(sb *strings.Builder, text string, indent string) {
	if text == "" {
		sb.WriteString(indent)
		sb.WriteString("(empty)\n")
		return
	}
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		sb.WriteString(indent)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}
