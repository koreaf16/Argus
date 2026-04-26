// Package compact
// File: micro.go
// Description: message-level microcompact — 오래된 tool_result를 요약으로 교체
// Responsibility: 캐시 친화적 tool_result 압축 (최근 N개 보존), prefix_marker boundary 유지
//
// Ported from: claude_cli/src/services/compact/microCompact.ts:253 microcompactMessages
// Ported from: claude_cli/src/query.ts:870-892 (CACHED_MICROCOMPACT 패턴)
// 차이: claude_cli는 cache-editing API 기반; InfraCtl은 content 기반 truncation 사용
//       (multi-LLM 지원을 위해 API 비종속 방식 채택)

package compact

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
)

// DefaultKeepRecentMessages는 micro compaction 시 원문 보존할 최근 메시지 수다.
const DefaultKeepRecentMessages = 10

// microSummaryMaxChars는 tool_result 요약의 최대 문자 수다.
const microSummaryMaxChars = 200

// Micro는 message-level microcompact 인터페이스다.
type Micro interface {
	CompactIfNeeded(s *State, keepRecent int) CompactionResult
}

// NewMicro는 Micro 구현체를 반환한다.
func NewMicro() Micro {
	return &microCompactor{}
}

type microCompactor struct{}

// CompactIfNeeded는 오래된 tool_result 메시지를 요약으로 교체한다.
// 최근 keepRecent 개 메시지는 원문 보존. prefix cache boundary 를 유지한다.
// Ported from: claude_cli/src/services/compact/microCompact.ts:253
// 핵심 패턴: tool_use_id 단위로 오래된 tool_result 를 짧은 요약으로 교체
func (mc *microCompactor) CompactIfNeeded(s *State, keepRecent int) CompactionResult {
	if keepRecent <= 0 {
		keepRecent = DefaultKeepRecentMessages
	}
	pre := len(s.Messages)
	if pre <= keepRecent {
		return CompactionResult{PreCount: pre, PostCount: pre, Strategy: "micro"}
	}

	// 보존 경계: 마지막 keepRecent 개는 손대지 않는다
	cutoff := pre - keepRecent
	compacted := make([]llm.Message, pre)
	copy(compacted, s.Messages)

	replaced := 0
	for i := range cutoff {
		msg := compacted[i]
		// tool 역할 메시지 (tool_result)
		if msg.Role == llm.RoleTool && len(msg.Content) > microSummaryMaxChars {
			original := msg.Content
			summary := summarizeToolResult(msg.ToolCallID, msg.Content)
			compacted[i].Content = summary
			tokensSavedThis := (len(original) - len(summary)) / AvgCharsPerToken
			_ = tokensSavedThis
			replaced++
			slog.Debug("compact micro: tool_result truncated",
				"tool_call_id", msg.ToolCallID,
				"original_chars", len(original),
				"summary_chars", len(summary),
			)
		}
	}

	if replaced == 0 {
		return CompactionResult{PreCount: pre, PostCount: pre, Strategy: "micro"}
	}

	preTokens := EstimateTokens(s.Messages[:cutoff])
	s.Messages = compacted
	tokensSaved := preTokens - EstimateTokens(s.Messages[:cutoff])

	slog.Info("compact micro: completed",
		"replaced", replaced,
		"pre_count", pre,
		"cutoff", cutoff,
		"tokens_saved", tokensSaved,
	)
	return CompactionResult{
		PreCount:    pre,
		PostCount:   len(s.Messages),
		Reason:      fmt.Sprintf("micro: %d tool_results truncated", replaced),
		Strategy:    "micro",
		TokensSaved: tokensSaved,
	}
}

// summarizeToolResult는 tool_result 내용을 짧게 요약한다.
func summarizeToolResult(toolCallID, content string) string {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) <= microSummaryMaxChars {
		return trimmed
	}
	lines := strings.SplitN(trimmed, "\n", 6)
	var preview strings.Builder
	for _, line := range lines[:min(3, len(lines))] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rem := microSummaryMaxChars - preview.Len()
		if rem <= 0 {
			break
		}
		if len(line) > rem {
			line = line[:rem]
		}
		preview.WriteString(line)
		preview.WriteString("\n")
	}
	previewChars := preview.Len()
	truncated := len(trimmed) - previewChars
	if truncated <= 0 {
		return trimmed
	}
	return fmt.Sprintf("%s[... %d chars truncated (tool_id: %.8s)]",
		preview.String(), truncated, toolCallID)
}

var _ Micro = (*microCompactor)(nil)
