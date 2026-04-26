// Package main
// File: main.go
// Description: infractl CLI 진입점 및 메인 루프 오케스트레이션
// Responsibility: 전역 의존성 주입(composition root) 및 실행 모드 선택(TUI / REPL)

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourorg/infractl/internal/agent"
	"github.com/yourorg/infractl/internal/background"
	"github.com/yourorg/infractl/internal/checkpoint"
	"github.com/yourorg/infractl/internal/config"
	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/cost"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/hooks"
	infrainit "github.com/yourorg/infractl/internal/infrainit"
	"github.com/yourorg/infractl/internal/lifecycle"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/mcp"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/schedule"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/subagent"
	"github.com/yourorg/infractl/internal/tools"
	"github.com/yourorg/infractl/internal/tui"
)

const appVersion = "1.0.0"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args[1:]
	filtered := args[:0]
	for _, a := range args {
		if a != "--tui" {
			filtered = append(filtered, a)
		}
	}

	if len(filtered) == 0 {
		return runTUI()
	}

	switch filtered[0] {
	case "init":
		return runInit()
	case "version", "--version", "-v":
		fmt.Printf("infractl v%s\n", appVersion)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	case "hooks":
		return runHooks(filtered[1:])
	case "plan":
		return runPlan(filtered[1:])
	case "audit":
		return runAudit(filtered[1:])
	case "daemon":
		fmt.Println("daemon 모드는 Phase 7에서 구현 예정입니다")
		return nil
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 서브커맨드: %s\n\n", filtered[0])
		printUsage()
		return nil
	}
}

func runInit() error {
	if err := infrainit.Run(context.Background()); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	return nil
}

// deps??buildDeps?? ?????? ???????????.
type deps struct {
	cfg             *config.Config
	serverStore     store.ServerStore
	discoveryStore  store.DiscoveryStore
	connectorStore  store.ConnectorStore
	sessionStore    store.SessionStore
	execLogStore    store.ExecLogStore
	historyStore    store.HistoryStore
	knowledgeStore  store.KnowledgeStore
	learnedSysStore store.LearnedSystemStore
	userToolStore   store.UserToolStore
	ragSourceStore  store.RAGSourceStore
	costStore       store.CostStore
	checkpointStore store.CheckpointStore
	hookStore       store.HookStore
	scheduleStore   store.ScheduleStore

	execMgr   *executor.Manager
	registry  *tools.Registry
	llmClient llm.Client

	llmRegistry *llm.Registry

	externalEmbedder rag.EmbeddingGenerator
	memoryService    *rag.MemoryService
	ragManager       *rag.Manager

	connectorMgr *connector.Manager
	mcpClients   []*mcp.Client

	costTracker     *cost.Tracker
	bgManager       *background.Manager
	checkpointMgr   *checkpoint.Manager
	hooksMgr        *lifecycle.Manager
	scheduler       *schedule.Scheduler
	subRunner       *subagent.Runner
	auditStore      agent.AuditStore
	auditSyncEngine *store.SyncEngine
}

