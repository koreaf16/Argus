package query

import (
	"context"
	"encoding/json"

	"github.com/koreaf16/argus/internal/shelljobs"
	"github.com/koreaf16/argus/internal/services/workspace"
)

type ApproveToolFunc func(ctx context.Context, toolName string, input json.RawMessage) (bool, error)

type Deps struct {
	ApproveTool ApprovalGate
	WorkingDir  string
	Workspace   *workspace.Manager
	ShellJobs   *shelljobs.Manager
	AIDebug     AIDebugConfig
	// BaseDir, SessionID — ArtifactStore 초기화에 사용
	BaseDir   string
	SessionID string
}

type ApprovalGate struct {
	Prompt ApproveToolFunc
}
