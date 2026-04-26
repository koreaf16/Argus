// Package compact
// File: snip.go
// Description: 오래된 turn 통째 요약 — keepRecentTurns 이전 turn을 LLM 요약으로 교체
// Responsibility: 오래된 배경 잡음 제거, 최근 N turn 원문 보존
//
// ★ 자체 설계: claude_cli/src/services/compact/snipCompact.js는 feature-gated 미존재
// 근거: claude_cli/src/query.ts:401-410 호출부 + docs_mig/03_phase_c §5 C.1 기술

package compact

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
)

// DefaultKeepRecentTurns는 snip 시 원문 보존할 최근 turn 수다.
const DefaultKeepRecentTurns = 20

// Snip은 오래된 turn 요약 인터페이스다.
type Snip interface {
	Apply(ctx context.Context, s *State, keepRecentTurns int) CompactionResult
}

// NewSnip은 Snip 구현체를 반환한다.
func NewSnip(client llm.Client) Snip {
	return &snipApplier{client: client}
}

type snipApplier struct {
	client llm.Client
}

// Apply는 오래된 turn을 LLM 요약으로 교체한다.
// 최근 keepRecentTurns 개 turn은 원문 보존. 이전은 요약 텍스트로 교체.
func (sn *snipApplier) Apply(ctx context.Context, s *State, keepRecentTurns int) CompactionResult {
	if keepRecentTurns <= 0 {
		keepRecentTurns = DefaultKeepRecentTurns
	}
	pre := len(s.Messages)

	rounds := groupByRound(s.Messages)
	if len(rounds) <= keepRecentTurns {
		return CompactionResult{PreCount: pre, PostCount: pre, Strategy: "snip"}
	}

	splitAt := len(rounds) - keepRecentTurns
	toSnip := rounds[:splitAt]
	toKeep := rounds[splitAt:]

	var sb strings.Builder
	for _, r := range toSnip {
		for _, msg := range r.messages {
			if msg.Content != "" {
				sb.WriteString(msg.Content)
				sb.WriteString("\n")
			}
		}
	}
	rawText := sb.String()
	if strings.TrimSpace(rawText) == "" {
		return CompactionResult{PreCount: pre, PostCount: pre, Strategy: "snip"}
	}

	summary, err := sn.requestSummary(ctx, rawText)
	if err != nil {
		slog.Warn("compact snip: summary failed, skipping",
			"rounds_to_snip", len(toSnip),
			"err", err,
		)
		return CompactionResult{PreCount: pre, PostCount: pre, Strategy: "snip", Reason: "summary_failed"}
	}

	kept := flattenRounds(toKeep)
	newMessages := make([]llm.Message, 0, 2+len(kept))
	newMessages = append(newMessages,
		llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("[이전 대화 요약]\n%s", summary)},
		llm.Message{Role: llm.RoleAssistant, Content: "이전 맥락을 파악했습니다. 계속 진행하겠습니다."},
	)
	newMessages = append(newMessages, kept...)

	tokensSaved := EstimateTokens(s.Messages) - EstimateTokens(newMessages)
	s.Messages = newMessages

	slog.Info("compact snip: completed",
		"snipped_rounds", len(toSnip),
		"kept_rounds", keepRecentTurns,
		"pre_count", pre,
		"post_count", len(s.Messages),
		"tokens_saved", tokensSaved,
	)
	return CompactionResult{
		PreCount:    pre,
		PostCount:   len(s.Messages),
		Reason:      fmt.Sprintf("snipped %d old turns", len(toSnip)),
		Strategy:    "snip",
		TokensSaved: tokensSaved,
	}
}

const snipSystemPrompt = `인프라 관리 대화의 오래된 기록입니다.
핵심 작업 내용, 실행 결과, 발견된 문제를 2-4문장으로 간결하게 요약하세요.
세부 명령어 출력은 제외하고 핵심 사실만 보존하세요.`

func (sn *snipApplier) requestSummary(ctx context.Context, text string) (string, error) {
	if sn.client == nil {
		return "[snip: no client]", nil
	}
	resp, err := sn.client.Chat(ctx, []llm.Message{
		{Role: llm.RoleUser, Content: snipSystemPrompt + "\n\n" + text},
	}, nil, nil)
	if err != nil {
		return "", fmt.Errorf("snip summary: %w", err)
	}
	result := strings.TrimSpace(resp.Content)
	if result == "" {
		return "", fmt.Errorf("snip summary: empty response")
	}
	return result, nil
}

var _ Snip = (*snipApplier)(nil)
