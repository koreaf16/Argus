// Package types — 권한 모드·규칙·결과 타입.
//
// 파일 역할: 도구 실행 권한 시스템에서 사용하는 모든 타입을 정의한다.
//
//	순환 임포트를 방지하기 위해 구현 없이 타입만 포함한다.
//	실제 권한 로직은 internal/utils/permissions/ 에 있다.
//
// 포함 모듈:
//   - PermissionMode: 권한 모드 열거형 (acceptEdits / bypassPermissions / default 등).
//   - PermissionBehavior: allow / deny / ask.
//   - PermissionRule: 허용·거부 규칙 구조체.
//   - PermissionUpdate: 규칙 추가·제거·교체 업데이트 타입.
//   - PermissionDecision / PermissionResult: 권한 확인 결과.
//   - ToolPermissionContext: 도구가 권한 검사 시 참조하는 컨텍스트.
//   - ClassifierResult / YoloClassifierResult: 자동 분류기 결과.
//
// 호출/사용 방식:
//   - internal/utils/permissions/, internal/tool/tool.go 에서 참조.
//
// 연결:
//   - 원본: src/types/permissions.ts
package types

// PermissionMode 는 현재 세션의 권한 모드다.
type PermissionMode string

const (
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeAuto              PermissionMode = "auto"
	PermissionModeBubble            PermissionMode = "bubble"
)

// ExternalPermissionModes 는 사용자가 설정할 수 있는 권한 모드 목록이다.
var ExternalPermissionModes = []PermissionMode{
	PermissionModeAcceptEdits,
	PermissionModeBypassPermissions,
	PermissionModeDefault,
	PermissionModeDontAsk,
	PermissionModePlan,
}

// PermissionBehavior 는 권한 확인 결과 행동이다.
type PermissionBehavior string

const (
	BehaviorAllow       PermissionBehavior = "allow"
	BehaviorDeny        PermissionBehavior = "deny"
	BehaviorAsk         PermissionBehavior = "ask"
	BehaviorPassthrough PermissionBehavior = "passthrough"
)

// PermissionRuleSource 는 권한 규칙의 출처다.
type PermissionRuleSource string

const (
	RuleSourceUserSettings    PermissionRuleSource = "userSettings"
	RuleSourceProjectSettings PermissionRuleSource = "projectSettings"
	RuleSourceLocalSettings   PermissionRuleSource = "localSettings"
	RuleSourceFlagSettings    PermissionRuleSource = "flagSettings"
	RuleSourcePolicySettings  PermissionRuleSource = "policySettings"
	RuleSourceCLIArg          PermissionRuleSource = "cliArg"
	RuleSourceCommand         PermissionRuleSource = "command"
	RuleSourceSession         PermissionRuleSource = "session"
)

// PermissionRuleValue 는 권한 규칙의 값 (도구 이름 + 선택적 내용 패턴)이다.
type PermissionRuleValue struct {
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent,omitempty"`
}

// PermissionRule 은 출처·행동·값을 가진 단일 권한 규칙이다.
type PermissionRule struct {
	Source       PermissionRuleSource `json:"source"`
	RuleBehavior PermissionBehavior   `json:"ruleBehavior"`
	RuleValue    PermissionRuleValue  `json:"ruleValue"`
}

// PermissionUpdateDestination 은 권한 업데이트가 저장될 위치다.
type PermissionUpdateDestination string

const (
	UpdateDestUserSettings    PermissionUpdateDestination = "userSettings"
	UpdateDestProjectSettings PermissionUpdateDestination = "projectSettings"
	UpdateDestLocalSettings   PermissionUpdateDestination = "localSettings"
	UpdateDestSession         PermissionUpdateDestination = "session"
	UpdateDestCLIArg          PermissionUpdateDestination = "cliArg"
)

// PermissionUpdateType 은 권한 업데이트 종류다.
type PermissionUpdateType string

const (
	UpdateTypeAddRules          PermissionUpdateType = "addRules"
	UpdateTypeReplaceRules      PermissionUpdateType = "replaceRules"
	UpdateTypeRemoveRules       PermissionUpdateType = "removeRules"
	UpdateTypeSetMode           PermissionUpdateType = "setMode"
	UpdateTypeAddDirectories    PermissionUpdateType = "addDirectories"
	UpdateTypeRemoveDirectories PermissionUpdateType = "removeDirectories"
)

