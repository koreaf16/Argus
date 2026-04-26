// Package executor
// File: parallel.go
// Description: 다중 서버 병렬 명령 실행 (fan-out/fan-in 패턴)
// Responsibility: errgroup 기반으로 동일 명령을 다수 서버에 동시에 실행하고 결과를 집계

package executor

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"
)

// ParallelResult는 단일 서버에 대한 병렬 실행 결과이다.
type ParallelResult struct {
	Target   string
	Result   ExecResult
	Err      error
	Duration time.Duration
}

// ParallelConfig는 병렬 실행 설정이다.
type ParallelConfig struct {
	// Concurrency는 동시 실행 goroutine 수 한계이다. 0이면 len(targets)와 동일.
	Concurrency int
	// Timeout은 개별 서버 실행 타임아웃이다. 0이면 ctx 타임아웃을 따른다.
	Timeout time.Duration
}

// DefaultParallelConfig는 권장 기본 병렬 실행 설정이다.
var DefaultParallelConfig = ParallelConfig{
	Concurrency: 10,
	Timeout:     0,
}

// FanOut은 동일 command를 targets의 모든 서버에 병렬로 실행한다.
// 결과는 targets와 동일한 순서로 반환된다.
// read-only 용도로만 사용해야 한다.
func FanOut(ctx context.Context, mgr *Manager, targets []string, command string, cfg ParallelConfig) []ParallelResult {
	return FanOutFunc(ctx, mgr, targets, func(ctx context.Context, exec Executor) (ExecResult, error) {
		return exec.Execute(ctx, command)
	}, cfg)
}

// FanOutFunc는 fn을 targets의 모든 서버에 병렬로 실행한다.
// fn은 read-only 작업이어야 한다.
func FanOutFunc(ctx context.Context, mgr *Manager, targets []string, fn func(ctx context.Context, exec Executor) (ExecResult, error), cfg ParallelConfig) []ParallelResult {
	results := make([]ParallelResult, len(targets))

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = len(targets)
	}
	if concurrency == 0 {
		return results
	}

	eg, gCtx := errgroup.WithContext(ctx)
	eg.SetLimit(concurrency)

	for i, target := range targets {
		eg.Go(func() error {
			exec, err := mgr.Get(target)
			if err != nil {
				results[i] = ParallelResult{Target: target, Err: err}
				return nil // 개별 서버 실패는 전체를 중단하지 않음
			}

			runCtx := gCtx
			var cancel context.CancelFunc
			if cfg.Timeout > 0 {
				runCtx, cancel = context.WithTimeout(gCtx, cfg.Timeout)
				defer cancel()
			}

			start := time.Now()
			res, execErr := fn(runCtx, exec)
			results[i] = ParallelResult{
				Target:   target,
				Result:   res,
				Err:      execErr,
				Duration: time.Since(start),
			}
			return nil
		})
	}

	// 개별 실패는 nil 반환으로 처리했으므로 eg.Wait() 에러는 ctx 취소만 발생
	eg.Wait() //nolint: errcheck
	return results
}
