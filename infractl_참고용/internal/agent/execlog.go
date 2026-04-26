// Package agent
// File: execlog.go
// Description: 도구 실행 이력 기록 로직
// Responsibility: 모든 도구 실행 결과를 execution_logs에 자동 저장, 재시도 체인 관리

package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/store"
)

const maxOutputLen = 10000

var (
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|passwd|token|secret|apikey|api_key)\s*[=:]\s*['"]?[^'"\s]+`),
		regexp.MustCompile(`(?i)authorization:\s*bearer\s+[^\s]+`),
		regexp.MustCompile(`(?i)pgpassword=[^\s]+`),
	}
)

// execContext는 단일 도구 실행에 필요한 컨텍스트 정보를 담는다.
type execContext struct {
	sessionID    int64
	toolName     string
	target       string
	args         map[string]interface{}
	output       string
	errMsg       string
	exitCode     int
	duration     time.Duration
	success      bool
	userPrompt   string
	metadataJSON string
}

// recordExecLog는 도구 실행 결과를 execution_logs에 저장한다.
func (a *Agent) recordExecLog(ctx context.Context, ec execContext) int64 {
	if a.execLogStore == nil {
		return 0
	}

	output := ec.output
	if len(output) > maxOutputLen {
		output = output[:maxOutputLen] + "\n...[truncated]"
	}

	paramsJSON := "{}"
	if b, err := json.Marshal(ec.args); err == nil {
		paramsJSON = string(b)
	}

	target := ec.target
	if target == "" {
		target = "localhost"
	}

	var retryOf *int64
	if a.lastFailedLogID > 0 && !ec.success {
		// 이전 실패가 있었고 현재도 실패면 체인 없음 (다른 도구일 수 있음)
	} else if a.lastFailedLogID > 0 && ec.success {
		// 이전 실패 후 성공 → retry_of 연결
		id := a.lastFailedLogID
		retryOf = &id
	}

	log := store.ExecutionLog{
		SessionID:    a.currentSessionID,
		ToolName:     ec.toolName,
		TargetServer: target,
		InputParams:  paramsJSON,
		Output:       sanitizeSensitiveText(output),
		ErrorMessage: sanitizeSensitiveText(ec.errMsg),
		ExitCode:     ec.exitCode,
		DurationMs:   ec.duration.Milliseconds(),
		Success:      ec.success,
		RetryOf:      retryOf,
		UserPrompt:   a.lastUserPrompt,
		TaskKey:      buildTaskKey(a.lastUserPrompt),
		CommandKey:   buildCommandKey(ec.toolName, ec.args),
		FailureSig:   buildFailureSignature(ec.errMsg, output, ec.exitCode),
		MetadataJSON: buildExecMetadataJSON(ec),
	}

	id, err := a.execLogStore.SaveExecutionLog(ctx, log)
	if err != nil {
		slog.Warn("save execution log", "tool", ec.toolName, "err", err)
		return 0
	}
	a.auditAppendStep(ctx, id, ec.toolName, ec.success)

	// 실패 로그 ID 추적
	if !ec.success {
		a.lastFailedLogID = id
	} else {
		a.lastFailedLogID = 0
	}
	return id
}

func sanitizeSensitiveText(s string) string {
	out := s
	for _, re := range secretPatterns {
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			if idx := strings.IndexAny(m, "=:"); idx > 0 {
				return m[:idx+1] + " [REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return out
}

func buildExecMetadataJSON(ec execContext) string {
	if strings.TrimSpace(ec.metadataJSON) != "" {
		return ec.metadataJSON
	}

	meta := map[string]interface{}{
		"tool":      ec.toolName,
		"target":    ec.target,
		"success":   ec.success,
		"exit_code": ec.exitCode,
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// sessionMessageFromLLM은 llm.Message를 store.SessionMessage로 변환한다.
func sessionMessageFromLLM(msg llm.Message, convID int64) store.SessionMessage {
	toolCallsJSON := "[]"
	if len(msg.ToolCalls) > 0 {
		if b, err := json.Marshal(msg.ToolCalls); err == nil {
			toolCallsJSON = string(b)
		}
	}
	return store.SessionMessage{
		ConversationID: convID,
		Role:           string(msg.Role),
		Content:        msg.Content,
		ToolCalls:      toolCallsJSON,
		ToolCallID:     msg.ToolCallID,
		Kind:           "message",
		MetadataJSON:   "{}",
	}
}

// llmMessageFromSession은 store.SessionMessage를 llm.Message로 변환한다.
func llmMessageFromSession(sm store.SessionMessage) llm.Message {
	msg := llm.Message{
		Role:       llm.Role(sm.Role),
		Content:    sm.Content,
		ToolCallID: sm.ToolCallID,
	}
	if sm.ToolCalls != "" && sm.ToolCalls != "[]" {
		var tcs []llm.ToolCall
		if err := json.Unmarshal([]byte(sm.ToolCalls), &tcs); err == nil {
			msg.ToolCalls = tcs
		}
	}
	return msg
}
