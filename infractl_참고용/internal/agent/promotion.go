// Package agent
// File: promotion.go
// Description: shell_exec 자동 백그라운드 프로모션 로직
// Responsibility: threshold 타이머와 도구 실행을 레이스, 초과 시 background.Manager에 등록

package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/yourorg/infractl/internal/background"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

// promotableTool은 auto-promotion 대상 도구 이름 집합이다.
// shell_exec 외에 장시간 실행이 예상되는 도구를 추가할 수 있다.
var promotableTools = map[string]bool{
	"shell_exec": true,
}

// promotionOutcome은 tryPromotion의 실행 결과이다.
type promotionOutcome struct {
	promoted bool
	jobID    int
	outcome  tools.ToolOutcome
	err      error
}

// tryPromotion은 tool.Execute를 goroutine으로 실행하고 threshold와 레이스한다.
//   - threshold 이내 완료: promoted=false, outcome에 결과
//   - threshold 초과: promoted=true, jobID에 등록된 작업 ID
//
// bgManager가 nil이면 프로모션 없이 동기 실행(블로킹)으로 폴백한다.
func tryPromotion(
	toolCtx context.Context,
	bgManager *background.Manager,
	tool tools.Tool,
	args map[string]interface{},
	exec executor.Executor,
	description string,
	threshold time.Duration,
) promotionOutcome {
	type execResult struct {
		outcome tools.ToolOutcome
		err     error
	}

	ch := make(chan execResult, 1)
	// 프로모션 가능성이 있는 도구는 원본 toolCtx가 취소되어도 계속 실행되어야 하므로
	// context.WithoutCancel을 사용하여 분리된 컨텍스트에서 실행한다.
	detachedCtx := context.WithoutCancel(toolCtx)
	go func() {
		o, e := tool.Execute(detachedCtx, args, exec)
		ch <- execResult{o, e}
	}()

	if bgManager == nil || threshold <= 0 {
		// 프로모션 비활성 — 결과가 올 때까지 블로킹
		r := <-ch
		return promotionOutcome{promoted: false, outcome: r.outcome, err: r.err}
	}

	timer := time.NewTimer(threshold)
	defer timer.Stop()

	select {
	case r := <-ch:
		// threshold 이내 완료
		return promotionOutcome{promoted: false, outcome: r.outcome, err: r.err}

	case <-timer.C:
		// threshold 초과 → 프로모션
		jobID, complete := bgManager.RegisterPending(description)

		// 원래 goroutine이 끝나면 job을 완료 처리
		go func() {
			r := <-ch
			complete(r.outcome.Content, r.err)
		}()

		return promotionOutcome{promoted: true, jobID: jobID}
	}
}

// promotionMessage는 프로모션된 작업의 LLM 응답 메시지를 생성한다.
func promotionMessage(tc llm.ToolCall, jobID int) llm.Message {
	return llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: tc.ID,
		Content: fmt.Sprintf(
			"Command is taking longer than expected and has been moved to the background as task #%d.\n"+
				"background_jobs(action=result, job_id=%d) waits up to 25 s and returns the result.\n"+
				"If the response is still 'running', do other useful work or check once more after a while.\n"+
				"Do NOT call the same tool repeatedly in a tight loop.\n"+
				"background_jobs(action=list) shows all running tasks.",
			jobID, jobID),
	}
}
