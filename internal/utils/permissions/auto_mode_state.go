// Package permissions — auto 모드 전역 상태 관리.
//
// 파일 역할: auto 모드의 활성 상태·CLI 플래그·서킷 브레이커를 패키지 수준 변수로 관리한다.
//
//	sync.Mutex 로 고루틴 안전을 보장한다.
//
// 포함 모듈:
//   - SetAutoModeActive / IsAutoModeActive(): 현재 auto 모드 활성 여부.
//   - SetAutoModeFlagCli / GetAutoModeFlagCli(): CLI --auto 플래그 기록.
//   - SetAutoModeCircuitBroken / IsAutoModeCircuitBroken(): GrowthBook 킥아웃 서킷 브레이커.
//   - ResetAutoModeStateForTesting(): 테스트용 초기화.
//
// 호출/사용 방식:
//   - internal/utils/permissions/bypass_killswitch.go 의 IsAutoModeAvailable() 에서 참조.
//   - REPL 슬래시 커맨드가 모드 전환 시 SetAutoModeActive 를 호출한다.
//
// 연결:
//   - 원본: src/utils/permissions/autoModeState.ts
package permissions

import "sync"

var autoMu sync.Mutex

var (
	autoModeActive        bool
	autoModeFlagCli       bool
	autoModeCircuitBroken bool
)

// SetAutoModeActive 는 auto 모드 활성 상태를 설정한다.
func SetAutoModeActive(active bool) {
	autoMu.Lock()
	defer autoMu.Unlock()
	autoModeActive = active
}

// IsAutoModeActive 는 auto 모드가 현재 활성 상태인지 반환한다.
func IsAutoModeActive() bool {
	autoMu.Lock()
	defer autoMu.Unlock()
	return autoModeActive
}

// SetAutoModeFlagCli 는 CLI --auto 플래그 전달 여부를 기록한다.
func SetAutoModeFlagCli(passed bool) {
	autoMu.Lock()
	defer autoMu.Unlock()
	autoModeFlagCli = passed
}

// GetAutoModeFlagCli 는 CLI --auto 플래그가 전달되었는지 반환한다.
func GetAutoModeFlagCli() bool {
	autoMu.Lock()
	defer autoMu.Unlock()
	return autoModeFlagCli
}

// SetAutoModeCircuitBroken 은 GrowthBook 킥아웃으로 인한 서킷 브레이커 상태를 설정한다.
func SetAutoModeCircuitBroken(broken bool) {
	autoMu.Lock()
	defer autoMu.Unlock()
	autoModeCircuitBroken = broken
}

// IsAutoModeCircuitBroken 은 auto 모드가 서킷 브레이커로 차단된 상태인지 반환한다.
func IsAutoModeCircuitBroken() bool {
	autoMu.Lock()
	defer autoMu.Unlock()
	return autoModeCircuitBroken
}

// ResetAutoModeStateForTesting 은 테스트용으로 모든 auto 모드 상태를 초기화한다.
func ResetAutoModeStateForTesting() {
	autoMu.Lock()
	defer autoMu.Unlock()
	autoModeActive = false
	autoModeFlagCli = false
	autoModeCircuitBroken = false
}
