//go:build tools
// +build tools

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/agent"
	"github.com/yourorg/infractl/internal/agent/query"
	"github.com/yourorg/infractl/internal/agent/todo"
	"github.com/yourorg/infractl/internal/config"
	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/connector/oracle"
	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

const fullOracleScenarioPrompt = `서버 oracle의 저장된 Oracle connector와 SSH target을 사용해서 종합 검증을 수행하라.
모든 단계는 지체 없이 즉시 수행한다.

검증 순서:
1. 리스너(Listener) 제어:
   - shell_exec로 'lsnrctl status' 확인 후, 'lsnrctl stop && lsnrctl start'를 즉시 실행하여 리스너를 재기동한다.

2. CDB/PDB 상태 조회:
   - oracle_ai26_ai_db.query 도구(인자명 'sql')를 사용하여 'SELECT name, cdb FROM v$database'와 'SELECT name, open_mode FROM v$pdbs'를 실행한다.
   - CDB와 PDB를 명확히 구분하여 한 줄로 요약한다.

3. 느린 쿼리 진단 (핵심):
   - oracle_ai26_ai_db.trace_path 도구를 사용하여 트레이스 파일 경로를 **한 번에** 얻는다.
   - 트레이스 파일 경로가 확인되면 즉시 shell_exec로 'tkprof <경로> /tmp/verif.txt'를 실행한다.
   - '/tmp/verif.txt'의 내용을 head로 읽어 실행 계획과 성능 지표를 분석하고 튜닝 방안을 한 문장으로 제시한다.

4. DB 생명주기 관리:
   - 즉시 'SHUTDOWN IMMEDIATE'를 수행하고, 이어서 'STARTUP'을 수행한다.
   - 'ALTER PLUGGABLE DATABASE ALL OPEN'을 실행하여 모든 PDB를 기동한다.

5. 최종 보고:
   - 위 모든 과정의 성공 여부를 표 형식으로 간략히 보고한다.`

type printingSink struct {
	logFile *os.File
}

func (s *printingSink) HandleQueryEvent(ev query.QueryEvent) {
	switch e := ev.(type) {
	case query.EventStreamStart:
		s.linef("\n[stream_start] tier=%s model=%s\n", e.Tier, e.Model)
	case query.EventAssistantChunk:
		if e.Thinking {
			s.write("[thinking] " + e.Text)
			return
		}
		s.write(e.Text)
	case query.EventAssistantResponse:
		if strings.TrimSpace(e.Text) != "" {
			s.linef("\n[assistant_response]\n%s\n", strings.TrimSpace(e.Text))
		}
		if len(e.ToolCalls) > 0 {
			s.linef("[assistant_tool_calls] count=%d\n", len(e.ToolCalls))
			for _, tc := range e.ToolCalls {
				s.linef("  - id=%s name=%s args=%s\n", tc.ID, tc.Function.Name, truncate(tc.Function.Arguments, 2000))
			}
		}
	case query.EventToolUseStart:
		s.linef("\n[tool_use_start] id=%s name=%s input=%s\n", e.ID, e.Name, truncate(e.Input, 3000))
	case query.EventToolResult:
		status := "ok"
		if e.IsError {
			status = "error"
		}
		if e.SiblingSkipped {
			status = "skipped"
		}
		s.linef("\n[tool_result] id=%s name=%s status=%s\n%s\n", e.ID, e.Name, status, truncate(e.Output, 12000))
	case query.EventError:
		s.linef("\n[event_error] recoverable=%v err=%v\n", e.Recoverable, e.Err)
	case query.EventTerminal:
		s.linef("\n[terminal] reason=%s err=%v\n", e.Reason, e.Err)
	}
}

func (s *printingSink) write(text string) {
	fmt.Print(text)
	if s.logFile != nil {
		_, _ = s.logFile.WriteString(text)
	}
}

func (s *printingSink) linef(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Print(msg)
	if s.logFile != nil {
		_, _ = s.logFile.WriteString(msg)
	}
}

type consoleHandler struct{}

