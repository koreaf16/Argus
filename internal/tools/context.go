package tools

import (
	"context"

	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/shelljobs"
	"github.com/koreaf16/argus/internal/state"
)

type ExecuteSubQueryFunc func(ctx context.Context, systemPrompt string, userPrompt string) (string, error)

// TraceEmitterFunc forwards aidebug NDJSON traces from a tool to the engine.
// Tools may use it to surface events that don't have a natural ToolEvent
// (e.g. shell channel open/reuse decisions). Nil-safe: callers must check.
type TraceEmitterFunc func(traceType, callID string, data any)

type Context struct {
	Context    context.Context
	State      *state.AppState
	WorkingDir string
	Workspace  *workspace.Manager
	ShellJobs  *shelljobs.Manager
	Registry   *Registry

	ExecuteSubQuery ExecuteSubQueryFunc
	EmitTrace       TraceEmitterFunc
}
