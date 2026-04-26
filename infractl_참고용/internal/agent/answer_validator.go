// Package agent
// File: answer_validator.go
// Description: 최종 답변의 일관성을 도구 실행 결과와 비교하여 검증한다.
// Responsibility: 도구 출력 데이터와 LLM 답변 간의 모순 감지 및 교정 유도.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
)

// validateAnswerConsistency는 최종 답변이 이전 도구 실행 결과와 모순되지 않는지 확인한다.
// 모순이 발견되면 LLM에게 재확인을 요청하기 위한 메시지를 반환하거나 빈 문자열을 반환한다.
func (a *Agent) validateAnswerConsistency(ctx context.Context, lastText string) string {
	if lastText == "" {
		return ""
	}

	// 1. 히스토리에서 도구 결과 추출
	toolResults := a.extractRelevantToolResults()
	if len(toolResults) == 0 {
		return ""
	}

	// 2. 모순 감지 로직 (간이 규칙 기반)
	// 향후 고도화 시 LLM을 사용하여 "이 답변이 도구 결과와 모순되나?"라고 물어볼 수 있음.
	var contradictions []string

	for _, res := range toolResults {
		if res.ToolName == "kubectl" || res.ToolName == "remote_shell" {
			// vLLM/Gemma 관련 모순 체크
			if strings.Contains(res.Output, "vllm") && strings.Contains(res.Output, "Running") {
				if isNegativeResponse(lastText, "vllm", "gemma") {
					contradictions = append(contradictions, 
						fmt.Sprintf("도구 결과에는 vLLM/Gemma가 Running 상태로 나타나지만, 답변은 이를 부정하고 있습니다. (Tool: %s)", res.ToolName))
				}
			}
		}
		
		if res.ToolName == "nvidia-smi" {
			if strings.Contains(res.Output, "NVIDIA-SMI") && !strings.Contains(res.Output, "failed") {
				if strings.Contains(strings.ToLower(lastText), "gpu를 찾을 수") || strings.Contains(strings.ToLower(lastText), "no gpu") {
					contradictions = append(contradictions, "GPU가 정상적으로 인식되었으나 답변에는 GPU를 찾을 수 없다고 되어 있습니다.")
				}
			}
		}
	}

	if len(contradictions) > 0 {
		msg := "최종 답변 검증 중 다음과 같은 모순이 발견되었습니다:\n"
		for _, c := range contradictions {
			msg += "- " + c + "\n"
		}
		msg += "\n도구의 실제 출력 결과를 다시 한 번 면밀히 검토하여 정확한 답변을 작성해 주세요."
		slog.Warn("answer consistency check failed", "contradictions", len(contradictions))
		return msg
	}

	return ""
}

type toolExecutionInfo struct {
	ToolName string
	Output   string
}

func (a *Agent) extractRelevantToolResults() []toolExecutionInfo {
	var results []toolExecutionInfo
	
	// Create a map of tool call IDs to names for faster lookup
	idToName := make(map[string]string)
	for i := len(a.history) - 1; i >= 0; i-- {
		msg := a.history[i]
		if msg.Role == llm.RoleAssistant {
			for _, tc := range msg.ToolCalls {
				idToName[tc.ID] = tc.Function.Name
			}
		}
		if len(a.history)-i > 50 { // Scan back up to 50 messages for tool calls
			break
		}
	}

	// 히스토리를 뒤에서부터 탐색하여 최근 도구 결과 수집
	for i := len(a.history) - 1; i >= 0; i-- {
		msg := a.history[i]
		if msg.Role == llm.RoleTool {
			name := idToName[msg.ToolCallID]
			if name == "" {
				name = "unknown"
			}
			results = append(results, toolExecutionInfo{
				ToolName: name,
				Output:   msg.Content,
			})
		}
		// 최근 20개 메시지 정도만 실제 분석 대상
		if len(a.history)-i > 20 {
			break
		}
	}
	return results
}

func isNegativeResponse(text string, keywords ...string) bool {
	lower := strings.ToLower(text)
	hasKeyword := false
	for _, k := range keywords {
		if strings.Contains(lower, k) {
			hasKeyword = true
			break
		}
	}
	if !hasKeyword {
		return false
	}

	negatives := []string{
		"없음", "없습니다", "찾을 수", "동작하지", "않습니다", "not found", "no active", "not running", "failed to find",
	}
	for _, n := range negatives {
		if strings.Contains(lower, n) {
			return true
		}
	}
	return false
}