func (h *consoleHandler) OnThinking(tier string, model string) {
	fmt.Printf("[handler_thinking] tier=%s model=%s\n", tier, model)
}
func (h *consoleHandler) OnThinkingToken(token string) {}
func (h *consoleHandler) OnToken(token string)         { fmt.Print(token) }
func (h *consoleHandler) OnToolStart(toolID string, toolName string, target string, args map[string]any) {
	b, _ := json.Marshal(args)
	fmt.Printf("\n[handler_tool_start] id=%s name=%s target=%s args=%s\n", toolID, toolName, target, truncate(string(b), 3000))
}
func (h *consoleHandler) OnToolOutput(toolID string, line string) {
	fmt.Printf("[handler_tool_output] id=%s %s\n", toolID, line)
}
func (h *consoleHandler) OnToolEnd(toolID string, toolName string, result string, duration time.Duration, success bool, metadataJSON string) {
	fmt.Printf("\n[handler_tool_end] id=%s name=%s success=%v duration=%s\n%s\n", toolID, toolName, success, duration, truncate(result, 8000))
}
func (h *consoleHandler) OnResponse(content string) {
	fmt.Printf("\n[handler_response]\n%s\n", strings.TrimSpace(content))
}
func (h *consoleHandler) OnError(err error) {
	fmt.Printf("\n[handler_error] %v\n", err)
}
func (h *consoleHandler) OnUsageUpdate(inputTokens, outputTokens int, costUSD float64, durationMs int64) {
	fmt.Printf("[handler_usage] input=%d output=%d cost=%.6f duration_ms=%d\n", inputTokens, outputTokens, costUSD, durationMs)
}
func (h *consoleHandler) OnJobComplete(jobID int, description string, success bool) {
	fmt.Printf("[handler_job_complete] job=%d success=%v %s\n", jobID, success, description)
}
func (h *consoleHandler) OnRAGContext(count int) {
	fmt.Printf("[handler_rag_context] count=%d\n", count)
}

type storePrivilegeHandler struct {
	st store.ServerStore
}

