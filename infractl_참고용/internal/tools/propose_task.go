// Package tools
// File: propose_task.go
// Description: LLM이 복합 작업 시작 전 사용자에게 TaskContext 제안을 제출하는 도구
// Responsibility: 제안 파싱·검증, ProposeTaskMeta 직렬화, 사용자 확인 프롬프트 생성

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/executor"
)

// ProposeTaskMeta 는 agent 루프가 PendingProposal 을 생성할 수 있도록 MetadataJSON 에 직렬화하는 페이로드.
type ProposeTaskMeta struct {
	Draft        taskctx.TaskContextDraft `json:"draft"`
	RiskAnalysis string                   `json:"risk_analysis"`
	RollbackPlan []string                 `json:"rollback_plan"`
	Reasoning    string                   `json:"reasoning"`
}

// ProposeTaskTool 은 LLM이 복합 작업 시작 전 사용자에게 TaskContext 를 제안할 때 호출한다.
// MetadataJSON 만 생성하며 실제 Declare 는 수행하지 않는다.
type ProposeTaskTool struct{}

func (t *ProposeTaskTool) Name() string { return "propose_task" }

func (t *ProposeTaskTool) Description() string {
	return "복합 작업(설치·서비스·설정·마이그레이션 등) 시작 전 사용자에게 작업 컨텍스트를 제안합니다. " +
		"사용자가 확인하면 declare_task 로 실제 선언합니다."
}

func (t *ProposeTaskTool) ModelPrompt() string {
	return "복잡 작업 시작 전 반드시 이 도구 먼저 호출하세요. " +
		"Kind에 따라 kind_fields에 필수 추가 필드를 포함하세요. " +
		"install이면 어떤 계정(owner_account)/어디(install_path)/어떤 옵션(install_options)/어떤 방식(install_method) 네 가지 반드시 명시. " +
		"risk_analysis: 위험 요소와 영향 범위를 상세히 기술. " +
		"rollback_plan: 작업 실패 시 복원 단계를 목록으로. " +
		"reasoning: 이 작업이 필요한 기술적 근거."
}

func (t *ProposeTaskTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "작업 제목 (예: Oracle 19c 설치)",
			},
			"kind": map[string]any{
				"type":        "string",
				"enum":        []string{"install", "service", "configure", "migrate", "backup", "patch", "inspect"},
				"description": "작업 유형",
			},
			"target_server": map[string]any{
				"type":        "string",
				"description": "대상 서버 호스트명 또는 IP",
			},
			"target_account": map[string]any{
				"type":        "string",
				"description": "대상 OS 계정 (선택)",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "작업 디렉토리 절대경로 (선택)",
			},
			"forbidden": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "금지 명령 패턴 목록 — regex 또는 literal prefix (선택)",
			},
			"preconditions": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "작업 전 만족해야 할 전제조건 목록 (선택)",
			},
			"plan": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "수행 단계 설명 목록 (선택)",
			},
			"kind_fields": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Kind 별 추가 필수 필드 (예: install → software_name, install_path 등)",
			},
			"risk_analysis": map[string]any{
				"type":        "string",
				"description": "위험 요소 및 영향 범위 분석 (필수)",
			},
			"rollback_plan": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "작업 실패 시 복원 단계 목록 (필수)",
			},
			"reasoning": map[string]any{
				"type":        "string",
				"description": "이 작업이 필요한 기술적 근거 (필수)",
			},
		},
		"required": []string{"title", "kind", "target_server", "risk_analysis", "rollback_plan", "reasoning"},
	}
}

func (t *ProposeTaskTool) IsReadOnly() bool { return true }
func (t *ProposeTaskTool) IsEnabled() bool  { return true }

func (t *ProposeTaskTool) Execute(_ context.Context, args map[string]any, _ executor.Executor) (ToolOutcome, error) {
	draft, err := parseDraftArgs(args)
	if err != nil {
		return ToolOutcome{Content: err.Error(), Success: false}, nil
	}

	if err := taskctx.ValidateDraft(draft); err != nil {
		return ToolOutcome{
			Content: "필수 필드 누락: " + err.Error() + "\n해당 필드를 채워서 다시 제안하세요.",
			Success: false,
		}, nil
	}

	riskAnalysis, _ := args["risk_analysis"].(string)
	riskAnalysis = strings.TrimSpace(riskAnalysis)
	if riskAnalysis == "" {
		return ToolOutcome{Content: "risk_analysis 는 필수입니다", Success: false}, nil
	}

	rollbackPlan := parseStringSlice(args, "rollback_plan")
	if len(rollbackPlan) == 0 {
		return ToolOutcome{Content: "rollback_plan 은 최소 한 항목이 필요합니다", Success: false}, nil
	}

	reasoning, _ := args["reasoning"].(string)
	reasoning = strings.TrimSpace(reasoning)

	meta := ProposeTaskMeta{
		Draft:        draft,
		RiskAnalysis: riskAnalysis,
		RollbackPlan: rollbackPlan,
		Reasoning:    reasoning,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("메타데이터 직렬화 실패: %s", err), Success: false}, nil
	}

	content := formatProposeTaskContent(draft, riskAnalysis, rollbackPlan, reasoning)

	return ToolOutcome{
		Content:      content,
		Success:      true,
		MetadataJSON: string(metaBytes),
	}, nil
}

