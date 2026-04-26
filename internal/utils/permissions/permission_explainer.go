// Package permissions — 권한 결정 이유 설명 생성기.
//
// 파일 역할: 권한 결정 이유를 위험 수준과 함께 사용자용 설명으로 변환한다.
//
//	Phase 1 에서는 기본적인 위험 수준 분류만 구현한다.
//
// 포함 모듈:
//   - ExplainPermission(): PermissionDecisionReason → PermissionExplanation.
//
// 호출/사용 방식:
//   - 권한 프롬프트 UI 에서 사용자에게 이유를 표시할 때 참조.
//
// 연결:
//   - 원본: src/utils/permissions/permissionExplainer.ts
package permissions

import "github.com/koreaf16/argus/internal/types"

// ExplainPermission 은 권한 결정 이유를 사용자용 설명으로 변환한다.
func ExplainPermission(reason *types.PermissionDecisionReason, toolName string) types.PermissionExplanation {
	if reason == nil {
		return types.PermissionExplanation{
			RiskLevel:   types.RiskMedium,
			Explanation: toolName + " 도구가 실행을 요청합니다.",
			Reasoning:   "명시적인 이유가 없습니다.",
			Risk:        "알 수 없는 위험 수준",
		}
	}

	switch reason.Type {
	case types.DecisionReasonSafetyCheck:
		return types.PermissionExplanation{
			RiskLevel:   types.RiskHigh,
			Explanation: reason.Reason,
			Reasoning:   "보안 검사에서 위험이 감지되었습니다.",
			Risk:        "높은 위험 — 시스템 보안 또는 데이터에 영향을 줄 수 있습니다.",
		}
	case types.DecisionReasonRule:
		return types.PermissionExplanation{
			RiskLevel:   types.RiskMedium,
			Explanation: "권한 규칙이 확인을 요청합니다.",
			Reasoning:   "설정된 규칙에 따라 사용자 확인이 필요합니다.",
			Risk:        "중간 위험 — 규칙에 의해 검토가 필요합니다.",
		}
	case types.DecisionReasonMode:
		return types.PermissionExplanation{
			RiskLevel:   types.RiskLow,
			Explanation: "현재 권한 모드가 확인을 요청합니다.",
			Reasoning:   "기본 권한 모드에서는 작업 전 확인이 필요합니다.",
			Risk:        "낮은 위험 — 표준 권한 확인입니다.",
		}
	default:
		return types.PermissionExplanation{
			RiskLevel:   types.RiskMedium,
			Explanation: reason.Reason,
			Reasoning:   "권한 확인이 필요합니다.",
			Risk:        "알 수 없는 위험 수준",
		}
	}
}
