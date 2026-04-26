// Package tui
// File: chatview_scroll.go
// Description: offset 배열 기반 스크롤 엔진
// Responsibility: dirty 아이템 재렌더, cumulative offset 계산, 뷰포트 범위 결정

package tui

import (
	"sort"
	"strings"
)

// refresh는 dirty 아이템을 재렌더하고 스크롤 위치를 반영하여 viewCache를 갱신한다.
func (cv *chatView) refresh() {
	cv.ensureArraySize()
	cv.rerenderDirty()
	cv.recomputeOffsets()

	if cv.sticky {
		cv.vpTop = max(0, cv.total-cv.height)
	}

	cv.viewCache = cv.buildViewport()
}

// ensureArraySize는 lines/dirty/heights 배열을 items 크기에 맞춘다.
func (cv *chatView) ensureArraySize() {
	n := len(cv.items)
	for len(cv.lines) < n {
		cv.lines = append(cv.lines, nil)
		cv.dirty = append(cv.dirty, true)
		cv.heights = append(cv.heights, 0)
	}
}

// rerenderDirty는 dirty 표시된 아이템만 재렌더하여 lines/heights를 갱신한다.
func (cv *chatView) rerenderDirty() {
	for i := range cv.items {
		if i >= len(cv.dirty) || !cv.dirty[i] {
			continue
		}
		cv.lines[i] = cv.renderItemLines(i, cv.items[i])
		cv.heights[i] = len(cv.lines[i])
		cv.dirty[i] = false
	}
}

// recomputeOffsets는 heights로부터 cumulative offsets와 total을 재계산한다.
func (cv *chatView) recomputeOffsets() {
	n := len(cv.items)
	if cap(cv.offsets) < n {
		cv.offsets = make([]int, n)
	} else {
		cv.offsets = cv.offsets[:n]
	}

	sum := 0
	for i := 0; i < n; i++ {
		cv.offsets[i] = sum
		sum += cv.heights[i]
	}
	cv.total = sum
}

// buildViewport는 vpTop부터 height줄을 잘라내어 뷰포트 문자열을 생성한다.
func (cv *chatView) buildViewport() string {
	if cv.total == 0 {
		return ""
	}

	vpTop := cv.vpTop
	if vpTop < 0 {
		vpTop = 0
	}
	if vpTop > cv.total-cv.height && cv.total > cv.height {
		vpTop = cv.total - cv.height
	}
	cv.vpTop = vpTop

	vpBottom := vpTop + cv.height

	// binary search: vpTop이 속한 아이템 인덱스 찾기
	startItem := sort.Search(len(cv.offsets), func(i int) bool {
		return cv.offsets[i]+cv.heights[i] > vpTop
	})

	var result []string
	for i := startItem; i < len(cv.items); i++ {
		itemTop := cv.offsets[i]
		if itemTop >= vpBottom {
			break
		}
		lines := cv.lines[i]
		// 아이템 내 시작/끝 줄 계산
		lineStart := 0
		if vpTop > itemTop {
			lineStart = vpTop - itemTop
		}
		lineEnd := len(lines)
		if itemTop+len(lines) > vpBottom {
			lineEnd = vpBottom - itemTop
		}
		if lineStart < lineEnd && lineStart < len(lines) {
			if lineEnd > len(lines) {
				lineEnd = len(lines)
			}
			result = append(result, lines[lineStart:lineEnd]...)
		}
	}

	// 뷰포트 높이에 맞추기 (짧으면 빈 줄 추가)
	for len(result) < cv.height {
		result = append(result, "")
	}
	if len(result) > cv.height {
		result = result[:cv.height]
	}

	return strings.Join(result, "\n")
}

// ScrollUp은 위로 스크롤한다.
func (cv *chatView) ScrollUp(lines int) {
	cv.vpTop -= lines
	if cv.vpTop < 0 {
		cv.vpTop = 0
	}
	cv.sticky = false
	cv.viewCache = cv.buildViewport()
}

// ScrollDown은 아래로 스크롤한다.
func (cv *chatView) ScrollDown(lines int) {
	cv.vpTop += lines
	maxTop := max(0, cv.total-cv.height)
	if cv.vpTop >= maxTop {
		cv.vpTop = maxTop
		cv.sticky = true
	}
	cv.viewCache = cv.buildViewport()
}

// SelectUp은 선택을 위로 이동한다.
func (cv *chatView) SelectUp() {
	if len(cv.items) == 0 {
		return
	}
	if cv.selectedIdx == -1 {
		cv.selectedIdx = len(cv.items) - 1
	} else if cv.selectedIdx > 0 {
		cv.selectedIdx--
	}
	cv.sticky = false
	cv.markAllDirty()
	cv.refresh()
}

// SelectDown은 선택을 아래로 이동한다.
func (cv *chatView) SelectDown() {
	if len(cv.items) == 0 || cv.selectedIdx == -1 {
		return
	}
	if cv.selectedIdx < len(cv.items)-1 {
		cv.selectedIdx++
	} else {
		cv.selectedIdx = -1
		cv.sticky = true
	}
	cv.markAllDirty()
	cv.refresh()
}

// ToggleSelected는 현재 선택된 박스의 접기/펼치기를 토글한다.
func (cv *chatView) ToggleSelected() {
	if cv.selectedIdx >= 0 && cv.selectedIdx < len(cv.items) {
		item := &cv.items[cv.selectedIdx]
		if item.kind == kindBox && item.box != nil {
			item.box.ToggleCollapse()
			cv.markDirty(cv.selectedIdx)
			cv.refresh()
		}
	}
}

// markDirty는 특정 아이템을 dirty로 표시한다.
func (cv *chatView) markDirty(idx int) {
	if idx >= 0 && idx < len(cv.dirty) {
		cv.dirty[idx] = true
	}
}

// markAllDirty는 모든 아이템을 dirty로 표시한다.
func (cv *chatView) markAllDirty() {
	for i := range cv.dirty {
		cv.dirty[i] = true
	}
}
