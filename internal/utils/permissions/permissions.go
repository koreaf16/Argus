// Package permissions — 권한 확인 메인 엔진.
//
// 파일 역할: 도구 실행 전 권한을 확인하는 핵심 파이프라인을 구현한다.
//
//	7단계 파이프라인: deny 규칙 → ask 규칙 → 도구별 checkPermission →
//	acceptEdits 패스스루 → always-allow 규칙 → auto 분류기 → 모드 기반 결정.
//
// 포함 모듈:
//   - HasPermissionToUseTool(): 메인 권한 확인 함수.
//   - GetAllowRules() / GetDenyRules() / GetAskRules(): 컨텍스트에서 규칙 추출.
//   - ToolAlwaysAllowedRule(): 도구 전체 허용 규칙 검색.
//   - CreatePermissionRequestMessage(): 사용자 메시지 생성.
//   - ApplyPermissionRulesToContext(): 디스크 규칙을 컨텍스트에 적용.
//
// 호출/사용 방식:
//   - internal/query/engine.go 의 도구 실행 전 권한 게이트에서 호출.
//
// 연결:
//   - 원본: src/utils/permissions/permissions.ts
//   - internal/types/permissions.go: 권한 타입 정의
package permissions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/koreaf16/argus/internal/types"
)

// ToolInterface 는 권한 확인에 필요한 도구 인터페이스 최소 집합이다.
type ToolInterface interface {
	Name() string
	IsReadOnly() bool
	CheckPermission(ctx context.Context, permCtx types.ToolPermissionContext, input json.RawMessage) (types.PermissionResult, error)
}

// GetAllowRules 는 컨텍스트에서 allow 규칙을 추출한다.
func GetAllowRules(ctx types.ToolPermissionContext) []types.PermissionRule {
	var rules []types.PermissionRule
	for source, patterns := range ctx.AlwaysAllowRules {
		for _, p := range patterns {
			rules = append(rules, types.PermissionRule{
				Source:       source,
				RuleBehavior: types.BehaviorAllow,
				RuleValue:    PermissionRuleValueFromString(p),
			})
		}
	}
	return rules
}

// GetDenyRules 는 컨텍스트에서 deny 규칙을 추출한다.
func GetDenyRules(ctx types.ToolPermissionContext) []types.PermissionRule {
	var rules []types.PermissionRule
	for source, patterns := range ctx.AlwaysDenyRules {
		for _, p := range patterns {
			rules = append(rules, types.PermissionRule{
				Source:       source,
				RuleBehavior: types.BehaviorDeny,
				RuleValue:    PermissionRuleValueFromString(p),
			})
		}
	}
	return rules
}

// GetAskRules 는 컨텍스트에서 ask 규칙을 추출한다.
func GetAskRules(ctx types.ToolPermissionContext) []types.PermissionRule {
	var rules []types.PermissionRule
	for source, patterns := range ctx.AlwaysAskRules {
		for _, p := range patterns {
			rules = append(rules, types.PermissionRule{
				Source:       source,
				RuleBehavior: types.BehaviorAsk,
				RuleValue:    PermissionRuleValueFromString(p),
			})
		}
	}
	return rules
}

// toolMatchesRule 은 도구가 도구 전체 규칙(content 없는 규칙)과 일치하는지 확인한다.
func toolMatchesRule(toolName string, rule types.PermissionRule) bool {
	return rule.RuleValue.RuleContent == "" && rule.RuleValue.ToolName == toolName
}

// ToolAlwaysAllowedRule 은 도구 전체에 적용되는 allow 규칙을 반환한다.
func ToolAlwaysAllowedRule(ctx types.ToolPermissionContext, toolName string) *types.PermissionRule {
	for _, r := range GetAllowRules(ctx) {
		if toolMatchesRule(toolName, r) {
			r := r // 지역 복사
			return &r
		}
	}
	return nil
}