// buildDeps???⑤벏????뤵?源놁뱽 鈺곌퀡???뺣뼄.
func buildDeps(ctx context.Context) (*deps, error) {
	if !config.Exists() {
		return nil, fmt.Errorf("config file not found; run 'infractl init' first")
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	llm.SetRawLogEnabled(cfg.EnableRawLog)

	configDir, err := config.DefaultConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config dir: %w", err)
	}
	dbPath := filepath.Join(configDir, "infractl.db")
	sqliteStore, err := store.NewSQLiteStore(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open server store: %w", err)
	}

	generalCfg := cfg.GeneralLLM()
	localExec := executor.NewLocalExecutor(time.Duration(generalCfg.Timeout) * time.Second)
	execMgr := executor.NewManager(localExec)

	if err := loadSavedServers(ctx, sqliteStore, execMgr); err != nil {
		slog.Warn("failed to load some saved servers", "err", err)
	}

	// LLM Registry 鈺곌퀡?????怨쀫선癰??????곷섧???源낆쨯
	llmReg := llm.NewRegistry()

	// Qwen 계열 모델 여부 확인 헬퍼
	isQwen := func(model string) bool {
		return strings.Contains(strings.ToLower(model), "qwen")
	}

	generalClient := llm.NewOpenAIClient(
		generalCfg.Endpoint,
		generalCfg.Model,
		generalCfg.APIKey,
		time.Duration(generalCfg.Timeout)*time.Second,
	)
	// Qwen 계열 모델들은 스트리밍 중 tool_calls 필드가 불안정하므로
	// 본문 내 XML(<tool_call>)을 직접 가로채서 파싱하는 특수 모드를 활성화한다.
	if isQwen(generalCfg.Model) {
		generalClient.SetUseInlineToolCalls(true)
	}
	generalClient.SetTemperature(generalCfg.Temperature)
	llmReg.Register(llm.TierGeneral, generalClient, generalCfg.Model)

	if cfg.Models.Reasoning != nil {
		rc := cfg.Models.Reasoning
		reasoningClient := llm.NewOpenAIClient(rc.Endpoint, rc.Model, rc.APIKey, time.Duration(rc.Timeout)*time.Second)
		if isQwen(rc.Model) {
			reasoningClient.SetUseInlineToolCalls(true)
		}
		reasoningClient.SetTemperature(rc.Temperature)
		llmReg.Register(llm.TierReasoning, reasoningClient, rc.Model)
	}
	if cfg.Models.Fast != nil {
		fc := cfg.Models.Fast
		fastClient := llm.NewOpenAIClient(fc.Endpoint, fc.Model, fc.APIKey, time.Duration(fc.Timeout)*time.Second)
		if isQwen(fc.Model) {
			fastClient.SetUseInlineToolCalls(true)
		}
		fastClient.SetTemperature(fc.Temperature)
		llmReg.Register(llm.TierFast, fastClient, fc.Model)
	}

	// general ?????곷섧?紐? llmClient嚥?????(??륁맄?紐낆넎 ??疫꿸퀣???꾨뗀諭?癰궰野?筌ㅼ뮇???
	llmClient := generalClient

	registry := tools.NewRegistry()
	connectorMgr := connector.NewManager(registry, sqliteStore)
	registerConnectorFactories(connectorMgr)

	// Phase 6: ??????袁㏓럡 筌띲끇??? (??쎄쾿?깆????遺얠젂?醫듼봺: ~/.infractl/scripts)
	scriptsDir := filepath.Join(configDir, "scripts")
	userToolMgr := tools.NewUserToolManager(sqliteStore, registry, scriptsDir)
	bgMgr := background.NewManager(cfg.Background.StorageDir, cfg.Background.MaxFileSize)

	// Phase 7: ??? CPU 筌롫뗀?덄뵳?+ ?紐? RAG 野꺜???┛ ??밴쉐
	externalEmbedder := rag.NewEmbeddingGenerator(cfg.Embedding)
	memoryService := rag.NewMemoryService(sqliteStore, sqliteStore, sqliteStore, sqliteStore, externalEmbedder, bgMgr)
	externalSearcher := rag.NewExternalSearcher(execMgr)
	reranker := rag.NewReranker(cfg.Reranker)
	ragMgr := rag.NewManager(memoryService, externalSearcher, sqliteStore, sqliteStore, sqliteStore, externalEmbedder, reranker)

	// Phase 8: 筌ｋ똾寃?????+ ??+ ??뺥닏?癒?뵠?袁る뱜 + ???餓?筌띲끇??? ??밴쉐 (defaultTools??雅뚯눘???袁⑹뒄)
	cpMgr := checkpoint.NewManager(sqliteStore)
	hooksMgr := lifecycle.NewManager(sqliteStore)
	subRunner := subagent.NewRunner(llmClient, registry, execMgr, nil) // costTracker???袁⑸퓠 雅뚯눘??
	subRunner.SetLLMRegistry(llmReg)                                   // fast ?怨쀫선 ?怨쀪퐨 ????
	subOrchestrator := subagent.NewOrchestrator(subRunner)
	subOrchestrator.SetMaxParallel(cfg.Subagent.MaxParallel) // Phase F: errgroup 동시성 제한
	scheduleLogger := schedule.NewLogger(cfg.Schedule.Log.Path, cfg.Schedule.Log.MaxSize)
	// Phase F.8 완료 전 placeholder — daemon AgentRunner 연결 후 교체
	scheduleRunner := schedule.AgentRunner(func(ctx context.Context, prompt string) (string, error) {
		return "(스케줄 실행: daemon AgentRunner 미연결 — Phase F.8 완료 후 교체)", nil
	})
	scheduler := schedule.NewScheduler(sqliteStore, scheduleRunner)
	scheduler.SetLogger(scheduleLogger)
	oneshotMgr := schedule.NewOneshotManager(sqliteStore, scheduleRunner, scheduleLogger)
	scheduler.SetOneshotManager(oneshotMgr)

	for _, t := range defaultTools(sqliteStore, sqliteStore, sqliteStore, sqliteStore, userToolMgr, execMgr, connectorMgr, llmClient, llmReg, ragMgr, sqliteStore, externalEmbedder, memoryService, cpMgr, hooksMgr, subOrchestrator, scheduler) {
		if err := registry.Register(t); err != nil {
			return nil, fmt.Errorf("register tool: %w", err)
		}
	}

	// Phase 6: ???貫留???????袁㏓럡 嚥≪뮆諭?(?????쎈뱜?귐딅퓠 ??슦????袁㏓럡揶쎛 ?源낆쨯????
	if err := userToolMgr.LoadAll(ctx); err != nil {
		slog.Warn("load user tools", "err", err)
	}

	// ?怨대럡 ???貫留??뚣끇苑??嚥≪뮆諭?
	if err := connectorMgr.LoadSaved(ctx); err != nil {
		slog.Warn("load saved connectors", "err", err)
	}

	// MCP ??뺤쒔 ?怨뚭퍙
	mcpClients := connectMCPServers(ctx, cfg, registry)

	// Phase 8: ??쑴???곕뗄?삥묾???밴쉐
	pricing := make(map[string]cost.ModelPricing)
	for model, p := range cfg.CostPerMTokens {
		pricing[model] = cost.ModelPricing{Input: p.Input, Output: p.Output}
	}
	costTracker := cost.NewTracker(sqliteStore, generalCfg.Model, pricing)

	// Phase 8: 獄쏄퉫???깆뒲???臾믩씜 筌띲끇??? ??밴쉐

	// Phase F: BackgroundManageTool + OutputTool 등록
	if err := registry.Register(&tools.BackgroundManageTool{Manager: bgMgr}); err != nil {
		slog.Warn("register background_jobs tool", "err", err)
	}
	if err := registry.Register(&tools.OutputTool{
		Manager:    bgMgr,
		MaxPerCall: cfg.Monitor.MaxPerCall,
		MaxCumul:   cfg.Monitor.MaxCumulative,
	}); err != nil {
		slog.Warn("register background_output tool", "err", err)
	}
	if err := registry.Register(&tools.MonitorTool{
		Manager:    bgMgr,
		MaxPerCall: cfg.Monitor.MaxPerCall,
		MaxCumul:   cfg.Monitor.MaxCumulative,
	}); err != nil {
		slog.Warn("register monitor tool", "err", err)
	}

	// Phase 8: ??뺥닏?癒?뵠?袁る뱜 ??쑴???곕뗄?삥묾?雅뚯눘??(costTracker ??밴쉐 ??
	subRunner.SetCostTracker(costTracker)

	// Phase 8: on_connect ???怨뚭퍙
	connectorMgr.SetConnectHook(func(ctx context.Context, server, serviceType string) {
		hooksMgr.Fire(ctx, lifecycle.HookContext{
			Event:       lifecycle.EventOnConnect,
			Server:      server,
			ServiceType: serviceType,
		})
	})

	return &deps{
		cfg:              cfg,
		serverStore:      sqliteStore,
		discoveryStore:   sqliteStore,
		connectorStore:   sqliteStore,
		sessionStore:     sqliteStore,
		execLogStore:     sqliteStore,
		historyStore:     sqliteStore,
		knowledgeStore:   sqliteStore,
		learnedSysStore:  sqliteStore,
		userToolStore:    sqliteStore,
		ragSourceStore:   sqliteStore,
		costStore:        sqliteStore,
		checkpointStore:  sqliteStore,
		hookStore:        sqliteStore,
		execMgr:          execMgr,
		registry:         registry,
		llmClient:        llmClient,
		llmRegistry:      llmReg,
		externalEmbedder: externalEmbedder,
		memoryService:    memoryService,
		ragManager:       ragMgr,
		connectorMgr:     connectorMgr,
		mcpClients:       mcpClients,
		costTracker:      costTracker,
		bgManager:        bgMgr,
		checkpointMgr:    cpMgr,
		hooksMgr:         hooksMgr,
		scheduleStore:    sqliteStore,
		scheduler:        scheduler,
		subRunner:        subRunner,
		auditStore:       sqliteStore,
		auditSyncEngine:  store.NewSyncEngine(sqliteStore),
	}, nil
}