// PermissionUpdate 는 권한 설정 변경 작업이다.
type PermissionUpdate struct {
	Type        PermissionUpdateType        `json:"type"`
	Destination PermissionUpdateDestination `json:"destination"`
	Rules       []PermissionRuleValue       `json:"rules,omitempty"`
	Behavior    PermissionBehavior          `json:"behavior,omitempty"`
	Mode        PermissionMode              `json:"mode,omitempty"`
	Directories []string                    `json:"directories,omitempty"`
}

// AdditionalWorkingDirectory 는 권한 범위에 포함된 추가 디렉토리다.
type AdditionalWorkingDirectory struct {
	Path   string               `json:"path"`
	Source PermissionRuleSource `json:"source"`
}

// PermissionDecisionReasonType 은 권한 결정 이유 종류다.
type PermissionDecisionReasonType string

const (
	DecisionReasonRule            PermissionDecisionReasonType = "rule"
	DecisionReasonMode            PermissionDecisionReasonType = "mode"
	DecisionReasonClassifier      PermissionDecisionReasonType = "classifier"
	DecisionReasonWorkingDir      PermissionDecisionReasonType = "workingDir"
	DecisionReasonSafetyCheck     PermissionDecisionReasonType = "safetyCheck"
	DecisionReasonOther           PermissionDecisionReasonType = "other"
	DecisionReasonHook            PermissionDecisionReasonType = "hook"
	DecisionReasonSandboxOverride PermissionDecisionReasonType = "sandboxOverride"
)

// PermissionDecisionReason 은 권한 결정이 내려진 이유다.
type PermissionDecisionReason struct {
	Type                 PermissionDecisionReasonType `json:"type"`
	Rule                 *PermissionRule              `json:"rule,omitempty"`
	Mode                 PermissionMode               `json:"mode,omitempty"`
	Classifier           string                       `json:"classifier,omitempty"`
	Reason               string                       `json:"reason,omitempty"`
	HookName             string                       `json:"hookName,omitempty"`
	ClassifierApprovable bool                         `json:"classifierApprovable,omitempty"`
}

// PermissionResult 는 도구 실행 전 권한 확인 결과다.
type PermissionResult struct {
	Behavior       PermissionBehavior        `json:"behavior"`
	Message        string                    `json:"message,omitempty"`
	DecisionReason *PermissionDecisionReason `json:"decisionReason,omitempty"`
	Suggestions    []PermissionUpdate        `json:"suggestions,omitempty"`
	BlockedPath    string                    `json:"blockedPath,omitempty"`
	UpdatedInput   map[string]any            `json:"updatedInput,omitempty"`
}

// ToolPermissionRulesBySource 는 출처별 규칙 패턴 목록이다.
type ToolPermissionRulesBySource map[PermissionRuleSource][]string

// ToolPermissionContext 는 도구가 권한 확인 시 참조하는 불변 컨텍스트다.
type ToolPermissionContext struct {
	Mode                             PermissionMode
	AdditionalWorkingDirectories     map[string]AdditionalWorkingDirectory
	AlwaysAllowRules                 ToolPermissionRulesBySource
	AlwaysDenyRules                  ToolPermissionRulesBySource
	AlwaysAskRules                   ToolPermissionRulesBySource
	IsBypassPermissionsModeAvailable bool
	ShouldAvoidPermissionPrompts     bool
	PrePlanMode                      PermissionMode
	StrippedDangerousRules           ToolPermissionRulesBySource
}

// RiskLevel 은 권한 설명기가 판단한 위험 수준이다.
type RiskLevel string

const (
	RiskLow    RiskLevel = "LOW"
	RiskMedium RiskLevel = "MEDIUM"
	RiskHigh   RiskLevel = "HIGH"
)

// PermissionExplanation 은 권한 설명기의 설명 결과다.
type PermissionExplanation struct {
	RiskLevel   RiskLevel `json:"riskLevel"`
	Explanation string    `json:"explanation"`
	Reasoning   string    `json:"reasoning"`
	Risk        string    `json:"risk"`
}

// ClassifierResult 는 Bash 명령 자동 분류기의 결과다.
type ClassifierResult struct {
	Matches            bool   `json:"matches"`
	MatchedDescription string `json:"matchedDescription,omitempty"`
	Confidence         string `json:"confidence"` // "high" | "medium" | "low"
	Reason             string `json:"reason"`
}

// ClassifierBehavior 는 분류기가 권고하는 행동이다.
type ClassifierBehavior string

const (
	ClassifierBehaviorDeny  ClassifierBehavior = "deny"
	ClassifierBehaviorAsk   ClassifierBehavior = "ask"
	ClassifierBehaviorAllow ClassifierBehavior = "allow"
)
