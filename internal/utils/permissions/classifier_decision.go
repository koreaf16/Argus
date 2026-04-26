// Package permissions — auto 모드 분류기 허용 목록.
//
// 파일 역할: auto 모드에서 LLM 분류기 호출 없이 즉시 허용되는 안전한 도구 목록을 정의한다.
//
//	읽기 전용 파일 작업·태스크 관리·Plan 모드 전환 도구 등이 포함된다.
//
// 포함 모듈:
//   - IsAutoModeAllowlistedTool(): 도구 이름이 허용 목록에 있는지 확인.
//
// 호출/사용 방식:
//   - internal/utils/permissions/permissions.go 의 HasPermissionToUseTool() 에서
//     auto 모드 분기 시 분류기 호출 전 먼저 확인한다.
//
// 연결:
//   - 원본: src/utils/permissions/classifierDecision.ts
package permissions

import "github.com/koreaf16/argus/internal/constants"

// safeAutoModeTools 는 auto 모드에서 분류기 없이 즉시 허용되는 도구 이름 집합이다.
var safeAutoModeTools = map[string]bool{
	// 읽기 전용 파일 작업
	"FileRead": true,
	// 검색·탐색
	"Grep":             true,
	"Glob":             true,
	"LSP":              true,
	"ToolSearch":       true,
	"ListMcpResources": true,
	"ReadMcpResource":  true,
	// 태스크 관리 (메타데이터만)
	constants.TodoWriteToolName: true,
	"TaskCreate":                true,
	"TaskGet":                   true,
	"TaskUpdate":                true,
	"TaskList":                  true,
	"TaskStop":                  true,
	"TaskOutput":                true,
	// Plan 모드·UI
	constants.AskUserToolName:             true,
	constants.LegacyAskUserToolName:       true,
	constants.LegacyAskUserAliasName:      true,
	constants.EnterPlanModeToolName:       true,
	constants.ExitPlanModeToolName:        true,
	constants.LegacyEnterPlanModeToolName: true,
	constants.LegacyExitPlanModeToolName:  true,
	// Swarm 조율 (하위 에이전트 자체 권한 검사 있음)
	"TeamCreate":  true,
	"TeamDelete":  true,
	"SendMessage": true,
	// 기타 안전한 도구
	"Sleep": true,
	// 내부 분류기 도구
	"YoloClassifier": true,
}

// IsAutoModeAllowlistedTool 은 도구가 auto 모드 허용 목록에 있는지 반환한다.
func IsAutoModeAllowlistedTool(toolName string) bool {
	return safeAutoModeTools[toolName]
}
