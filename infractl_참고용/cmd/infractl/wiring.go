// Package main
// File: wiring.go
// Description: 의존성 조립 헬퍼 함수 모음
// Responsibility: 도구 등록, 서버 로드, MCP 연결, 로그 파일 관리

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/agent/todo"
	"github.com/yourorg/infractl/internal/checkpoint"
	"github.com/yourorg/infractl/internal/config"
	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/connector/generic"
	"github.com/yourorg/infractl/internal/connector/mysql"
	"github.com/yourorg/infractl/internal/connector/oracle"
	"github.com/yourorg/infractl/internal/connector/postgresql"
	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/connector/tomcat"
	"github.com/yourorg/infractl/internal/connector/weblogic"
	"github.com/yourorg/infractl/internal/discovery"
	"github.com/yourorg/infractl/internal/executor"
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
	"github.com/yourorg/infractl/internal/web"
)

// defaultTools는 빌트인 도구 목록을 반환한다.
func defaultTools(
	st store.ServerStore,
	ds store.DiscoveryStore,
	ks store.KnowledgeStore,
	ls store.LearnedSystemStore,
	utm *tools.UserToolManager,
	mgr *executor.Manager,
	connMgr *connector.Manager,
	lc llm.Client,
	llmReg *llm.Registry,
	ragMgr *rag.Manager,
	ragStore store.RAGSourceStore,
	externalEmbedder rag.EmbeddingGenerator,
	memSvc *rag.MemoryService,
	cpMgr *checkpoint.Manager,
	hooksMgr *lifecycle.Manager,
	subOrchestrator *subagent.Orchestrator,
	sched *schedule.Scheduler,
) []tools.Tool {
	scanner := discovery.NewScanner()
	fetcher := web.NewFetcher(100, 15*time.Minute)
	todoStore := todo.NewStore()
	todoTracker := todo.NewTracker(todoStore)
	todoWriteTool := &todo.WriteTool{Tracker: todoTracker}
	todoReadTool := &todo.ReadTool{Store: todoStore}

	privCache := privilege.NewCache()
	apprCache := tools.NewApprovalCache()
	// TUI prompter will be injected by the TUI loop if needed, 
	// but here we can define the base tools with cache.
	
	return []tools.Tool{
		// 빌트인 도구
		&tools.ShellExecTool{PrivilegeCache: privCache, ApprovalCache: apprCache},
		&tools.FileTransferTool{},
		&tools.SessionContextTool{Store: st, Manager: mgr},
		&tools.FileReadTool{PrivilegeCache: privCache},
		&tools.FileWriteTool{PrivilegeCache: privCache, ApprovalCache: apprCache, CheckpointManager: cpMgr},
		&tools.FileLineEditTool{PrivilegeCache: privCache, ApprovalCache: apprCache, CheckpointManager: cpMgr},
		&tools.SysctlManageTool{PrivilegeCache: privCache, ApprovalCache: apprCache},
		&tools.ReadPlaceholderTool{},
		&tools.ProcessListTool{},
		&tools.NetworkInfoTool{},
		&tools.SystemInfoTool{},
		&tools.ServiceStatusTool{},
		&tools.LogTailTool{},
		&tools.K8sQueryTool{},
		&tools.DiskUsageTool{},
		&tools.ServerAddTool{Store: st, Manager: mgr},
		&tools.ServerAddTool{Store: st, Manager: mgr, ToolName: "workspace_add"},
		&tools.ServerListTool{Store: st},
		&tools.ServerListTool{Store: st, ToolName: "workspace_list"},
		&tools.ServerRemoveTool{Store: st, Manager: mgr, ConnectorCleanup: connMgr},
		&tools.ServerRemoveTool{Store: st, Manager: mgr, ConnectorCleanup: connMgr, ToolName: "workspace_remove"},
		&discovery.DiscoverServicesTool{Store: ds, Scanner: scanner, ServerStore: st},
		&discovery.DiscoverWebServersTool{Scanner: scanner},
		&connector.OSAuthProbeTool{Manager: connMgr, ServerStore: st, DiscoveryStore: ds},
		&connector.ActivateTool{Manager: connMgr, ServerStore: st, DiscoveryStore: ds, Scanner: &discoveryAutoScanner{scanner: scanner}},
		&connector.LearnedActivateTool{Manager: connMgr, Store: ls},
		// Phase 6: 웹 도구
		&tools.WebSearchTool{Fetcher: fetcher, LLMClient: lc, LLMRegistry: llmReg},
		&tools.WebFetchTool{Fetcher: fetcher, LLMClient: lc, LLMRegistry: llmReg},
		// Phase 6: 지식 베이스 도구
		&tools.KnowledgeSearchTool{Store: ks},
		&tools.KnowledgeAddTool{Store: ks, Memory: memSvc},
		// Phase 6: 적응형 학습 도구
		&tools.SaveLearnedSystemTool{Store: ls, Memory: memSvc},
		// Phase 6: 사용자 정의 도구 생성
		&tools.UserToolCreateTool{Manager: utm},
		// Phase 7: RAG 검색 및 등록 도구
		&tools.RAGSearchTool{Manager: ragMgr},
		&tools.RAGProbeSourceTool{ExecManager: mgr},
		&tools.RAGRegisterTool{Store: ragStore, ExecManager: mgr, Embedder: externalEmbedder},
		&tools.MemorySearchTool{Memory: memSvc},
		// Phase 8: 체크포인트 + 훅 도구
		&checkpoint.ListTool{Manager: cpMgr},
		&checkpoint.RollbackTool{Manager: cpMgr},
		&lifecycle.RegisterTool{Manager: hooksMgr},
		// Phase 8: 서브에이전트 도구
		&subagent.AnalyzeTool{Orchestrator: subOrchestrator},
		&subagent.DelegateAgentTool{Runner: subOrchestrator.Runner()},
		// Phase 8: 스케줄 도구
		&schedule.CreateTool{Scheduler: sched},
		&schedule.ListTool{Scheduler: sched},
		// 활성 서버 포커스 도구
		&tools.ServerFocusTool{Store: st, Manager: mgr},
		&tools.ServerFocusTool{Store: st, Manager: mgr, ToolName: "workspace_focus"},
		// 작업 완료 검증 도구
		&tools.VerifyCompleteTool{},
		// 질의응답 도구
		&tools.AskUserQuestionTool{},
		// 명령 제안 도구 (사용자 확인 흐름)
		&tools.ProposeActionTool{},
		// Tier escalation 도구 (general → reasoning 재실행)
		&tools.EscalateToReasoningTool{},
		// Phase A: 다단계 작업 todo 도구
		todoWriteTool, todoReadTool,
	}
}

