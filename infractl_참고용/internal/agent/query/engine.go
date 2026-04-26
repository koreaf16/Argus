// Package query
// File: engine.go
// Description: query.Engine 구조체 및 Run() 진입점
// Responsibility: Engine 생성 및 채널 기반 query loop 시작
//
// Ported from: claude_cli/src/query.ts:219-238 (query 함수),
//              claude_cli/src/QueryEngine.ts:209-250 (submitMessage — context fetch + query delegate)
// 핵심 패턴: AsyncGenerator<StreamEvent|Message, Terminal> → <-chan QueryEvent (goroutine + defer close)

package query

import (
	"context"
	"log/slog"

	"github.com/yourorg/infractl/internal/agent/compact"
	"github.com/yourorg/infractl/internal/llm"
)

const (
	defaultMaxTurns = 50
	eventBufSize    = 8
)

// ToolRunner 는 단일 도구 호출을 실행하는 함수 타입이다.
// output 은 도구의 텍스트 출력, isError 는 도구가 에러를 반환했는지 여부다.
// 구현은 agent.executeSingleTool (Phase B 기간) 또는 tool_invoker.Invoke (Phase D 활성화 후).
type ToolRunner func(ctx context.Context, tc llm.ToolCall) (output string, isError bool, metadataJSON string)

// Engine 은 state machine + streaming 기반 query loop 를 실행하는 엔진이다.
// claude_cli 의 queryLoop AsyncGenerator 에 대응한다.
type Engine struct {
	logger   *slog.Logger
	compact  *compact.Stack
	recovery *compact.Recovery
}

// New 는 Engine 을 생성한다.
func New(logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{logger: logger}
}

// SetCompact 는 compaction stack 을 주입한다.
// nil 을 전달하면 compaction 이 비활성화된다.
func (e *Engine) SetCompact(s *compact.Stack) {
	e.compact = s
}

// SetRecovery 는 에러 복구 핸들러를 주입한다.
// nil 을 전달하면 복구가 비활성화된다.
func (e *Engine) SetRecovery(r *compact.Recovery) {
	e.recovery = r
}

// Run 은 query loop 를 시작하고 이벤트 채널을 반환한다.
// 채널은 EventTerminal 이 방출된 직후 goroutine 내부에서 닫힌다.
// 소비자는 채널을 닫으면 안 된다.
//
// claude_cli: query.ts:219-238 — async function* query() 에 대응.
// Params.RunTool 이 nil 이면 모든 도구 호출을 "no runner" 에러로 처리한다.
func (e *Engine) Run(ctx context.Context, p Params) <-chan QueryEvent {
	out := make(chan QueryEvent, eventBufSize)
	if p.MaxTurns <= 0 {
		p.MaxTurns = defaultMaxTurns
	}
	go func() {
		defer close(out)
		e.runLoop(ctx, p, out)
	}()
	return out
}

// send 는 이벤트를 채널로 전송한다. ctx 취소 시 즉시 반환한다.
// sender 가 모든 send 호출에 사용해야 하는 표준 helper.
func send(ctx context.Context, out chan<- QueryEvent, ev QueryEvent) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// sendDirect 는 ctx 취소 여부와 무관하게 이벤트를 전달한다.
// EventTerminal 전용 — 소비자가 반드시 채널을 비우고 있어야 한다.
func sendDirect(out chan<- QueryEvent, ev QueryEvent) {
	out <- ev
}