// parseDraftArgs 는 args 맵에서 TaskContextDraft 를 파싱한다.
func parseDraftArgs(args map[string]any) (taskctx.TaskContextDraft, error) {
	title, _ := args["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		return taskctx.TaskContextDraft{}, fmt.Errorf("title 은 필수입니다")
	}

	kindStr, _ := args["kind"].(string)
	kindStr = strings.TrimSpace(kindStr)
	if kindStr == "" {
		return taskctx.TaskContextDraft{}, fmt.Errorf("kind 는 필수입니다")
	}

	targetServer, _ := args["target_server"].(string)
	targetServer = strings.TrimSpace(targetServer)
	if targetServer == "" {
		return taskctx.TaskContextDraft{}, fmt.Errorf("target_server 는 필수입니다")
	}

	targetAccount, _ := args["target_account"].(string)
	workingDir, _ := args["working_dir"].(string)

	forbidden := parseStringSlice(args, "forbidden")
	preconditions := parseStringSlice(args, "preconditions")
	planSteps := parseStringSlice(args, "plan")
	kindFields := parseStringMap(args, "kind_fields")

	return taskctx.TaskContextDraft{
		Title:         title,
		Kind:          taskctx.TaskKind(kindStr),
		TargetServer:  targetServer,
		TargetAccount: strings.TrimSpace(targetAccount),
		WorkingDir:    strings.TrimSpace(workingDir),
		Forbidden:     forbidden,
		Preconditions: preconditions,
		PlanSteps:     planSteps,
		KindFields:    kindFields,
	}, nil
}

// parseStringSlice 는 args 에서 []any 를 []string 으로 변환한다.
func parseStringSlice(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			result = append(result, strings.TrimSpace(s))
		}
	}
	return result
}

// parseStringMap 는 args 에서 map[string]any 를 map[string]string 으로 변환한다.
func parseStringMap(args map[string]any, key string) map[string]string {
	raw, ok := args[key].(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

func formatProposeTaskContent(
	draft taskctx.TaskContextDraft,
	riskAnalysis string,
	rollbackPlan []string,
	reasoning string,
) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "**작업 제안: %s**\n", draft.Title)
	fmt.Fprintf(&sb, "- 유형: %s\n", string(draft.Kind))
	fmt.Fprintf(&sb, "- 대상 서버: %s\n", draft.TargetServer)
	if draft.TargetAccount != "" {
		fmt.Fprintf(&sb, "- 대상 계정: %s\n", draft.TargetAccount)
	}
	if draft.WorkingDir != "" {
		fmt.Fprintf(&sb, "- 작업 디렉토리: %s\n", draft.WorkingDir)
	}

	if len(draft.PlanSteps) > 0 {
		sb.WriteString("\n**수행 단계**\n")
		for i, step := range draft.PlanSteps {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, step)
		}
	}

	if len(draft.Forbidden) > 0 {
		sb.WriteString("\n**금지 사항**\n")
		for _, f := range draft.Forbidden {
			fmt.Fprintf(&sb, "- %s\n", f)
		}
	}

	if len(draft.Preconditions) > 0 {
		sb.WriteString("\n**전제조건**\n")
		for _, pc := range draft.Preconditions {
			fmt.Fprintf(&sb, "- %s\n", pc)
		}
	}

	sb.WriteString("\n**위험 분석**\n")
	sb.WriteString(riskAnalysis)
	sb.WriteString("\n")

	sb.WriteString("\n**롤백 계획**\n")
	for i, step := range rollbackPlan {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, step)
	}

	if reasoning != "" {
		sb.WriteString("\n**근거**\n")
		sb.WriteString(reasoning)
		sb.WriteString("\n")
	}

	if len(draft.KindFields) > 0 {
		sb.WriteString("\n**추가 정보**\n")
		for k, v := range draft.KindFields {
			fmt.Fprintf(&sb, "- %s: %s\n", k, v)
		}
	}

	sb.WriteString("\n확인하시면 'yes', 수정하려면 자연어로 지시, 취소하려면 '취소'.")

	return sb.String()
}
