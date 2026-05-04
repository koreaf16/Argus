// Package permissions — 권한 모드 사이클링.
//
// 파일 역할: Shift+Tab 으로 권한 모드를 순환할 때 다음 모드를 결정한다.
//
//	모드 전환 순서: default → plan → bypassPermissions(YORO) → default.
//
// 포함 모듈:
//   - GetNextPermissionMode(): 다음 모드 반환.
//   - CyclePermissionMode(): 다음 모드 + 컨텍스트 반환.
//
// 호출/사용 방식:
//   - internal/tui/model.go cyclePermissionMode() 가 Shift+Tab 처리 시 호출.
//   - YORO 즉시 진입은 Ctrl+Y(toggleYoroMode) 가 별도 처리.
//
// 연결:
//   - internal/types/permissions.go: PermissionMode, ToolPermissionContext
package permissions

import "github.com/koreaf16/argus/internal/types"

// GetNextPermissionMode 는 현재 모드에서 다음 모드를 반환한다.
// 사용자에게 노출되는 모드는 default / plan / YORO(bypassPermissions) 3가지뿐이며,
// 그 외 내부 모드(acceptEdits, dontAsk 등)에서 진입한 경우 default 로 빠져나온다.
func GetNextPermissionMode(ctx types.ToolPermissionContext) types.PermissionMode {
	switch ctx.Mode {
	case types.PermissionModeDefault:
		return types.PermissionModePlan

	case types.PermissionModePlan:
		if ctx.IsBypassPermissionsModeAvailable {
			return types.PermissionModeBypassPermissions
		}
		return types.PermissionModeDefault

	case types.PermissionModeBypassPermissions:
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
