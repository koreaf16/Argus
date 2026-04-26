// Package compact
// File: types.go
// Description: compaction 패키지 공유 타입 — TokenState, CompactionResult, State, 토큰 추정
// Responsibility: 패키지 내 모든 전략이 공유하는 타입 선언과 파티션 헬퍼

package compact

import "github.com/yourorg/infractl/internal/llm"

// TokenState는 컨텍스트 창의 토큰 사용 상태다.
// Ported from: claude_cli/src/services/compact/autoCompact.ts:93 calculateTokenWarningState
// 핵심 패턴: 토큰 버퍼 절댓값 (%), contextWindow - bufferK < used → 해당 상태
type TokenState int

const (
	TokenOK       TokenState = iota // remaining >= 20K
	TokenWarning                    // 13K <= remaining < 20K
	TokenCritical                   // 3K <= remaining < 13K
	TokenOverflow                   // remaining < 3K
)

// 토큰 버퍼 임계값 (claude_cli autoCompact.ts:62-65)
const (
	AutoCompactBufferTokens = 13_000 // Critical 임계
	WarningBufferTokens     = 20_000 // Warning 임계
	BlockingBufferTokens    = 3_000  // Overflow 임계
	ReservedOutputTokens    = 8_000  // 출력용 예약
	AvgCharsPerToken        = 3      // 한국어/영어 혼합 평균
)

// CompactionResult는 compaction 한 단계의 결과를 기록한다.
type CompactionResult struct {
	PreCount    int
	PostCount   int
	Reason      string
	Strategy    string
	TokensSaved int
}

// State는 compact 패키지 전용 turn 가변 상태다.
// query.state 에서 필요 필드만 추출하여 circular dependency 방지.
type State struct {
	Messages                     []llm.Message
	ContextWindow                int
	SystemTokens                 int
	HasAttemptedReactiveCompact  bool
	MaxOutputTokensRecoveryCount int
	MaxOutputTokensOverride      *int
}

// EstimateTokens는 메시지 목록의 토큰 수를 문자 수 기반으로 추정한다.
func EstimateTokens(messages []llm.Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content) / AvgCharsPerToken
		for _, tc := range msg.ToolCalls {
			total += len(tc.Function.Arguments) / AvgCharsPerToken
		}
	}
	return total
}

// CalculateRemainingTokens는 남은 토큰 수를 반환한다.
func CalculateRemainingTokens(s *State) int {
	cw := s.ContextWindow
	if cw <= 0 {
		cw = 128_000
	}
	effective := cw - ReservedOutputTokens
	if effective <= 0 {
		effective = cw
	}
	used := EstimateTokens(s.Messages) + s.SystemTokens
	return effective - used
}

// CalculateTokenState는 현재 토큰 상태를 반환한다.
// Ported from: claude_cli/src/services/compact/autoCompact.ts:93
func CalculateTokenState(s *State) TokenState {
	remaining := CalculateRemainingTokens(s)
	switch {
	case remaining < BlockingBufferTokens:
		return TokenOverflow
	case remaining < AutoCompactBufferTokens:
		return TokenCritical
	case remaining < WarningBufferTokens:
		return TokenWarning
	default:
		return TokenOK
	}
}

// --- partition helpers ---
// groupByApiRound, flattenRounds 는 agent/history.go 의 private 함수와 동일 로직.
// circular dependency 방지를 위해 compact 패키지 내에 독립 정의.

type round struct {
	messages []llm.Message
}

func groupByRound(history []llm.Message) []round {
	var rounds []round
	var current []llm.Message
	for _, msg := range history {
		if msg.Role == llm.RoleUser && len(current) > 0 {
			rounds = append(rounds, round{messages: current})
			current = []llm.Message{msg}
		} else {
			current = append(current, msg)
		}
	}
	if len(current) > 0 {
		rounds = append(rounds, round{messages: current})
	}
	return rounds
}

func flattenRounds(rounds []round) []llm.Message {
	total := 0
	for _, r := range rounds {
		total += len(r.messages)
	}
	result := make([]llm.Message, 0, total)
	for _, r := range rounds {
		result = append(result, r.messages...)
	}
	return result
}