// loadSavedServers는 SQLite에서 서버를 읽어 SSHExecutor를 생성하고 ExecutorManager에 등록한다.
func loadSavedServers(ctx context.Context, st store.ServerStore, mgr *executor.Manager) error {
	servers, err := st.List(ctx)
	if err != nil {
		return fmt.Errorf("list servers: %w", err)
	}

	for _, srv := range servers {
		cfg := &sshconn.Config{
			Host:         srv.Host,
			Port:         srv.Port,
			User:         srv.User,
			AuthType:     string(srv.AuthType),
			WorkspaceDir: srv.WorkspaceDir,
		}
		if srv.AuthType == store.AuthTypeKey {
			cfg.KeyPath = srv.Credential
		} else {
			cfg.Password = srv.Credential
		}

		client := sshconn.NewClient(cfg)
		sshExec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
		mgr.Register(srv.Name, sshExec)
		slog.Info("loaded server from store", "name", srv.Name, "host", srv.Host)
	}
	return nil
}

// connectMCPServers는 config의 mcp_servers를 순회하여 연결하고 도구를 등록한다.
func connectMCPServers(ctx context.Context, cfg *config.Config, registry *tools.Registry) []*mcp.Client {
	var clients []*mcp.Client
	for name, mcpCfg := range cfg.MCPServers {
		client := mcp.NewClient(name, mcp.MCPServerConfig{
			Command: mcpCfg.Command,
			Args:    mcpCfg.Args,
			Env:     mcpCfg.Env,
		})
		if err := client.Connect(ctx); err != nil {
			slog.Warn("mcp server connect failed", "name", name, "err", err)
			clients = append(clients, client)
			continue
		}
		for _, t := range client.ToRegistryTools() {
			if err := registry.Register(t); err != nil {
				slog.Warn("register mcp tool", "name", t.Name(), "err", err)
			}
		}
		slog.Info("mcp server connected", "name", name, "tools", len(client.ToRegistryTools()))
		clients = append(clients, client)
	}
	return clients
}

// closeMCPClients는 모든 MCP 클라이언트 연결을 종료한다.
func closeMCPClients(clients []*mcp.Client) {
	for _, c := range clients {
		if err := c.Close(); err != nil {
			slog.Debug("close mcp client", "name", c.Name, "err", err)
		}
	}
}

