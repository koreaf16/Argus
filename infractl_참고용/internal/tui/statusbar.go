package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yourorg/infractl/internal/store"
)

const tuiVersion = "1.0.0"

type statusBar struct {
	model        string
	serverCount  int
	width        int
	busy         bool
	yoloMode     bool
	elevatedMode bool
	planMode     bool
	planPending  int
	activeServer *store.Server

	totalCostUSD   float64
	inputTokens    int
	outputTokens   int
	contextPercent float64
	turnDurationMs int64

	taskSummary string
	queueLen    int
}

func newStatusBar(model string, serverCount int) statusBar {
	return statusBar{model: model, serverCount: serverCount}
}

func (s statusBar) Init() tea.Cmd { return nil }

func (s statusBar) Update(msg tea.Msg) (statusBar, tea.Cmd) {
	return s, nil
}

func (s statusBar) View() string {
	arrow := StyleInfoBarArrow.Render("Model")
	sep := StyleFooterDim.Render(" | ")

	leftParts := []string{arrow + " " + s.model}
	if s.totalCostUSD > 0 {
		leftParts = append(leftParts, formatCost(s.totalCostUSD))
	}
	totalTokens := s.inputTokens + s.outputTokens
	if totalTokens > 0 {
		leftParts = append(leftParts, formatTokens(totalTokens)+" tokens")
	}
	if s.contextPercent > 0 {
		leftParts = append(leftParts, formatContext(s.contextPercent)+" ctx")
	}
	left := strings.Join(leftParts, sep)

	rightParts := []string{}
	if s.queueLen > 0 {
		rightParts = append(rightParts, StyleQueueCount.Render(fmt.Sprintf("%d queued", s.queueLen)))
	}
	if s.planMode {
		badge := "PLAN"
		if s.planPending > 0 {
			badge = fmt.Sprintf("PLAN (%d pending)", s.planPending)
		}
		rightParts = append(rightParts, StyleSuccess.Render(badge))
	}
	if s.taskSummary != "" {
		rightParts = append(rightParts, StyleTaskDim.Render("Task | "+s.taskSummary))
	}
	if s.yoloMode {
		rightParts = append(rightParts, StyleWarning.Render("YORO"))
	}
	if s.elevatedMode {
		rightParts = append(rightParts, StyleWarning.Render("ELEVATED"))
	}
	if s.activeServer != nil {
		dot := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("*")
		name := lipgloss.NewStyle().Bold(true).Render(s.activeServer.Name)
		rightParts = append(rightParts, dot+" "+name+" ("+s.activeServer.Host+")")
	}
	if s.serverCount > 0 {
		rightParts = append(rightParts, fmt.Sprintf("%d workspaces", s.serverCount))
	}
	rightParts = append(rightParts, "v"+tuiVersion)
	right := StyleInfoBarDim.Render(strings.Join(rightParts, sep))

	if s.width <= 0 {
		return left + "  " + right
	}

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := s.width - leftWidth - rightWidth
	if gap < 2 {
		gap = 2
	}
	return left + strings.Repeat(" ", gap) + right
}

func (s *statusBar) setWidth(w int)                    { s.width = w }
func (s *statusBar) setServerCount(n int)              { s.serverCount = n }
func (s *statusBar) setBusy(b bool)                    { s.busy = b }
func (s *statusBar) setYoloMode(v bool)                { s.yoloMode = v }
func (s *statusBar) setElevatedMode(v bool)            { s.elevatedMode = v }
func (s *statusBar) setPlanMode(v bool)                { s.planMode = v }
func (s *statusBar) setPlanPending(n int)              { s.planPending = n }
func (s *statusBar) setActiveServer(srv *store.Server) { s.activeServer = srv }
func (s *statusBar) setTaskSummary(summary string)     { s.taskSummary = summary }
func (s *statusBar) setQueueLen(n int)                 { s.queueLen = n }

func (s *statusBar) updateUsage(msg UsageUpdateMsg) {
	s.totalCostUSD += msg.CostUSD
	s.inputTokens += msg.InputTokens
	s.outputTokens += msg.OutputTokens
	s.turnDurationMs = msg.DurationMs
}

func formatCost(usd float64) string {
	if usd < 0.01 {
		return fmt.Sprintf("$%.4f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}

func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func formatContext(pct float64) string {
	text := fmt.Sprintf("%.0f%%", pct)
	switch {
	case pct >= 95:
		return StyleError.Render(text)
	case pct >= 80:
		return StyleWarning.Render(text)
	default:
		return StyleSuccess.Render(text)
	}
}
