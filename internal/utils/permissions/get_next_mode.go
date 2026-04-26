// Package permissions — 권한 모드 사이클링.
//
// 파일 역할: Shift+Tab 으로 권한 모드를 순환할 때 다음 모드를 결정한다.
//
//	모드 전환 순서: default → acceptEdits → plan → bypassPermissions → default.
//
// 포함 모듈:
//   - GetNextPermissionMode(): 다음 모드 반환.
//   - CyclePermissionMode(): 다음 모드 + 컨텍스트 반환.
//
// 호출/사용 방식:
//   - internal/repl/ 에서 Shift+Tab 키 처리 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/getNextPermissionMode.ts
//   - internal/types/permissions.go: PermissionMode, ToolPermissionContext
package permissions

import "github.com/koreaf16/argus/internal/types"

// GetNextPermissionMode 는 현재 모드에서 다음 모드를 반환한다.
func GetNextPermissionMode(ctx types.ToolPermissionContext) types.PermissionMode {
	switch ctx.Mode {
	case types.PermissionModeDefault:
		return types.PermissionModeAcceptEdits

	case types.PermissionModeAcceptEdits:
		return types.PermissionModePlan

	case types.PermissionModePlan:
		if ctx.IsBypassPermissionsModeAvailable {
			return types.PermissionModeBypassPermissions
		}
		return types.PermissionModeDefault

	case types.PermissionModeBypassPermissions:
		return types.PermissionModeDefault

	case types.PermissionModeDontAsk:
		return types.PermissionModeDefault

	default:
		return types.PermissionModeDefault
	}
}

// CyclePermissionMode 는 다음 모드와 업데이트된 컨텍스트를 반환한다.
func CyclePermissionMode(ctx types.ToolPermissionContext) (types.PermissionMode, types.ToolPermissionContext) {
	nextMode := GetNextPermissionMode(ctx)
	newCtx := ctx
	newCtx.Mode = nextMode
	return nextMode, newCtx
}
