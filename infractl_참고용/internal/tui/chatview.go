// Package tui
// File: chatview.go
// Description: 대화 히스토리와 도구 실행 박스를 표시하는 스크롤 영역
// Responsibility: 아이템 관리, 마크다운 렌더링 위임, 공개 API 제공

package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type chatItemKind int

const (
	kindUser chatItemKind = iota
	kindAssistant
	kindThinking
	kindBox
	kindError
)

// mdRenderInterval은 스트리밍 중 마크다운 재렌더 최소 간격이다.
const mdRenderInterval = 150 * time.Millisecond

// refreshInterval은 스트리밍 중 viewport refresh 최소 간격이다.
const refreshInterval = 50 * time.Millisecond

type chatItem struct {
	kind      chatItemKind
	text      string // user / error
	assistant string // assistant 누적 토큰 (raw)

	// 마크다운 렌더 캐시 ([]string 기반)
	renderedLines []string    // 최종 렌더 결과 줄 슬라이스
	mdCache       stableCache // 스트리밍 stablePrefix 캐시
	lastMDRender  time.Time   // 마지막 렌더 시각
	finalized     bool        // FinalizeAssistant 완료 여부

	thinkText  string        // thinking 토큰 누적
	box        *cmdBox       // tool execution box
	spinner    spinner.Model // thinking 스피너
	startTime  time.Time     // thinking 시작 시각
	isThinking bool          // 활성 thinking 여부
}

// chatView는 offset 배열 기반 스크롤을 사용하는 대화 영역이다.
type chatView struct {
	items  []chatItem
	width  int
	height int

	// offset 배열 스크롤 엔진 (chatview_scroll.go)
	lines   [][]string // renderItemLines(i) 결과 캐시
	dirty   []bool     // true면 재렌더 필요
	heights []int      // len(lines[i])
	offsets []int      // cumulative sum
	total   int

	sticky bool // true면 항상 최하단 유지
	vpTop  int  // 뷰포트 상단의 전체 줄 오프셋

	mdRender          *mdRenderer
	selectedIdx       int // -1은 선택 없음
	viewCache         string
	needsNewAssistant bool
	lastRefresh       time.Time
	pendingRefresh    bool
}

func newChatView(width, height int) chatView {
	return chatView{
		items:       make([]chatItem, 0),
		mdRender:    newMdRenderer(width - 4),
		width:       width,
		height:      height,
		sticky:      true,
		selectedIdx: -1,
	}
}

func (cv *chatView) setSize(width, height int) {
	cv.width = width
	cv.height = height
	cv.mdRender = newMdRenderer(width - 4)
	cv.markAllDirty()
	cv.refresh()
}

func (cv *chatView) AddUserInput(text string) {
	cv.sticky = true
	cv.needsNewAssistant = true
	cv.items = append(cv.items, chatItem{kind: kindUser, text: text})
	cv.refresh()
}

func (cv *chatView) AddSystemMessage(text string) {
	cv.sticky = true
	lines := strings.Split(text, "\n")
	cv.items = append(cv.items, chatItem{
		kind:          kindAssistant,
		renderedLines: lines,
		finalized:     true,
	})
	cv.refresh()
}

func (cv *chatView) AddThinkingItem() tea.Cmd {
	cv.sticky = true
	cv.needsNewAssistant = true
	cv.removeThinkingItems()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = StyleSpinner
	item := chatItem{
		kind:       kindThinking,
		spinner:    s,
		startTime:  time.Now(),
		isThinking: true,
	}
	cv.items = append(cv.items, item)
	cv.refresh()
	return cv.items[len(cv.items)-1].spinner.Tick
}

func (cv *chatView) AppendThinkingToken(token string) {
	for i := len(cv.items) - 1; i >= 0; i-- {
		if cv.items[i].kind == kindThinking {
			cv.items[i].thinkText += token
			return
		}
	}
}

func (cv *chatView) ClearThinkingItem() {
	cv.removeThinkingItems()
	cv.refresh()
}