func runTUI() error {
	ctx := context.Background()

	// TUI 筌뤴뫀諭?癒?퐣??slog??嚥≪뮄?????뵬嚥??귐됰뼄????紐낅립??
	// stderr嚥?筌욊낯???怨뺛늺 bubbletea ?遺얇늺??bleeding??獄쏆뮇源??뺣뼄.
	if logFile, err := openLogFile(); err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}

	d, err := buildDeps(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer d.serverStore.Close()
	defer d.execMgr.Close()
	defer closeMCPClients(d.mcpClients)

	handler := tui.NewTUIHandler()
	ag := agent.New(d.llmClient, d.registry, d.execMgr, handler, d.serverStore)
	ag.SetConnectorManager(d.connectorMgr)
	ag.SetSessionStore(d.sessionStore)
	ag.SetExecLogStore(d.execLogStore)
	ag.SetKnowledgeStore(d.knowledgeStore)
	// Phase 6: ?癒???덈뮸 ?뚮똾猷??곕뱜 雅뚯눘??
	ag.SetKnowledgeLearner(agent.NewKnowledgeLearner(d.knowledgeStore, d.execLogStore, d.llmClient, d.memoryService))
	ag.SetTaskMemoryLearner(agent.NewTaskMemoryLearner(d.knowledgeStore, d.execLogStore, d.memoryService))
	ag.SetAdaptiveLearner(agent.NewAdaptiveLearner(d.learnedSysStore))
	// Phase 7: RAG 筌띲끇??? 雅뚯눘??
	ag.SetRAGManager(d.ragManager)
	// 筌렺??LLM ?????쎈뱜??+ ??깆뒭??雅뚯눘??
	ag.SetLLMRegistry(d.llmRegistry)
	ag.SetBackgroundManager(d.bgManager)
	ag.SetPromotionThreshold(d.cfg.Background.PromotionDuration())
	ag.SetCostTracker(d.costTracker)
	ag.SetModelName(d.cfg.GeneralLLM().Model)
	ag.SetCheckpointManager(d.checkpointMgr)
	ag.SetAuditStore(d.auditStore)
	ag.SetAuditSyncEngine(d.auditSyncEngine)
	// hooks.yaml 스냅샷 캡처 + hookRunner 주입 (Phase D: 최초 실행 시 default yaml + builtins 부트스트랩)
	if cfgDir, err := config.DefaultConfigDir(); err == nil {
		if bsErr := infrainit.BootstrapHooks(cfgDir); bsErr != nil {
			slog.Warn("hooks bootstrap failed", "err", bsErr)
		}
		hooksYAML := filepath.Join(cfgDir, "hooks.yaml")
		if snapErr := hooks.CaptureSnapshot(hooksYAML); snapErr != nil {
			slog.Warn("hooks.yaml load failed", "err", snapErr)
		}
		// Phase G: hooks.yaml 핫리로드 감시 (ctx 취소 시 goroutine 정리)
		go func() {
			if err := hooks.Watch(ctx, hooksYAML); err != nil {
				slog.Warn("hooks.yaml watcher exited", "err", err)
			}
		}()
	}
	hookRunner := hooks.NewRunner(d.llmRegistry)
	hookRunner.SetAgentRunner(subagent.NewHookAgentRunner(d.subRunner))
	ag.SetHookRunner(hookRunner)
	ag.SetSubagentRunner(d.subRunner)
	// 프로그레시브 컨텍스트 요약 관리자 주입 (maxHistory/5 라운드 기준으로 동적 임계값 설정)
	sessionSummaryMgr := agent.NewSessionSummaryManager(d.llmClient)
	sessionSummaryMgr.SetThreshold(50 / 5) // defaultMaxHistory=50, avgMsgsPerRound≈5
	ag.SetSessionSummary(sessionSummaryMgr)
	d.memoryService.SubmitBackfill(ctx)
	if ct, ok := d.registry.Get("session_context"); ok {
		if tool, ok2 := ct.(*tools.SessionContextTool); ok2 {
			tool.ActiveServer = ag.ActiveServerSnapshot
		}
	}
	for _, name := range []string{"server_remove", "workspace_remove"} {
		if rt, ok := d.registry.Get(name); ok {
			if tool, ok2 := rt.(*tools.ServerRemoveTool); ok2 {
				tool.ActiveServer = ag.ActiveServerSnapshot
				tool.OnActiveServerClear = ag.ClearActiveServer
			}
		}
	}

	slog.Debug("checkpoint: before NewSession")
	if d.sessionStore != nil {
		if id, err := ag.NewSession(ctx); err == nil && id > 0 {
			slog.Debug("tui session created", "id", id)
		}
	}
	slog.Debug("checkpoint: after NewSession")

	// 서버 자동 활성화: 1개면 무조건, 여러 개면 config.DefaultServer 참조
	if d.serverStore != nil {
		if servers, err := d.serverStore.List(ctx); err == nil {
			if autoSrv := autoSelectServer(servers, d.cfg.DefaultServer); autoSrv != nil {
				ag.SetActiveServer(*autoSrv)
				slog.Info("auto-activated server", "name", autoSrv.Name)
			}
		}
	}

	// ?紐껋뵬??筌뤴뫀諭? ?뚣끉苑???곌때 writer嚥?IME 鈺곌퀬鍮 ?袁⑺뒄 ?⑥쥙??
	parker := tui.NewCursorParkWriter(os.Stdout)
	// program 筌〓챷???뚢뫂???瑗???tea.NewProgram() ??곸읈??AppModel??雅뚯눘??
	box := tui.NewProgramBox()

	// /server 클래시 명령 핸들러 p 생성 후 SetProgram으로 초기화
	slashSelectHandler := &tui.TUISelectHandler{}
	formHandler := &tui.TUIFormHandler{}
	privCache := privilege.NewCache()
	app := tui.NewAppWithOptions(ag, d.cfg, d.serverStore, d.execMgr, tui.AppOptions{
		InitialSessionID: ag.CurrentSessionID(),
		HistoryStore:     d.historyStore,
		ConnectorMgr:     d.connectorMgr,
		MCPClients:       d.mcpClients,
		SessionStore:     d.sessionStore,
		ExecLogStore:     d.execLogStore,
		CursorParker:     parker,
		ProgramBox:       box,
		SelectHandler:    slashSelectHandler,
		FormHandler:      formHandler,
		KnowledgeStore:   d.knowledgeStore,
		RAGSourceStore:   d.ragSourceStore,
		DiscoveryStore:   d.discoveryStore,
		CostTracker:      d.costTracker,
		CheckpointMgr:    d.checkpointMgr,
		HooksMgr:         d.hooksMgr,
		ScheduleMgr:      d.scheduler,
		PrivCache:        privCache,
		AuditStore:       d.auditStore,
	})
	// WithAltScreen() ??볤탢 ???紐껋뵬??筌뤴뫀諭? tea.WithOutput??곗쨮 ?뚣끉苑???곌때 writer 雅뚯눘??
	p := tea.NewProgram(app, tea.WithOutput(parker))
	box.Set(p) // p.Run() ?袁⑸퓠 ??쇱젟 ??Send()?? ?????怨뺣굡????곸벉
	handler.SetProgram(p)
	// query.Engine 이벤트(스트리밍 토큰/도구 결과/종료)를 TUI 메시지로 변환해 전달
	querySink := tui.NewTUIQueryEventSink()
	querySink.SetProgram(p)
	ag.SetQueryEventSink(querySink)
	d.bgManager.SetNotifyFunc(ag.ChainBackgroundNotify(handler.OnJobComplete))
	d.bgManager.SetWatchdog(background.WatchdogNotifierFunc(func(ev background.NoticeEvent) {
		if ev.Kind != "stall" {
			return
		}
		p.Send(tui.StallNoticeMsg{JobID: ev.JobID, Tail: ev.Tail})
	}))
	ag.SetActiveServerNotifier(func(srv *store.Server) {
		p.Send(tui.ActiveServerMsg{Server: srv})
	})
	// shell_exec OutputCb??tool_exec.go?먯꽌 ?ㅽ뻾 ?쒕쭏??toolID? ?④퍡 二쇱엯??
	// connector ?꾧뎄???대쫫 異⑸룎 disambiguation UI ?곌껐
	selectHandler := tui.NewTUISelectHandler(p)
	slashSelectHandler.SetProgram(p) // 직접 초기화, p 생성 후 /server 슬래시 핸들러에 p 주입
	formHandler.SetProgram(p)
	disambig := &tuiDisambiguateAdapter{h: selectHandler}
	if at, ok := d.registry.Get("connector_activate"); ok {
		if tool, ok2 := at.(*connector.ActivateTool); ok2 {
			tool.Disambiguate = disambig
		}
	}
	if oat, ok := d.registry.Get("connector_probe_os_auth"); ok {
		if tool, ok2 := oat.(*connector.OSAuthProbeTool); ok2 {
			tool.Disambiguate = disambig
		}
	}
	// server_focus ?꾧뎄???쒖꽦 ?쒕쾭 肄쒕갚 + ?좏깮 UI ?곌껐
	for _, name := range []string{"server_focus", "workspace_focus"} {
		if ft, ok := d.registry.Get(name); ok {
			if tool, ok2 := ft.(*tools.ServerFocusTool); ok2 {
				tool.OnChange = func(srv *store.Server) {
					if srv == nil {
						ag.ClearActiveServer()
					} else {
						ag.SetActiveServer(*srv)
					}
				}
				tool.SelectFn = tuiFocusSelectAdapter(selectHandler)
			}
		}
	}
	// subagent_analyze ?꾧뎄???ㅼ떆媛??대깽??肄쒕갚 ?곌껐
	if at, ok := d.registry.Get("subagent_analyze"); ok {
		if analyzeTool, ok2 := at.(*subagent.AnalyzeTool); ok2 {
			analyzeTool.EventCb = handler.SubagentEventCallback()
		}
	}
	// delegate_task 도구 실시간 이벤트 콜백 연결 (서브에이전트 내부 도구 진행 상황을 TUI에 표시)
	if dt, ok := d.registry.Get("delegate_task"); ok {
		if delegateTool, ok2 := dt.(*subagent.DelegateAgentTool); ok2 {
			delegateTool.EventCb = handler.SubagentEventCallback()
		}
	}
	// Phase 5: TUI 확인 핸들러 대신 QuestionHandler 사용 (이미 SetQuestionHandler로 등록됨)
	ag.SetIdleInputHandler(ag.NewSmartIdleInputHandler())
	// 공유 privilege cache: ShellExecTool과 /ospermission이 같은 인스턴스를 참조한다.
	if sh, ok := d.registry.Get("shell_exec"); ok {
		if tool, ok2 := sh.(*tools.ShellExecTool); ok2 {
			tool.PrivilegeCache = privCache
			tool.PromptHandler = newStorePrivilegeHandler(d.serverStore, tui.NewPrivilegePromptHandler(p))
			tool.IsYoloMode = ag.IsYOROMode
			tool.PrivilegeAudit = ag.NewPrivilegeAuditHook()
		}
	}
	if aq, ok := d.registry.Get("ask_user_question"); ok {
		if tool, ok2 := aq.(*tools.AskUserQuestionTool); ok2 {
			tool.IsYoroMode = ag.IsYOROMode
		}
	}

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui run: %w", err)
	}
	return nil
}

func printUsage() {
	fmt.Printf("infractl v%s - AI infrastructure management CLI\n\n", appVersion)
	fmt.Println("Usage:")
	fmt.Println("  infractl           Run TUI mode")
	fmt.Println("  infractl init      Initialize configuration")
	fmt.Println("  infractl version   Show version")
	fmt.Println("  infractl help      Show help")
	fmt.Println("  infractl hooks     Hook 관리 (list/test/validate/reload)")
	fmt.Println("  infractl plan      Plan Mode 관리 (enter/exit/status)")
	fmt.Println("  infractl daemon    Run daemon mode (Phase 7)")
}
