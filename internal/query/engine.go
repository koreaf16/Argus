package query

import (
	"sync"
	"time"

	ctxpkg "github.com/koreaf16/argus/internal/context"
	"github.com/koreaf16/argus/internal/hooks"
	"github.com/koreaf16/argus/internal/services/llm"
	stools "github.com/koreaf16/argus/internal/services/tools"
	"github.com/koreaf16/argus/internal/state"
	itools "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type Engine struct {
	mu sync.RWMutex

	llm            llm.LLM
	registry       *itools.Registry
	hookRegistry   *stools.HookRegistry
	hookDispatcher *hooks.HookDispatcher
	fileWatcher    *hooks.FileWatcher
	state          *state.AppState
	messages       []llm.Message // legacy: snapshot load/save 호환용으로 유지
	systemFn       func() []llm.SystemBlock

	cfg       Config
	deps      Deps
	stopHooks []StopHook
	budget    *TokenBudget

	// --- context management (graph-based) ---
	graph     *ctxpkg.Graph
	est       *ctxpkg.TokenEstimator
	distiller *ctxpkg.Distiller
	artStore  *ctxpkg.ArtifactStore
	artMF     *ctxpkg.ArtifactManifest

	debugSeq            int
	debugTurn           int
	debugSessionStarted bool
	debugSessionID      string
	hookSessionStartKey string

	permissionRulesCache      []types.PermissionRule
	permissionRulesCachedAt   time.Time
	permissionRulesCacheValid bool
}

const permissionRuleCacheTTL = 2 * time.Second

func NewEngine(client llm.LLM, registry *itools.Registry, appState *state.AppState, systemFn func() []llm.SystemBlock) *Engine {
	if systemFn == nil {
		systemFn = DefaultSystemPrompt
	}
	if registry == nil {
		registry = itools.NewRegistry()
	}
	if appState == nil {
		appState = state.NewAppState()
	}
	mf := ctxpkg.NewArtifactManifest()
	eg := &Engine{
		llm:          client,
		registry:     registry,
		hookRegistry: stools.NewHookRegistry(),
		state:        appState,
		messages:     make([]llm.Message, 0, 32),
		systemFn:     systemFn,
		cfg:          DefaultConfig(),
		budget:       NewTokenBudget(),
		graph:        ctxpkg.NewGraph(),
		est:          ctxpkg.NewTokenEstimator(),
		artMF:        mf,
	}
	return eg
}
