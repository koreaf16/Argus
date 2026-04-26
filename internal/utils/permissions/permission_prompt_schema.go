// Package permissions — 권한 프롬프트 도구 결과 스키마.
//
// 파일 역할: MCP SDK 호스트가 반환하는 권한 프롬프트 도구 결과 타입을 정의한다.
//
//	allow / deny 두 가지 행동을 가진 결합 타입으로 표현한다.
//
// 포함 모듈:
//   - PermissionDecisionClassification: 결정 분류 (user_temporary 등).
//   - PermissionPromptAllowResult / PermissionPromptDenyResult: 허용·거부 결과.
//   - PermissionPromptResult: 결합 결과 타입.
//   - PermissionPromptInput: 권한 프롬프트 도구 입력.
//
// 호출/사용 방식:
//   - Phase 2 에서 MCP SDK 호스트와 권한 프롬프트를 주고받을 때 사용.
//
// 연결:
//   - 원본: src/utils/permissions/PermissionPromptToolResultSchema.ts
package permissions

import "github.com/koreaf16/argus/internal/types"

// PermissionDecisionClassification 은 권한 결정의 영속성 분류다.
type PermissionDecisionClassification string

const (
	DecisionClassUserTemporary PermissionDecisionClassification = "user_temporary"
	DecisionClassUserPermanent PermissionDecisionClassification = "user_permanent"
	DecisionClassUserReject    PermissionDecisionClassification = "user_reject"
)

// PermissionPromptAllowResult 는 SDK 호스트가 반환하는 허용 결과다.
type PermissionPromptAllowResult struct {
	Behavior               string                           `json:"behavior"` // "allow"
	UpdatedInput           map[string]any                   `json:"updatedInput"`
	UpdatedPermissions     []types.PermissionUpdate         `json:"updatedPermissions,omitempty"`
	ToolUseID              string                           `json:"toolUseID,omitempty"`
	DecisionClassification PermissionDecisionClassification `json:"decisionClassification,omitempty"`
}

// PermissionPromptDenyResult 는 SDK 호스트가 반환하는 거부 결과다.
type PermissionPromptDenyResult struct {
	Behavior               string                           `json:"behavior"` // "deny"
	Message                string                           `json:"message"`
	Interrupt              bool                             `json:"interrupt,omitempty"`
	ToolUseID              string                           `json:"toolUseID,omitempty"`
	DecisionClassification PermissionDecisionClassification `json:"decisionClassification,omitempty"`
}

// PermissionPromptResult 는 SDK 호스트 권한 프롬프트 도구의 결과다.
// Behavior 필드로 허용(allow)·거부(deny) 를 구분한다.
type PermissionPromptResult struct {
	Allow *PermissionPromptAllowResult
	Deny  *PermissionPromptDenyResult
}

// PermissionPromptInput 은 권한 프롬프트 도구에 전달되는 입력이다.
type PermissionPromptInput struct {
	ToolName  string         `json:"tool_name"`
	Input     map[string]any `json:"input"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
}
