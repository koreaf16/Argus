// Package tui
// File: queue.go
// Description: 에이전트 실행 중 추가 입력을 순서대로 보관하는 FIFO 큐
// Responsibility: 입력 큐잉, 드레인, 취소, 큐 상태 표시

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/muesli/reflow/ansi"
)

// queueEntry는 큐에 쌓인 단일 사용자 입력이다.
type queueEntry struct {
	displayInput  string
	expandedInput string
	queuedAt      time.Time
}

// inputQueue는 에이전트 실행 중 추가 입력을 순서대로 보관한다.
type inputQueue struct {
	entries []queueEntry
}

// Enqueue는 새 입력을 큐 끝에 추가한다.
func (q *inputQueue) Enqueue(displayInput, expandedInput string) {
	q.entries = append(q.entries, queueEntry{
		displayInput:  displayInput,
		expandedInput: expandedInput,
		queuedAt:      time.Now(),
	})
}

// Dequeue는 큐 앞에서 항목 하나를 꺼내 반환한다. 큐가 비어 있으면 false를 반환한다.
func (q *inputQueue) Dequeue() (queueEntry, bool) {
	if len(q.entries) == 0 {
		return queueEntry{}, false
	}
	entry := q.entries[0]
	q.entries = q.entries[1:]
	return entry, true
}

// Clear는 큐에 쌓인 모든 항목을 제거한다.
func (q *inputQueue) Clear() {
	q.entries = nil
}

// Len은 현재 큐에 쌓인 항목 수를 반환한다.
func (q *inputQueue) Len() int {
	return len(q.entries)
}

// View는 큐 상태를 한 줄로 렌더링한다.
// 예: "  ⏳ 2 queued  "check disk space" → "restart nginx""
func (q *inputQueue) View(width int) string {
	if len(q.entries) == 0 {
		return ""
	}

	countStr := StyleQueueCount.Render(fmt.Sprintf("⏳ %d queued", len(q.entries)))
	left := "  " + countStr

	// 첫 두 항목 미리보기
	var previews []string
	for i, e := range q.entries {
		if i >= 2 {
			break
		}
		previews = append(previews, fmt.Sprintf("%q", truncateForQueue(e.displayInput, 30)))
	}

	if len(previews) > 0 {
		preview := "  " + strings.Join(previews, " → ")
		previewStyled := StyleQueuePreview.Render(preview)
		leftW := ansi.PrintableRuneWidth(left)
		previewW := ansi.PrintableRuneWidth(previewStyled)
		if leftW+previewW <= width {
			return left + previewStyled
		}
	}

	return left
}

// truncateForQueue는 표시용 문자열을 maxLen 이하로 자른다.
func truncateForQueue(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
