package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

type pendingVerificationTask struct {
	ToolID string
	Meta   tools.TaskProgressMetadata
}

func (a *Agent) rememberTaskProgress(toolID string, meta tools.TaskProgressMetadata) {
	if strings.TrimSpace(toolID) == "" {
		return
	}

	a.taskProgressMu.Lock()
	defer a.taskProgressMu.Unlock()

	if a.taskProgress == nil {
		a.taskProgress = make(map[string]tools.TaskProgressMetadata)
	}
	a.taskProgress[toolID] = meta
}

func (a *Agent) resolveVerificationTarget(args map[string]interface{}) (string, tools.TaskProgressMetadata, error) {
	var requestedToolID, requestedTaskKey string
	if raw, ok := args["tool_id"].(string); ok {
		requestedToolID = strings.TrimSpace(raw)
	}
	if raw, ok := args["task_key"].(string); ok {
		requestedTaskKey = normalizeVerificationLookup(raw)
	}

	pending := a.pendingVerificationTasks()
	if len(pending) == 0 {
		return "", tools.TaskProgressMetadata{}, fmt.Errorf("there is no pending shell task waiting for verification")
	}

	if requestedToolID != "" {
		for _, task := range pending {
			if task.ToolID == requestedToolID {
				return task.ToolID, task.Meta, nil
			}
		}
		return "", tools.TaskProgressMetadata{}, fmt.Errorf("tool_id %q is not waiting for verification", requestedToolID)
	}

	if requestedTaskKey != "" {
		for _, task := range pending {
			if normalizeVerificationLookup(task.Meta.TaskKey) == requestedTaskKey {
				return task.ToolID, task.Meta, nil
			}
			if normalizeVerificationLookup(task.Meta.TaskSummary) == requestedTaskKey {
				return task.ToolID, task.Meta, nil
			}
		}
		return "", tools.TaskProgressMetadata{}, fmt.Errorf("task_key %q did not match any pending shell task", requestedTaskKey)
	}

	if len(pending) == 1 {
		return pending[0].ToolID, pending[0].Meta, nil
	}

	return "", tools.TaskProgressMetadata{}, fmt.Errorf("multiple shell tasks are waiting for verification; provide tool_id or task_key")
}

func (a *Agent) applyVerification(toolID, verifyToolID, summary, evidence string) (tools.TaskProgressMetadata, error) {
	a.taskProgressMu.Lock()
	defer a.taskProgressMu.Unlock()

	if a.taskProgress == nil {
		return tools.TaskProgressMetadata{}, fmt.Errorf("there is no shell task state to verify")
	}

	meta, ok := a.taskProgress[toolID]
	if !ok {
		return tools.TaskProgressMetadata{}, fmt.Errorf("shell task %q was not found", toolID)
	}
	if !meta.PendingVerification() {
		return tools.TaskProgressMetadata{}, fmt.Errorf("shell task %q is not waiting for verification", toolID)
	}

	updated := meta.ApplyVerification(verifyToolID, summary, evidence)
	a.taskProgress[toolID] = updated
	return updated, nil
}

func (a *Agent) pendingVerificationTasks() []pendingVerificationTask {
	a.taskProgressMu.Lock()
	defer a.taskProgressMu.Unlock()

	if len(a.taskProgress) == 0 {
		return nil
	}

	pending := make([]pendingVerificationTask, 0, len(a.taskProgress))
	for toolID, meta := range a.taskProgress {
		if meta.PendingVerification() {
			pending = append(pending, pendingVerificationTask{
				ToolID: toolID,
				Meta:   meta,
			})
		}
	}

	sort.Slice(pending, func(i, j int) bool {
		left := strings.TrimSpace(pending[i].Meta.TaskSummary)
		right := strings.TrimSpace(pending[j].Meta.TaskSummary)
		if left == right {
			return pending[i].ToolID < pending[j].ToolID
		}
		if left == "" {
			return false
		}
		if right == "" {
			return true
		}
		return left < right
	})
	return pending
}

func (a *Agent) pendingVerificationNudge() (llm.Message, bool) {
	pending := a.pendingVerificationTasks()
	if len(pending) == 0 {
		return llm.Message{}, false
	}

	var b strings.Builder
	b.WriteString("[TASK NOT COMPLETE] You still have executed shell tasks waiting for verification.\n")
	b.WriteString("Do not give the user a final completion response yet.\n")
	b.WriteString("Run any needed follow-up checks, then call verify_complete for each pending task.\n")
	b.WriteString("Pending tasks:\n")
	for _, task := range pending {
		summary := strings.TrimSpace(task.Meta.TaskSummary)
		if summary == "" {
			summary = "shell task"
		}
		b.WriteString(fmt.Sprintf("- tool_id=%s", task.ToolID))
		if task.Meta.TaskKey != "" {
			b.WriteString(fmt.Sprintf(", task_key=%s", task.Meta.TaskKey))
		}
		b.WriteString(fmt.Sprintf(", summary=%s", summary))
		if hint := strings.TrimSpace(task.Meta.VerificationHint); hint != "" {
			b.WriteString(fmt.Sprintf(", verification_hint=%s", hint))
		}
		b.WriteString("\n")
	}

	return llm.Message{
		Role:    llm.RoleUser,
		Content: strings.TrimSpace(b.String()),
	}, true
}

func normalizeVerificationLookup(raw string) string {
	return strings.ToLower(strings.TrimSpace(tools.NormalizeTaskSummary(raw)))
}
