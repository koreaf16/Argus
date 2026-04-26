// Package permissions — 권한 결과 설명 헬퍼.
//
// 파일 역할: 권한 결정 행동(allow/deny/ask)을 사용자 메시지로 변환한다.
// 포함 모듈:
//   - GetRuleBehaviorDescription(): 행동 → 사용자용 설명 문자열.
//
// 호출/사용 방식:
//   - 권한 프롬프트 UI, 도구 결과 메시지 생성 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/PermissionResult.ts
package permissions

import "github.com/koreaf16/argus/internal/types"

// GetRuleBehaviorDescription 은 권한 행동의 사용자용 설명 문자열을 반환한다.
func GetRuleBehaviorDescription(behavior types.PermissionBehavior) string {
	switch behavior {
	case types.BehaviorAllow:
		return "allowed"
	case types.BehaviorDeny:
		return "denied"
	case types.BehaviorAsk:
		return "asked for confirmation for"
	default:
		return string(behavior)
	}
}
