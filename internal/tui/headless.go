package tui

import (
	"context"
	"os"
	"strings"

	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/query"
	"golang.org/x/term"
)

// HeadlessMirror는 bubbletea 이벤트 루프 없이 TUI 렌더링 파이프라인을 그대로 구동해
// 완성된 entry를 stdout으로 흘려보내는 헤드리스 미러다. --aidebug 모드에서 TUI
// 사용자가 보는 것과 동일한 ANSI 출력을 얻기 위해 사용한다.
//
// 동작 원리:
//   - 내부적으로 uiModel 인스턴스를 생성해 모든 이벤트를 applyPresentationEvent로
//     반영한다 (transcript.go / render.go / toolui 플러그인 전부 재사용).
//   - 새로 추가된 entries 중 비활성(IsActive=false) 상태인 것들만 stdout에 flush.
//   - 활성 중인 entry(스트리밍 도구, 진행 중인 도구 그룹 등)는 다음 이벤트 처리 후
//     완료 시점에 출력된다. 따라서 출력 순서는 항상 "완성 상태"만 보장된다.
type HeadlessMirror struct {
	model       uiModel
	out         *os.File
	flushedIdx  int
	emitNewline bool
}

// NewHeadlessMirror는 헤드리스 미러를 초기화한다. cfg.AIDebug는 false로 강제되어
// 풀 ANSI 색상이 살아남는다 (trace-only 모드와 다른 점). 단 cfg.State 등 footer
// 정보는 그대로 사용된다.
func NewHeadlessMirror(cfg Config, out *os.File) *HeadlessMirror {
	mirrorCfg := cfg
	mirrorCfg.AIDebug = false
	// 미러에서는 애니메이션을 끄고 정적 아이콘만 사용하도록 강제 (인코딩 호환성)
	mirrorCfg.UI.Motion.Enabled = false
	// 미러에서는 thinking 과정을 항상 보여주도록 설정 (aidebug 목적 부합)
	mirrorCfg.UI.ViewThinking = true

	m := newUIModelHeadless(mirrorCfg, detectWidth(out))
	return &HeadlessMirror{
		model: m,
		out:   out,
	}
}

// newUIModelHeadless는 newUIModel과 동일하지만 bubbletea 의존부(textarea/viewport
// focus)와 cfg.Engine.Messages() 자동 리플레이를 건너뛴다. 헤드리스 미러는 신규
// 턴만 출력해야 하므로 과거 메시지 리플레이가 불필요하다.
func newUIModelHeadless(cfg Config, width int) uiModel {
	resolvedUI := ResolveUISettings(cfg.UI, cfg.Theme, cfg.AIDebug)
	theme := resolveUIThemeWithVariant(resolvedUI.Theme, resolvedUI.Variant, cfg.AIDebug)

	m := uiModel{
		app:                nil,
		cfg:                cfg,
		theme:              theme,
		ui:                 resolvedUI,
		width:              width,
		height:             24,
		entries:            make([]transcriptEntry, 0, 256),
		assistantStreamIdx: -1,
		thinkingStreamIdx:  -1,
		toolUseStreamIdx:   -1,
		footer:             presentation.BuildFooterState(cfg.State, cfg.WorkDir),
		historyIdx:         -1,
		maxEntries:         4000,
		toolEntryByTaskID:  make(map[string]int),
		transcriptMode:     true,
	}
	return m
}

// Apply는 query.UIEvent 한 건을 미러에 반영하고, 완성된 entry가 있으면 stdout에
// flush한다. presentation.FromUIEvent 변환에 실패하는 이벤트(예: state-only)는
// 무시한다.
func (h *HeadlessMirror) Apply(evt query.UIEvent) {
	pevt, ok := presentation.FromUIEvent(evt)
	if !ok {
		return
	}
	h.model.applyPresentationEvent(pevt)

	// Thinking 출력을 실시간으로 흘려보내기 위한 특별 처리.
	// 일반 flush는 entryActive가 false가 될 때(완료 시점)만 출력되므로,
	// Thinking처럼 스트리밍이 필요한 경우 강제로 flushUpTo를 호출한다.
	if pevt.Kind == presentation.EventThinkingDelta || pevt.Kind == presentation.EventThinkingDone {
		if h.model.ui.ViewThinking && h.model.thinkingStreamIdx >= 0 {
			h.flushUpTo(h.model.thinkingStreamIdx+1, true, pevt)
		}
	}

	h.flush()
}

// FlushAll은 active 여부와 무관하게 남은 entry를 모두 출력한다. 턴 종료 시
// 호출한다.
func (h *HeadlessMirror) FlushAll() {
	h.flushUpTo(len(h.model.entries), true, presentation.Event{})
}

