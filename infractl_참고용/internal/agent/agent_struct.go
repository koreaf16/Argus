// Package agent
// File: agent_struct.go
// Description: Agent 구조체 정의와 의존성 주입 setter 메서드
// Responsibility: Agent 타입 선언, 생성자, 모든 setter/getter 메서드 관리

package agent

import (
	"sync"
	"time"

	"github.com/yourorg/infractl/internal/agent/compact"
	"github.com/yourorg/infractl/internal/agent/installpath"
	"github.com/yourorg/infractl/internal/agent/planmode"
	"github.com/yourorg/infractl/internal/agent/query"
	"github.com/yourorg/infractl/internal/agent/taskctx"
	todoagent "github.com/yourorg/infractl/internal/agent/todo"
	"github.com/yourorg/infractl/internal/background"
	"github.com/yourorg/infractl/internal/checkpoint"
	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/cost"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/hooks"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/subagent"
	"github.com/yourorg/infractl/internal/tools"
)

// Agent??LLM, ??ш낄猷?? ExecutorManager???釉뚰??????筌뚯슦肉???????ш낄援θキ???룸Ŧ爾??녿쑟筌?????덈틖??筌먲퐢??
type Agent struct {
	llmClient           llm.Client
	llmRegistry         *llm.Registry
	registry            *tools.Registry
	manager             *executor.Manager
	store               store.ServerStore
	handler             EventHandler
	sessionStore        store.SessionStore
	execLogStore        store.ExecLogStore
	knowledgeStore      store.KnowledgeStore
	history             []llm.Message
	activeServer        *store.Server
	activeServerNotify  func(*store.Server)
	connectorMgr        *connector.Manager
	knowledgeLearner    *KnowledgeLearner
	taskMemoryLearner   *TaskMemoryLearner
	adaptiveLearner     *AdaptiveLearner
	ragManager          *rag.Manager
	costTracker         *cost.Tracker
	bgManager           *background.Manager
	checkpointMgr       *checkpoint.Manager
	hookRunner          *hooks.Runner
	modelName           string
	sessionHookFired    bool
	maxHistory          int
	maxToolLoop         int
	maxContextTokens    int
	currentSessionID    int64
	lastUserPrompt      string
	lastFailedLogID     int64
	lastSystemPromptLen int
	promptCache         *promptCache
	promptInputs        *promptInputCache
	lastPromptProfile   promptProfile
	compactBreaker      *llm.CircuitBreaker
	compactStack        *compact.Stack
	sessionSummary      *SessionSummaryManager
	yoroMode            bool
	elevatedMode        bool
	planState           *planmode.State
	todoStore           *todoagent.Store
	todoEnforcer        *todoagent.Enforcer
	idleHandler         IdleInputHandler
	questionHandler     QuestionHandler
	subagentRunner      *subagent.Runner
	taskProgressMu      sync.Mutex
	taskProgress        map[string]tools.TaskProgressMetadata
	pendingAction       PendingActionTracker
	historyTurnCounter  int
	currentPlan         *PlanState
	pendingProposal     *taskctx.PendingProposal
	taskMgr             *taskctx.Manager
	installPath         *installpath.Pipeline
	elevationTrk        *privilege.Tracker
	credVault           *privilege.Vault
	queryEngine         *query.Engine
	querySink           query.QueryEventSink
	privPromptHandler   privilege.PromptHandler
	pendingInputSource  interface{}

	// Phase F: auto-promotion 임계값 (0이면 비활성)
	promotionThreshold time.Duration

	// Audit 레이어
	auditStore    AuditStore
	auditRunID    string
	auditTitleSet bool
	// Audit 동기화
	auditSyncEngine *store.SyncEngine
	auditSync       *auditSyncCoordinator
}

// New????????ш낄援θキ????獄쏅똻???筌먲퐢??
func New(client llm.Client, registry *tools.Registry, mgr *executor.Manager, handler EventHandler, st store.ServerStore) *Agent {
	stack := compact.NewStack(client, nil)
	reactive := stack.NewReactive(client)
	recovery := compact.NewRecovery(stack.Collapse(), reactive, client)

	engine := query.New(nil)
	engine.SetCompact(stack)
	engine.SetRecovery(recovery)

	ps := planmode.NewState()
	tStore := todoStoreFromRegistry(registry)
	a := &Agent{
		llmClient:        client,
		registry:         registry,
		manager:          mgr,
		store:            st,
		handler:          handler,
		history:          make([]llm.Message, 0),
		maxHistory:       defaultMaxHistory,
		maxToolLoop:      defaultMaxToolLoop,
		maxContextTokens: defaultMaxContextTokens,
		promptCache:      newPromptCache(),
		promptInputs:     newPromptInputCache(),
		compactBreaker:   llm.NewCircuitBreaker(maxConsecutiveCompactFailures, 5*time.Minute),
		compactStack:     stack,
		pendingAction:    newPendingActionTracker(),
		queryEngine:      engine,
		querySink:        query.NoopEventSink{},
		planState:        ps,
		todoStore:        tStore,
		todoEnforcer:     todoagent.NewEnforcer(tStore),
	}
	a.installPath = installpath.New(func() taskctx.TaskKind {
		if a.taskMgr == nil {
			return taskctx.KindOther
		}
		current := a.taskMgr.Current()
		if current == nil {
			return taskctx.KindOther
		}
		return current.Kind
	})
	return a
}

