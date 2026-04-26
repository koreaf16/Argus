// Package agent
// File: history.go
// Description: 에이전트 대화 히스토리 관리 유틸리티
// Responsibility: 히스토리 트리밍, API 라운드 그룹화, 메시지 빌딩, 초기화 로직 분리

package agent

import (
	"log/slog"
	"sort"

	"github.com/yourorg/infractl/internal/llm"
)

// preserveRecentRounds는 히스토리 압축/트리밍 시 원문 보존할 최근 API 라운드 수다.
const preserveRecentRounds = 4

// apiRound는 원자적으로 보존해야 하는 대화 API 라운드이다.
// user 메시지를 시작으로, 이에 대한 assistant 응답과 tool_result들로 구성된다.
type apiRound struct {
	messages []llm.Message
}

// groupByApiRound는 히스토리를 API 라운드 단위 슬라이스로 분할한다.
// 경계 조건: user 메시지가 시작될 때마다 새 라운드를 시작한다.
// tool_result들은 선행 assistant(with tool_calls)와 같은 라운드에 묶인다.
// 이를 통해 trimHistory/compaction 시 tool_call+tool_result 쌍이 분리되지 않는다.
func groupByApiRound(history []llm.Message) []apiRound {
	var rounds []apiRound
	var current []llm.Message

	for _, msg := range history {
		if msg.Role == llm.RoleUser && len(current) > 0 {
			// 새 user 메시지 = 이전 라운드 확정 후 새 라운드 시작
			rounds = append(rounds, apiRound{messages: current})
			current = []llm.Message{msg}
		} else {
			current = append(current, msg)
		}
	}
	if len(current) > 0 {
		rounds = append(rounds, apiRound{messages: current})
	}
	return rounds
}

// flattenRounds는 apiRound 슬라이스를 단일 메시지 슬라이스로 평탄화한다.
func flattenRounds(rounds []apiRound) []llm.Message {
	total := 0
	for _, r := range rounds {
		total += len(r.messages)
	}
	result := make([]llm.Message, 0, total)
	for _, r := range rounds {
		result = append(result, r.messages...)
	}
	return result
}

// filterHistory는 오래된 대화 라운드에서 단순 도구 호출 내역을 제거하여 토큰 낭비를 줄인다.
func (a *Agent) filterHistory(history []llm.Message) []llm.Message {
	rounds := groupByApiRound(history)
	if len(rounds) <= preserveRecentRounds {
		return history
	}

	var filtered []llm.Message
	for i, r := range rounds {
		if i >= len(rounds)-preserveRecentRounds {
			filtered = append(filtered, r.messages...)
			continue
		}

		for _, msg := range r.messages {
			if msg.Role == llm.RoleUser {
				filtered = append(filtered, msg)
			} else if msg.Role == llm.RoleAssistant {
				// 텍스트 응답이 있는 assistant 메시지만 보존, 도구 호출 내역은 삭제
				if msg.Content != "" {
					newMsg := msg
					newMsg.ToolCalls = nil
					filtered = append(filtered, newMsg)
				}
			}
			// RoleTool 메시지(도구 실행 결과)는 생략
		}
	}
	return filtered
}

// buildMessages는 시스템 프롬프트와 히스토리를 조합하여 LLM 요청용 메시지 슬라이스를 반환한다.
// sessionSummary가 있으면 요약 기반 컨텍스트를 주입하고, 없으면 기존 filterHistory 방식을 사용한다.
func (a *Agent) buildMessages(systemMsg llm.Message) []llm.Message {
	if a.sessionSummary != nil && a.sessionSummary.HasContext() {
		return a.sessionSummary.BuildContextMessages(systemMsg, a.history)
	}
	filteredHistory := a.filterHistory(a.history)
	messages := make([]llm.Message, 0, len(filteredHistory)+1)
	messages = append(messages, systemMsg)
	messages = append(messages, filteredHistory...)
	return messages
}

// ClearHistory는 대화 히스토리를 초기화한다.
func (a *Agent) ClearHistory() {
	a.history = make([]llm.Message, 0)
}

