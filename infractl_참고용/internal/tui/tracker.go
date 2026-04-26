// Package tui
// File: tracker.go
// Description: TUI 상태 추적기 (채팅창 위, 입력바 아래에 위치)
// Responsibility: 'Thinking...', 도구 실행 중 등 현재 활성 상태 표시

package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type trackerState int

const (
	trackerIdle trackerState = iota
	trackerThinking
	trackerTool
)

// activeTracker는 현재 진행 중인 배경 작업 상태를 보여준다.
type activeTracker struct {
	state     trackerState
	message   string
	spinner   spinner.Model
	startTime time.Time
	width     int
}

func newActiveTracker() activeTracker {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = StyleSpinner
	return activeTracker{
		state:   trackerIdle,
		spinner: s,
	}
}

func (t *activeTracker) setWidth(w int) {
	t.width = w
}

func (t *activeTracker) Init() tea.Cmd {
	return t.spinner.Tick
}

func (t *activeTracker) Update(msg tea.Msg) (activeTracker, tea.Cmd) {
	if t.state == trackerIdle {
		return *t, nil
	}
	var cmd tea.Cmd
	t.spinner, cmd = t.spinner.Update(msg)
	return *t, cmd
}

func (t *activeTracker) startThinking() {
	t.state = trackerThinking
	t.message = "Thinking"
	t.startTime = time.Now()
}

func (t *activeTracker) startTool(name string) {
	t.state = trackerTool
	t.message = fmt.Sprintf("Running %s", name)
	t.startTime = time.Now()
}

func (t *activeTracker) clear() {
	t.state = trackerIdle
}

func (t activeTracker) View() string {
	if t.state == trackerIdle {
		// 빈 고정 1줄 렌더링. 외부에서 \n으로 이어지므로 빈 문자열 반환으로 충분
		return ""
	}

	elapsed := time.Since(t.startTime)
	elapsedStr := fmt.Sprintf("%02dm %02ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
	
	var label string
	if t.state == trackerThinking {
		dots := thinkingDots(elapsed)
		label = fmt.Sprintf("%-3s", dots)
	} else {
		label = "..."
	}

	header := fmt.Sprintf("%s %s%s (esc to cancel, %s)", t.spinner.View(), t.message, label, elapsedStr)
	res := StyleThinking.Render(header)

	// 폭 초과 방지. 띄어쓰기로 채우면 터미널에서 강제 줄바꿈이 일어날 수 있음
	return res
}
