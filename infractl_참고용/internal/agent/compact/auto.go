// Package compact
// File: auto.go
// Description: proactive auto compaction — 토큰 상태 기반 mild/aggressive 자동 실행
// Responsibility: TokenState 별 압축 전략 결정, circuit breaker 보호, trim 폴백
//
// Ported from: claude_cli/src/services/compact/autoCompact.ts:241 autoCompactIfNeeded
// 기존 코드 이전: internal/agent/compaction.go:100 compactIfNeeded,
//               :184 requestCompactionSummaryFor, :220 doCompactionRequest

package compact

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
)

const (
	// preserveRecentRounds는 aggressive compaction 후 원문 보존할 최근 라운드 수다.
	preserveRecentRounds = 4
	// maxPTLRetries는 PTL 에러 발생 시 재시도 최대 횟수다.
	maxPTLRetries = 3
	// trimKeepMessages는 trim 폴백 시 유지할 최근 메시지 수다.
	trimKeepMessages = 20
)

// MildCompactor는 mild compaction 실행자 인터페이스다.
// SessionSummaryManager가 구현한다. compact→agent circular dependency 방지.
type MildCompactor interface {
	UpdateIfNeeded(ctx context.Context, messages []llm.Message)
}

// Auto는 proactive auto compaction 인터페이스다.
type Auto interface {
	Apply(ctx context.Context, s *State, tokenState TokenState, breaker Breaker) CompactionResult
}

// NewAuto는 Auto 구현체를 반환한다.
func NewAuto(client llm.Client, mild MildCompactor) Auto {
	return &autoCompactor{client: client, mild: mild}
}

type autoCompactor struct {
	client llm.Client
	mild   MildCompactor
}

// Apply는 tokenState에 따라 mild 또는 aggressive compaction을 실행한다.
// Ported from: claude_cli/src/services/compact/autoCompact.ts:241
// 핵심 패턴: Warning→mild, Critical/Overflow→aggressive, breaker open→trim 폴백
func (ac *autoCompactor) Apply(ctx context.Context, s *State, tokenState TokenState, breaker Breaker) CompactionResult {
	pre := len(s.Messages)
	if tokenState < TokenWarning {
		return CompactionResult{PreCount: pre, PostCount: pre, Strategy: "auto"}
	}

	if !breaker.Allow() {
		slog.Warn("compact auto: circuit breaker open, fallback to trim",
			"failures", breaker.Failures(),
			"token_state", tokenState,
		)
		ac.trimHistory(s)
		return CompactionResult{
			PreCount:  pre,
			PostCount: len(s.Messages),
			Reason:    "breaker_open_trim",
			Strategy:  "auto",
		}
	}

	if tokenState == TokenWarning {
		if ac.mild != nil {
			ac.mild.UpdateIfNeeded(ctx, s.Messages)
		}
		slog.Debug("compact auto: mild compaction triggered",
			"pre_count", pre,
			"token_state", "warning",
		)
		return CompactionResult{
			PreCount:  pre,
			PostCount: len(s.Messages),
			Reason:    "warning_mild",
			Strategy:  "auto",
		}
	}

	// Critical / Overflow: aggressive
	slog.Info("compact auto: aggressive compaction",
		"pre_count", pre,
		"token_state", tokenState,
	)

	toCompact, toKeep := partitionForCompaction(s.Messages)
	summary, err := ac.requestSummary(ctx, toCompact)
	if err != nil {
		breaker.RecordFailure()
		slog.Warn("compact auto: aggressive failed, fallback to trim",
			"err", err,
			"failures", breaker.Failures(),
		)
		ac.trimHistory(s)
		return CompactionResult{
			PreCount:  pre,
			PostCount: len(s.Messages),
			Reason:    fmt.Sprintf("aggressive_failed_trim: %s", err.Error()),
			Strategy:  "auto",
		}
	}
	breaker.RecordSuccess()

	newMessages := make([]llm.Message, 0, 2+len(toKeep))
	newMessages = append(newMessages,
		llm.Message{
			Role:    llm.RoleUser,
			Content: fmt.Sprintf("[Previous conversation summary]\n%s\n\n[Continue from here]", summary),
		},
		llm.Message{
			Role:    llm.RoleAssistant,
			Content: "Understood. I have the summarized context. How can I help you continue?",
		},
	)
	newMessages = append(newMessages, toKeep...)

	tokensSaved := EstimateTokens(s.Messages) - EstimateTokens(newMessages)
	s.Messages = newMessages

	slog.Info("compact auto: aggressive done",
		"pre_count", pre,
		"post_count", len(s.Messages),
		"tokens_saved", tokensSaved,
	)
	return CompactionResult{
		PreCount:    pre,
		PostCount:   len(s.Messages),
		Reason:      "aggressive",
		Strategy:    "auto",
		TokensSaved: tokensSaved,
	}
}

