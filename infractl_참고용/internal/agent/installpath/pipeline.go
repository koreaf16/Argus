// Package installpath
// File: pipeline.go
// Description: Stateful install pipeline wrapper around Decide.
// Responsibility: Track state transitions and enqueue auto-invoked tool calls.

package installpath

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/infractl/internal/agent/taskctx"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/tools"
)

// Pipeline keeps install/migrate runtime state across tool events.
type Pipeline struct {
	mu         sync.Mutex
	state      PipelineState
	taskKindFn func() taskctx.TaskKind
	queue      []QueuedAction
	recent     []string
}

// New builds a pipeline. taskKindFn may be nil.
func New(taskKindFn func() taskctx.TaskKind) *Pipeline {
	return &Pipeline{taskKindFn: taskKindFn}
}

// OnToolEnd ingests one completed tool call.
func (p *Pipeline) OnToolEnd(_ context.Context, tc llm.ToolCall, outcome tools.ToolOutcome) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	kind := p.currentTaskKindLocked()
	p.recordStateFromToolOutcomeLocked(kind, tc.Function.Name, outcome)
	action := Decide(kind, tc.Function.Name, outcome, p.state)
	p.enqueueActionLocked(action)
}

// OnJobComplete ingests one background job completion event.
func (p *Pipeline) OnJobComplete(jobID int, success bool) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.state.LastJobID = jobID
	outcome := tools.ToolOutcome{Success: success}
	action := Decide(p.currentTaskKindLocked(), "job_complete", outcome, p.state)
	p.enqueueActionLocked(action)
	if action.AutoInvoke {
		p.state.PendingVerify = PendingVerify{}
	}
}

// Dequeue returns one queued auto-invoke action.
func (p *Pipeline) Dequeue() (QueuedAction, bool) {
	if p == nil {
		return QueuedAction{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) == 0 {
		return QueuedAction{}, false
	}
	action := p.queue[0]
	p.queue = p.queue[1:]
	return action, true
}

func (p *Pipeline) currentTaskKindLocked() taskctx.TaskKind {
	if p.taskKindFn == nil {
		return taskctx.KindOther
	}
	kind := p.taskKindFn()
	if kind == "" {
		return taskctx.KindOther
	}
	return kind
}

func (p *Pipeline) enqueueActionLocked(next NextAction) {
	if !next.AutoInvoke {
		return
	}
	tool := strings.TrimSpace(next.Tool)
	if tool == "" {
		return
	}

	sig := actionSignature(tool, next.Args)
	for _, recent := range p.recent {
		if recent == sig {
			return
		}
	}

	argsJSON, err := json.Marshal(next.Args)
	if err != nil {
		return
	}
	callID := fmt.Sprintf("auto-%d", time.Now().UnixNano())
	p.queue = append(p.queue, QueuedAction{
		ToolCall: llm.ToolCall{
			ID:   callID,
			Type: "function",
			Function: llm.FunctionCall{
				Name:      tool,
				Arguments: string(argsJSON),
			},
		},
		Reason: next.Reason,
	})

	p.recent = append(p.recent, sig)
	if len(p.recent) > 3 {
		p.recent = p.recent[len(p.recent)-3:]
	}
}

func (p *Pipeline) recordStateFromToolOutcomeLocked(kind taskctx.TaskKind, toolName string, outcome tools.ToolOutcome) {
	p.state.LastShellMeta = strings.TrimSpace(outcome.MetadataJSON)
	if !requiresAutoVerify(kind) {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(toolName), "submit_job") {
		return
	}
	meta, ok := tools.ParseTaskProgressMetadata(outcome.MetadataJSON)
	if !ok || meta.JobID <= 0 {
		return
	}
	p.state.LastJobID = meta.JobID
	taskKey := strings.TrimSpace(meta.TaskKey)
	if taskKey == "" {
		taskKey = fmt.Sprintf("job:%d", meta.JobID)
	}
	p.state.PendingVerify = PendingVerify{
		JobID:   meta.JobID,
		TaskKey: taskKey,
	}
}

func actionSignature(tool string, args map[string]any) string {
	raw, err := json.Marshal(args)
	if err != nil {
		return tool
	}
	return tool + ":" + string(raw)
}