// GetDenyRuleForTool 은 도구 전체에 적용되는 deny 규칙을 반환한다.
func GetDenyRuleForTool(ctx types.ToolPermissionContext, toolName string) *types.PermissionRule {
	for _, r := range GetDenyRules(ctx) {
		if toolMatchesRule(toolName, r) {
			r := r
			return &r
		}
	}
	return nil
}

// GetAskRuleForTool 은 도구 전체에 적용되는 ask 규칙을 반환한다.
func GetAskRuleForTool(ctx types.ToolPermissionContext, toolName string) *types.PermissionRule {
	for _, r := range GetAskRules(ctx) {
		if toolMatchesRule(toolName, r) {
			r := r
			return &r
		}
	}
	return nil
}

// CreatePermissionRequestMessage 는 도구 사용 권한 요청 메시지를 생성한다.
func CreatePermissionRequestMessage(toolName string, reason *types.PermissionDecisionReason) string {
	if reason == nil {
		return fmt.Sprintf("%s 도구를 사용하려면 권한이 필요합니다.", toolName)
	}
	switch reason.Type {
	case types.DecisionReasonRule:
		if reason.Rule != nil {
			rs := PermissionRuleValueToString(reason.Rule.RuleValue)
			return fmt.Sprintf("권한 규칙 '%s' 이 %s 명령 승인을 요청합니다.", rs, toolName)
		}
	case types.DecisionReasonMode:
		return fmt.Sprintf("현재 권한 모드(%s)가 %s 명령 승인을 요청합니다.", reason.Mode, toolName)
	case types.DecisionReasonHook:
		if reason.Reason != "" {
			return fmt.Sprintf("훅 '%s' 이 이 작업을 차단했습니다: %s", reason.HookName, reason.Reason)
		}
		return fmt.Sprintf("훅 '%s' 이 %s 명령 승인을 요청합니다.", reason.HookName, toolName)
	case types.DecisionReasonSafetyCheck, types.DecisionReasonWorkingDir, types.DecisionReasonOther:
		return reason.Reason
	}
	return fmt.Sprintf("%s 도구를 사용하려면 권한이 필요합니다.", toolName)
}

// HasPermissionToUseTool 은 도구 실행 전 권한을 확인한다.
// 7단계 파이프라인:
//  1. bypassPermissions 모드 → 바로 허용
//  2. deny 규칙 확인
//  3. ask 규칙 확인
//  4. 도구별 CheckPermission() 호출
//  5. always-allow 규칙 확인
//  6. plan 모드 게이트 (읽기 전용만 허용)
//  7. 모드 기반 기본 결정
func HasPermissionToUseTool(
	ctx context.Context,
	tool ToolInterface,
	permCtx types.ToolPermissionContext,
	input json.RawMessage,
) types.PermissionResult {

	// 1. bypassPermissions 모드: 안전 체크 외 모든 허용
	if permCtx.Mode == types.PermissionModeBypassPermissions {
		return types.PermissionResult{Behavior: types.BehaviorAllow}
	}

	// 1.5 plan 모드: 읽기 전용 도구만 허용
	if permCtx.Mode == types.PermissionModePlan && !tool.IsReadOnly() {
		reason := &types.PermissionDecisionReason{
			Type:   types.DecisionReasonMode,
			Mode:   types.PermissionModePlan,
			Reason: "plan 모드에서는 쓰기 작업이 허용되지 않습니다.",
		}
		return types.PermissionResult{
			Behavior:       types.BehaviorDeny,
			Message:        reason.Reason,
			DecisionReason: reason,
		}
	}

	// 2. deny 규칙 확인
	if r := GetDenyRuleForTool(permCtx, tool.Name()); r != nil {
		reason := &types.PermissionDecisionReason{Type: types.DecisionReasonRule, Rule: r}
		return types.PermissionResult{
			Behavior:       types.BehaviorDeny,
			Message:        CreatePermissionRequestMessage(tool.Name(), reason),
			DecisionReason: reason,
		}
	}

	// 3. ask 규칙 확인
	if r := GetAskRuleForTool(permCtx, tool.Name()); r != nil {
		reason := &types.PermissionDecisionReason{Type: types.DecisionReasonRule, Rule: r}
		return types.PermissionResult{
			Behavior:       types.BehaviorAsk,
			Message:        CreatePermissionRequestMessage(tool.Name(), reason),
			DecisionReason: reason,
		}
	}

	// 4. 도구별 권한 확인
	if result, err := tool.CheckPermission(ctx, permCtx, input); err == nil {
		if result.Behavior == types.BehaviorDeny || result.Behavior == types.BehaviorAsk || result.Behavior == types.BehaviorAllow {
			return result
		}
	}

	// 5. always-allow 규칙 확인
	if ToolAlwaysAllowedRule(permCtx, tool.Name()) != nil {
		return types.PermissionResult{Behavior: types.BehaviorAllow}
	}

	// 7. 모드 기반 기본 결정
	switch permCtx.Mode {
	case types.PermissionModeDontAsk:
		return types.PermissionResult{Behavior: types.BehaviorAllow}
	case types.PermissionModeAcceptEdits:
		return types.PermissionResult{Behavior: types.BehaviorAllow}
	default: // "default" 모드: 확인 요청
		reason := &types.PermissionDecisionReason{
			Type: types.DecisionReasonMode,
			Mode: permCtx.Mode,
		}
		return types.PermissionResult{
			Behavior:       types.BehaviorAsk,
			Message:        CreatePermissionRequestMessage(tool.Name(), reason),
			DecisionReason: reason,
		}
	}
}

