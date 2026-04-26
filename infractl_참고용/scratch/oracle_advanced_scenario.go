//go:build tools
// +build tools

package main

import (
	"context"
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

const advancedOraclePrompt = `서버 oracle의 Oracle connector를 사용하여 다음의 심화 장애 진단 및 분석을 수행하라.

1. 락(Lock) 경합 분석 및 해소:
   - oracle_ai26_ai_db.locks 도구를 사용하여 현재 DB에서 락 대기(Wait)가 발생 중인지 확인한다.
   - 락이 발견되면, 어떤 세션(SID, SERIAL#)이 다른 세션을 블로킹하고 있는지 식별하라.
   - 대기 중인 세션이 실행하려는 SQL이 무엇인지 파악하라.
   - 블로킹 세션을 안전하게 종료(KILL SESSION)하기 위한 SQL 문을 제시하라.

2. Alert Log 심층 분석:
   - oracle_ai26_ai_db.alert_log 도구로 최근 로그 300줄을 읽는다.
   - 로그에서 'ORA-'로 시작하는 에러 코드를 모두 추출한다.
   - 추출된 각 에러에 대해 DBA의 관점에서 '현상-원인-조치방안'을 한국어로 상세히 설명하라.
   - 만약 시스템 파라미터(v$parameter)와 관련된 에러라면 현재 설정값도 확인하여 비교 분석하라.

3. 최종 보고:
   - 락 경합의 주범과 해결책, 그리고 Alert Log 에러 분석 결과를 포함한 '장애 진단 보고서'를 작성하라.`

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	dbPath := filepath.Join("bin", ".infractl", "infractl.db")
	absDB, _ := filepath.Abs(dbPath)
	st, err := store.NewSQLiteStore(ctx, absDB)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	// 1. 락 경합 시뮬레이션 준비
	fmt.Println("[setup] Preparing lock contention simulation...")
	servers, _ := st.List(ctx)
	var srv *store.Server
	for _, s := range servers {
		if strings.EqualFold(s.Name, "oracle") {
			srv = &s
			break
		}
	}
	if srv == nil {
		log.Fatal("server 'oracle' not found")
	}

	execCfg := &sshconn.Config{
		Host: srv.Host, Port: srv.Port, User: srv.User, AuthType: string(srv.AuthType),
	}
	if srv.AuthType == store.AuthTypeKey {
		execCfg.KeyPath = srv.Credential
	} else {
		execCfg.Password = srv.Credential
	}
	sshClient := sshconn.NewClient(execCfg)
	exec := sshconn.NewSSHExecutor(srv.Name, sshClient, srv.OS, srv.WorkspaceDir)

	// 테이블 생성 및 락 발생 (백그라운드 세션 유지용 sqlplus 실행)
	setupSQL := `
	CREATE TABLE INFRA_LOCK_TEST (id NUMBER, val VARCHAR2(10));
	INSERT INTO INFRA_LOCK_TEST VALUES (1, 'OLD');
	COMMIT;
	`
	exec.Execute(ctx, fmt.Sprintf("echo \"%s\" | sqlplus -s / as sysdba", setupSQL))

	// 세션 1: 락 홀더 (커밋 안 함)
	go func() {
		lockCmd := "echo \"UPDATE INFRA_LOCK_TEST SET val = 'LOCK' WHERE id = 1; HOST sleep 300;\" | sqlplus -s / as sysdba"
		_, _ = exec.Execute(context.Background(), lockCmd)
	}()
	time.Sleep(2 * time.Second)

	// 세션 2: 락 웨이터 (대기 발생)
	go func() {
		waitCmd := "echo \"UPDATE INFRA_LOCK_TEST SET val = 'WAIT' WHERE id = 1; COMMIT;\" | sqlplus -s / as sysdba"
		_, _ = exec.Execute(context.Background(), waitCmd)
	}()
	fmt.Println("[setup] Contention created: Blocker and Waiter are now active.")

	// 2. 에이전트 실행
	cfg, _ := config.Load()
	generalCfg := cfg.GeneralLLM()
	
	registry := tools.NewRegistry()
	privCache := privilege.NewCache()
	storeHandler := storePrivilegeHandler{st: st}
	mustRegister(registry,
		&tools.ShellExecTool{PrivilegeCache: privCache, PromptHandler: storeHandler},
		&tools.SessionContextTool{Store: st},
		&tools.SystemInfoTool{},
		&todo.WriteTool{Tracker: todo.NewTracker(todo.NewStore())},
	)

	execMgr := executor.NewManager(executor.NewLocalExecutor(60 * time.Second))
	execMgr.Register(srv.Name, exec)

	connMgr := connector.NewManager(registry, st)
	connMgr.RegisterFactory("oracle", func() connector.Connector { return oracle.New() })
	_ = connMgr.LoadSaved(ctx)
	
	// Oracle 커넥터 활성화
	info, creds, _ := connMgr.GetSavedConnector(ctx, srv.Name, "oracle", "AI26", "AI_DB")
	info.Details["oracle_home"] = "/home/oracle/app/oracle/product/23.0.0"
	_ = connMgr.Activate(ctx, info, creds, connector.SaveSession, exec)

	llmReg := llm.NewRegistry()
	genClient := llm.NewOpenAIClient(generalCfg.Endpoint, generalCfg.Model, generalCfg.APIKey, 180*time.Second)
	llmReg.Register(llm.TierGeneral, genClient, generalCfg.Model)
	if cfg.Models.Reasoning != nil {
		rc := *cfg.Models.Reasoning
		llmReg.Register(llm.TierReasoning, llm.NewOpenAIClient(rc.Endpoint, rc.Model, rc.APIKey, 180*time.Second), rc.Model)
	}

	sink := &printingSink{}
	ag := agent.New(genClient, registry, execMgr, &consoleHandler{}, st)
	ag.SetLLMRegistry(llmReg)
	ag.SetModelName(generalCfg.Model)
	ag.SetConnectorManager(connMgr)
	ag.SetQueryEventSink(sink)
	ag.SetActiveServer(*srv)
	ag.SetYoroMode(true)

	fmt.Printf("\n[prompt]\n%s\n\n", advancedOraclePrompt)
	if err := ag.Run(ctx, advancedOraclePrompt); err != nil {
		log.Fatalf("agent run: %v", err)
	}
}

