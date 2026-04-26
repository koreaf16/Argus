// Package permissions — Bash 명령 자동 분류기.
//
// 파일 역할: auto 모드에서 Bash 명령의 안전성을 AI 분류기로 판단한다.
//
//	화이트리스트로 판정 불가한 명령을 LLM에 위임해 read-only 여부를 판정한다.
//	LLM 불가(nil subQuery, timeout, 파싱 실패) 시 Unavailable=true로 기존 Ask 동작 유지.
//
// 포함 모듈:
//   - ClassifyBashCommand(): 명령 안전성 분류.
//
// 호출/사용 방식:
//   - internal/tools/permission_policy.go 의 EvaluateShellCommandPermission() 에서 호출.
//
// 연결:
//   - 원본: src/utils/permissions/bashClassifier.ts
package permissions

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/koreaf16/argus/internal/types"
)

// SubQueryFunc 는 LLM 단발 호출 함수 타입.
// tool.ExecuteSubQueryFunc 와 시그니처가 동일하며, 순환 임포트 없이 사용 가능.
type SubQueryFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// BashClassifierResult 는 Bash 명령 분류 결과다.
type BashClassifierResult struct {
	Behavior    types.ClassifierBehavior
	Reason      string
	Unavailable bool
}

// classifierCache 는 명령어→판정 결과 메모리 캐시.
// 동일 명령은 LLM 재호출 없이 캐시에서 반환한다.
var classifierCache sync.Map // key: string, value: BashClassifierResult

// classifierSystemPrompt 는 LLM 분류기 시스템 프롬프트.
const classifierSystemPrompt = `You are a shell command safety classifier. Respond with JSON only, no explanation outside the JSON.`

// classifierTimeout 은 LLM 분류기 호출 최대 대기 시간.
const classifierTimeout = 8 * time.Second

// llmClassifierResponse 는 LLM이 반환하는 JSON 구조.
type llmClassifierResponse struct {
	ReadOnly   bool   `json:"read_only"`
	Confidence string `json:"confidence"` // "high" | "medium" | "low"
	Reason     string `json:"reason"`
}

// ClassifyBashCommand 는 Bash 명령의 안전성을 분류한다.
//
// subQuery 가 nil이거나 LLM 호출에 실패하면 Unavailable=true 를 반환해
// 호출자가 기존 Ask 동작으로 폴백하도록 한다.
func ClassifyBashCommand(
	ctx context.Context,
	command string,
	denialState DenialTrackingState,
	subQuery SubQueryFunc,
) BashClassifierResult {
	if subQuery == nil {
		return BashClassifierResult{
			Behavior:    types.ClassifierBehaviorAsk,
			Reason:      "LLM 분류기를 사용할 수 없습니다 (subQuery nil).",
			Unavailable: true,
		}
	}

	if cached, ok := classifierCache.Load(command); ok {
		return cached.(BashClassifierResult)
	}

	result := callLLMClassifier(ctx, command, subQuery)
	if !result.Unavailable {
		classifierCache.Store(command, result)
	}
	return result
}

// callLLMClassifier 는 LLM에 명령 안전성 판정을 요청하고 결과를 반환한다.
func callLLMClassifier(ctx context.Context, command string, subQuery SubQueryFunc) BashClassifierResult {
	timeoutCtx, cancel := context.WithTimeout(ctx, classifierTimeout)
	defer cancel()

	userPrompt := fmt.Sprintf(
		`Is this shell command read-only? (does NOT modify filesystem, install packages, or change persistent system state)
Command: %s

Respond exactly with this JSON and nothing else:
{"read_only": true|false, "confidence": "high"|"medium"|"low", "reason": "one sentence"}`,
		"`"+command+"`",
	)

	raw, err := subQuery(timeoutCtx, classifierSystemPrompt, userPrompt)
	if err != nil {
		return BashClassifierResult{
			Behavior:    types.ClassifierBehaviorAsk,
			Reason:      "LLM 분류기 호출 실패: " + err.Error(),
			Unavailable: true,
		}
	}

	var resp llmClassifierResponse
	if err := json.Unmarshal([]byte(extractJSON(raw)), &resp); err != nil {
		return BashClassifierResult{
			Behavior:    types.ClassifierBehaviorAsk,
			Reason:      "LLM 응답 파싱 실패: " + err.Error(),
			Unavailable: true,
		}
	}

	// medium/low 신뢰도는 불확실하므로 Ask로 폴백
	if resp.Confidence != "high" {
		return BashClassifierResult{
			Behavior: types.ClassifierBehaviorAsk,
			Reason:   resp.Reason,
		}
	}

	if resp.ReadOnly {
		return BashClassifierResult{
			Behavior: types.ClassifierBehaviorAllow,
			Reason:   resp.Reason,
		}
	}
	return BashClassifierResult{
		Behavior: types.ClassifierBehaviorAsk,
		Reason:   resp.Reason,
	}
}

// extractJSON 은 LLM 응답에서 첫 번째 JSON 객체를 추출한다.
// LLM이 JSON 앞뒤에 설명 텍스트를 붙이는 경우를 처리한다.
func extractJSON(s string) string {
	start := -1
	depth := 0
	for i, ch := range s {
		switch ch {
		case '{':
			if start == -1 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start != -1 {
				return s[start : i+1]
			}
		}
	}
	return s
}