func (cv *chatView) removeThinkingItems() {
	filtered := cv.items[:0]
	for _, item := range cv.items {
		if item.kind != kindThinking {
			filtered = append(filtered, item)
		}
	}
	cv.items = filtered
	// 배열 크기를 items에 맞춤
	cv.lines = cv.lines[:len(cv.items)]
	cv.dirty = cv.dirty[:len(cv.items)]
	cv.heights = cv.heights[:len(cv.items)]
}

func (cv *chatView) StartAssistant() {
	cv.items = append(cv.items, chatItem{kind: kindAssistant})
}

func (cv *chatView) AppendToken(token string) {
	var idx int
	if cv.needsNewAssistant {
		cv.needsNewAssistant = false
		cv.items = append(cv.items, chatItem{kind: kindAssistant, assistant: token})
		idx = len(cv.items) - 1
	} else {
		found := false
		for i := len(cv.items) - 1; i >= 0; i-- {
			if cv.items[i].kind == kindAssistant {
				cv.items[i].assistant += token
				idx = i
				found = true
				break
			}
		}
		if !found {
			cv.items = append(cv.items, chatItem{kind: kindAssistant, assistant: token})
			idx = len(cv.items) - 1
		}
	}

	now := time.Now()
	if now.Sub(cv.items[idx].lastMDRender) >= mdRenderInterval {
		cv.items[idx].renderedLines = cv.mdRender.RenderStreaming(
			cv.items[idx].assistant, &cv.items[idx].mdCache)
		cv.items[idx].lastMDRender = now
		cv.markDirty(idx)
	}

	if now.Sub(cv.lastRefresh) >= refreshInterval {
		cv.refresh()
		cv.lastRefresh = now
		cv.pendingRefresh = false
	} else {
		cv.pendingRefresh = true
	}
}

func (cv *chatView) FinalizeAssistant(content string) {
	if content == "" {
		cv.needsNewAssistant = false
		return
	}
	if cv.needsNewAssistant {
		cv.needsNewAssistant = false
		rendered := cv.mdRender.Render(content)
		cv.items = append(cv.items, chatItem{
			kind:          kindAssistant,
			assistant:     content,
			renderedLines: rendered,
			finalized:     true,
		})
		cv.refresh()
		return
	}
	for i := len(cv.items) - 1; i >= 0; i-- {
		if cv.items[i].kind == kindAssistant {
			cv.items[i].assistant = content
			cv.items[i].renderedLines = cv.mdRender.Render(content)
			cv.items[i].mdCache.Reset()
			cv.items[i].finalized = true
			cv.markDirty(i)
			cv.refresh()
			return
		}
	}
}

func (cv *chatView) AddCmdBox(toolName, target string, args map[string]interface{}) tea.Cmd {
	box := newCmdBox(toolName, target, args)
	cv.items = append(cv.items, chatItem{kind: kindBox, box: &box})
	cv.sticky = true
	cv.needsNewAssistant = true
	cv.refresh()
	return box.Init()
}

func (cv *chatView) AppendBoxOutput(line string) {
	for i := len(cv.items) - 1; i >= 0; i-- {
		if cv.items[i].kind == kindBox && cv.items[i].box != nil &&
			cv.items[i].box.status == boxRunning {
			cv.items[i].box.AppendOutput(line)
			cv.markDirty(i)
			cv.refresh()
			return
		}
	}
}

func (cv *chatView) CompleteLastBox(result string, dur time.Duration, success bool, metadataJSON string) {
	for i := len(cv.items) - 1; i >= 0; i-- {
		if cv.items[i].kind == kindBox && cv.items[i].box != nil &&
			cv.items[i].box.status == boxRunning {
			cv.items[i].box.SetCompleted(result, dur, success, metadataJSON)
			cv.sticky = true
			cv.needsNewAssistant = true
			cv.markDirty(i)
			cv.refresh()
			return
		}
	}
}

func (cv *chatView) AddError(msg string) {
	cv.items = append(cv.items, chatItem{kind: kindError, text: msg})
	cv.sticky = true
	cv.refresh()
}
