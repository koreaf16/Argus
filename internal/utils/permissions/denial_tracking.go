// Package permissions — 분류기 거부 추적.
//
// 파일 역할: auto 모드 분류기의 연속 거부·총 거부 횟수를 추적해
//
//	일정 한도를 초과하면 수동 프롬프트로 폴백하도록 한다.
//
// 포함 모듈:
//   - DenialTrackingState: 거부 추적 상태 구조체.
//   - DenialLimits: 최대 연속·총 거부 횟수 상수.
//   - NewDenialTrackingState() / RecordDenial() / RecordSuccess() / ShouldFallbackToPrompting().
//
// 호출/사용 방식:
//   - internal/utils/permissions/permissions.go 에서 auto 모드 분류기 호출 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/denialTracking.ts
package permissions

// DenialLimits 는 폴백 임계값이다.
var DenialLimits = struct {
	MaxConsecutive int
	MaxTotal       int
}{
	MaxConsecutive: 3,
	MaxTotal:       20,
}

// DenialTrackingState 는 분류기 거부 횟수 추적 상태다.
type DenialTrackingState struct {
	ConsecutiveDenials int
	TotalDenials       int
}

// NewDenialTrackingState 는 초기화된 DenialTrackingState 를 반환한다.
func NewDenialTrackingState() DenialTrackingState {
	return DenialTrackingState{}
}

// RecordDenial 은 거부 1건을 기록한 새 상태를 반환한다.
func RecordDenial(state DenialTrackingState) DenialTrackingState {
	return DenialTrackingState{
		ConsecutiveDenials: state.ConsecutiveDenials + 1,
		TotalDenials:       state.TotalDenials + 1,
	}
}

// RecordSuccess 는 성공 기록 후 연속 거부 카운터를 리셋한 새 상태를 반환한다.
func RecordSuccess(state DenialTrackingState) DenialTrackingState {
	if state.ConsecutiveDenials == 0 {
		return state
	}
	return DenialTrackingState{
		ConsecutiveDenials: 0,
		TotalDenials:       state.TotalDenials,
	}
}

// ShouldFallbackToPrompting 은 폴백 임계값에 도달했는지 확인한다.
func ShouldFallbackToPrompting(state DenialTrackingState) bool {
	return state.ConsecutiveDenials >= DenialLimits.MaxConsecutive ||
		state.TotalDenials >= DenialLimits.MaxTotal
}