func (ac *autoCompactor) requestSummary(ctx context.Context, messages []llm.Message) (string, error) {
	current := messages
	for attempt := range maxPTLRetries {
		summary, err := ac.doSummaryRequest(ctx, current)
		if err == nil {
			return summary, nil
		}
		if !llm.IsPTLError(err) {
			return "", fmt.Errorf("compaction request: %w", err)
		}
		rounds := groupByRound(current)
		if len(rounds) < 2 {
			return "", fmt.Errorf("compaction PTL: messages too short to retry: %w", err)
		}
		drop := max(1, len(rounds)/5)
		if drop >= len(rounds) {
			drop = len(rounds) - 1
		}
		current = flattenRounds(rounds[drop:])
		slog.Warn("compact auto: PTL retry",
			"attempt", attempt+1,
			"dropped_rounds", drop,
			"remaining", len(current),
		)
	}
	return "", fmt.Errorf("compaction: exhausted PTL retries (%d)", maxPTLRetries)
}

func (ac *autoCompactor) doSummaryRequest(ctx context.Context, messages []llm.Message) (string, error) {
	if ac.client == nil {
		return "[no client]", nil
	}
	msgs := append(messages, llm.Message{Role: llm.RoleUser, Content: compactSummaryPrompt})
	system := llm.Message{Role: llm.RoleSystem, Content: compactSystemPrompt}
	apiMsgs := append([]llm.Message{system}, msgs...)
	resp, err := ac.client.ChatStream(ctx, apiMsgs, nil, nil,
		func(string) {}, func(string) {},
	)
	if err != nil {
		return "", fmt.Errorf("compaction api: %w", err)
	}
	if strings.TrimSpace(resp.Content) == "" {
		return "", fmt.Errorf("compaction: empty response")
	}
	return resp.Content, nil
}

func (ac *autoCompactor) trimHistory(s *State) {
	if len(s.Messages) <= trimKeepMessages {
		return
	}
	s.Messages = s.Messages[len(s.Messages)-trimKeepMessages:]
}

func partitionForCompaction(messages []llm.Message) (toCompact, toKeep []llm.Message) {
	rounds := groupByRound(messages)
	if len(rounds) <= preserveRecentRounds+1 {
		return messages, nil
	}
	split := len(rounds) - preserveRecentRounds
	return flattenRounds(rounds[:split]), flattenRounds(rounds[split:])
}

const compactSystemPrompt = `당신은 인프라 관리 대화의 컨텍스트 압축기입니다.
주어진 대화 내용을 핵심 정보가 보존되도록 압축하세요.`

const compactSummaryPrompt = `위 대화를 다음 정보를 반드시 포함하여 압축하세요:
1. 다룬 서버 목록과 각 서버에서 수행한 주요 작업
2. 발견된 문제와 해결 여부
3. 사용자가 명시한 목표와 현재 진행 상태
4. 중요한 설정값이나 명령 결과

간결하고 명확하게 작성하세요. 불필요한 명령어 출력 전문은 생략하세요.`

var _ Auto = (*autoCompactor)(nil)