// todoStoreFromRegistry reuses the TodoWrite tool store so enforcement sees the same list.
func todoStoreFromRegistry(registry *tools.Registry) *todoagent.Store {
	if registry != nil {
		if tool, ok := registry.Get(todoagent.WriteToolName); ok {
			if wt, ok := tool.(*todoagent.WriteTool); ok && wt.Tracker != nil {
				if store := wt.Tracker.Store(); store != nil {
					return store
				}
			}
		}
	}
	return todoagent.NewStore()
}

// SetConnectorManager????節뗪콪???癲ル슢?????????낆뒩????筌먲퐢??
func (a *Agent) SetConnectorManager(mgr *connector.Manager) {
	a.connectorMgr = mgr
}

// SetHandler?????濚???嶺뚮ㅎ?볠뤃??? ??낆뒩????筌먲퐢??
func (a *Agent) SetHandler(h EventHandler) {
	a.handler = h
}

// SetIdleInputHandler??癲ル슢?뤸뤃???????덈틖 濚??嶺뚮ㅏ援????怨뺥룂????ш끽維???ш낄援θキ???좊즴?? ?嶺뚮ㅎ?볠뤃??? ??낆뒩????筌먲퐢??
func (a *Agent) SetIdleInputHandler(h IdleInputHandler) {
	a.idleHandler = h
}

// SetQuestionHandler?????湲깅룪 ???ャ뀕??癲ル슣??????쑩?젆??嶺뚮ㅎ?볠뤃??? ??낆뒩????筌먲퐢??
func (a *Agent) SetQuestionHandler(h QuestionHandler) {
	a.questionHandler = h
}

// NewSmartIdleInputHandler????????ш낄援θキ??LLM ???????⑤９苑?嶺? ?????嚥▲꺂痢?SmartIdleInputHandler????獄쏅똻???筌먲퐢??
// LLM??癲ル슢?꾤땟????嶺뚮ㅏ援????怨뺥룂????ш끽維???ш낄援θキ?????筌??????????????????곸죷 ?????? ????명렡.
func (a *Agent) NewSmartIdleInputHandler() *SmartIdleInputHandler {
	h := NewSmartIdleInputHandler(a.llmClient)
	h.SetServerStore(a.store)
	return h
}

// SetSessionStore???嶺뚮ㅎ??????????롢걫???낆뒩????筌먲퐢??
// SetSessionSummary는 프로그레시브 요약 관리자를 주입하고, compact stack 의 mild 전략으로 연결한다.
func (a *Agent) SetSessionSummary(sm *SessionSummaryManager) {
	a.sessionSummary = sm
	if a.compactStack != nil {
		a.compactStack.SetMild(sm)
	}
}

func (a *Agent) SetSessionStore(s store.SessionStore) {
	a.sessionStore = s
}

// SetExecLogStore??????덈틖 ????????????롢걫???낆뒩????筌먲퐢??
func (a *Agent) SetExecLogStore(s store.ExecLogStore) {
	a.execLogStore = s
}

// SetKnowledgeStore injects knowledge store for task memory retrieval.
func (a *Agent) SetKnowledgeStore(s store.KnowledgeStore) {
	a.knowledgeStore = s
}

// SetSessionID????ш끽維???????嶺뚮ㅎ???ID?????源놁젳??筌먲퐢??
func (a *Agent) SetSessionID(id int64) {
	a.currentSessionID = id
}

// SetMaxContextTokens?????爾?????덉쉐 ???ャ뀕????癰귙렢?嚥▲룗?????源놁젳??筌먲퐢??
func (a *Agent) SetMaxContextTokens(n int) {
	a.maxContextTokens = n
}

// SetKnowledgeLearner??Phase 6 ?????????????????れ삀?? ??낆뒩????筌먲퐢??
func (a *Agent) SetKnowledgeLearner(kl *KnowledgeLearner) {
	a.knowledgeLearner = kl
}

// SetTaskMemoryLearner injects task success/failure learner.
func (a *Agent) SetTaskMemoryLearner(tml *TaskMemoryLearner) {
	a.taskMemoryLearner = tml
}

