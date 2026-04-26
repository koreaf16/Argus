// Package planmode — Plan mode state and operations.
//
// 파일 역할: Manages the plan mode lifecycle (enter, exit, check status).
//
//	Plan mode is a structured execution mode for decomposing complex tasks.
//
// 포함 모듈:
//   - EnterPlanMode: Enters plan mode
//   - ExitPlanMode: Exits plan mode
//   - IsInPlanMode: Checks if currently in plan mode
//
// 호출/사용 방식:
//   - REPL uses this to manage mode transitions
//   - EnterPlanMode and ExitPlanMode tools call these functions
//
// 연결:
//   - internal/planmode: plans.go
//   - internal/state: app state management
//   - 원본: src/utils/planModeV2.ts
package planmode

import (
	"sync"
	"time"
)

// PlanModeManager manages plan mode state.
type PlanModeManager struct {
	mu        sync.RWMutex
	enabled   bool
	startedAt time.Time
	plan      string
}

// GlobalPlanMode is the global plan mode manager instance.
var GlobalPlanMode = &PlanModeManager{
	enabled: false,
}

// EnterPlanMode enters plan mode with a given plan description.
func EnterPlanMode(planDescription string) error {
	GlobalPlanMode.mu.Lock()
	defer GlobalPlanMode.mu.Unlock()

	GlobalPlanMode.enabled = true
	GlobalPlanMode.startedAt = time.Now()
	GlobalPlanMode.plan = planDescription

	return nil
}

// ExitPlanMode exits plan mode.
func ExitPlanMode() error {
	GlobalPlanMode.mu.Lock()
	defer GlobalPlanMode.mu.Unlock()

	GlobalPlanMode.enabled = false
	GlobalPlanMode.plan = ""

	return nil
}

// IsInPlanMode checks if currently in plan mode.
func IsInPlanMode() bool {
	GlobalPlanMode.mu.RLock()
	defer GlobalPlanMode.mu.RUnlock()

	return GlobalPlanMode.enabled
}

// GetCurrentPlan returns the current plan description.
func GetCurrentPlan() string {
	GlobalPlanMode.mu.RLock()
	defer GlobalPlanMode.mu.RUnlock()

	return GlobalPlanMode.plan
}

// GetPlanStartTime returns when the current plan started.
func GetPlanStartTime() time.Time {
	GlobalPlanMode.mu.RLock()
	defer GlobalPlanMode.mu.RUnlock()

	return GlobalPlanMode.startedAt
}

// GetPlanElapsedTime returns the elapsed time since plan mode started.
func GetPlanElapsedTime() time.Duration {
	GlobalPlanMode.mu.RLock()
	defer GlobalPlanMode.mu.RUnlock()

	if !GlobalPlanMode.enabled {
		return 0
	}

	return time.Since(GlobalPlanMode.startedAt)
}
