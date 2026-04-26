package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

const llmLogPath = "llm.log"
const llmRawLogPath = "llmraw.log"

var rawLogEnabled bool

// SetRawLogEnabled는 llmraw.log 기록 여부를 설정한다.
func SetRawLogEnabled(enabled bool) {
	rawLogEnabled = enabled
}

type logBlock struct {
	Direction string
	Role      string
	Model     string
	Text      string
}

func LogRequestJSON(model string, data []byte) {
	var req struct {
		Messages []struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCalls  []ToolCall      `json:"tool_calls"`
			ToolCallID string          `json:"tool_call_id"`
			Name       string          `json:"name"`
		} `json:"messages"`
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		logToFile(model, "SND", string(data))
		return
	}

	var sb strings.Builder
	for i, msg := range req.Messages {
		content, _ := jsonContentToString(msg.Content)

		// 각 메시지를 구분하는 내부 헤더 추가
		fmt.Fprintf(&sb, "--- [MSG #%d: %s] ---\n", i, strings.ToUpper(msg.Role))
		if strings.TrimSpace(content) != "" {
			fmt.Fprintf(&sb, "%s\n", strings.TrimSpace(content))
		}
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&sb, "[Tool Call Request] %s\n%s\n", tc.Function.Name, prettyJSON(tc.Function.Arguments))
			}
		}
		if msg.ToolCallID != "" || msg.Name != "" {
			id := msg.ToolCallID
			if id == "" {
				id = msg.Name
			}
			fmt.Fprintf(&sb, "[Tool Result ID: %s]\n", id)
		}
		fmt.Fprintf(&sb, "\n")
	}

	if len(req.Tools) > 0 {
		fmt.Fprintf(&sb, "--- [FULL TOOL DEFINITIONS] ---\n")
		for _, t := range req.Tools {
			fmt.Fprintf(&sb, "%s\n", prettyJSON(string(t)))
		}
	}

	writeLogBlocks([]logBlock{{
		Direction: "SND",
		Role:      "CONTEXT",
		Model:     model,
		Text:      sb.String(),
	}})
}

func LogResponseJSON(model string, data []byte) {
	blocks := make([]logBlock, 0, 3)

	content, ok := extractJSONLogContent(string(data))
	if ok && strings.TrimSpace(content) != "" {
		// Thinking 부분 추출 및 별도 블록 추가
		if thinking := extractThinkingText(content); thinking != "" {
			blocks = append(blocks, logBlock{
				Direction: "RESPONSE",
				Role:      "thinking",
				Model:     model,
				Text:      thinking,
			})
		}

		// Assistant 본문 추가 (Thinking 상단 스트립)
		cleanContent := stripThinkingText(content)
		if cleanContent != "" {
			blocks = append(blocks, logBlock{
				Direction: "RESPONSE",
				Role:      "assistant",
				Model:     model,
				Text:      cleanContent,
			})
		}

		// 인라인 도구 호출 추출
		if mCalls, _ := extractInlineToolCalls(cleanContent); len(mCalls) > 0 {
			var sb strings.Builder
			for _, tc := range mCalls {
				fmt.Fprintf(&sb, "[Tool Call] %s\n%s\n", tc.Function.Name, prettyJSON(tc.Function.Arguments))
			}
			blocks = append(blocks, logBlock{
				Direction: "RESPONSE",
				Role:      "tool_call",
				Model:     model,
				Text:      strings.TrimSpace(sb.String()),
			})
		}
	}

	// 표준 JSON tool_calls 추출
	if toolCallText, ok := extractToolCallsLog(string(data)); ok {
		blocks = append(blocks, logBlock{
			Direction: "RESPONSE",
			Role:      "tool_call",
			Model:     model,
			Text:      toolCallText,
		})
	}

	if len(blocks) > 0 {
		writeLogBlocks(blocks)
	}
}

// extractToolCallsLog는 비스트리밍 응답 JSON에서 tool_calls를 파싱하여 로그 텍스트를 반환한다.
func extractToolCallsLog(text string) (string, bool) {
	if text == "" || !strings.HasPrefix(text, "{") {
		return "", false
	}

	var raw struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return "", false
	}

	var sb strings.Builder
	for _, choice := range raw.Choices {
		for _, tc := range choice.Message.ToolCalls {
			if tc.Function.Name == "" {
				continue
			}
			fmt.Fprintf(&sb, "[Tool Call] %s\n%s\n", tc.Function.Name, prettyJSON(tc.Function.Arguments))
		}
	}
	result := strings.TrimSpace(sb.String())
	if result == "" {
		return "", false
	}
	return result, true
}

