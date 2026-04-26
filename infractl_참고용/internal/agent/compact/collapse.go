// Package compact
// File: collapse.go
// Description: staged context collapse — 대형 메시지를 우선순위 큐에 적재 후 매 turn drain
// Responsibility: 즉각 압축 대신 점진적 drain으로 컨텍스트 창 서서히 해방
//
// ★ 자체 설계: claude_cli/src/services/contextCollapse/ 는 feature-gated 미존재
// 근거: claude_cli/src/query.ts:441 (applyCollapsesIfNeeded 호출부),
//       query.ts:1094-1116 (recoverFromOverflow + collapse_drain_retry),
//       services/compact/grouping.ts:22 (groupMessagesByApiRound 패턴)

package compact

import (
	"container/heap"
	"fmt"
	"log/slog"
	"strings"
)

// collapseMinChars는 collapse staged 대상이 되는 메시지 최소 문자 수다.
const collapseMinChars = 5_000

// collapseMaxChars는 collapse 후 메시지의 최대 문자 수다.
const collapseMaxChars = 300

// Collapse는 staged context collapse 인터페이스다.
type Collapse interface {
	// ApplyIfNeeded는 Warning 이상일 때 대형 메시지를 staged queue에 적재한다.
	ApplyIfNeeded(s *State, tokenState TokenState)
	// Drain은 staged queue에서 1개를 꺼내 실제로 압축한다.
	// 다음 turn 진입 시 recovery 또는 stack.Apply 에서 호출한다.
	Drain(s *State) bool
	// HasStaged는 staged queue에 대기 중인 항목이 있는지 반환한다.
	HasStaged() bool
}

// NewCollapse는 Collapse 구현체를 반환한다.
func NewCollapse() Collapse {
	return &collapseCompactor{queue: &collapseHeap{}}
}

// collapseItem은 staged queue의 단일 항목이다.
type collapseItem struct {
	msgIdx int
	size   int
}

// collapseHeap은 max-heap (큰 메시지 우선)이다.
type collapseHeap []collapseItem

func (h collapseHeap) Len() int            { return len(h) }
func (h collapseHeap) Less(i, j int) bool  { return h[i].size > h[j].size } // max-heap
func (h collapseHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *collapseHeap) Push(x any) { *h = append(*h, x.(collapseItem)) }
func (h *collapseHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

type collapseCompactor struct {
	queue *collapseHeap
}

// ApplyIfNeeded는 Warning 이상일 때 큰 메시지를 staged queue에 적재한다.
// Ported from: claude_cli/src/query.ts:441 applyCollapsesIfNeeded 호출부
// 핵심 패턴: 즉시 압축하지 않고 queue에 적재 → drain 으로 매 turn 1개씩 처리
func (cc *collapseCompactor) ApplyIfNeeded(s *State, tokenState TokenState) {
	if tokenState < TokenWarning {
		return
	}

	staged := 0
	for i, msg := range s.Messages {
		if len(msg.Content) >= collapseMinChars {
			heap.Push(cc.queue, collapseItem{msgIdx: i, size: len(msg.Content)})
			staged++
		}
	}

	if staged > 0 {
		slog.Debug("compact collapse: staged messages",
			"count", staged,
			"queue_len", cc.queue.Len(),
			"token_state", tokenState,
		)
	}
}

// Drain은 staged queue에서 가장 큰 항목 1개를 실제로 압축한다.
// Ported from: claude_cli/src/query.ts:1094-1116 (collapse drain retry 패턴)
// 반환값: true이면 drain이 실행됨, false이면 queue 비어 있음
func (cc *collapseCompactor) Drain(s *State) bool {
	if cc.queue.Len() == 0 {
		return false
	}
	item := heap.Pop(cc.queue).(collapseItem)

	// 메시지 인덱스 유효성 검사
	if item.msgIdx >= len(s.Messages) {
		slog.Warn("compact collapse: drain skipped, invalid msg index",
			"msg_idx", item.msgIdx,
			"messages_len", len(s.Messages),
		)
		return false
	}

	msg := s.Messages[item.msgIdx]
	if len(msg.Content) <= collapseMaxChars {
		// 이미 충분히 짧아진 경우 (이전 round에서 처리됨)
		return true
	}

	original := msg.Content
	collapsed := collapseContent(msg.Content)
	s.Messages[item.msgIdx].Content = collapsed

	tokensSaved := (len(original) - len(collapsed)) / AvgCharsPerToken
	slog.Info("compact collapse: drain applied",
		"msg_idx", item.msgIdx,
		"original_chars", len(original),
		"collapsed_chars", len(collapsed),
		"tokens_saved", tokensSaved,
		"remaining_staged", cc.queue.Len(),
	)
	return true
}

// HasStaged는 staged queue에 대기 항목이 있는지 반환한다.
// Recovery.Handle 에서 collapse drain 우선 시도 여부 결정에 사용된다.
func (cc *collapseCompactor) HasStaged() bool {
	return cc.queue.Len() > 0
}

// collapseContent는 긴 내용을 앞 일부 + 요약 마커로 압축한다.
func collapseContent(content string) string {
	if len(content) <= collapseMaxChars {
		return content
	}
	lines := strings.SplitN(content, "\n", 5)
	var preview strings.Builder
	for _, line := range lines[:min(3, len(lines))] {
		remaining := collapseMaxChars - preview.Len()
		if remaining <= 0 {
			break
		}
		if len(line) > remaining {
			line = line[:remaining]
		}
		preview.WriteString(line)
		preview.WriteString("\n")
	}
	previewChars := preview.Len()
	removed := len(content) - previewChars
	if removed <= 0 {
		return content
	}
	return fmt.Sprintf("%s[collapsed: %d chars removed]", preview.String(), removed)
}

var _ Collapse = (*collapseCompactor)(nil)
