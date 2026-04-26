// Package installpath
// File: orchestrator.go
// Description: Pure decision logic for install/migrate auto-followups.
// Responsibility: Decide whether runtime should enqueue a next tool invocation.

package installpath

import (
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/tools"
)

// Decide computes the next automatic action from the latest tool outcome.
func Decide(taskKind taskctx.TaskKind, lastTool string, outcome tools.ToolOutcome, state PipelineState) NextAction {
	if !requiresAutoVerify(taskKind) {
		return NextAction{AutoInvoke: false}
	}

	switch strings.ToLower(strings.TrimSpace(lastTool)) {
	case "submit_job":
		meta, ok := tools.ParseTaskProgressMetadata(outcome.MetadataJSON)
		if !ok || meta.JobID <= 0 {
			return NextAction{AutoInvoke: false}
		}
		return NextAction{AutoInvoke: false}

	case "job_complete":
		if !outcome.Success {
			return NextAction{AutoInvoke: false}
		}
		if state.PendingVerify.JobID <= 0 || state.PendingVerify.JobID != state.LastJobID {
			return NextAction{AutoInvoke: false}
		}
		taskKey := strings.TrimSpace(state.PendingVerify.TaskKey)
		if taskKey == "" {
			taskKey = fmt.Sprintf("job:%d", state.PendingVerify.JobID)
		}

		return NextAction{
			AutoInvoke: true,
			Tool:       "verify_complete",
			Reason:     "auto chained after successful background install/migrate job",
			Args: map[string]any{
				"task_key":              taskKey,
				"summary":               fmt.Sprintf("Background job %d completed successfully", state.PendingVerify.JobID),
				"verification_evidence": fmt.Sprintf("background manager reported success for job:%d", state.PendingVerify.JobID),
				"auto_chained":          true,
			},
		}
	}

	return NextAction{AutoInvoke: false}
}

func requiresAutoVerify(kind taskctx.TaskKind) bool {
	return kind == taskctx.KindInstall || kind == taskctx.KindMigrate
}
