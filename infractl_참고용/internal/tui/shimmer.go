// Package tui
// File: shimmer.go
// Description: Claude CLI 스타일 Thinking shimmer 애니메이션 렌더링
// Responsibility: LLM 응답 대기 중 shimmer 효과와 경과시간을 표시

package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	shimmerInterval = 50 * time.Millisecond // 고주파 50ms (20fps)
	shimmerWidth    = 4                     // 하이라이트 너비 확장
	shimmerPause    = 30                    // 간격이 짧아졌으므로 pause도 조정
)

// shimmerState는 shimmer 애니메이션 상태를 관리한다.
type shimmerState struct {
	text      string
	hint      string // 우측 보조 텍스트 (현재 작업 설명 등)
	startTime time.Time
	pos       int
	width     int
	bgCount   int // "N in background" 표시용
	active    bool
}

// newShimmer는 새 shimmerState를 생성한다.
func newShimmer() shimmerState {
	return shimmerState{}
}

// Start는 shimmer 애니메이션을 시작한다.
func (s *shimmerState) Start(text string) tea.Cmd {
	s.text = text
	s.startTime = time.Now()
	s.pos = 0
	s.active = true
	return shimmerTickCmd()
}

// Stop은 shimmer 애니메이션을 정지한다.
func (s *shimmerState) Stop() {
	s.active = false
	s.pos = 0
}

// RecordActivity는 하위 호환을 위해 남겨둔 no-op 메서드다.
func (s *shimmerState) RecordActivity() {}

// SetText는 shimmer 텍스트를 변경한다 (도구명 등).
func (s *shimmerState) SetText(text string) {
	s.text = text
}

// SetHint는 shimmer 우측 보조 텍스트를 변경한다.
// thinking 스트리밍 내용 등을 실시간으로 표시하는 데 사용한다.
func (s *shimmerState) SetHint(hint string) {
	s.hint = hint
}

// Tick은 shimmer 위치를 전진시킨다.
func (s *shimmerState) Tick() tea.Cmd {
	if !s.active {
		return nil
	}
	s.pos++
	return shimmerTickCmd()
}

// View는 shimmer 애니메이션을 렌더링한다.
// 형식: "● thinking... (3s)          2 in background"
func (s *shimmerState) View() string {
	if !s.active {
		return ""
	}

	text := s.text
	if text == "" {
		text = "thinking..."
	}

	// shimmer 색상 적용 (항상 정상 애니메이션 유지)
	colored := s.colorize(text)

	// 경과시간
	elapsed := formatElapsed(time.Since(s.startTime))
	elapsedStr := StyleInfoBarDim.Render("(" + elapsed + ")")

	left := StyleClaude().Render("●") + " " + colored + " " + elapsedStr

	// 우측: hint(작업 설명) 또는 background count
	var right string
	if s.bgCount > 0 {
		right = StyleInfoBarDim.Render(fmt.Sprintf("%d in background", s.bgCount))
	} else if s.hint != "" {
		right = StyleInfoBarDim.Render(s.hint)
	}

	if right != "" && s.width > 0 {
		leftW := lipgloss.Width(left)
		rightW := lipgloss.Width(right)
		gap := s.width - leftW - rightW
		if gap < 2 {
			return left
		}
		return left + strings.Repeat(" ", gap) + right
	}

	return left
}

// colorize는 텍스트에 shimmer 하이라이트를 적용한다.
// 항상 정상 shimmer 그라디언트를 사용하여 애니메이션이 멈추지 않는다.
func (s *shimmerState) colorize(text string) string {
	if utf8.RuneCountInString(text) == 0 {
		return ""
	}

	runes := []rune(text)
	textLen := len(runes)
	cycleLen := textLen + shimmerPause
	peakPos := s.pos % cycleLen

	var buf strings.Builder
	for i, r := range runes {
		dist := peakPos - i
		if dist < 0 {
			dist = -dist
		}

		var style lipgloss.Style
		switch {
		case dist == 0:
			style = lipgloss.NewStyle().Foreground(ColorShimmerPeak).Bold(true)
		case dist <= 1:
			style = lipgloss.NewStyle().Foreground(ColorShimmerMid)
		case dist <= shimmerWidth/2:
			style = lipgloss.NewStyle().Foreground(ColorShimmerBase)
		default:
			style = StyleInfoBarDim
		}
		buf.WriteString(style.Render(string(r)))
	}

	return buf.String()
}

// StyleClaude는 Claude orange bold 스타일을 반환한다.
func StyleClaude() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ColorClaude).Bold(true)
}

// thinkHint는 누적된 thinking 내용에서 마지막 비어있지 않은 줄을 추출하여
// shimmer hint로 표시할 짧은 문자열을 반환한다.
// maxRunes를 초과하는 경우 앞부분을 "…"으로 대체한다.
func thinkHint(content string, maxRunes int) string {
	content = strings.ReplaceAll(content, "<think>", "")
	content = strings.ReplaceAll(content, "</think>", "")
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) <= maxRunes {
			return line
		}
		return "…" + string(runes[len(runes)-maxRunes:])
	}
	return ""
}

// shimmerTickCmd는 shimmer 간격으로 틱을 생성하는 Cmd를 반환한다.
func shimmerTickCmd() tea.Cmd {
	return tea.Tick(shimmerInterval, func(time.Time) tea.Msg {
		return ShimmerTickMsg{}
	})
}

// formatElapsed는 Duration을 간결한 문자열로 변환한다.
func formatElapsed(d time.Duration) string {
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s = s % 60
	return fmt.Sprintf("%dm %ds", m, s)
}
