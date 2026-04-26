// Package tui
// File: task_panel.go
// Description: 활성 작업 컨텍스트 패널 렌더링 — 작업 진행 상태 표시
// Responsibility: ActiveTaskData 스냅샷을 받아 패널 문자열 렌더링

package tui

import (
	"fmt"
	"strings"
)

// ActiveTaskData holds minimal data for rendering the active task panel.
type ActiveTaskData struct {
	TaskID        string
	Title         string
	Kind          string
	TargetServer  string
	TargetAccount string
	Status        string
	PlanSteps     []string
	CurrentStep   int    // 0-based index of current step (-1 if none)
	ElevationUser string // current user after elevation ("" if not elevated)
}

// RenderActiveTaskPanel returns a short status panel string for the active task.
func RenderActiveTaskPanel(d ActiveTaskData, width int) string {
	if d.TaskID == "" {
		return ""
	}
	if width <= 0 {
		width = 80
	}

	var sb strings.Builder

	// Title bar
	titleLine := StyleTaskTitle.Render("▶ "+d.Title) + "  " + StyleTaskDim.Render("["+d.Kind+"]")
	sb.WriteString(titleLine + "\n")

	// Server / account
	meta := fmt.Sprintf("  %s / %s", nvl(d.TargetServer, "-"), nvl(d.TargetAccount, "-"))
	if d.ElevationUser != "" {
		var badge string
		if d.ElevationUser == "root" {
			badge = StyleTaskBadgeRoot.Render("root")
		} else {
			badge = StyleTaskBadgeAccount.Render(d.ElevationUser)
		}
		meta += "  " + badge
	}
	sb.WriteString(meta + "\n")

	// Plan steps
	if len(d.PlanSteps) > 0 {
		sb.WriteString("\n")
		for i, step := range d.PlanSteps {
			prefix := "  □"
			style := StyleTaskDim
			if i == d.CurrentStep {
				prefix = "  ⟳"
				style = StyleWarning
			} else if i < d.CurrentStep {
				prefix = "  ✓"
				style = StyleSuccess
			}
			sb.WriteString(style.Render(fmt.Sprintf("%s %s", prefix, step)) + "\n")
		}
	}

	return StyleTaskProposalOuter.Width(width - 2).Render(sb.String())
}
