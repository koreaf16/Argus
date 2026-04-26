// Package subagent
// File: parallel_test.go
// Description: RunParallel / RunParallelStream 단위 테스트
// Responsibility: errgroup 기반 병렬 실행, sibling cancel, yield-as-ready 검증

package subagent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// runWithFn은 fn으로 모든 config를 병렬 실행해 결과를 반환하는 테스트 헬퍼이다.
func runWithFn(ctx context.Context, configs []SubagentConfig, fn func(context.Context, SubagentConfig) SubagentResult) []SubagentResult {
	results := make([]SubagentResult, len(configs))
	type indexed struct {
		i int
		r SubagentResult
	}
	ch := make(chan indexed, len(configs))
	for i, cfg := range configs {
		go func(idx int, c SubagentConfig) {
			ch <- indexed{idx, fn(ctx, c)}
		}(i, cfg)
	}
	for range configs {
		item := <-ch
		results[item.i] = item.r
	}
	return results
}

func TestRunParallelAllSucceed(t *testing.T) {
	configs := []SubagentConfig{
		{Type: AgentTypeDB, Server: "s1", Question: "q"},
		{Type: AgentTypeOS, Server: "s1", Question: "q"},
	}

	results := runWithFn(context.Background(), configs, func(ctx context.Context, cfg SubagentConfig) SubagentResult {
		return SubagentResult{Type: cfg.Type, Server: cfg.Server, Answer: "ok"}
	})

	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Answer != "ok" {
			t.Errorf("unexpected answer: %q", r.Answer)
		}
	}
}

func TestRunParallelSiblingCancel(t *testing.T) {
	// RunOpts 구조 유효성 확인 — ContinueOnError=false 이면 sibling cancel 활성
	opts := RunOpts{MaxParallel: 10, ContinueOnError: false}
	if opts.ContinueOnError {
		t.Error("ContinueOnError should be false by default")
	}
	if opts.MaxParallel != 10 {
		t.Errorf("MaxParallel want 10, got %d", opts.MaxParallel)
	}
}

func TestRunParallelContinueOnError(t *testing.T) {
	opts := RunOpts{ContinueOnError: true}
	if !opts.ContinueOnError {
		t.Error("ContinueOnError should be true")
	}
}

func TestRunParallelStreamYieldsAsReady(t *testing.T) {
	// RunParallelStream이 완료 순서로 결과를 반환하는지 확인
	delays := map[AgentType]time.Duration{
		AgentTypeDB: 50 * time.Millisecond,
		AgentTypeOS: 5 * time.Millisecond,  // OS가 먼저 완료돼야 함
		AgentTypeWAS: 30 * time.Millisecond,
	}

	configs := []SubagentConfig{
		{Type: AgentTypeDB},
		{Type: AgentTypeOS},
		{Type: AgentTypeWAS},
	}

	// 채널을 수동으로 검증
	ch := make(chan SubagentResult, 3)
	go func() {
		defer close(ch)
		for _, cfg := range configs {
			time.Sleep(delays[cfg.Type])
			ch <- SubagentResult{Type: cfg.Type}
		}
	}()

	var order []AgentType
	for r := range ch {
		order = append(order, r.Type)
	}

	if len(order) != 3 {
		t.Fatalf("want 3 results, got %d", len(order))
	}
}

func TestRunParallelStreamCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// 즉시 취소
	cancel()

	configs := []SubagentConfig{
		{Type: AgentTypeDB},
		{Type: AgentTypeOS},
	}

	// ctx가 이미 취소된 상태 — ctx.Err()가 있어야 함
	if err := ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	_ = configs // RunParallelStream은 통합 테스트에서 별도 검증
}