// flush는 비활성(완성) 상태가 된 연속 entry를 stdout으로 내보낸다. 활성 중인
// entry를 만나면 거기서 멈추고 다음 이벤트를 기다린다.
func (h *HeadlessMirror) flush() {
	end := h.flushedIdx
	for i := h.flushedIdx; i < len(h.model.entries); i++ {
		if entryActive(h.model.entries[i]) {
			break
		}
		end = i + 1
	}
	if end > h.flushedIdx {
		h.flushUpTo(end, false, presentation.Event{})
	}
}

func (h *HeadlessMirror) flushUpTo(end int, includeActive bool, pevt presentation.Event) {
	for i := h.flushedIdx; i < end; i++ {
		e := h.model.entries[i]
		if !includeActive && entryActive(e) {
			break
		}

		// thinking 엔트리는 항상 델타 방식으로 처리 (실시간 + 완료 후 중복 방지)
		if e.Kind == "thinking" {
			lines := strings.Split(e.Body, "\n")
			style := h.model.theme.style(h.model.theme.ThinkingTitleColor)

			// 스트리밍 중에는 마지막 라인(진행 중인 라인)을 제외하고 출력
			// Done 이벤트이거나 엔트리가 비활성 상태면 마지막 라인까지 모두 출력
			endLine := len(lines) - 1
			if pevt.Kind == presentation.EventThinkingDone || !e.IsActive {
				endLine = len(lines)
			}

			if endLine > h.model.thinkingFlushedLines {
				newLines := lines[h.model.thinkingFlushedLines:endLine]
				for _, line := range newLines {
					if strings.TrimSpace(line) == "" {
						continue
					}
					if h.emitNewline {
						_, _ = h.out.WriteString("\n")
					}
					_, _ = h.out.WriteString("  " + style.Render(line))
					h.emitNewline = true
				}
				h.model.thinkingFlushedLines += len(newLines)
			}

			// 완료된 엔트리라면 인덱스를 넘겨서 다시 출력되지 않게 함
			if !e.IsActive {
				h.flushedIdx = i + 1
			}
			continue
		}

		rendered := h.model.renderTranscriptEntryAt(i, e)
		if strings.TrimSpace(stripANSI(rendered)) == "" {
			if !e.IsActive {
				h.flushedIdx = i + 1
			}
			continue
		}
		if h.emitNewline {
			_, _ = h.out.WriteString("\n")
		}
		_, _ = h.out.WriteString(rendered)
		_, _ = h.out.WriteString("\n")
		h.emitNewline = true

		if !e.IsActive {
			h.flushedIdx = i + 1
		}
	}
}

// entryActive는 entry가 아직 진행 중인지 판단한다. 활성 entry는 출력을 미루어
// 최종 상태가 확정된 후에만 보여준다.
func entryActive(e transcriptEntry) bool {
	if e.IsActive {
		return true
	}
	for _, sub := range e.SubEntries {
		if sub.IsActive {
			return true
		}
	}
	return false
}

// UpdateFooter는 외부에서 footer state를 갱신할 때 호출한다 (presentation.EventState
// 가 흘러올 때). 현재는 applyPresentationEvent 내부에서 자동 처리되므로 명시 호출
// 불필요. 보존용 export.
func (h *HeadlessMirror) UpdateFooter(footer presentation.FooterState) {
	h.model.footer = footer
}

// UpdateUISettings는 미러의 UI 설정을 갱신한다. 실시간 설정 변경 반영 용도.
func (h *HeadlessMirror) UpdateUISettings(ui UISettings) {
	resolved := ResolveUISettings(ui, h.model.theme.Name, false)
	h.model.ui = resolved
	h.model.theme = resolveUIThemeWithVariant(resolved.Theme, resolved.Variant, false)
}

// detectWidth는 stdout이 TTY일 때 실제 터미널 폭을, 아닐 때 기본값(120)을
// 반환한다. lipgloss/wordwrap 폭 계산에 사용된다.
func detectWidth(out *os.File) int {
	if out == nil {
		return 120
	}
	fd := int(out.Fd())
	if !term.IsTerminal(fd) {
		return 120
	}
	w, _, err := term.GetSize(fd)
	if err != nil || w <= 0 {
		return 120
	}
	return w
}

// HandleSlashOutput은 슬래시 명령 등이 직접 stdout에 쓴 텍스트와 미러 출력 사이에
// 빈 줄을 보장하기 위한 헬퍼다. 호출 시 다음 미러 출력 앞에 \n이 한 번 더 들어간다.
func (h *HeadlessMirror) HandleSlashOutput() {
	h.emitNewline = true
}

// ContextDone은 ctx 취소를 감지해 헤드리스 미러를 종료시키기 위한 외부 훅이다.
// 현재는 단순 noop. 추후 spinner goroutine 등을 도입하면 여기서 정리한다.
func (h *HeadlessMirror) ContextDone(ctx context.Context) {}
