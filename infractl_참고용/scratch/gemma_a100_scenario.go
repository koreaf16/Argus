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
	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
	"github.com/yourorg/infractl/internal/web"
)

const defaultGemmaScenarioPrompt = `Treat this as if the user asked:
"Connect to the a100 server in our system and find the Gemma/Gemma4 model that runs best on our system."

Validation goals:
- Verify that you inspect the real a100 server, Kubernetes state, and currently running model services with tools.
- Verify that you use web_search for current or uncertain model information, and web_fetch when a specific result needs deeper reading.
- Verify that you organize findings into a comparison table and include source links in the final answer.
- If "Gemma 4" is not clearly an official/current model name, do not guess. State the uncertainty using official or high-confidence sources.
- Do not download model weights. To validate download behavior, create a small Markdown research report on the remote a100 workspace and download only that report with file_transfer.

Required steps:
1. Use session_context or server_list to confirm the a100 target/context.
2. On a100, inspect GPU, memory, disk, and OS information. Use nvidia-smi and kubectl state if available.
3. Use k8s_query or shell_exec to read the dbs namespace services, deployments, and pods related to gemma, vllm, and llama.
4. Use web_search for current Gemma-family model information, vLLM support, and A100 serving/quantization guidance. Use the queries array for independent searches.
5. Use web_fetch on 1 to 3 important URLs from search results to strengthen evidence.
6. Combine local observations and web evidence into a candidate model/serving comparison table.
7. Create a small Markdown report in the remote a100 workspace named gemma-a100-research-<timestamp>.md.
8. Use file_transfer action=download to download that report into the local scratch\a100_gemma_downloads directory.
9. Final answer must include pass/fail summary, tools actually used, downloaded local file path, improvement opportunities, and a Sources section.

Safety rules:
- Do not delete, restart, scale, patch, or modify existing resources related to vllm, llama, gemma, or qwen.
- Do not download model weights, container images, or large files.
- Do not print secrets.
- If web_search fails, state that clearly and do not answer current facts from training memory.`

type printingSink struct {
	logFile *os.File
}

