// Package permissions — 권한 모드 초기화·전환.
//
// 파일 역할: 권한 모드 전환 시 필요한 컨텍스트 정리 작업을 수행한다.
//
//	auto 모드 진입 시 위험한 bash 허용 규칙을 제거한다.
//
// 포함 모듈:
//   - TransitionPermissionMode(): 모드 전환 + 컨텍스트 정리.
//   - IsDangerousBashPermission(): Bash 허용 규칙이 위험한지 판단.
//   - StripDangerousPermissions(): 위험한 규칙을 컨텍스트에서 제거.
//
// 호출/사용 방식:
//   - internal/utils/permissions/get_next_mode.go 의 CyclePermissionMode() 에서 참조.
//
// 연결:
//   - 원본: src/utils/permissions/permissionSetup.ts (핵심만 포함)
package permissions

import (
	"strings"

	"github.com/koreaf16/argus/internal/types"
)

// IsDangerousBashPermission 은 Bash 허용 규칙 패턴이 위험한지 판단한다.
// 코드 인터프리터·패키지 실행기·셸 등의 접두사로 시작하면 위험하다.
func IsDangerousBashPermission(ruleContent string) bool {
	lower := strings.ToLower(strings.TrimSpace(ruleContent))
	for _, pattern := range DangerousBashPatterns {
		p := strings.ToLower(pattern)
		if lower == p ||
			strings.HasPrefix(lower, p+" ") ||
			strings.HasPrefix(lower, p+":") {
			return true
		}
	}
	return false
}

// StripDangerousPermissions 는 auto 모드 진입 시 위험한 허용 규칙을 컨텍스트에서 제거한다.
func StripDangerousPermissions(ctx types.ToolPermissionContext) types.ToolPermissionContext {
	newAllowRules := make(types.ToolPermissionRulesBySource)
	for source, patterns := range ctx.AlwaysAllowRules {
		for _, p := range patterns {
			rv := PermissionRuleValueFromString(p)
			if rv.ToolName == "Bash" && rv.RuleContent != "" && IsDangerousBashPermission(rv.RuleContent) {
				// 위험한 Bash 규칙 제거
				if ctx.StrippedDangerousRules == nil {
					ctx.StrippedDangerousRules = make(types.ToolPermissionRulesBySource)
				}
				ctx.StrippedDangerousRules[source] = append(ctx.StrippedDangerousRules[source], p)
				continue
			}
			newAllowRules[source] = append(newAllowRules[source], p)
		}
	}
	ctx.AlwaysAllowRules = newAllowRules
	return ctx
}

// TransitionPermissionMode 는 모드 전환 시 컨텍스트를 정리하여 반환한다.
func TransitionPermissionMode(
	fromMode, toMode types.PermissionMode,
	ctx types.ToolPermissionContext,
) types.ToolPermissionContext {
	ctx.Mode = toMode
	// auto 모드 진입 시 위험한 규칙 제거
	if toMode == types.PermissionModeAuto {
		ctx = StripDangerousPermissions(ctx)
	}
	return ctx
}
