// Package compact
// File: reactive.go
// Description: PTL 413 발생 시 1회 한정 reactive compaction — aggressive 요약 + media strip
// Responsibility: hasAttemptedReactiveCompact 플래그로 무한 재시도 방지, 실패 시 trim 폴백
//
// ★ 자체 설계: claude_cli/src/services/compact/reactiveCompact.js는 feature-gated 미존재
// 근거: claude_cli/src/query.ts:1119-1166 호출부 (tryReactiveCompact 패턴)

package compact

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
)

// reactiveStripChars는 media strip 시 base64 콘텐츠의 최대 보존 길이다.
const reactiveStripChars = 100

// Reactive는 PTL 413 reactive compaction 인터페이스다.
type Reactive interface {
	// TryCompact는 1회 한정 reactive compaction을 시도한다.
	// s.HasAttemptedReactiveCompact가 true이면 즉시 noop을 반환한다.
	TryCompact(ctx context.Context, s *State) (CompactionResult, error)
}

// NewReactive는 Reactive 구현체를 반환한다.
func NewReactive(client llm.Client) Reactive {
	return &reactiveCompactor{client: client}
}

type reactiveCompactor struct {
	client llm.Client
}

// TryCompact는 PTL 413 복구를 위한 1회 한정 reactive compaction을 실행한다.
// Ported from: claude_cli/src/query.ts:1119-1166 tryReactiveCompact 호출부
// 핵심 패턴: hasAttempted 플래그로 1회 제한, aggressive 요약 + media strip
func (rc *reactiveCompactor) TryCompact(ctx context.Context, s *State) (CompactionResult, error) {
	pre := len(s.Messages)
	if s.HasAttemptedReactiveCompact {
		return CompactionResult{PreCount: pre, PostCount: pre, Strategy: "reactive", Reason: "already_attempted"}, nil
	}
	s.HasAttemptedReactiveCompact = true

	slog.Info("compact reactive: starting PTL recovery",
		"pre_count", pre,
	)

	// 1단계: media strip (base64 이미지 등 대형 콘텐츠 제거)
	stripped := rc.stripMedia(s.Messages)
	tokensBefore := EstimateTokens(s.Messages)
	tokensAfter := EstimateTokens(stripped)
	mediaTokensSaved := tokensBefore - tokensAfter

	if mediaTokensSaved > 0 {
		slog.Debug("compact reactive: media stripped",
			"tokens_saved", mediaTokensSaved,
		)
		s.Messages = stripped
	}

	// 2단계: aggressive 요약 시도
	toCompact, toKeep := partitionForCompaction(s.Messages)
	if len(toCompact) == 0 {
		// 나눌 수 없으면 trim 폴백
		rc.trimFallback(s)
		return CompactionResult{
			PreCount:  pre,
			PostCount: len(s.Messages),
			Reason:    "reactive_trim_fallback",
			Strategy:  "reactive",
		}, nil
	}

	summary, err := rc.requestSummary(ctx, toCompact)
	if err != nil {
		slog.Warn("compact reactive: summary failed, trim fallback", "err", err)
		rc.trimFallback(s)
		return CompactionResult{
			PreCount:  pre,
			PostCount: len(s.Messages),
			Reason:    fmt.Sprintf("reactive_failed_trim: %s", err.Error()),
			Strategy:  "reactive",
		}, nil
	}

	newMessages := make([]llm.Message, 0, 2+len(toKeep))
	newMessages = append(newMessages,
		llm.Message{
			Role:    llm.RoleUser,
			Content: fmt.Sprintf("[Reactive compaction summary]\n%s\n\n[Continue from here]", summary),
		},
		llm.Message{
			Role:    llm.RoleAssistant,
			Content: "Understood. Context has been compacted to recover from token limit. Continuing.",
		},
	)
	newMessages = append(newMessages, toKeep...)

	tokensSaved := EstimateTokens(s.Messages) - EstimateTokens(newMessages)
	s.Messages = newMessages

	slog.Info("compact reactive: completed",
		"pre_count", pre,
		"post_count", len(s.Messages),
		"tokens_saved", tokensSaved,
	)
	return CompactionResult{
		PreCount:    pre,
		PostCount:   len(s.Messages),
		Reason:      "reactive_ptl_recovery",
		Strategy:    "reactive",
		TokensSaved: tokensSaved,
	}, nil
}

// stripMedia는 base64 이미지 또는 대용량 binary 콘텐츠를 제거한다.
func (rc *reactiveCompactor) stripMedia(messages []llm.Message) []llm.Message {
	result := make([]llm.Message, len(messages))
	copy(result, messages)
	for i, msg := range result {
		if isBase64Content(msg.Content) {
			if len(msg.Content) > reactiveStripChars {
				result[i].Content = msg.Content[:reactiveStripChars] + " [media stripped]"
			}
		}
	}
	return result
}

// isBase64Content는 Content 가 base64 인코딩된 대용량 바이너리인지 간단히 판단한다.
func isBase64Content(s string) bool {
	if len(s) < 1000 {
		return false
	}
	// base64 문자만 포함된 긴 문자열 (줄바꿈 없는 연속 비ASCII 없음)
	sampleLen := min(200, len(s))
	sample := s[:sampleLen]
	for _, c := range sample {
		if c > 127 {
			return false
		}
	}
	return !strings.Contains(sample, " ") && !strings.Contains(sample[:50], "\n")
}

// trimFallback은 가장 오래된 tool_result 메시지 10개를 제거하는 최소 폴백이다.
func (rc *reactiveCompactor) trimFallback(s *State) {
	const dropCount = 10
	dropped := 0
	newMsgs := make([]llm.Message, 0, len(s.Messages))
	for _, msg := range s.Messages {
		if dropped < dropCount && len(msg.ToolCalls) == 0 && msg.Role == llm.RoleUser && strings.HasPrefix(msg.Content, "[Tool result") {
			dropped++
			continue
		}
		newMsgs = append(newMsgs, msg)
	}
	if dropped == 0 && len(s.Messages) > trimKeepMessages {
		s.Messages = s.Messages[len(s.Messages)-trimKeepMessages:]
		return
	}
	s.Messages = newMsgs
}

func (rc *reactiveCompactor) requestSummary(ctx context.Context, messages []llm.Message) (string, error) {
	if rc.client == nil {
		return "[no client]", nil
	}
	msgs := append(messages, llm.Message{Role: llm.RoleUser, Content: compactSummaryPrompt})
	system := llm.Message{Role: llm.RoleSystem, Content: compactSystemPrompt}
	apiMsgs := append([]llm.Message{system}, msgs...)
	resp, err := rc.client.ChatStream(ctx, apiMsgs, nil, nil,
		func(string) {}, func(string) {},
	)
	if err != nil {
		return "", fmt.Errorf("reactive summary: %w", err)
	}
	if strings.TrimSpace(resp.Content) == "" {
		return "", fmt.Errorf("reactive summary: empty response")
	}
	return resp.Content, nil
}

var _ Reactive = (*reactiveCompactor)(nil)