func (s *printingSink) HandleQueryEvent(ev query.QueryEvent) {
	switch e := ev.(type) {
	case query.EventStreamStart:
		s.linef("\n[stream_start] tier=%s model=%s\n", e.Tier, e.Model)
	case query.EventAssistantChunk:
		if !e.Thinking {
			s.write(e.Text)
		}
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
	if metadataJSON != "" {
		fmt.Printf("[handler_tool_metadata] %s\n", truncate(metadataJSON, 1000))
	}
}
func (h *consoleHandler) OnToolCancelled(toolID string, toolName string, result string, duration time.Duration) {
	fmt.Printf("\n[handler_tool_cancelled] id=%s name=%s duration=%s\n%s\n", toolID, toolName, duration, truncate(result, 8000))
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
		return privilege.PromptResponse{Password: srv.Credential}, nil
	}
	return privilege.PromptResponse{}, fmt.Errorf("no stored privilege password for target %q", req.Target)
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	dbPath := filepath.Join("bin", ".infractl", "infractl.db")
	serverName := "a100"
	prompt := defaultGemmaScenarioPrompt
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		dbPath = os.Args[1]
	}
	if len(os.Args) > 2 && strings.TrimSpace(os.Args[2]) != "" {
		serverName = os.Args[2]
	}
	if len(os.Args) > 3 && strings.TrimSpace(os.Args[3]) != "" {
		prompt = os.Args[3]
	}

	downloadDir := filepath.Join("scratch", "a100_gemma_downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		log.Fatalf("create download dir: %v", err)
	}
	absDownloadDir, err := filepath.Abs(downloadDir)
	if err != nil {
		log.Fatalf("resolve download dir: %v", err)
	}
	prompt += "\n\nLocal download directory: " + absDownloadDir

	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		log.Fatalf("resolve db path: %v", err)
	}
	logPath := filepath.Join("scratch", fmt.Sprintf("gemma_a100_scenario_%s.log", time.Now().Format("20060102_150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		log.Fatalf("create log: %v", err)
	}
	defer logFile.Close()

	fmt.Printf("[setup] db=%s\n[setup] server=%s\n[setup] log=%s\n[setup] download_dir=%s\n", absDB, serverName, logPath, absDownloadDir)

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

	localExec := executor.NewLocalExecutor(time.Duration(generalCfg.Timeout) * time.Second)
	execMgr := executor.NewManager(localExec)
	defer execMgr.Close()

	registry := tools.NewRegistry()
	privCache := privilege.NewCache()
	shellTool := &tools.ShellExecTool{
		PrivilegeCache: privCache,
		PromptHandler:  storePrivilegeHandler{st: st},
	}

	todoStore := todo.NewStore()
	todoTracker := todo.NewTracker(todoStore)
	fetcher := web.NewFetcher(100, 15*time.Minute)
	llmReg := llm.NewRegistry()
	generalClient := newOpenAIClient(generalCfg)
	llmReg.Register(llm.TierGeneral, generalClient, generalCfg.Model)
	if cfg.Models.Reasoning != nil {
		rc := *cfg.Models.Reasoning
		llmReg.Register(llm.TierReasoning, newOpenAIClient(rc), rc.Model)
	}
	if cfg.Models.Fast != nil {
		fc := *cfg.Models.Fast
		llmReg.Register(llm.TierFast, newOpenAIClient(fc), fc.Model)
	}

	mustRegister(registry,
		shellTool,
		&tools.FileTransferTool{},
		&tools.K8sQueryTool{},
		&tools.SessionContextTool{Store: st, Manager: execMgr},
		&tools.SystemInfoTool{},
		&tools.ServerListTool{Store: st},
		&tools.ServerFocusTool{Store: st, Manager: execMgr, ToolName: "workspace_focus"},
		&tools.ServerFocusTool{Store: st, Manager: execMgr, ToolName: "server_focus"},
		&tools.WebSearchTool{Fetcher: fetcher, LLMClient: generalClient, LLMRegistry: llmReg},
		&tools.WebFetchTool{Fetcher: fetcher, LLMClient: generalClient, LLMRegistry: llmReg},
		&tools.ReadPlaceholderTool{},
		&tools.AskUserQuestionTool{},
		&tools.ProposeActionTool{},
		&todo.WriteTool{Tracker: todoTracker},
		&todo.ReadTool{Store: todoStore},
	)

	active, err := loadServers(ctx, st, execMgr, serverName)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("[setup] registered tools:")
	for _, t := range registry.List() {
		fmt.Printf("  - %s readonly=%v enabled=%v\n", t.Name(), t.IsReadOnly(), t.IsEnabled())
	}

	handler := &consoleHandler{}
	sink := &printingSink{logFile: logFile}
	ag := agent.New(generalClient, registry, execMgr, handler, st)
	ag.SetLLMRegistry(llmReg)
	ag.SetModelName(generalCfg.Model)
	ag.SetQueryEventSink(sink)
	ag.SetActiveServer(*active)
	ag.SetYoroMode(true)
	ag.SetElevatedMode(true)
	shellTool.IsYoloMode = ag.IsYOROMode

	fmt.Printf("\n[prompt]\n%s\n\n", prompt)
	if err := ag.Run(ctx, prompt); err != nil {
		log.Fatalf("agent run: %v", err)
	}

	fmt.Printf("\n[done] gemma A100 scenario completed. log=%s download_dir=%s\n", logPath, absDownloadDir)
}

func mustRegister(reg *tools.Registry, toolList ...tools.Tool) {
	for _, t := range toolList {
		if err := reg.Register(t); err != nil {
			log.Fatalf("register tool %s: %v", t.Name(), err)
		}
	}
}

func loadServers(ctx context.Context, st store.ServerStore, mgr *executor.Manager, serverName string) (*store.Server, error) {
	servers, err := st.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
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
		mgr.Register(srv.Name, sshExec)
		if strings.EqualFold(srv.Name, serverName) {
			cp := srv
			active = &cp
		}
	}
	if active == nil {
		return nil, fmt.Errorf("server %q not found", serverName)
	}
	return active, nil
}

func newOpenAIClient(cfg config.LLMConfig) *llm.OpenAIClient {
	client := llm.NewOpenAIClient(cfg.Endpoint, cfg.Model, cfg.APIKey, time.Duration(cfg.Timeout)*time.Second)
	if strings.Contains(strings.ToLower(cfg.Model), "qwen") {
		client.SetUseInlineToolCalls(true)
	}
	client.SetTemperature(cfg.Temperature)
	return client
}

func truncate(s string, max int) string {
	s = strings.TrimRight(s, "\n")
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n[... truncated ...]"
}