// openLogFile은 TUI 모드용 로그 파일을 열어 반환한다.
func openLogFile() (*os.File, error) {
	dir, err := config.DefaultConfigDir()
	if err != nil {
		return nil, fmt.Errorf("config dir: %w", err)
	}
	return os.OpenFile(
		filepath.Join(dir, "infractl.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o600,
	)
}

// registerConnectorFactories는 빌트인 커넥터 팩토리를 매니저에 등록한다.
func registerConnectorFactories(mgr *connector.Manager) {
	mgr.RegisterFactory("oracle", func() connector.Connector { return oracle.New() })
	mgr.RegisterFactory("mysql", func() connector.Connector { return mysql.New() })
	mgr.RegisterFactory("postgresql", func() connector.Connector { return postgresql.New() })
	mgr.RegisterFactory("tomcat", func() connector.Connector { return tomcat.New() })
	mgr.RegisterFactory("weblogic", func() connector.Connector { return weblogic.New() })
	mgr.RegisterFactory("generic", func() connector.Connector { return generic.New() })
}

// tuiFocusSelectAdapter는 tui.TUISelectHandler를 tools.FocusSelectFn으로 어댑트한다.
func tuiFocusSelectAdapter(h *tui.TUISelectHandler) func(string, []tools.FocusSelectOption) (int, error) {
	return func(question string, opts []tools.FocusSelectOption) (int, error) {
		tuiOpts := make([]tui.SelectOption, len(opts))
		for i, o := range opts {
			tuiOpts[i] = tui.SelectOption{
				Label:       o.Label,
				Description: o.Description,
				Preview:     o.Preview,
				HideOther:   true,
			}
		}
		result, err := h.RequestSelect(question, tuiOpts)
		if err != nil {
			return -1, err
		}
		return result.Index, nil
	}
}

// discoveryAutoScanner는 discovery.Scanner를 connector.ServiceScanner 인터페이스로 어댑트한다.
// cmd/infractl(composition root)에서만 사용하여 connector → discovery 직접 의존성 방지.
type discoveryAutoScanner struct {
	scanner *discovery.Scanner
}

func (a *discoveryAutoScanner) ScanAndSave(ctx context.Context, exec executor.Executor, serverName string, ds store.DiscoveryStore) error {
	result, err := a.scanner.Scan(ctx, exec)
	if err != nil {
		return fmt.Errorf("scan %s: %w", serverName, err)
	}
	entries := make([]store.DiscoveredServiceEntry, 0, len(result.Services))
	for _, svc := range result.Services {
		entries = append(entries, store.DiscoveredServiceEntry{
			ServerName:  svc.ServerName,
			ServiceType: string(svc.Type),
			ServiceName: svc.Name,
			Port:        svc.Port,
			Details:     svc.Details,
			Confidence:  svc.Confidence,
		})
	}
	return ds.SaveDiscoveredServices(ctx, serverName, entries)
}

// tuiDisambiguateAdapter는 tui.TUISelectHandler를 connector.DisambiguateHandler로 어댑트한다.
// cmd/infractl(composition root)에서만 사용하여 connector → tui 의존성 방지.
type tuiDisambiguateAdapter struct {
	h *tui.TUISelectHandler
}

func (a *tuiDisambiguateAdapter) RequestDisambiguate(question string, opts []connector.DisambiguateOption) (int, error) {
	tuiOpts := make([]tui.SelectOption, len(opts))
	for i, o := range opts {
		tuiOpts[i] = tui.SelectOption{
			Label:       o.Label,
			Description: o.Description,
			Preview:     o.Preview,
			Tag:         o.Tag,
			HideOther:   o.HideOther,
		}
	}
	result, err := a.h.RequestSelect(question, tuiOpts)
	if err != nil {
		return -1, err
	}
	return result.Index, nil
}

// autoSelectServer는 시작 시 자동 활성화할 server를 결정한다.
// server가 1개면 그것을, 여러 개면 defaultName과 일치하는 것을 반환한다.
func autoSelectServer(servers []store.Server, defaultName string) *store.Server {
	if len(servers) == 1 {
		return &servers[0]
	}
	if len(servers) > 1 && strings.TrimSpace(defaultName) != "" {
		for i := range servers {
			if strings.EqualFold(servers[i].Name, strings.TrimSpace(defaultName)) {
				return &servers[i]
			}
		}
	}
	return nil
}