// trimHistoryPreservingPending은 PendingAction이 가리키는 라운드를 보존하면서 trim한다.
// PTL 재시도 시 direct trimHistory 대신 이 메서드를 사용한다.
func (a *Agent) trimHistoryPreservingPending() {
	pending := a.pendingAction.Current()
	if pending == nil {
		a.trimHistory()
		return
	}
	if len(a.history) <= a.maxHistory {
		return
	}

	rounds := groupByApiRound(a.history)
	pendingTurn := pending.TurnIndex

	// pending 라운드: pendingTurn 번째 user 메시지가 있는 라운드를 보존
	// rounds는 0-indexed API 라운드 목록
	for len(rounds) > 1 {
		total := 0
		for _, r := range rounds {
			total += len(r.messages)
		}
		if total <= a.maxHistory {
			break
		}
		// 가장 오래된 라운드가 pending 라운드인지 확인
		if len(rounds[0].messages) > 0 {
			firstMsg := rounds[0].messages[0]
			// pending이 가리키는 userTurn이 첫 라운드라면 건너뛰고 두 번째부터 제거
			if isRoundFromTurn(firstMsg, pendingTurn) {
				if len(rounds) > 2 {
					rounds = append(rounds[:1], rounds[2:]...)
					continue
				}
				break
			}
		}
		rounds = rounds[1:]
	}

	before := len(a.history)
	a.history = flattenRounds(rounds)
	slog.Debug("trimHistoryPreservingPending complete",
		"before", before, "after", len(a.history), "pending_turn", pendingTurn)
}

// isRoundFromTurn은 라운드의 첫 메시지가 특정 턴 카운터에 해당하는지 확인한다.
// 현재는 단순히 항상 false를 반환하여 기본 FIFO trim이 동작하도록 한다.
// 정확한 턴 추적이 필요하면 메시지에 턴 메타데이터를 추가해야 한다.
func isRoundFromTurn(_ llm.Message, _ int) bool {
	return false
}

// roundPriorityScore는 API 라운드의 제거 우선순위를 반환한다.
// 낮은 값일수록 먼저 제거된다.
// 0: 텍스트 전용(도구 호출 없음) — 증거 없는 대화 라운드
// 1: 도구 호출 또는 도구 결과 포함 — 실행 증거 보존
func roundPriorityScore(r apiRound) int {
	for _, msg := range r.messages {
		if msg.Role == llm.RoleAssistant && len(msg.ToolCalls) > 0 {
			return 1
		}
		if msg.Role == llm.RoleTool {
			return 1
		}
	}
	return 0
}

// trimHistory는 히스토리를 maxHistory 이하로 유지한다.
// API 라운드(user+assistant+tool_results) 단위로 원자적으로 제거하며,
// 텍스트 전용 라운드를 도구 호출 라운드보다 우선적으로 제거한다.
// 동점 시에는 오래된 라운드부터 제거한다(FIFO).
// 최근 preserveRecentRounds 라운드는 항상 보존한다.
func (a *Agent) trimHistory() {
	if len(a.history) <= a.maxHistory {
		return
	}

	rounds := groupByApiRound(a.history)
	before := len(a.history)
	n := len(rounds)

	// 최근 preserveRecentRounds 라운드는 무조건 보존한다.
	// 나머지(trimmable) 라운드에 우선순위 점수를 부여하여 낮은 점수부터 제거한다.
	trimCandidateCount := n - preserveRecentRounds
	if trimCandidateCount <= 0 {
		// 보존 라운드만 남아 있으면 FIFO fallback
		for len(rounds) > 1 {
			total := 0
			for _, r := range rounds {
				total += len(r.messages)
			}
			if total <= a.maxHistory {
				break
			}
			rounds = rounds[1:]
		}
		a.history = flattenRounds(rounds)
		slog.Debug("trimHistory complete (fallback)", "before", before, "after", len(a.history))
		return
	}

	type candidate struct {
		idx   int
		score int
	}
	cands := make([]candidate, trimCandidateCount)
	for i := 0; i < trimCandidateCount; i++ {
		cands[i] = candidate{idx: i, score: roundPriorityScore(rounds[i])}
	}
	// 낮은 score 먼저, 동점이면 오래된 것 먼저(FIFO)
	sort.SliceStable(cands, func(a, b int) bool {
		if cands[a].score != cands[b].score {
			return cands[a].score < cands[b].score
		}
		return cands[a].idx < cands[b].idx
	})

	removed := make(map[int]bool, trimCandidateCount)
	for _, c := range cands {
		total := 0
		for i, r := range rounds {
			if !removed[i] {
				total += len(r.messages)
			}
		}
		if total <= a.maxHistory {
			break
		}
		removed[c.idx] = true
	}

	kept := make([]apiRound, 0, n)
	for i, r := range rounds {
		if !removed[i] {
			kept = append(kept, r)
		}
	}
	a.history = flattenRounds(kept)
	slog.Debug("trimHistory complete",
		"before", before, "after", len(a.history), "remaining_rounds", len(kept))
}