// SetAdaptiveLearner??Phase 6 ???ㅼ굣甕??????? 癲ル슢?????????낆뒩????筌먲퐢??
func (a *Agent) SetAdaptiveLearner(al *AdaptiveLearner) {
	a.adaptiveLearner = al
}

// SetRAGManager??Phase 7 RAG ???????덉쉐???源낇꼧??? ??낆뒩????筌먲퐢??
func (a *Agent) SetRAGManager(rm *rag.Manager) {
	a.ragManager = rm
}

// SetYoroMode??YORO 癲ル슢?꾤땟?????筌????????????源놁젳??筌먲퐢??
func (a *Agent) SetYoroMode(enabled bool) {
	a.yoroMode = enabled
}

// ToggleYoroMode??YORO 癲ル슢?꾤땟????ｏ쭗???????寃뗏??怨뚮뼚??濡ろ뜑?恝彛????ㅺ컼????袁⑸즵????筌먲퐢??
func (a *Agent) ToggleYoroMode() bool {
	a.yoroMode = !a.yoroMode
	return a.yoroMode
}

// SetElevatedMode toggles managed privilege escalation mode independently.
func (a *Agent) SetElevatedMode(enabled bool) {
	a.elevatedMode = enabled
}

// ToggleElevatedMode toggles managed privilege escalation mode.
func (a *Agent) ToggleElevatedMode() bool {
	a.elevatedMode = !a.elevatedMode
	return a.elevatedMode
}

// Tools???濚밸Ŧ援욃ㅇ????ш낄猷??癲ル슢?꾤땟戮⑤뭄???袁⑸즵????筌먲퐢??
func (a *Agent) Tools() []tools.Tool {
	return a.registry.List()
}

// Manager??executor 癲ル슢????????袁⑸즵????筌먲퐢??
func (a *Agent) Manager() *executor.Manager {
	return a.manager
}

// SetActiveServer????ш끽維?????????嶺뚮ㅎ????????ㅼ굣獄?癲ル슣???嶺뚮쮳?년뵲??
func (a *Agent) SetActiveServer(srv store.Server) {
	a.applyActiveServer(&srv)
}

// ClearActiveServer?????湲깅룪 ??筌먦끉裕????굿?????ㅺ컼???棺??짆?쏆춾???れ삀??????怨뚮옖甕걔???筌먲퐢??
func (a *Agent) ClearActiveServer() {
	a.applyActiveServer(nil)
}

// SetActiveServerNotifier registers a callback fired when active server changes.
func (a *Agent) SetActiveServerNotifier(fn func(*store.Server)) {
	a.activeServerNotify = fn
}

// ToggleYOROMode??YORO 癲ル슢?꾤땟????ｏ쭗???????寃뗏??怨뚮뼚????????ㅺ컼????袁⑸즵????筌먲퐢??
// YORO(You Only Run Once) 癲ル슢?꾤땟??? ?嶺뚮Ĳ?됮?????? 癲꾧퀗??????ㅼ뒭???袁⑸즴??繞?????덈틖??筌먲퐢??
// ?袁⑸즲??罹?留왖?癲ル슪???띿물????嶺뚮ㅎ?볢짆?YORO 癲ル슢?꾤땟??????????숆강筌???????깃탾??筌먲퐢??
func (a *Agent) ToggleYOROMode() bool {
	a.yoroMode = !a.yoroMode
	return a.yoroMode
}

// IsYOROMode??YORO 癲ル슢?꾤땟?????筌?????????袁⑸즵????筌먲퐢??
func (a *Agent) IsYOROMode() bool { return a.yoroMode }

// IsElevatedMode reports whether managed privilege escalation mode is active.
func (a *Agent) IsElevatedMode() bool { return a.elevatedMode }

// SetSubagentRunner는 Pre-flight 인텔리전스 수집에 사용할 서브에이전트 러너를 주입한다.
func (a *Agent) SetSubagentRunner(r *subagent.Runner) {
	a.subagentRunner = r
}

// SetQueryEventSink 는 query.Engine 이벤트를 소비할 sink 를 주입한다.
// nil 이면 NoopEventSink 로 대체된다.
func (a *Agent) SetQueryEventSink(s query.QueryEventSink) {
	if s == nil {
		a.querySink = query.NoopEventSink{}
	} else {
		a.querySink = s
	}
}

// SetPrivilegePromptHandler sets the privilege prompt handler.
func (a *Agent) SetPrivilegePromptHandler(h privilege.PromptHandler) {
	a.privPromptHandler = h
}

// SetPendingSource sets the pending input source.
func (a *Agent) SetPendingSource(s interface{}) {
	a.pendingInputSource = s
}
