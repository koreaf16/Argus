// Package tui
// File: background_render.go
// Description: Rendering helpers for background runtime notices.
// Responsibility: Render completed background tasks and soft stall notices.

package tui

import (
	"fmt"
	"strings"
	"time"
)

// renderBackgroundDone renders a compact completion note for backgrounded tools.
func renderBackgroundDone(name string, duration time.Duration, success bool) string {
	bracket := StyleResponseBracket.Render("  ⎿ ")
	dur := StyleCmdBoxDim.Render(fmt.Sprintf(" (%s)", formatElapsedShort(duration)))
	displayName := StyleCmdBoxToolName.Render(toolDisplayName(name))
	suffix := StyleCmdBoxDim.Render(" completed in background")
	if success {
		return bracket + displayName + suffix + dur
	}
	return bracket + StyleError.Render("x") + " " + displayName + suffix + dur
}

// renderStallNotice renders a soft non-terminal watchdog hint.
func renderStallNotice(jobID int, tail []string) string {
	bracket := StyleResponseBracket.Render("  ⎿ ")
	hint := StyleCmdBoxDim.Render(fmt.Sprintf("job #%d waiting for input", jobID))
	if len(tail) == 0 {
		return bracket + hint
	}
	last := strings.TrimSpace(tail[len(tail)-1])
	if len(last) > 80 {
		last = last[:77] + "..."
	}
	return bracket + hint + StyleCmdBoxDim.Render(": ") + StyleClaude().Render(last)
}