// ApplyPermissionRulesToContext 는 디스크에서 로드한 규칙을 ToolPermissionContext 에 적용한다.
func ApplyPermissionRulesToContext(ctx *types.ToolPermissionContext, rules []types.PermissionRule) {
	if ctx.AlwaysAllowRules == nil {
		ctx.AlwaysAllowRules = make(types.ToolPermissionRulesBySource)
	}
	if ctx.AlwaysDenyRules == nil {
		ctx.AlwaysDenyRules = make(types.ToolPermissionRulesBySource)
	}
	if ctx.AlwaysAskRules == nil {
		ctx.AlwaysAskRules = make(types.ToolPermissionRulesBySource)
	}
	for _, r := range rules {
		ruleStr := PermissionRuleValueToString(r.RuleValue)
		switch r.RuleBehavior {
		case types.BehaviorAllow:
			ctx.AlwaysAllowRules[r.Source] = append(ctx.AlwaysAllowRules[r.Source], ruleStr)
		case types.BehaviorDeny:
			ctx.AlwaysDenyRules[r.Source] = append(ctx.AlwaysDenyRules[r.Source], ruleStr)
		case types.BehaviorAsk:
			ctx.AlwaysAskRules[r.Source] = append(ctx.AlwaysAskRules[r.Source], ruleStr)
		}
	}
}

// NewDefaultPermissionContext 는 기본 설정으로 ToolPermissionContext 를 생성한다.
func NewDefaultPermissionContext() types.ToolPermissionContext {
	ctx := types.ToolPermissionContext{
		Mode:                             types.PermissionModeDefault,
		AlwaysAllowRules:                 make(types.ToolPermissionRulesBySource),
		AlwaysDenyRules:                  make(types.ToolPermissionRulesBySource),
		AlwaysAskRules:                   make(types.ToolPermissionRulesBySource),
		AdditionalWorkingDirectories:     make(map[string]types.AdditionalWorkingDirectory),
		IsBypassPermissionsModeAvailable: IsBypassPermissionsModeAvailable(),
	}
	rules := LoadAllPermissionRulesFromDisk()
	ApplyPermissionRulesToContext(&ctx, rules)
	return ctx
}