type printingSink struct{}
func (s *printingSink) HandleQueryEvent(ev query.QueryEvent) {
	switch e := ev.(type) {
	case query.EventAssistantChunk:
		if !e.Thinking { fmt.Print(e.Text) }
	case query.EventToolResult:
		fmt.Printf("\n[Tool Result] %s\n", truncate(e.Output, 500))
	}
}

type consoleHandler struct{}
func (h *consoleHandler) OnThinking(tier, model string) {}
func (h *consoleHandler) OnThinkingToken(t string)      {}
func (h *consoleHandler) OnToken(t string)               {}
func (h *consoleHandler) OnToolStart(id, name, target string, args map[string]any) {
	fmt.Printf("\n[Tool Call] %s on %s\n", name, target)
}
func (h *consoleHandler) OnToolOutput(id, line string) {}
func (h *consoleHandler) OnToolEnd(id, name, result string, d time.Duration, s bool, m string) {}
func (h *consoleHandler) OnResponse(c string)          { fmt.Printf("\n[Response]\n%s\n", c) }
func (h *consoleHandler) OnError(err error)            { fmt.Printf("\n[Error] %v\n", err) }
func (h *consoleHandler) OnUsageUpdate(i, o int, c float64, d int64) {}
func (h *consoleHandler) OnJobComplete(id int, d string, s bool) {}
func (h *consoleHandler) OnRAGContext(c int)           {}

type storePrivilegeHandler struct{ st store.ServerStore }
func (h storePrivilegeHandler) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	servers, _ := h.st.List(ctx)
	for _, srv := range servers {
		if srv.AuthType == store.AuthTypePassword && srv.Credential != "" && (strings.EqualFold(srv.Name, req.Target) || strings.EqualFold(srv.Host, req.Target)) {
			return privilege.PromptResponse{Password: srv.Credential}, nil
		}
	}
	return privilege.PromptResponse{}, fmt.Errorf("no password")
}

func mustRegister(reg *tools.Registry, toolList ...tools.Tool) {
	for _, t := range toolList { _ = reg.Register(t) }
}

func truncate(s string, max int) string {
	if len(s) <= max { return s }
	return s[:max] + "..."
}
