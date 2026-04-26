package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m AppModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !msg.Paste {

		if m.selection.active {
			handled, respMsg := m.selection.handleKey(msg)
			if handled {
				if respMsg != nil {
					return m.Update(respMsg)
				}
				return m, nil
			}
		}

		if m.form.active {
			switch msg.Type {
			case tea.KeyUp:
				if m.form.IsSelectField() {
					m.form.SelectUp()
					return m, nil // inputBar history 이동 차단
				}
			case tea.KeyDown:
				if m.form.IsSelectField() {
					m.form.SelectDown()
					return m, nil // inputBar history 이동 차단
				}
			case tea.KeyEnter:
				if m.form.IsSelectField() {
					// SELECT 필드: 텍스트 입력 없이 즉시 다음 필드로
					m.input.Reset()
					if m.form.AdvanceToNext() {
						return m.Update(FormResponseMsg{
							Result:  m.form.BuildResult(),
							ReplyCh: m.form.replyCh,
						})
					}
					m.input.ti.Placeholder = m.form.CurrentPlaceholder()
					return m, nil
				}
			case tea.KeyTab:
				m.form.NextField()
				m.input.Reset()
				m.input.ti.Placeholder = m.form.CurrentPlaceholder()
				return m, nil
			case tea.KeyShiftTab:
				m.form.PrevField()
				m.input.Reset()
				m.input.ti.Placeholder = m.form.CurrentPlaceholder()
				return m, nil
			case tea.KeyEsc:
				return m.Update(FormResponseMsg{
					Result:  FormResult{Cancelled: true},
					ReplyCh: m.form.replyCh,
				})
			}
			// 나머지 키는 inputBar로 fall-through (텍스트 필드 타이핑/Enter)
		}

		if m.privilege.active {
			handled, respMsg := m.privilege.handleKey(msg)
			if handled {
				if respMsg != nil {
					return m.Update(respMsg)
				}
				return m, nil
			}
		}

		if msg.Type != tea.KeyCtrlC && m.ctrlCCount > 0 {
			m.ctrlCCount = 0
			m.input.ti.Placeholder = "Press Ctrl+C again to quit."
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			if m.ctrlCCount == 0 {
				m.ctrlCCount = 1
				m.input.ti.SetValue("")
				m.input.ti.Placeholder = "Press Ctrl+C again to quit."
				return m, nil
			}
			m.cancel()
			return m, tea.Quit
		}

		if msg.String() == "ctrl+o" {
			if m.histOverlay.isActive() {
				m.histOverlay.close()
			} else {
				m.histOverlay.open(m.history.Len())
			}
			return m, nil
		}

		if msg.String() == "ctrl+l" {
			return m, tea.ClearScreen
		}

		if msg.String() == "ctrl+r" && !m.busy {
			m.input.advanceHistorySearch()
			return m, nil
		}

		if msg.String() == "ctrl+b" && m.busy {
			count := m.activeTools.BackgroundAll()
			if count > 0 {
				m.shimmer.bgCount = m.activeTools.BackgroundCount()
				if m.box != nil {
					m.box.Println(renderSystemLine(
						fmt.Sprintf("%d tool(s) moved to background", count)))
				}
			}
			return m, nil
		}

		if msg.String() == "ctrl+y" {
			m.yoloMode = m.ag.ToggleYoroMode()
			m.statusBar.setYoloMode(m.yoloMode)
			return m, nil
		}
		if msg.String() == "ctrl+shift+y" {
			m.elevatedMode = m.ag.ToggleElevatedMode()
			m.statusBar.setElevatedMode(m.elevatedMode)
			return m, nil
		}

		if msg.Type == tea.KeyShiftTab {
			// Plan Mode 종료 시 pending 명령이 있으면 큐 내용을 알림으로 표시한다.
			// 실제 승인/실행은 `infractl plan exit [--approve|--reject-all]` CLI 로 처리한다.
			if m.planMode {
				if ps := m.ag.PlanState(); ps != nil && ps.Queue().Len() > 0 {
					pending := ps.Queue().Peek()
					notice := fmt.Sprintf("📋 Plan Mode 종료: %d개의 보류 명령이 있습니다.\n", len(pending))
					for i, p := range pending {
						notice += fmt.Sprintf("  [%d] %s (id=%s)\n", i+1, p.Tool, p.ID)
					}
					notice += "승인: infractl plan exit --approve <id,...>  |  거부: infractl plan exit --reject-all"
					m.planMode = m.ag.TogglePlanMode()
					m.statusBar.setPlanMode(m.planMode)
					m.statusBar.setPlanPending(0)
					return m, func() tea.Msg { return SystemMsg(notice) }
				}
			}
			m.planMode = m.ag.TogglePlanMode()
			m.statusBar.setPlanMode(m.planMode)
			// Plan Mode 진입 시 pending 수 초기화 (이미 0이지만 명시적으로)
			m.statusBar.setPlanPending(0)
			return m, nil
		}

		if m.histOverlay.isActive() {
			switch msg.Type {
			case tea.KeyUp:
				m.histOverlay.scrollUp()
				return m, nil
			case tea.KeyDown:
				m.histOverlay.scrollDown(m.history.Len())
				return m, nil
			case tea.KeyEsc:
				m.histOverlay.close()
				return m, nil
			}
		}

		if msg.String() == "esc" && m.busy {
			m.cancel()
			ctx, cancel := context.WithCancel(context.Background())
			m.ctx = ctx
			m.cancel = cancel
			m.busy = false
			m.input.SetBusy(false)
			m.statusBar.setQueueLen(0)
			queueLen := m.queue.Len()
			m.queue.Clear()
			m.streamTokens = ""
			m.streamLines = nil
			m.streamCache.Reset()
			if m.box != nil {
				if queueLen > 0 {
					m.box.Println(renderSystemLine(
						fmt.Sprintf("cancelled by user (%d queued items cleared)", queueLen)))
				} else {
					m.box.Println(renderSystemLine("cancelled by user"))
				}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m = m.resize()
	return m, cmd
}

func welcomeBanner(model string, serverCount int) string {
	title := StyleBannerTitle.Render("Infractl") + " " + StyleBannerInfo.Render("v1.0.0")
	info := StyleInfoBarDim.Render(fmt.Sprintf("model: %s | %d workspaces", model, serverCount))
	return fmt.Sprintf("\n  %s\n  %s\n", title, info)
}
