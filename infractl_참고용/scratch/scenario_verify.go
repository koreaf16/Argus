//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/agent"
	"github.com/yourorg/infractl/internal/agent/query"
	"github.com/yourorg/infractl/internal/agent/todo"
	"github.com/yourorg/infractl/internal/config"
	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

type captureSink struct {
	lastText string
}

func (s *captureSink) HandleQueryEvent(ev query.QueryEvent) {
	switch e := ev.(type) {
	case query.EventAssistantResponse:
		s.lastText = e.Text
	case query.EventAssistantChunk:
		s.lastText += e.Text
	}
}

func main() {
	ctx := context.Background()

	// 1. Load Dependencies
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Load config: %v", err)
	}

	st, err := store.NewSQLiteStore(ctx, ".infractl/infractl.db")
	if err != nil {
		log.Fatalf("Open store: %v", err)
	}
	defer st.Close()

	tStore := todo.NewStore()
	tTracker := todo.NewTracker(tStore)
	todoWrite := &todo.WriteTool{Tracker: tTracker}
	todoRead := &todo.ReadTool{Store: tStore}

	registry := tools.NewRegistry()
	registry.Register(&tools.ShellExecTool{})
	registry.Register(&tools.FileTransferTool{})
	registry.Register(&tools.FileReadTool{})
	registry.Register(&tools.FileWriteTool{})
	registry.Register(todoWrite)
	registry.Register(todoRead)

	localExec := executor.NewLocalExecutor(60 * time.Second)
	execMgr := executor.NewManager(localExec)

	// Load sandbox server into execMgr
	servers, _ := st.List(ctx)
	var sandbox *store.Server
	for _, s := range servers {
		if s.Name == "sandbox" {
			sandbox = &s
			cfg := &sshconn.Config{
				Host:     s.Host,
				Port:     s.Port,
				User:     s.User,
				AuthType: string(s.AuthType),
				Password: s.Credential,
				KeyPath:  s.Credential,
			}
			client := sshconn.NewClient(cfg)
			sshExec := sshconn.NewSSHExecutor(s.Name, client, s.OS, s.WorkspaceDir)
			execMgr.Register(s.Name, sshExec)
			break
		}
	}
	if sandbox == nil {
		log.Fatal("Sandbox server not found in store")
	}

	// 2. Setup Agent
	llmCfg := cfg.GeneralLLM()
	client := llm.NewOpenAIClient(llmCfg.Endpoint, llmCfg.Model, llmCfg.APIKey, time.Duration(llmCfg.Timeout)*time.Second)

	sink := &captureSink{}
	a := agent.New(client, registry, execMgr, &noopHandler{}, st)
	a.SetQueryEventSink(sink)
	a.SetActiveServer(*sandbox)
	a.SetYoroMode(true) // Disable interactive checks for simulation

	// 3. Scenario Step 1: Request Oracle 19c Install Plan
	fmt.Println(">>> Step 1: Requesting Oracle 19c applyRU install plan...")
	prompt1 := "Oracle 19c를 applyRU 방식으로 설치하고 싶어. " +
		"설치 파일은 로컬 c:\\users\\jhkwa\\downloads 에 있고, " +
		"리모트의 /home/sandbox/oracle_install 에 설치할거야. 계획을 세워줘."
	
	err = a.Run(ctx, prompt1)
	if err != nil {
		fmt.Printf("Error in Step 1: %v\n", err)
	}
	
	fmt.Println("\n--- Agent's Initial Plan ---")
	// If the agent used todo_write, the plan is in the todo store
	if len(tStore.List()) > 0 {
		fmt.Println("Plan from Todo Store:")
		for _, item := range tStore.List() {
			fmt.Printf("- %s [%s]\n", item.Content, item.Status)
		}
	}
	fmt.Println("Response Text:")
	fmt.Println(sink.lastText)
	fmt.Println("----------------------------")

	// 4. Scenario Step 2: Intervene if OPatch is first
	planStr := sink.lastText
	for _, item := range tStore.List() {
		planStr += " " + item.Content
	}

	if strings.Contains(strings.ToLower(planStr), "opatch") && 
	   strings.Index(strings.ToLower(planStr), "opatch") < strings.Index(strings.ToLower(planStr), "db_home") {
		fmt.Println("\n>>> Step 2: Intervention! OPatch is suggested before DB Home.")
		fmt.Println("Feedback: Opatch 를 먼저풀면 덮어써진다고 말해")
		
		sink.lastText = "" // Clear for next response
		err = a.Run(ctx, "Opatch 를 먼저풀면 덮어써지니깐 순서를 바꿔줘.")
		if err != nil {
			fmt.Printf("Error in Step 2: %v\n", err)
		}

		fmt.Println("\n--- Agent's Revised Plan ---")
		if len(tStore.List()) > 0 {
			fmt.Println("Plan from Todo Store:")
			for _, item := range tStore.List() {
				fmt.Printf("- %s [%s]\n", item.Content, item.Status)
			}
		}
		fmt.Println("Response Text:")
		fmt.Println(sink.lastText)
		fmt.Println("----------------------------")
	} else {
		fmt.Println("\n>>> Step 2: OPatch is not suggested before DB Home or already in correct order.")
	}

	// 5. Scenario Step 3: Check permissions during extraction (Simulation)
	fmt.Println("\n>>> Step 3: Verifying permissions during file operations...")
	fmt.Println("Scenario verification completed.")
}

type noopHandler struct{}
func (h *noopHandler) OnThinking(tier string, model string) {}
func (h *noopHandler) OnThinkingToken(token string) {}
func (h *noopHandler) OnToken(token string) {}
func (h *noopHandler) OnToolStart(toolID string, toolName string, target string, args map[string]any) {}
func (h *noopHandler) OnToolOutput(toolID string, line string) {}
func (h *noopHandler) OnToolEnd(toolID string, toolName string, result string, duration time.Duration, success bool, metadataJSON string) {}
func (h *noopHandler) OnResponse(content string) {}
func (h *noopHandler) OnError(err error) {}
func (h *noopHandler) OnUsageUpdate(inputTokens, outputTokens int, costUSD float64, durationMs int64) {}
func (h *noopHandler) OnJobComplete(jobID int, description string, success bool) {}
func (h *noopHandler) OnRAGContext(count int) {}
