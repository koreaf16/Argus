package bash

import (
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
)

// syncActiveTargetFromLane reads the lane snapshot for alias (if any) and
// writes the result into AppState.ActiveTarget so the TUI status bar and the
// LLM system prompt always see the authoritative cwd / account stack.
//
// When no lane is open (local workspace, lane path disabled, etc.) the active
// target is reduced to the alias only — there is no persistent shell context
// to surface.
func syncActiveTargetFromLane(ctx tool.Context, alias string) {
	if ctx.State == nil {
		return
	}
	target := state.ActiveTarget{Alias: alias}
	if ctx.Workspace != nil {
		if info, ok := ctx.Workspace.LaneInfo(alias); ok {
			target.AccountStack = append([]string(nil), info.AccountStack...)
			target.CWD = info.CWD
			target.LaneID = laneIDFromInfo(info)
		}
	}
	ctx.State.SetActiveTarget(target)
}

// laneIDFromInfo synthesizes a stable identifier for a LaneInfo without
// pulling lane.LaneIDFor into the tools layer (which would create a deeper
// dependency than necessary).
func laneIDFromInfo(info workspace.LaneInfo) string {
	if len(info.AccountStack) == 0 {
		return info.Alias
	}
	return info.Alias + "#" + strings.Join(info.AccountStack, ">")
}
