// Package permissions — 도달 불가 규칙 감지.
//
// 파일 역할: 더 넓은 규칙(도구 전체 deny/ask)이 좁은 규칙(특정 내용 allow)을
//
//	가리는 경우를 감지하여 경고한다.
//
// 포함 모듈:
//   - UnreachableRule: 도달 불가 규칙 정보.
//   - DetectUnreachableRules(): 규칙 목록에서 가려진 규칙 검출.
//
// 호출/사용 방식:
//   - 설정 검증 또는 권한 규칙 편집 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/shadowedRuleDetection.ts
package permissions

import "github.com/koreaf16/argus/internal/types"

// ShadowType 은 가리기 방식이다.
type ShadowType string

const (
	ShadowTypeAsk  ShadowType = "ask"
	ShadowTypeDeny ShadowType = "deny"
)

// UnreachableRule 은 다른 규칙에 의해 가려진 규칙이다.
type UnreachableRule struct {
	Rule       types.PermissionRule
	ShadowedBy types.PermissionRule
	ShadowType ShadowType
}

// DetectUnreachableRules 는 가려진(도달 불가) 규칙 목록을 반환한다.
// 도구 전체 deny/ask 규칙이 같은 도구의 구체적인 allow 규칙을 가리면 검출된다.
func DetectUnreachableRules(rules []types.PermissionRule) []UnreachableRule {
	var unreachable []UnreachableRule

	// 도구별 와이드 deny/ask 규칙 수집
	type wideShadow struct {
		rule       types.PermissionRule
		shadowType ShadowType
	}
	wide := map[string][]wideShadow{}
	for _, r := range rules {
		if r.RuleValue.RuleContent == "" {
			if r.RuleBehavior == types.BehaviorDeny {
				wide[r.RuleValue.ToolName] = append(wide[r.RuleValue.ToolName], wideShadow{r, ShadowTypeDeny})
			} else if r.RuleBehavior == types.BehaviorAsk {
				wide[r.RuleValue.ToolName] = append(wide[r.RuleValue.ToolName], wideShadow{r, ShadowTypeAsk})
			}
		}
	}

	// 구체적인 allow 규칙 중 가려진 것 검출
	for _, r := range rules {
		if r.RuleBehavior != types.BehaviorAllow || r.RuleValue.RuleContent == "" {
			continue
		}
		for _, shadow := range wide[r.RuleValue.ToolName] {
			unreachable = append(unreachable, UnreachableRule{
				Rule:       r,
				ShadowedBy: shadow.rule,
				ShadowType: shadow.shadowType,
			})
		}
	}
	return unreachable
}
