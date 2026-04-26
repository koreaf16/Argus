// Package permissions — 권한 업데이트 스키마 유틸리티.
//
// 파일 역할: types.PermissionUpdate 슬라이스를 ToolPermissionContext 에 적용하고
//
//	설정 파일에 영속화하는 헬퍼를 제공한다.
//
// 포함 모듈:
//   - ApplyPermissionUpdates(): 업데이트 슬라이스를 컨텍스트에 적용.
//   - PersistPermissionUpdates(): 설정 파일에 업데이트를 저장.
//
// 호출/사용 방식:
//   - Phase 2 에서 MCP SDK 권한 프롬프트 결과를 반영할 때 사용.
//
// 연결:
//   - 원본: src/utils/permissions/PermissionUpdateSchema.ts
package permissions

import (
	"github.com/koreaf16/argus/internal/types"
)

// ApplyPermissionUpdates 는 업데이트 목록을 ToolPermissionContext 에 적용하여 반환한다.
func ApplyPermissionUpdates(
	ctx types.ToolPermissionContext,
	updates []types.PermissionUpdate,
) types.ToolPermissionContext {
	for _, u := range updates {
		switch u.Type {
		case types.UpdateTypeAddRules:
			ctx = applyAddRules(ctx, u)
		case types.UpdateTypeRemoveRules:
			ctx = applyRemoveRules(ctx, u)
		case types.UpdateTypeReplaceRules:
			ctx = applyReplaceRules(ctx, u)
		case types.UpdateTypeSetMode:
			ctx = TransitionPermissionMode(ctx.Mode, u.Mode, ctx)
		case types.UpdateTypeAddDirectories:
			ctx = applyAddDirectories(ctx, u)
		case types.UpdateTypeRemoveDirectories:
			ctx = applyRemoveDirectories(ctx, u)
		}
	}
	return ctx
}

// PersistPermissionUpdates 는 영속 대상 업데이트를 설정 파일에 저장한다.
func PersistPermissionUpdates(updates []types.PermissionUpdate) {
	var toAdd []types.PermissionUpdate
	for _, u := range updates {
		if u.Destination != types.UpdateDestSession {
			toAdd = append(toAdd, u)
		}
	}
	if len(toAdd) == 0 {
		return
	}
	// 간소화: 허용 규칙만 영속화. 전체 구현은 Phase 2.
	for _, u := range toAdd {
		if u.Type == types.UpdateTypeAddRules {
			_ = AddPermissionRulesToSettings(u.Rules, u.Behavior)
		}
	}
}

func applyAddRules(ctx types.ToolPermissionContext, u types.PermissionUpdate) types.ToolPermissionContext {
	src := types.PermissionRuleSource(u.Destination)
	rules := rulesToStrings(u.Rules)
	switch u.Behavior {
	case types.BehaviorAllow:
		ctx.AlwaysAllowRules[src] = append(ctx.AlwaysAllowRules[src], rules...)
	case types.BehaviorDeny:
		ctx.AlwaysDenyRules[src] = append(ctx.AlwaysDenyRules[src], rules...)
	case types.BehaviorAsk:
		ctx.AlwaysAskRules[src] = append(ctx.AlwaysAskRules[src], rules...)
	}
	return ctx
}

func applyRemoveRules(ctx types.ToolPermissionContext, u types.PermissionUpdate) types.ToolPermissionContext {
	src := types.PermissionRuleSource(u.Destination)
	toRemove := make(map[string]bool)
	for _, rv := range u.Rules {
		toRemove[PermissionRuleValueToString(rv)] = true
	}
	switch u.Behavior {
	case types.BehaviorAllow:
		ctx.AlwaysAllowRules[src] = filterRules(ctx.AlwaysAllowRules[src], toRemove)
	case types.BehaviorDeny:
		ctx.AlwaysDenyRules[src] = filterRules(ctx.AlwaysDenyRules[src], toRemove)
	case types.BehaviorAsk:
		ctx.AlwaysAskRules[src] = filterRules(ctx.AlwaysAskRules[src], toRemove)
	}
	return ctx
}

func applyReplaceRules(ctx types.ToolPermissionContext, u types.PermissionUpdate) types.ToolPermissionContext {
	src := types.PermissionRuleSource(u.Destination)
	rules := rulesToStrings(u.Rules)
	switch u.Behavior {
	case types.BehaviorAllow:
		ctx.AlwaysAllowRules[src] = rules
	case types.BehaviorDeny:
		ctx.AlwaysDenyRules[src] = rules
	case types.BehaviorAsk:
		ctx.AlwaysAskRules[src] = rules
	}
	return ctx
}

func applyAddDirectories(ctx types.ToolPermissionContext, u types.PermissionUpdate) types.ToolPermissionContext {
	if ctx.AdditionalWorkingDirectories == nil {
		ctx.AdditionalWorkingDirectories = make(map[string]types.AdditionalWorkingDirectory)
	}
	src := types.PermissionRuleSource(u.Destination)
	for _, d := range u.Directories {
		ctx.AdditionalWorkingDirectories[d] = types.AdditionalWorkingDirectory{Path: d, Source: src}
	}
	return ctx
}

func applyRemoveDirectories(ctx types.ToolPermissionContext, u types.PermissionUpdate) types.ToolPermissionContext {
	for _, d := range u.Directories {
		delete(ctx.AdditionalWorkingDirectories, d)
	}
	return ctx
}

func rulesToStrings(rules []types.PermissionRuleValue) []string {
	out := make([]string, len(rules))
	for i, rv := range rules {
		out[i] = PermissionRuleValueToString(rv)
	}
	return out
}

func filterRules(rules []string, remove map[string]bool) []string {
	out := rules[:0]
	for _, r := range rules {
		if !remove[r] {
			out = append(out, r)
		}
	}
	return out
}
