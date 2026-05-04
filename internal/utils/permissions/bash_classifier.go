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
type SubQueryFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// SearchFunc 는 웹 검색 함수 타입.
type SearchFunc func(ctx context.Context, query string) (string, error)

// FetchFunc 는 웹 페이지 페칭 함수 타입.
type FetchFunc func(ctx context.Context, url string) (string, error)

// BashClassifierResult 는 Bash 명령 분류 결과다.
type BashClassifierResult struct {
	Behavior    types.ClassifierBehavior
	Reason      string
	Unavailable bool
}

// classifierCache 는 명령어→판정 결과 메모리 캐시.
var classifierCache sync.Map // key: string, value: BashClassifierResult

const classifierSystemPrompt = `You are a shell command safety classifier. Respond with JSON only, no explanation outside the JSON.`
const classifierTimeout = 10 * time.Second // LLM 분류에 15초는 과다 — 실패 시 Ask fallback

type llmClassifierResponse struct {
	ReadOnly   bool   `json:"read_only"`
	Confidence string `json:"confidence"` // "high" | "medium" | "low"
	Reason     string `json:"reason"`
	NeedsWeb   bool   `json:"needs_web"` // 웹 검색이 필요한지 여부
}

// ClassifyBashCommand 는 Bash 명령의 안전성을 분류한다.
func ClassifyBashCommand(
	ctx context.Context,
	command string,
	denialState DenialTrackingState,
	subQuery SubQueryFunc,
	search SearchFunc,
	fetch FetchFunc,
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

	result := callLLMClassifier(ctx, command, subQuery, search, fetch)
	if !result.Unavailable {
		classifierCache.Store(command, result)
	}
	return result
}

// callLLMClassifier 는 LLM과 웹 검색을 연동하여 명령 안전성을 판정한다.
func callLLMClassifier(ctx context.Context, command string, subQuery SubQueryFunc, search SearchFunc, fetch FetchFunc) BashClassifierResult {
	timeoutCtx, cancel := context.WithTimeout(ctx, classifierTimeout)
	defer cancel()

	userPrompt := fmt.Sprintf(
		`Is this shell command read-only? (does NOT modify filesystem, install packages, or change persistent system state)
Command: %s

Respond exactly with this JSON:
{"read_only": true|false, "confidence": "high"|"medium"|"low", "reason": "...", "needs_web": true|false}`,
		"`"+command+"`",
	)

	raw, err := subQuery(timeoutCtx, classifierSystemPrompt, userPrompt)
	if err != nil {
		return BashClassifierResult{Behavior: types.ClassifierBehaviorAsk, Reason: "LLM 호출 실패: " + err.Error(), Unavailable: true}
	}

	var resp llmClassifierResponse
	if err := json.Unmarshal([]byte(extractJSON(raw)), &resp); err != nil {
		return BashClassifierResult{Behavior: types.ClassifierBehaviorAsk, Reason: "파싱 실패", Unavailable: true}
	}

	// 1차 판정 결과가 확실하거나 웹 검색을 쓸 수 없는 경우
	if (resp.Confidence == "high" && !resp.NeedsWeb) || search == nil {
		return finalizeResult(resp)
	}

	// 2차: 웹 검색을 통한 지식 보강
	searchQuery := fmt.Sprintf("shell command behavior and side effects: %s", command)
	searchData, err := search(timeoutCtx, searchQuery)
	if err != nil {
		return finalizeResult(resp) // 검색 실패 시 1차 결과 반환
	}

	// 보강된 정보로 재심사
	userPrompt = fmt.Sprintf(
		`I searched for the command behavior. 
Command: %s
Search Evidence: %s

Based on this evidence, is the command read-only?
Respond with JSON: {"read_only": true|false, "confidence": "high"|"medium"|"low", "reason": "..."}`,
		"`"+command+"`", searchData,
	)

	raw, err = subQuery(timeoutCtx, classifierSystemPrompt, userPrompt)
	if err == nil {
		if err := json.Unmarshal([]byte(extractJSON(raw)), &resp); err == nil {
			resp.Reason = "[Web Search Used] " + resp.Reason
			return finalizeResult(resp)
		}
	}

	return finalizeResult(resp)
}

func finalizeResult(resp llmClassifierResponse) BashClassifierResult {
	if resp.Confidence != "high" {
		return BashClassifierResult{Behavior: types.ClassifierBehaviorAsk, Reason: "신뢰도 낮음: " + resp.Reason}
	}
	if resp.ReadOnly {
		return BashClassifierResult{Behavior: types.ClassifierBehaviorAllow, Reason: resp.Reason}
	}
	return BashClassifierResult{Behavior: types.ClassifierBehaviorAsk, Reason: resp.Reason}
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
