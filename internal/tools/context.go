package tools

import (
	"context"

	"github.com/koreaf16/argus/internal/shelljobs"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/state"
)

type ExecuteSubQueryFunc func(ctx context.Context, systemPrompt string, userPrompt string) (string, error)

type Context struct {
	Context    context.Context
	State      *state.AppState
	WorkingDir string
	Workspace  *workspace.Manager
	ShellJobs  *shelljobs.Manager

	ExecuteSubQuery ExecuteSubQueryFunc
}
