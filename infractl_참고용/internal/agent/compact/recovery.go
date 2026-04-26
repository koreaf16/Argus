// Package compact
// File: recovery.go
// Description: LLM 에러 복구 분기 — PTL/max_output/media/fallback 에러별 액션 결정
// Responsibility: Handle(err) → RecoveryAction 반환, retry/escalate/surface 결정
//
// Ported from: claude_cli/src/query.ts:1062-1183
// 핵심 패턴: isPTL→collapse drain우선→reactive→surface,
//            isMaxOutput→64k재시도→multi-turn, isMedia→strip, isFallback→stripSig

package compact

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
)

// RecoveryAction은 에러 복구 액션 열거형이다.
type RecoveryAction int

const (
	// ActionSurfaceError는 복구 불가 — 에러를 상위로 전파한다.
	ActionSurfaceError RecoveryAction = iota
	// ActionDrainCollapse는 staged collapse 1개를 drain 후 재시도한다.
	ActionDrainCollapse
	// ActionReactiveCompact는 1회 한정 reactive compaction 후 재시도한다.
	ActionReactiveCompact
	// ActionRetryMaxTokens는 max_output_tokens 를 64K로 확대 후 재시도한다.
	ActionRetryMaxTokens
	// ActionMultiTurnRecovery는 응답을 여러 turn으로 나눠 처리한다.
	ActionMultiTurnRecovery
	// ActionStripMedia는 미디어(이미지)를 제거 후 재시도한다.
	ActionStripMedia
	// ActionStripSignaturesAndRetry는 서명/부가 정보를 제거 후 재시도한다.
	ActionStripSignaturesAndRetry
)

// MaxTokensEscalation은 max_output_tokens 재시도 시 사용할 토큰 수다.
const MaxTokensEscalation = 64_000

// Recovery는 LLM 에러 복구 핸들러다.
type Recovery struct {
	collapse  Collapse
	reactive  Reactive
	llmClient llm.Client
}

// NewRecovery는 Recovery 인스턴스를 생성한다.
func NewRecovery(collapse Collapse, reactive Reactive, client llm.Client) *Recovery {
	return &Recovery{
		collapse:  collapse,
		reactive:  reactive,
		llmClient: client,
	}
}

// Handle은 LLM 에러를 분류하고 복구 액션을 반환한다.
// Ported from: claude_cli/src/query.ts:1062-1183
// 핵심 패턴:
//   - PTL 413: collapse drain → reactive compact → surface
//   - max_output_tokens: 64k 재시도 → multi-turn → surface
//   - media: strip media
//   - fallback: strip signatures and retry
func (r *Recovery) Handle(ctx context.Context, s *State, err error) (RecoveryAction, error) {
	class := classifyError(err)

	slog.Debug("compact recovery: handling error",
		"class", class,
		"err", err,
	)

	switch class {
	case errClassPTL:
		return r.handlePTL(ctx, s, err)
	case errClassMaxOutput:
		return r.handleMaxOutput(s, err)
	case errClassMedia:
		return ActionStripMedia, nil
	case errClassFallback:
		return ActionStripSignaturesAndRetry, nil
	default:
		return ActionSurfaceError, err
	}
}

func (r *Recovery) handlePTL(ctx context.Context, s *State, err error) (RecoveryAction, error) {
	// 1순위: staged collapse drain (저비용)
	if r.collapse != nil && r.collapse.HasStaged() {
		if r.collapse.Drain(s) {
			slog.Info("compact recovery: PTL drain collapse")
			return ActionDrainCollapse, nil
		}
	}

	// 2순위: reactive compact (1회 한정)
	if !s.HasAttemptedReactiveCompact && r.reactive != nil {
		result, err := r.reactive.TryCompact(ctx, s)
		if err != nil {
			slog.Warn("compact recovery: reactive compact failed", "err", err)
			return ActionSurfaceError, fmt.Errorf("reactive compact: %w", err)
		}
		slog.Info("compact recovery: PTL reactive compact",
			"tokens_saved", result.TokensSaved,
		)
		return ActionReactiveCompact, nil
	}

	// 3순위: surface
	return ActionSurfaceError, err
}

func (r *Recovery) handleMaxOutput(s *State, _ error) (RecoveryAction, error) {
	if s.MaxOutputTokensRecoveryCount == 0 {
		s.MaxOutputTokensRecoveryCount++
		limit := MaxTokensEscalation
		s.MaxOutputTokensOverride = &limit
		slog.Info("compact recovery: escalate max_output_tokens",
			"new_limit", limit,
		)
		return ActionRetryMaxTokens, nil
	}
	// 이미 시도했으면 multi-turn으로 처리
	return ActionMultiTurnRecovery, nil
}

// --- error classification ---

type errClass int

const (
	errClassUnknown   errClass = iota
	errClassPTL
	errClassMaxOutput
	errClassMedia
	errClassFallback
)

func classifyError(err error) errClass {
	if err == nil {
		return errClassUnknown
	}
	if llm.IsPTLError(err) {
		return errClassPTL
	}
	msg := err.Error()
	switch {
	case isMaxOutputErr(msg):
		return errClassMaxOutput
	case isMediaErr(msg):
		return errClassMedia
	case isFallbackErr(msg):
		return errClassFallback
	default:
		return errClassUnknown
	}
}

func isMaxOutputErr(msg string) bool {
	return strings.Contains(msg, "max_tokens") ||
		strings.Contains(msg, "max output tokens") ||
		strings.Contains(msg, "output token limit") ||
		strings.Contains(msg, "stop_reason: max_tokens")
}

func isMediaErr(msg string) bool {
	return strings.Contains(msg, "invalid image") ||
		strings.Contains(msg, "unsupported media") ||
		strings.Contains(msg, "image too large") ||
		strings.Contains(msg, "media_type")
}

func isFallbackErr(msg string) bool {
	return strings.Contains(msg, "overloaded") ||
		strings.Contains(msg, "529") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests")
}