// logToolCalls는 ToolCall 슬라이스를 llm.log에 기록한다 (스트리밍 응답용).
func logToolCalls(model string, calls []ToolCall, raw string) {
	if len(calls) == 0 && raw == "" {
		return
	}
	var sb strings.Builder
	if raw != "" {
		fmt.Fprintf(&sb, "--- RAW SOURCE ---\n%s\n\n", strings.TrimSpace(raw))
	}
	for _, tc := range calls {
		fmt.Fprintf(&sb, "[Tool Call] %s\n%s\n", tc.Function.Name, prettyJSON(tc.Function.Arguments))
	}
	writeLogBlocks([]logBlock{{
		Direction: "RESPONSE",
		Role:      "tool_call",
		Model:     model,
		Text:      strings.TrimSpace(sb.String()),
	}})
}

// LogToolResult는 도구 실행 결과를 llm.log에 기록한다.
func LogToolResult(toolName, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	writeLogBlocks([]logBlock{{
		Direction: "RESPONSE",
		Role:      "tool",
		Model:     toolName,
		Text:      strings.TrimSpace(content),
	}})
}

// LogSystemEvent는 에이전트의 프레임워크 결정(예: 티어 선택)을 llm.log에 기록한다.
func LogSystemEvent(title, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	writeLogBlocks([]logBlock{{
		Direction: "SYSTEM",
		Role:      "event",
		Model:     title,
		Text:      strings.TrimSpace(content),
	}})
}

// prettyJSON은 JSON 문자열을 들여쓰기 형태로 포맷한다. 파싱 실패 시 원본을 반환한다.
func prettyJSON(raw string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return raw
	}
	return string(b)
}

// logToFile records only readable message content in llm.log.
func logToFile(model, kind string, text string) {
	direction, role := logKind(kind)
	writeLogBlocks([]logBlock{{
		Direction: direction,
		Role:      role,
		Model:     model,
		Text:      normalizeLLMLogText(text),
	}})
}

func writeLogBlocks(blocks []logBlock) {
	if err := ensureUTF8BOM(llmLogPath); err != nil {
		return
	}
	f, err := os.OpenFile(llmLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	for _, block := range blocks {
		text := block.Text
		if strings.TrimSpace(text) == "" {
			continue
		}
		if !utf8.ValidString(text) {
			text = strings.ToValidUTF8(text, "\uFFFD")
		}
		label := resolveBlockLabel(block)
		// 20개의 ❌를 사용하여 더 명확한 구분선을 만든다.
		const sep = "❌❌❌❌❌❌❌❌❌❌❌❌❌❌❌❌❌❌❌❌"
		header := fmt.Sprintf("%s %s [%d chars]", sep, label, len(text))
		if block.Model != "" {
			header += " | " + block.Model
		}
		header += " " + sep
		fmt.Fprintf(f, "\n%s\n%s\n%s\n", header, strings.TrimSpace(text), sep)
	}
}

// resolveBlockLabel은 logBlock의 Direction과 Role을 조합해 로그 헤더 레이블을 결정한다.
func resolveBlockLabel(block logBlock) string {
	switch block.Direction {
	case "SND":
		switch block.Role {
		case "CONTEXT":
			return "REQUEST CONTEXT"
		case "tools":
			return "TOOLS"
		case "":
			return "PROMPT"
		default:
			return "PROMPT [" + block.Role + "]"
		}
	case "RESPONSE":
		if block.Role == "tool_call" {
			return "TOOL CALL"
		}
		if block.Role == "tool" {
			return "TOOL RESPONSE"
		}
		if block.Role == "thinking" {
			return "THINKING"
		}
		if block.Role == "assistant_raw" {
			return "RESPONSE RAW"
		}
		return "RESPONSE"
	case "ERROR":
		return "ERROR"
	case "SYSTEM":
		if block.Role == "event" {
			return "SYSTEM EVENT"
		}
		return "SYSTEM"
	default:
		if block.Direction != "" {
			return block.Direction
		}
		return "UNKNOWN"
	}
}

func logKind(kind string) (direction string, role string) {
	switch strings.ToUpper(kind) {
	case "USER":
		return "SND", "user"
	case "ASSISTANT", "RESPONSE":
		return "RESPONSE", "assistant"
	case "TOOL":
		return "SND", "tool"
	case "ERROR":
		return "ERROR", ""
	default:
		d := strings.ToUpper(kind)
		if d == "SEND" {
			d = "SND"
		}
		return d, ""
	}
}

func ensureUTF8BOM(path string) error {
	const bom = "\xEF\xBB\xBF"

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(bom), 0644)
		}
		return err
	}
	if bytes.HasPrefix(data, []byte(bom)) {
		return nil
	}
	return os.WriteFile(path, append([]byte(bom), data...), 0644)
}

func normalizeLLMLogText(text string) string {
	if content, ok := extractSSELogContent(text); ok {
		return decodeLogEscapes(content)
	}
	if content, ok := extractJSONLogContent(strings.TrimSpace(text)); ok {
		return decodeLogEscapes(content)
	}
	return decodeLogEscapes(text)
}

func stripThinkingText(text string) string {
	return sanitizeAssistantContent(text)
}

