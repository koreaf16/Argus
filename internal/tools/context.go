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

// ModelCaps describes the capabilities of the currently active LLM model.
// Zero value is safe — all booleans default to false, strings to "".
type ModelCaps struct {
	Thinking    bool   // extended thinking 지원 (예: claude-3-7-sonnet)
	Vision      bool   // 이미지 입력 지원
	WebSearch   bool   // 네이티브 웹 검색 지원 (예: web_search_20250305)
	PromptCache bool   // cache_control 사용 가능 여부
	ContextWin  int    // 전체 컨텍스트 윈도우 크기 (토큰). 0 = 알 수 없음
	Provider    string // "anthropic" | "openai" | "gemini" | …
}

// WorkspaceSummary describes registered workspaces at Context build time.
// Zero value is safe — Count 0 means no workspace manager.
type WorkspaceSummary struct {
	Count       int      // 등록된 워크스페이스 총 수 (서버 + 계정)
	RemoteCount int      // SSH 원격 서버 수
	HasWindows  bool     // Windows OSType 워크스페이스 1개 이상
	HasUnix     bool     // Linux/Unix OSType 원격 워크스페이스 1개 이상
	Aliases     []string // 등록된 별칭 목록 (정렬 순)
}

type Context struct {
	Context    context.Context
	State      *state.AppState
	WorkingDir string
	Workspace  *workspace.Manager
	ShellJobs  *shelljobs.Manager
	Registry   *Registry

	ExecuteSubQuery ExecuteSubQueryFunc
	EmitTrace       TraceEmitterFunc

	// 런타임 신호 — 도구 Description / SystemPromptGuide 동적 분기에 사용.
	// 모두 zero-value 안전 (기존 호출자는 영향 없음).
	Caps       ModelCaps
	Mode       string // "normal" | "plan" | "bypass" | "ask" | …
	MCPEnabled bool   // MCP 도구가 1개 이상 등록돼 있을 때 true
	Workspaces WorkspaceSummary
}

// BuildWorkspaceSummary는 workspace.Manager로부터 WorkspaceSummary를 구성한다.
// mgr가 nil이면 빈 요약을 반환한다.
func BuildWorkspaceSummary(mgr *workspace.Manager) WorkspaceSummary {
	if mgr == nil {
		return WorkspaceSummary{}
	}
	entries := mgr.Registry().List()
	sum := WorkspaceSummary{
		Count:   len(entries),
		Aliases: make([]string, 0, len(entries)),
	}
	for _, e := range entries {
		sum.Aliases = append(sum.Aliases, e.Alias)
		if e.Kind == workspace.ServerKindSSH {
			sum.RemoteCount++
		}
		switch e.OSType {
		case "windows":
			sum.HasWindows = true
		default:
			if e.Kind == workspace.ServerKindSSH {
				sum.HasUnix = true
			}
		}
	}
	return sum
}
