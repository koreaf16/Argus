// Package tools
// File: propose_action.go
// Description: LLM이 사용자에게 명령/계획을 제안할 때 호출하는 도구 — 제안 내용을 구조화하고 확인 UI를 트리거
// Responsibility: 명령·계획·이유 수신, 표준 포맷으로 출력, MetadataJSON에 제안 데이터 직렬화

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// ProposeActionMeta는 제안 내용을 agent가 읽을 수 있도록 MetadataJSON에 직렬화하는 페이로드다.
type ProposeActionMeta struct {
	Command string   `json:"command"`
	Reason  string   `json:"reason"`
	Plan    []string `json:"plan,omitempty"`
}

// ProposeActionTool은 LLM이 사용자에게 실행할 명령을 제안할 때 호출한다.
// 도구 실행 시 MetadataJSON에 제안 데이터를 담아 agent 루프가 PendingAction을 활성화할 수 있게 한다.
type ProposeActionTool struct{}

func (t *ProposeActionTool) Name() string { return "propose_action" }

func (t *ProposeActionTool) Description() string {
	return "사용자에게 실행할 명령 또는 작업 계획을 제안합니다. 제안 후 사용자가 '예'라고 하면 해당 명령이 즉시 실행됩니다."
}

func (t *ProposeActionTool) ModelPrompt() string {
	return "사용자에게 실행할 명령 또는 계획을 제안할 때 **반드시** 이 도구를 호출하세요. " +
		"자연어 텍스트로만 제안하면 시스템이 명령을 추적하지 못합니다. " +
		"command: 즉시 실행할 단일 명령 (필수), reason: 왜 이 명령이 필요한지 (필수), " +
		"plan: 이후 수행할 단계 목록 (선택)"
}

func (t *ProposeActionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "즉시 실행할 단일 쉘 명령 (예: 'sudo -u oracle unzip /home/oracle/file.zip -d /target')",
			},
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "이 명령이 필요한 이유 (예: 'sandbox 사용자로 실행했으나 oracle 사용자 권한 필요')",
			},
			"plan": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
				"description": "이 명령 실행 후 수행할 단계 목록 (선택사항). " +
					"예: ['OPatch 교체', 'Release Update 적용', 'Oracle Installer 실행']",
			},
		},
		"required": []string{"command", "reason"},
	}
}

func (t *ProposeActionTool) IsReadOnly() bool { return true }
func (t *ProposeActionTool) IsEnabled() bool  { return true }

func (t *ProposeActionTool) Execute(_ context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	command, _ := args["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return ToolOutcome{Content: "Error: command is required", Success: false}, nil
	}

	reason, _ := args["reason"].(string)
	reason = strings.TrimSpace(reason)

	var plan []string
	if rawPlan, ok := args["plan"].([]interface{}); ok {
		for _, item := range rawPlan {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				plan = append(plan, strings.TrimSpace(s))
			}
		}
	}

	content := formatProposeContent(command, reason, plan)

	meta := ProposeActionMeta{Command: command, Reason: reason, Plan: plan}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return ToolOutcome{Content: fmt.Sprintf("Error: failed to encode metadata: %s", err), Success: false}, nil
	}

	return ToolOutcome{
		Content:      content,
		Success:      true,
		MetadataJSON: string(metaBytes),
	}, nil
}

func formatProposeContent(command, reason string, plan []string) string {
	var sb strings.Builder

	if reason != "" {
		sb.WriteString("**문제 분석**\n")
		sb.WriteString(reason)
		sb.WriteString("\n\n")
	}

	sb.WriteString("**해결 방안**\n")
	sb.WriteString("```bash\n")
	sb.WriteString(command)
	sb.WriteString("\n```\n")

	if len(plan) > 0 {
		sb.WriteString("\n**다음 단계 계획**\n")
		for i, step := range plan {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
		}
	}

	sb.WriteString("\n계속 진행하시겠습니까? (예/아니오)")
	return sb.String()
}