func extractThinkingText(text string) string {
	var startTags = []string{"<think>", "<thought>", "<thinking>"}
	var endTags = []string{"</think>", "</thought>", "</thinking>"}

	for i, startTag := range startTags {
		startIdx := strings.Index(text, startTag)
		if startIdx >= 0 {
			endIdx := strings.Index(text[startIdx:], endTags[i])
			if endIdx >= 0 {
				return strings.TrimSpace(text[startIdx+len(startTag) : startIdx+endIdx])
			}
			// 아직 닫히지 않은 경우 나머지 전원
			return strings.TrimSpace(text[startIdx+len(startTag):])
		}
	}
	return ""
}

func extractSSELogContent(text string) (string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var sb strings.Builder
	foundData := false
	foundContent := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		foundData = true
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		content, ok := extractJSONLogContent(payload)
		if !ok {
			continue
		}
		foundContent = true
		sb.WriteString(content)
	}
	if foundContent {
		return sb.String(), true
	}
	return "", foundData
}

func extractJSONLogContent(text string) (string, bool) {
	if text == "" || !strings.HasPrefix(text, "{") {
		return "", false
	}

	var raw struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
			Delta struct {
				Content json.RawMessage `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return "", false
	}

	var sb strings.Builder
	found := false
	for _, choice := range raw.Choices {
		if content, ok := jsonContentToString(choice.Message.Content); ok {
			found = true
			sb.WriteString(content)
		}
		if content, ok := jsonContentToString(choice.Delta.Content); ok {
			found = true
			sb.WriteString(content)
		}
	}
	if content, ok := jsonContentToString(raw.Content); ok {
		found = true
		sb.WriteString(content)
	}
	return sb.String(), found
}

func jsonContentToString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}

	var blocks []struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", false
	}

	var sb strings.Builder
	for _, block := range blocks {
		if block.Text != "" {
			sb.WriteString(block.Text)
		} else {
			sb.WriteString(block.Content)
		}
	}
	return sb.String(), true
}

func decodeLogEscapes(text string) string {
	text = decodeUnicodeEscapes(text)
	replacer := strings.NewReplacer(
		`\r\n`, "\n",
		`\n`, "\n",
		`\r`, "\r",
		`\t`, "\t",
	)
	return replacer.Replace(text)
}

func decodeUnicodeEscapes(text string) string {
	if !strings.Contains(text, `\u`) {
		return text
	}

	var sb strings.Builder
	sb.Grow(len(text))
	for i := 0; i < len(text); {
		r, ok := parseUnicodeEscapeAt(text, i)
		if !ok {
			sb.WriteByte(text[i])
			i++
			continue
		}
		i += 6
		if isHighSurrogate(r) {
			if next, nextOK := parseUnicodeEscapeAt(text, i); nextOK && isLowSurrogate(next) {
				sb.WriteRune(utf16.DecodeRune(r, next))
				i += 6
				continue
			}
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func parseUnicodeEscapeAt(text string, idx int) (rune, bool) {
	if idx+6 > len(text) || text[idx] != '\\' || text[idx+1] != 'u' {
		return 0, false
	}
	var r rune
	for _, c := range text[idx+2 : idx+6] {
		v, ok := hexValue(byte(c))
		if !ok {
			return 0, false
		}
		r = r*16 + v
	}
	return r, true
}

func hexValue(c byte) (rune, bool) {
	switch {
	case c >= '0' && c <= '9':
		return rune(c - '0'), true
	case c >= 'a' && c <= 'f':
		return rune(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return rune(c-'A') + 10, true
	default:
		return 0, false
	}
}

func isHighSurrogate(r rune) bool {
	return r >= 0xD800 && r <= 0xDBFF
}

func isLowSurrogate(r rune) bool {
	return r >= 0xDC00 && r <= 0xDFFF
}

// LogRawRequest는 LLM으로 전송되는 원본 JSON 요청을 llmraw.log에 기록한다.
func LogRawRequest(model string, data []byte) {
	if !rawLogEnabled {
		return
	}
	writeRawLog(model, "REQUEST", data)
}

// LogRawResponse는 LLM으로부터 수신된 원본 JSON 응답을 llmraw.log에 기록한다.
func LogRawResponse(model string, data []byte) {
	if !rawLogEnabled {
		return
	}
	writeRawLog(model, "RESPONSE", data)
}

func writeRawLog(model, direction string, data []byte) {
	if err := ensureUTF8BOM(llmRawLogPath); err != nil {
		return
	}
	f, err := os.OpenFile(llmRawLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	const sep = "========================================"
	header := fmt.Sprintf("\n%s\n[%s] %s | Model: %s | Size: %d bytes\n%s\n",
		sep, time.Now().Format("2006-01-02 15:04:05"), direction, model, len(data), sep)

	f.Write([]byte(header))
	f.Write(data)
	f.Write([]byte("\n" + sep + "\n"))
}
