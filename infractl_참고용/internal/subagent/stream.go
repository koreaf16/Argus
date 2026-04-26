// Package subagent
// File: stream.go
// Description: yield-as-ready 결과 스트리밍 — 완료된 서브에이전트부터 채널로 반환
// Responsibility: RunParallelStream 제공 — 입력 순서가 아닌 완료 순서로 결과 수신

package subagent

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// RunParallelStream은 configs를 병렬로 실행하고 완료된 결과부터 채널로 반환한다.
// 채널은 모든 goroutine이 끝나면 닫힌다.
//
// 소비자는 for-range로 채널을 읽으면 된다:
//
//	for r := range RunParallelStream(ctx, configs, opts, runner) {
//	    process(r)
//	}
//
// 결과 순서는 입력 순서와 다를 수 있다 (완료된 순서). 순서가 중요하면 RunParallel을 사용한다.
func RunParallelStream(ctx context.Context, configs []SubagentConfig, opts RunOpts, runner *Runner) <-chan SubagentResult {
	// 버퍼 크기 = len(configs) — goroutine이 ch <- r 에서 블로킹되지 않도록
	ch := make(chan SubagentResult, len(configs))

	go func() {
		defer close(ch)

		g, gCtx := errgroup.WithContext(ctx)
		if opts.MaxParallel > 0 {
			g.SetLimit(opts.MaxParallel)
		}

		for _, cfg := range configs {
			g.Go(func() error {
				localRunner := *runner
				localRunner.eventCb = opts.EventCb

				r := localRunner.Execute(gCtx, cfg)

				// 결과를 즉시 채널로 전달 (yield-as-ready)
				select {
				case ch <- r:
				case <-gCtx.Done():
					// ctx가 취소됐으면 결과 드롭 후 에러 반환
					return gCtx.Err()
				}

				if r.Err != nil && !opts.ContinueOnError {
					return r.Err
				}
				return nil
			})
		}

		_ = g.Wait()
	}()

	return ch
}
