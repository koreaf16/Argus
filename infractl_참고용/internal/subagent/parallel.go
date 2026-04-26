// Package subagent
// File: parallel.go
// Description: errgroup 기반 병렬 서브에이전트 실행 (sibling cancel + 동시성 제한)
// Responsibility: RunParallel / RunOpts 제공 — WaitGroup 경로를 완전 교체

package subagent

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// RunOpts는 병렬 서브에이전트 실행 옵션이다.
type RunOpts struct {
	// MaxParallel은 동시에 실행할 최대 goroutine 수이다. 0이면 무제한.
	MaxParallel int
	// ContinueOnError가 true이면 sibling cancel을 비활성화한다.
	// false(기본)이면 첫 에러 발생 시 공유 ctx를 취소해 나머지 sibling이 조기 종료한다.
	ContinueOnError bool
	// EventCb는 실시간 이벤트 콜백이다. nil 허용.
	EventCb EventCallback
}

// RunParallel은 configs를 errgroup으로 병렬 실행하고 입력 순서와 동일한 결과 슬라이스를 반환한다.
//
// ContinueOnError=false (기본): 첫 SubagentResult.Err → errgroup ctx 취소 → sibling이
// 다음 llm.Client.Chat 호출에서 ctx.Done()을 감지하고 조기 종료한다.
//
// ContinueOnError=true: 에러가 있어도 sibling을 계속 실행한다 (부분 실패 허용).
func RunParallel(ctx context.Context, configs []SubagentConfig, opts RunOpts, runner *Runner) []SubagentResult {
	results := make([]SubagentResult, len(configs))

	g, gCtx := errgroup.WithContext(ctx)
	if opts.MaxParallel > 0 {
		g.SetLimit(opts.MaxParallel)
	}

	for i, cfg := range configs {
		g.Go(func() error {
			// runner 얕은 복사 — eventCb race 없음 (독립 필드)
			localRunner := *runner
			localRunner.eventCb = opts.EventCb

			r := localRunner.Execute(gCtx, cfg)
			results[i] = r

			if r.Err != nil && !opts.ContinueOnError {
				// 에러를 반환하면 errgroup이 gCtx를 취소 → sibling 조기 종료
				return r.Err
			}
			return nil
		})
	}

	// 에러는 SubagentResult.Err에 이미 저장됐으므로 errgroup 반환값은 무시한다.
	_ = g.Wait()
	return results
}
