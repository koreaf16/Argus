// Package permissions — bypassPermissions·auto 모드 킬스위치.
//
// 파일 역할: 조직 정책이나 서킷 브레이커에 의해 bypassPermissions / auto 모드를
//
//	비활성화할 수 있는 킬스위치를 관리한다.
//	Phase 1 에서는 스텁으로 항상 허용하며, Phase 2+ 에서 GrowthBook 피처 게이트와 연동한다.
//
// 포함 모듈:
//   - IsBypassPermissionsModeAvailable(): bypassPermissions 모드 사용 가능 여부.
//   - IsAutoModeAvailable(): auto 모드 사용 가능 여부.
//
// 호출/사용 방식:
//   - 부팅 시 ToolPermissionContext 초기화 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/bypassPermissionsKillswitch.ts
package permissions

// IsBypassPermissionsModeAvailable 은 bypassPermissions 모드 사용 가능 여부를 반환한다.
// Phase 1: 항상 true (Phase 2에서 피처 게이트 연동 예정).
func IsBypassPermissionsModeAvailable() bool {
	return true
}

// IsAutoModeAvailable 은 auto 모드 사용 가능 여부를 반환한다.
// Phase 1: false (TRANSCRIPT_CLASSIFIER 피처 게이트 미구현).
func IsAutoModeAvailable() bool {
	return false
}
