// Package permissions — 권한 모드 표시·변환 헬퍼.
//
// 파일 역할: 권한 모드의 표시 이름·심볼을 반환하고 문자열을 모드로 변환한다.
// 포함 모듈:
//   - PermissionModeFromString(): 문자열 → PermissionMode (유효성 검사 포함).
//   - PermissionModeSymbol(): 모드 → 짧은 심볼 문자.
//   - IsExternalPermissionMode(): 사용자가 설정 가능한 모드 여부.
//
// 호출/사용 방식:
//   - REPL 상태 표시줄, 권한 프롬프트 UI 에서 참조.
//
// 연결:
//   - 원본: src/utils/permissions/PermissionMode.ts
package permissions

import "github.com/koreaf16/argus/internal/types"

// validExternalModes 는 사용자가 설정 파일·CLI 플래그로 지정 가능한 모드 집합이다.
var validExternalModes = map[types.PermissionMode]bool{
	types.PermissionModeAcceptEdits:       true,
	types.PermissionModeBypassPermissions: true,
	types.PermissionModeDefault:           true,
	types.PermissionModeDontAsk:           true,
	types.PermissionModePlan:              true,
}

// PermissionModeFromString 은 문자열을 PermissionMode 로 변환한다.
// 유효하지 않은 문자열이면 두 번째 반환값이 false 다.
func PermissionModeFromString(s string) (types.PermissionMode, bool) {
	mode := types.PermissionMode(s)
	if validExternalModes[mode] || mode == types.PermissionModeAuto {
		return mode, true
	}
	return types.PermissionModeDefault, false
}

// PermissionModeSymbol 은 모드의 짧은 심볼 문자를 반환한다.
// 원본 claude_cli PermissionMode.ts 기준.
func PermissionModeSymbol(mode types.PermissionMode) string {
	switch mode {
	case types.PermissionModeDefault:
		return ""
	case types.PermissionModePlan:
		return "⏸"
	case types.PermissionModeAcceptEdits, types.PermissionModeBypassPermissions,
		types.PermissionModeDontAsk, types.PermissionModeAuto:
		return "⏵⏵"
	default:
		return ""
	}
}

// PermissionModeTitle 은 모드의 표시 이름을 반환한다.
// 원본 claude_cli PermissionMode.ts 기준.
func PermissionModeTitle(mode types.PermissionMode) string {
	switch mode {
	case types.PermissionModeDefault:
		return "Default"
	case types.PermissionModePlan:
		return "Plan Mode"
	case types.PermissionModeAcceptEdits:
		return "Accept edits"
	case types.PermissionModeBypassPermissions:
		return "Bypass Permissions"
	case types.PermissionModeDontAsk:
		return "Don't Ask"
	case types.PermissionModeAuto:
		return "Auto mode"
	default:
		return "Default"
	}
}

// IsExternalPermissionMode 는 사용자가 설정 가능한 모드인지 확인한다.
func IsExternalPermissionMode(mode types.PermissionMode) bool {
	return validExternalModes[mode]
}
