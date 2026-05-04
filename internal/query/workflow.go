package query

import (
	"encoding/json"
)

// ValidateWorkflowStep 은 도구 호출이 현재 워크플로우 단계에서 허용되는지 검증합니다.
// 이제 워크플로우 시스템이 제거되어 항상 허용합니다.
func ValidateWorkflowStep(_ any, _ string, _ json.RawMessage, _ bool) (bool, string) {
	return false, ""
}

// triggerWorkflowHeuristic 은 빌드 호환성을 위해 유지합니다.
func triggerWorkflowHeuristic(_ string) bool {
	return false
}
