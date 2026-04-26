// Package agent
// File: setters_phase8.go
// Description: Phase 8 컴포넌트 주입 setter 메서드 모음
// Responsibility: costTracker, bgManager, modelName 외부 주입 인터페이스 제공

package agent

import (
	"time"

	"github.com/yourorg/infractl/internal/background"
	"github.com/yourorg/infractl/internal/checkpoint"
	"github.com/yourorg/infractl/internal/cost"
	"github.com/yourorg/infractl/internal/hooks"
)

// SetCostTracker는 Phase 8 비용 추적기를 주입한다.
func (a *Agent) SetCostTracker(t *cost.Tracker) {
	a.costTracker = t
}

// SetBackgroundManager는 Phase 8 백그라운드 작업 매니저를 주입한다.
func (a *Agent) SetBackgroundManager(mgr *background.Manager) {
	a.bgManager = mgr
}

// SetModelName은 비용 기록에 사용할 LLM 모델명을 설정한다.
func (a *Agent) SetModelName(name string) {
	a.modelName = name
}

// SetCheckpointManager는 Phase 8 체크포인트 매니저를 주입한다.
func (a *Agent) SetCheckpointManager(mgr *checkpoint.Manager) {
	a.checkpointMgr = mgr
}

// SetHookRunner 는 claude_cli 스타일 hook runner 를 주입한다.
func (a *Agent) SetHookRunner(r *hooks.Runner) {
	a.hookRunner = r
}

// SetPromotionThreshold는 Phase F auto-promotion 임계값을 설정한다.
// 0이면 auto-promotion이 비활성화된다.
func (a *Agent) SetPromotionThreshold(d time.Duration) {
	a.promotionThreshold = d
}
