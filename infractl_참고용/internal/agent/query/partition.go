// Package query
// File: partition.go
// Description: 도구 호출 목록을 동시 실행 가능 배치와 직렬 배치로 분할
// Responsibility: PartitionToolCalls — 연속된 read-only 도구를 하나의 concurrent batch 로 묶기
//
// Ported from: claude_cli/src/services/tools/toolOrchestration.ts:91-116 (partitionToolCalls)
// 핵심 패턴: reduce로 연속 read-only 도구를 하나의 isConcurrencySafe 배치로 병합,
//            non-read-only 도구는 단독 배치로 분리.

package query

import (
	"encoding/json"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

// Batch 는 동시에 (또는 직렬로) 실행할 도구 호출 묶음이다.
// claude_cli: type Batch = { isConcurrencySafe: boolean; blocks: ToolUseBlock[] }
type Batch struct {
	// Concurrent == true 이면 이 배치의 모든 도구를 병렬로 실행한다.
	// Concurrent == false 이면 하나씩 순서대로 실행한다.
	Concurrent bool
	Calls      []llm.ToolCall
}

// PartitionToolCalls 는 도구 호출 목록을 배치로 분할한다.
//
// 분할 규칙 (claude_cli 동일):
//   - 연속된 read-only(IsConcurrencySafe) 도구는 하나의 concurrent 배치로 병합.
//   - 연속된 non-read-only 도구는 하나의 serial 배치로 병합 (sibling abort 적용).
//   - 배치 순서는 원래 도구 순서를 보존한다.
//
// registry 가 nil 이거나 도구를 찾을 수 없으면 보수적으로 non-concurrent 취급.
// claude_cli: partitionToolCalls (toolOrchestration.ts:91-116)
func PartitionToolCalls(calls []llm.ToolCall, registry *tools.Registry) []Batch {
	if len(calls) == 0 {
		return nil
	}

	var batches []Batch
	for _, tc := range calls {
		safe := isConcurrencySafe(tc, registry)
		if len(batches) > 0 && batches[len(batches)-1].Concurrent == safe {
			// 마지막 배치와 같은 유형 → 병합 (concurrent-concurrent 또는 serial-serial)
			batches[len(batches)-1].Calls = append(batches[len(batches)-1].Calls, tc)
		} else {
			// 유형이 달라짐 → 새 배치 시작
			batches = append(batches, Batch{Concurrent: safe, Calls: []llm.ToolCall{tc}})
		}
	}
	return batches
}

// isConcurrencySafe 는 단일 도구 호출이 동시 실행에 안전한지 판단한다.
// 현재는 tools.Registry 의 IsReadOnly() 를 사용한다.
// 향후 IsConcurrencySafe() 메서드 추가 시 이 함수를 업데이트한다.
func isConcurrencySafe(tc llm.ToolCall, registry *tools.Registry) bool {
	if registry == nil {
		return false
	}
	tool, ok := registry.Get(tc.Function.Name)
	if !ok {
		return false
	}

	args, ok := parseToolArgs(tc.Function.Arguments)
	if !ok {
		return false
	}

	if checker, ok := tool.(tools.ConcurrencySafeTool); ok {
		return callIsConcurrencySafe(checker, args)
	}
	return tool.IsReadOnly()
}

func parseToolArgs(raw string) (map[string]interface{}, bool) {
	if strings.TrimSpace(raw) == "" {
		return map[string]interface{}{}, true
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, false
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	return args, true
}

func callIsConcurrencySafe(checker tools.ConcurrencySafeTool, args map[string]interface{}) (safe bool) {
	defer func() {
		if recover() != nil {
			safe = false
		}
	}()
	return checker.IsConcurrencySafe(args)
}