func (h storePrivilegeHandler) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	if h.st == nil {
		return privilege.PromptResponse{}, fmt.Errorf("server store is not configured")
	}
	servers, err := h.st.List(ctx)
	if err != nil {
		return privilege.PromptResponse{}, err
	}
	target := strings.TrimSpace(req.Target)
	for _, srv := range servers {
		if srv.AuthType != store.AuthTypePassword || srv.Credential == "" {
			continue
		}
		if !strings.EqualFold(srv.Name, target) && !strings.EqualFold(srv.Host, target) {
			continue
		}
		becomeUser := strings.TrimSpace(req.User)
		if req.Method == privilege.MethodSudo || becomeUser == "" || becomeUser == "root" || strings.EqualFold(becomeUser, srv.User) {
			return privilege.PromptResponse{Password: srv.Credential}, nil
		}
	}
	return privilege.PromptResponse{}, fmt.Errorf("no stored privilege password for target %q", req.Target)
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	dbPath := filepath.Join("bin", ".infractl", "infractl.db")
	serverName := "oracle"
	prompt := fullOracleScenarioPrompt

	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		log.Fatalf("resolve db path: %v", err)
	}
	logPath := filepath.Join("scratch", fmt.Sprintf("oracle_full_verify_%s.log", time.Now().Format("20060102_150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		log.Fatalf("create log: %v", err)
	}
	defer logFile.Close()

	fmt.Printf("[setup] db=%s\n[setup] server=%s\n[setup] log=%s\n", absDB, serverName, logPath)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	generalCfg := cfg.GeneralLLM()

	st, err := store.NewSQLiteStore(ctx, absDB)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	registry := tools.NewRegistry()
	privCache := privilege.NewCache()
	shellTool := &tools.ShellExecTool{
		PrivilegeCache: privCache,
		PromptHandler:  storePrivilegeHandler{st: st},
	}

	todoStore := todo.NewStore()
	todoTracker := todo.NewTracker(todoStore)
	mustRegister(registry,
		shellTool,
		&tools.SessionContextTool{Store: st},
		&tools.SystemInfoTool{},
		&tools.ServerListTool{Store: st},
		&tools.ReadPlaceholderTool{},
		&tools.AskUserQuestionTool{},
		&tools.ProposeActionTool{},
		&todo.WriteTool{Tracker: todoTracker},
		&todo.ReadTool{Store: todoStore},
	)

	localExec := executor.NewLocalExecutor(time.Duration(generalCfg.Timeout) * time.Second)
	execMgr := executor.NewManager(localExec)
	defer execMgr.Close()

	servers, err := st.List(ctx)
	if err != nil {
		log.Fatalf("list servers: %v", err)
	}
	var active *store.Server
	for i := range servers {
		srv := servers[i]
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
		execMgr.Register(srv.Name, sshExec)
		if strings.EqualFold(srv.Name, serverName) {
			cp := srv
			active = &cp
		}
	}
	if active == nil {
		log.Fatalf("server %q not found in %s", serverName, absDB)
	}

	connMgr := connector.NewManager(registry, st)
	connMgr.RegisterFactory("oracle", func() connector.Connector { return oracle.New() })
	if err := connMgr.LoadSaved(ctx); err != nil {
		log.Printf("load saved connectors: %v", err)
	}
	if err := ensureOracleConnectorSession(ctx, connMgr, st, execMgr, *active); err != nil {
		log.Printf("oracle connector enrichment skipped: %v", err)
	}

	llmReg := llm.NewRegistry()
	generalClient := newOpenAIClient(generalCfg)
	llmReg.Register(llm.TierGeneral, generalClient, generalCfg.Model)
	if cfg.Models.Reasoning != nil {
		rc := *cfg.Models.Reasoning
		llmReg.Register(llm.TierReasoning, newOpenAIClient(rc), rc.Model)
	}

	handler := &consoleHandler{}
	sink := &printingSink{logFile: logFile}
	ag := agent.New(generalClient, registry, execMgr, handler, st)
	ag.SetLLMRegistry(llmReg)
	ag.SetModelName(generalCfg.Model)
	ag.SetConnectorManager(connMgr)
	ag.SetQueryEventSink(sink)
	ag.SetActiveServer(*active)
	ag.SetYoroMode(true)
	ag.SetElevatedMode(true)
	shellTool.IsYoloMode = ag.IsYOROMode

	fmt.Printf("\n[prompt]\n%s\n\n", prompt)
	if err := ag.Run(ctx, prompt); err != nil {
		log.Fatalf("agent run: %v", err)
	}

	fmt.Printf("\n[done] oracle full verification scenario completed. log=%s\n", logPath)
}

func mustRegister(reg *tools.Registry, toolList ...tools.Tool) {
	for _, t := range toolList {
		if err := reg.Register(t); err != nil {
			log.Fatalf("register tool %s: %v", t.Name(), err)
		}
	}
}

func newOpenAIClient(cfg config.LLMConfig) *llm.OpenAIClient {
	// 이제 NewOpenAIClient 내부에서 모델명에 따라 자동 활성화됨
	client := llm.NewOpenAIClient(cfg.Endpoint, cfg.Model, cfg.APIKey, time.Duration(cfg.Timeout)*time.Second)
	client.SetTemperature(cfg.Temperature)
	return client
}

func ensureOracleConnectorSession(ctx context.Context, mgr *connector.Manager, st *store.SQLiteStore, execMgr *executor.Manager, srv store.Server) error {
	info, creds, ok := mgr.GetSavedConnector(ctx, srv.Name, "oracle", "AI26", "AI_DB")
	if !ok {
		return nil
	}
	if info.Details == nil {
		info.Details = make(map[string]string)
	}
	info.ServerName = srv.Name
	info.ServiceType = "oracle"
	info.Name = "AI26"
	info.SubInstance = "AI_DB"
	info.Port = 1521
	if info.Details["host"] == "" {
		info.Details["host"] = srv.Host
	}
	if info.Details["cdb"] == "" {
		info.Details["cdb"] = "yes"
	}
	if info.Details["oracle_home"] == "" {
		info.Details["oracle_home"] = discoverOracleHome(ctx, execMgr, srv.Name, info.Name)
	}
	exec, err := execMgr.Get(srv.Name)
	if err != nil {
		return err
	}
	return mgr.Activate(ctx, info, creds, connector.SaveSession, exec)
}

func discoverOracleHome(ctx context.Context, execMgr *executor.Manager, target, sid string) string {
	exec, err := execMgr.Get(target)
	if err != nil {
		return ""
	}
	cmd := fmt.Sprintf("pid=$(pgrep -fo 'ora_pmon_%s' || true); if [ -n \"$pid\" ]; then readlink -f /proc/$pid/exe | sed 's#/bin/oracle$##'; fi", sid)
	res, err := exec.Execute(ctx, cmd)
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

func truncate(s string, max int) string {
	s = strings.TrimRight(s, "\n")
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n[... truncated ...]"
}
