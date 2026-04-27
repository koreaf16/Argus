package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/koreaf16/argus/internal/aidebug"
	"github.com/koreaf16/argus/internal/connector"
	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/hooks"
	"github.com/koreaf16/argus/internal/memdir"
	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/query"
	"github.com/koreaf16/argus/internal/repl/commands"
	"github.com/koreaf16/argus/internal/security"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/lsp"
	"github.com/koreaf16/argus/internal/services/mcp"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/session"
	"github.com/koreaf16/argus/internal/shelljobs"
	"github.com/koreaf16/argus/internal/skills"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/askuser"
	"github.com/koreaf16/argus/internal/tools/bash"
	"github.com/koreaf16/argus/internal/tools/connectortool"
	"github.com/koreaf16/argus/internal/tools/enterplanmode"
	"github.com/koreaf16/argus/internal/tools/enterworktree"
	"github.com/koreaf16/argus/internal/tools/exitplanmode"
	"github.com/koreaf16/argus/internal/tools/exitworktree"
	"github.com/koreaf16/argus/internal/tools/fileread"
	"github.com/koreaf16/argus/internal/tools/filewrite"
	"github.com/koreaf16/argus/internal/tools/fslist"
	"github.com/koreaf16/argus/internal/tools/glob"
	"github.com/koreaf16/argus/internal/tools/grep"
	"github.com/koreaf16/argus/internal/tools/listmcpresourcestool"
	"github.com/koreaf16/argus/internal/tools/lsptool"
	"github.com/koreaf16/argus/internal/tools/mcpauthtool"
	"github.com/koreaf16/argus/internal/tools/mcptool"
	"github.com/koreaf16/argus/internal/tools/powershell"
	"github.com/koreaf16/argus/internal/tools/readmcpresourcetool"
	"github.com/koreaf16/argus/internal/tools/serverconnect"
	"github.com/koreaf16/argus/internal/tools/servercopy"
	"github.com/koreaf16/argus/internal/tools/serverinspect"
	"github.com/koreaf16/argus/internal/tools/servermetrics"
	"github.com/koreaf16/argus/internal/tools/servertunnel"
	"github.com/koreaf16/argus/internal/tools/shelljob"
	"github.com/koreaf16/argus/internal/tools/shelljobcontrol"
	"github.com/koreaf16/argus/internal/tools/skilltool"
	"github.com/koreaf16/argus/internal/tools/snitptool"
	"github.com/koreaf16/argus/internal/tools/task"
	"github.com/koreaf16/argus/internal/tools/todoread"
	"github.com/koreaf16/argus/internal/tools/todowrite"
	"github.com/koreaf16/argus/internal/tools/webfetch"
	"github.com/koreaf16/argus/internal/tools/websearch"
	"github.com/koreaf16/argus/internal/tui"
	"golang.org/x/term"
)

type parsedFlags struct {
	help           bool
	version        bool
	init           bool
	model   string
	print   string
	resume  string
	aidebug bool
	autoOK  bool
}

func parseFlags() (*parsedFlags, error) {
	fs := flag.NewFlagSet("argus", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	out := &parsedFlags{}
	fs.BoolVar(&out.help, "help", false, "print help")
	fs.BoolVar(&out.help, "h", false, "print help")
	fs.BoolVar(&out.version, "version", false, "print version")
	fs.BoolVar(&out.version, "v", false, "print version")
	fs.BoolVar(&out.init, "init", false, "initialize ~/.argus files")
	fs.StringVar(&out.model, "model", "", "set active model alias for this session")
	fs.StringVar(&out.print, "print", "", "single prompt mode")
	fs.StringVar(&out.print, "p", "", "single prompt mode")
	fs.StringVar(&out.resume, "resume", "", "resume a conversation by session ID")
	fs.StringVar(&out.resume, "r", "", "resume a conversation by session ID")
	fs.BoolVar(&out.aidebug, "aidebug", false, "emit NDJSON analysis trace (stdout + session file)")
	fs.BoolVar(&out.autoOK, "auto-approve", false, "auto-approve tool permission prompts")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}
	return out, nil
}

func main() {
	// 윈도우 환경에서 한글 깨짐 방지를 위해 콘솔 코드 페이지를 UTF-8(65001)로 설정
	if runtime.GOOS == "windows" {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		setConsoleCP := kernel32.NewProc("SetConsoleCP")
		setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
		setConsoleCP.Call(65001)
		setConsoleOutputCP.Call(65001)
	}

	// SSH 접속 시 GUI 창이 뜨는 것을 방지 (터미널 입력 강제)
	os.Unsetenv("SSH_ASKPASS")
	os.Setenv("DISPLAY", "") // Linux 호환성
	flags, err := parseFlags()
	if err != nil {
		os.Exit(2)
	}
	switch {
	case flags.help:
		printHelp(os.Stdout)
		return
	case flags.version:
		printVersion(os.Stdout)
		return
	case flags.init:
		if err := runInit(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if flags.aidebug {
		// --aidebug 는 항상 헤드리스 REPL 모드로 진입한다.
		// 외부 CLI(claude_cli/gemini_cli/codex_cli)가 stdin/stdout으로 직접 제어할 수 있도록
		// 터미널 여부나 -p 동시 지정 여부와 무관하게 runAIDebug 로 라우팅한다.
		// (-p 가 함께 들어온 경우 runAIDebug 내부에서 단일 프롬프트로 처리됨)
		engine, reg, config, err := bootstrap(context.Background(), flags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap failed: %v\n", err)
			os.Exit(1)
		}
		if err := runAIDebug(flags, config, engine, reg); err != nil {
			fmt.Fprintf(os.Stderr, "aidebug failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if flags.print != "" {
		if err := runPrint(flags); err != nil {
			fmt.Fprintf(os.Stderr, "print failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runTUI(flags); err != nil {
		fmt.Fprintf(os.Stderr, "tui failed: %v\n", err)
		os.Exit(1)
	}
}

// ... (중략: bootstrap 함수 내의 debugSink 설정 부분도 수정 필요)


func runAIDebug(flags *parsedFlags, config *bootstrapConfig, engine *query.Engine, reg *llm.Registry) error {
	ctx := context.Background()

	if config.DebugSink == nil {
		return fmt.Errorf("aidebug sink is not configured")
	}
	defer config.DebugSink.Close()
	var decisionLLM llm.LLM
	if client, err := reg.Build(); err == nil {
		decisionLLM = client
	}

	engine.SetApprovalGate(query.ApprovalGate{
		Prompt: func(ctx context.Context, toolName string, input json.RawMessage) (bool, error) {
			allow, err := decideAIDebugToolApproval(ctx, decisionLLM, toolName, input)
			if err != nil {
				fmt.Fprintf(os.Stderr, "aidebug tool approval decision failed for %s: %v\n", toolName, err)
				return false, nil
			}
			decision := "deny"
			if allow {
				decision = "allow"
			}
			fmt.Fprintf(os.Stderr, "aidebug tool approval %s: %s\n", toolName, decision)
			return allow, nil
		},
	})

	reader := bufio.NewReader(os.Stdin)

	if config.Workspace != nil {
		config.Workspace.SetPromptFunc(func(alias, kind, prompt string) (string, error) {
			pwPrompt := workspace.FormatPasswordPrompt(config.Workspace.Registry(), alias, kind, prompt)

			if shouldAIDebugAutoInjectPrompt(pwPrompt) && decisionLLM != nil {
				injected, err := decideAIDebugPromptInput(ctx, decisionLLM, pwPrompt)
				if err == nil && injected != "" {
					fmt.Fprintf(os.Stderr, "aidebug injected input for workspace prompt %q: %q\n", strings.TrimSpace(pwPrompt), injected)
					return injected, nil
				}
			}

			fmt.Fprint(os.Stderr, pwPrompt+" ")
			pw, err := reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(pw), nil
		})
	}

	runTurn := func(input string) error {
		if strings.HasPrefix(strings.TrimSpace(input), "/") {
			// 디버그용 수동 디스패처 호출
			ctx := commands.CommandContext{
				Context:      ctx,
				Stdout:       os.Stdout,
				Registry:     reg,
				Engine:       engine,
				State:        config.State,
				ModelPath:    constants.ModelsPath(),
				SettingsPath: constants.SettingsPath(),
				WorkDir:      config.WorkDir,
				Memory:       config.Memory,
				Connector:    config.Connector,
				MCP:          config.MCP,
				LSP:          config.LSP,
				Workspace:    config.Workspace,
				Skills:       config.Skills,
				Credentials:  config.Credentials,
				MCPReload:    config.MCPReload,
			}
			_, err := commands.Dispatch(input, ctx)
			return err
		}
		events, err := engine.SubmitMessage(ctx, input)
		if err != nil {
			return err
		}
		var (
			plannedSteps []query.PlannedStep
			streamFailed bool
		)
		for event := range events {
			if event.Kind == query.UIEventAssistantDelta {
				// 헤드리스 모드라도 텍스트 출력을 직접 하지 않음 (NDJSON 트레이스에 포함됨)
			}
			if event.Kind == query.UIEventToolUse {
				input := ""
				if event.ToolUse != nil {
					input = string(event.ToolUse.Input)
				}
				fmt.Fprintf(os.Stderr, "aidebug tool call: %s(%s)\n", event.ToolName, truncateAIDebugOutput(input))
			}
			if event.Kind == query.UIEventPasswordPrompt {				pwPrompt := strings.TrimSpace(event.Prompt)
				if pwPrompt == "" {
					pwPrompt = "Password:"
				}
				if !strings.HasSuffix(pwPrompt, " ") {
					pwPrompt += " "
				}
				handledByLLM := false
				injected := ""
				if shouldAIDebugAutoInjectPrompt(pwPrompt) && decisionLLM != nil {
					injected, err = decideAIDebugPromptInput(ctx, decisionLLM, pwPrompt)
					if err == nil && injected != "" {
						handledByLLM = true
						fmt.Fprintf(os.Stderr, "aidebug injected input for prompt %q: %q\n", strings.TrimSpace(pwPrompt), injected)
					}
				}
				if event.PasswordResponse != nil {
					if handledByLLM {
						event.PasswordResponse <- injected
					} else {
						fmt.Fprint(os.Stderr, pwPrompt)
						pw, readErr := reader.ReadString('\n')
						if readErr != nil {
							event.PasswordResponse <- ""
						} else {
							event.PasswordResponse <- strings.TrimSpace(pw)
						}
					}
				}
			}
			if event.Kind == query.UIEventAskUserPrompt {
				resp := tool.AskUserResponse{}
				handledByLLM := false
				if decisionLLM != nil {
					resp, err = decideAIDebugAskUserResponse(ctx, decisionLLM, event.Question)
					if err == nil && !resp.Canceled && strings.TrimSpace(resp.Value) != "" {
						handledByLLM = true
						fmt.Fprintf(os.Stderr, "aidebug answered ask_user prompt: %q\n", resp.Value)
					}
				}
				if !handledByLLM {
					resp = promptAskUserFromReader(reader, os.Stderr, event.Question)
				}
				if event.AskUserResponse != nil {
					event.AskUserResponse <- resp
				}
			}
			if event.Kind == query.UIEventAskUserBatchPrompt {
				resp := tool.AskUserBatchResponse{}
				handledByLLM := false
				if decisionLLM != nil && len(event.Questions) > 0 {
					answersByIndex := make(map[string]string, len(event.Questions))
					answersByID := make(map[string]string, len(event.Questions))
					handledByLLM = true
					for i := range event.Questions {
						single, singleErr := decideAIDebugAskUserResponse(ctx, decisionLLM, &event.Questions[i])
						if singleErr != nil || single.Canceled || strings.TrimSpace(single.Value) == "" {
							handledByLLM = false
							break
						}
						value := strings.TrimSpace(single.Value)
						answersByIndex[strconv.Itoa(i)] = value
						if id := strings.TrimSpace(event.Questions[i].ID); id != "" {
							answersByID[id] = value
						}
					}
					if handledByLLM {
						resp = tool.AskUserBatchResponse{
							AnswersByIndex: answersByIndex,
							AnswersByID:    answersByID,
						}
						fmt.Fprintf(os.Stderr, "aidebug answered ask_user batch: %d questions\n", len(event.Questions))
					}
				}
				if !handledByLLM {
					resp = promptAskUserBatchFromReader(reader, os.Stderr, event.Questions)
				}
				if event.AskUserBatchResponse != nil {
					event.AskUserBatchResponse <- resp
				}
			}
			if event.Kind == query.UIEventToolDelta {
				if event.InputResponse != nil {
					go func(ch chan string) {
						select {
						case ch <- "":
						case <-ctx.Done():
						}
					}(event.InputResponse)
				}
			}
			if event.Kind == query.UIEventPlanExecutionReady {
				plannedSteps = append([]query.PlannedStep(nil), event.PlanSteps...)
			}
			if event.Kind == query.UIEventError && event.Err != nil {
				streamFailed = true
				return event.Err
			}
			if event.Kind == query.UIEventError {
				streamFailed = true
			}
		}
		fmt.Println() // AI 응답 종료 후 개행 추가
		if len(plannedSteps) > 0 && !streamFailed {
			if err := runAIDebugPlannedSteps(ctx, engine, decisionLLM, plannedSteps); err != nil {
				return err
			}
		}
		return nil
	}

	if strings.TrimSpace(flags.print) != "" {
		if err := runTurn(strings.TrimSpace(flags.print)); err != nil {
			return err
		}
	} else {
		for {
			line, readErr := reader.ReadString('\n')
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				if err := runTurn(trimmed); err != nil {
					return err
				}
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
	}

	if err := persistSessionSnapshot(config.Memory, config.State, engine.Messages()); err != nil {
		return err
	}
	printSessionID(os.Stderr, config.State)
	return nil
}

func runAIDebugPlannedSteps(ctx context.Context, engine *query.Engine, client llm.LLM, steps []query.PlannedStep) error {
	if engine == nil || len(steps) == 0 {
		return nil
	}
	for i, step := range steps {
		ok := false
		var err error
		if client != nil {
			ok, err = decideAIDebugPlanStepApproval(ctx, client, i+1, len(steps), step)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "aidebug step decision failed at %d/%d: %v\n", i+1, len(steps), err)
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "aidebug denied plan step %d/%d: %s: %s\n", i+1, len(steps), step.Tool, step.Prompt)
			return nil
		}
		fmt.Fprintf(os.Stderr, "aidebug approved plan step %d/%d: %s: %s\n", i+1, len(steps), step.Tool, step.Prompt)
		out, execErr := engine.ExecutePlannedStep(ctx, step)
		if execErr != nil {
			return fmt.Errorf("execute planned step %d/%d: %w", i+1, len(steps), execErr)
		}
		fmt.Fprintf(os.Stderr, "aidebug step %d/%d completed: %s\n", i+1, len(steps), truncateAIDebugOutput(out))
	}
	return nil
}

func truncateAIDebugOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500] + " ... (truncated)"
	}
	if s == "" {
		return "(no output)"
	}
	return s
}

func runTUI(flags *parsedFlags) error {
	ctx := context.Background()
	engine, reg, config, err := bootstrap(ctx, flags)
	if err != nil {
		return err
	}
	if err := tui.Run(ctx, tui.Config{
		Engine:       engine,
		Registry:     reg,
		State:        config.State,
		ModelPath:    constants.ModelsPath(),
		SettingsPath: constants.SettingsPath(),
		WorkDir:      config.WorkDir,
		Memory:       config.Memory,
		MCP:          config.MCP,
		LSP:          config.LSP,
		Workspace:    config.Workspace,
		Connector:    config.Connector,
		ShellJobs:    config.ShellJobs,
		Credentials:  config.Credentials,
		Skills:       config.Skills,
		MCPReload:    config.MCPReload,
		Theme:        config.Theme,
		UI:           config.UI,
		AIDebug:      flags.aidebug,
		AutoApprove:  flags.autoOK,
	}); err != nil {
		return err
	}
	if err := persistSessionSnapshot(config.Memory, config.State, engine.Messages()); err != nil {
		return err
	}
	printSessionID(os.Stderr, config.State)
	return nil
}


type bootstrapConfig struct {
	State       *state.AppState
	WorkDir     string
	Memory      *memdir.Store
	DebugSink   *aidebug.Sink
	Connector   *connector.Manager
	MCP         *mcp.Manager
	LSP         *lsp.Manager
	Workspace   *workspace.Manager
	ShellJobs   *shelljobs.Manager
	Credentials *workspace.CredentialStore
	Skills      *skills.Registry
	MCPReload   func() error
	Theme       string
	UI          tui.UISettings
}

func bootstrap(ctx context.Context, flags *parsedFlags) (*query.Engine, *llm.Registry, *bootstrapConfig, error) {
	reg := llm.NewRegistry()
	if err := reg.Load(constants.ModelsPath()); err != nil {
		return nil, nil, nil, fmt.Errorf("load models: %w", err)
	}

	// 등록된 모델이 없으면 자동으로 초기화 메뉴 실행
	if len(reg.List()) == 0 {
		fmt.Println("No models configured. Starting interactive setup...")
		if err := runInit(os.Stdin, os.Stdout); err != nil {
			return nil, nil, nil, fmt.Errorf("auto-init failed: %w", err)
		}
		// 초기화 후 다시 로드
		if err := reg.Load(constants.ModelsPath()); err != nil {
			return nil, nil, nil, fmt.Errorf("reload models failed: %w", err)
		}
		if len(reg.List()) == 0 {
			return nil, nil, nil, fmt.Errorf("initialization completed but no models were registered")
		}
	}

	if flags.model != "" {
		if err := reg.SetActive(flags.model); err != nil {
			return nil, nil, nil, err
		}
	}

	client, err := reg.Build()
	if err != nil {
		client = llm.NewErrorLLM("bootstrap", llm.Caps{}, err)
	}

	appState := state.NewAppState()
	if active, ok := reg.ActiveEntry(); ok {
		appState.SetActiveModel(active.Alias, active.Display, active.ContextWin)
	}

	memStore := memdir.NewStore(constants.ConfigDir())
	if err := memStore.Bootstrap(); err != nil {
		return nil, nil, nil, fmt.Errorf("bootstrap memdir: %w", err)
	}
	resumedSnapshot, err := initializeSession(flags, appState, memStore)
	if err != nil {
		return nil, nil, nil, err
	}
	var debugSink *aidebug.Sink
	if flags != nil && flags.aidebug {
		tracePath := memStore.TracePath(appState.SessionID())
		// TUI/UI 환경에서 Stdout을 사용하면 NDJSON이 화면을 덮어버리므로 Stderr로 출력
		debugSink, err = aidebug.NewSink(os.Stderr, tracePath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("initialize aidebug sink: %w", err)
		}
	}

	workspaceRegistry := workspace.NewRegistry("")
	if err := workspaceRegistry.Load(); err != nil {
		return nil, nil, nil, fmt.Errorf("load workspace registry: %w", err)
	}
	// 실행 시점엔 항상 local 워크스페이스로 시작한다. ssh_servers.json에
	// 저장된 active 값은 무시(파일에는 쓰지 않으므로 다음 실행에도 그대로 보존됨).
	_ = workspaceRegistry.SetActive(workspace.LocalAlias)

	workspaceManager := workspace.NewManager(workspaceRegistry, func(alias, kind, prompt string) (string, error) {
		return promptWorkspacePassword(workspaceRegistry, alias, kind, prompt)
	})
	shellJobManager := shelljobs.NewManager()
	appState.SetActiveWorkspace(workspaceManager.ActiveAlias())

	credStore := workspace.NewCredentialStore(constants.CredentialsPath(), security.Default())
	if err := credStore.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "argus: warning: could not load credential store: %v\n", err)
	} else {
		workspaceManager.SetCredentialStore(credStore)
	}

	mcpManager := mcp.NewManager("")
	if err := mcpManager.Load(); err != nil {
		return nil, nil, nil, fmt.Errorf("load mcp manager: %w", err)
	}
	appState.SetActiveMCPServers(mcpManager.ServerNames())

	connectorManager := connector.NewManager(mcpManager, filepath.Join(constants.ConfigDir(), "cache"))

	lspManager := lsp.NewManager()
	lsptool.SetManager(lspManager)
	skillRegistry := skills.NewRegistry()
	appState.SetActiveSkills(skillRegistry.List())

	toolRegistry := tool.NewRegistry()
	for _, t := range []tool.Tool{
		bash.NewBashTool(),
		powershell.NewPowerShellTool(),
		enterplanmode.NewEnterPlanModeTool(),
		exitplanmode.NewExitPlanModeTool(),
		todowrite.NewTodoWriteTool(),
		websearch.NewWebSearchTool(),
		fileread.NewFileReadTool(),
		fslist.NewFSListTool(),
		filewrite.NewFileWriteTool(),
		glob.NewGlobTool(),
		grep.NewGrepTool(),
		webfetch.NewWebFetchTool(),
		lsptool.NewLSPTool(),
		mcptool.NewMCPTool(),
		listmcpresourcestool.NewListMcpResourcesTool(),
		readmcpresourcetool.NewReadMcpResourceTool(),
		mcpauthtool.NewMcpAuthTool(),
		serverconnect.NewServerConnectTool(),
		servercopy.NewServerCopyTool(),
		serverinspect.NewServerInspectTool(),
		servermetrics.NewServerMetricsTool(),
		servertunnel.NewServerTunnelTool(),
		shelljob.NewShellJobTool(),
		shelljobcontrol.NewShellJobControlTool(),
		skilltool.NewSkillTool(skillRegistry),
		&snitptool.SnipTool{},
		task.NewTaskCreateTool(),
		task.NewTaskListTool(),
		task.NewTaskUpdateTool(),
		task.NewTaskDeleteTool(),
		askuser.NewAskUserQuestionTool(),
		enterworktree.NewEnterWorktreeTool(),
		exitworktree.NewExitWorktreeTool(),
		todoread.NewTodoReadTool(),
		connectortool.New(connectorManager),
	} {
		if err := toolRegistry.Register(t); err != nil {
			return nil, nil, nil, fmt.Errorf("register %s tool: %w", t.Name(), err)
		}
	}
	if err := registerLegacyToolAliases(toolRegistry); err != nil {
		return nil, nil, nil, err
	}
	if err := registerMCPBridgeTools(toolRegistry, mcpManager); err != nil {
		return nil, nil, nil, err
	}

	mcpReload := func() error {
		if err := mcpManager.Load(); err != nil {
			return err
		}
		appState.SetActiveMCPServers(mcpManager.ServerNames())
		return registerMCPBridgeTools(toolRegistry, mcpManager)
	}

	engine := query.NewEngine(client, toolRegistry, appState, query.DefaultSystemPrompt)
	engineCfg := query.DefaultConfig()
	engineCfg.DebugTools = false
	engine.SetConfig(engineCfg)
	if resumedSnapshot != nil {
		engine.ReplaceMessages(resumedSnapshot.Messages)
	}
	engine.AddStopHook(func(_ context.Context, _ query.StopSummary) {
		if err := persistSessionSnapshot(memStore, appState, engine.Messages()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: session autosave failed: %v\n", err)
		}
	})
	workDir := ""
	if wd, err := os.Getwd(); err == nil {
		workDir = wd
		approvalPrompt := promptToolApproval
		if flags != nil && flags.autoOK {
			approvalPrompt = func(context.Context, string, json.RawMessage) (bool, error) {
				return true, nil
			}
		}
		// settings.json 훅 디스패처 초기화 (SetDeps 전 설정해야 SessionStart 훅이 발화됨)
		if hookCfg, err := hooks.LoadConfig(); err == nil {
			d := hooks.NewDispatcher(hookCfg, appState.SessionID(), wd)
			d.SetSubQueryFn(engine.ExecuteSubQuery)
			engine.SetHookDispatcher(d)
		}

		engine.SetDeps(ctx, query.Deps{
			WorkingDir: wd,
			Workspace:  workspaceManager,
			ShellJobs:  shellJobManager,
			BaseDir:    memStore.BaseDir(),
			SessionID:  appState.SessionID(),
			AIDebug: query.AIDebugConfig{
				Enabled: debugSink != nil,
				Emitter: debugSink,
			},
			ApproveTool: query.ApprovalGate{
				Prompt: approvalPrompt,
			},
		})
	}

	uiSettings := loadUISettings(constants.SettingsPath())

	return engine, reg, &bootstrapConfig{
		State:       appState,
		WorkDir:     workDir,
		Memory:      memStore,
		DebugSink:   debugSink,
		Connector:   connectorManager,
		MCP:         mcpManager,
		LSP:         lspManager,
		Workspace:   workspaceManager,
		ShellJobs:   shellJobManager,
		Credentials: credStore,
		Skills:      skillRegistry,
		MCPReload:   mcpReload,
		Theme:       uiSettings.Theme,
		UI:          uiSettings,
	}, nil
}

func loadUISettings(settingsPath string) tui.UISettings {
	settings := tui.DefaultUISettings()

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return settings
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return settings
	}
	uiRaw, ok := root["ui"]
	if !ok {
		return settings
	}
	uiMap, ok := uiRaw.(map[string]any)
	if !ok {
		return settings
	}

	if v, ok := uiMap["theme"].(string); ok && strings.TrimSpace(v) != "" {
		settings.Theme = strings.TrimSpace(v)
	}
	if v, ok := uiMap["variant"].(string); ok && strings.TrimSpace(v) != "" {
		settings.Variant = strings.TrimSpace(v)
	}

	if motionRaw, ok := uiMap["motion"].(map[string]any); ok {
		if v, ok := motionRaw["enabled"].(bool); ok {
			settings.Motion.Enabled = v
		}
		if v, ok := motionRaw["level"].(string); ok && strings.TrimSpace(v) != "" {
			settings.Motion.Level = strings.TrimSpace(v)
		}
		if v, ok := motionRaw["tick_ms"]; ok {
			switch n := v.(type) {
			case float64:
				settings.Motion.TickMS = int(n)
			case int:
				settings.Motion.TickMS = n
			}
		}
		if v, ok := motionRaw["reduced"].(bool); ok {
			settings.Motion.Reduced = v
		}
		if v, ok := motionRaw["signature"].(bool); ok {
			settings.Motion.Signature = v
		}
	}

	if streamingRaw, ok := uiMap["streaming"].(map[string]any); ok {
		if v, ok := streamingRaw["mode"].(string); ok && strings.TrimSpace(v) != "" {
			settings.Streaming.Mode = strings.TrimSpace(v)
		}
		if v, ok := streamingRaw["hide_unstable_markdown_tail"].(bool); ok {
			settings.Streaming.HideUnstableMarkdown = v
		}
		if v, ok := streamingRaw["flush_plain_text_partial"].(bool); ok {
			settings.Streaming.FlushPlainTextPartial = v
		}
		if v, ok := streamingRaw["render_code_blocks_stable"].(bool); ok {
			settings.Streaming.RenderCodeBlocksStable = v
		}
	}

	return tui.ResolveUISettings(settings, settings.Theme, false)
}

func loadUITheme(settingsPath string) string {
	return loadUISettings(settingsPath).Theme
}

func initializeSession(flags *parsedFlags, appState *state.AppState, store *memdir.Store) (*session.Snapshot, error) {
	if appState == nil {
		return nil, fmt.Errorf("app state is unavailable")
	}
	if store == nil {
		return nil, fmt.Errorf("session store is unavailable")
	}
	if strings.TrimSpace(flags.resume) != "" {
		var snap session.Snapshot
		if err := store.LoadSession(flags.resume, &snap); err != nil {
			return nil, fmt.Errorf("resume session %q: %w", flags.resume, err)
		}
		appState.SetSessionID(flags.resume)
		return &snap, nil
	}
	id, err := session.NewID()
	if err != nil {
		return nil, fmt.Errorf("create session id: %w", err)
	}
	appState.SetSessionID(id)
	return nil, nil
}

func persistSessionSnapshot(store *memdir.Store, appState *state.AppState, messages []llm.Message) error {
	if store == nil || appState == nil {
		return nil
	}
	sessionID := strings.TrimSpace(appState.SessionID())
	if sessionID == "" {
		return nil
	}
	snap := session.Snapshot{
		Version:  session.SnapshotVersion,
		SavedAt:  time.Now().UTC(),
		Messages: query.SanitizeMessagesForStorage(messages), // v1 호환 유지
	}
	return store.SaveSession(sessionID, snap)
}

func printSessionID(w io.Writer, appState *state.AppState) {
	if appState == nil || w == nil {
		return
	}
	sessionID := strings.TrimSpace(appState.SessionID())
	if sessionID == "" {
		return
	}
	fmt.Fprintf(w, "session_id: %s\n", sessionID)
}

func registerLegacyToolAliases(registry *tool.Registry) error {
	if registry == nil {
		return nil
	}
	aliases := []struct {
		alias  string
		target string
	}{
		{alias: constants.LegacyEnterPlanModeToolName, target: constants.EnterPlanModeToolName},
		{alias: constants.LegacyExitPlanModeToolName, target: constants.ExitPlanModeToolName},
		{alias: constants.LegacyAskUserToolName, target: constants.AskUserToolName},
		{alias: constants.LegacyAskUserAliasName, target: constants.AskUserToolName},
	}
	for _, item := range aliases {
		if err := registry.RegisterAlias(item.alias, item.target); err != nil {
			return fmt.Errorf("register tool alias %s -> %s: %w", item.alias, item.target, err)
		}
	}
	return nil
}

func registerMCPBridgeTools(registry *tool.Registry, manager *mcp.Manager) error {
	if registry == nil || manager == nil {
		return nil
	}
	desired := make(map[string]*mcp.BridgeTool)
	for _, server := range manager.ServerNames() {
		for _, cfg := range manager.ToolConfigs(server) {
			if cfg.Name == "" {
				continue
			}
			bridge := mcp.NewBridgeTool(server, cfg.Name, cfg.Description, manager)
			desired[bridge.Name()] = bridge
		}
	}

	for _, t := range registry.List() {
		name := t.Name()
		if len(name) < 5 || name[:5] != "mcp__" {
			continue
		}
		if _, ok := desired[name]; ok {
			continue
		}
		registry.Unregister(name)
	}

	for name, bridge := range desired {
		if _, ok := registry.Lookup(name); ok {
			continue
		}
		if err := registry.RegisterFromMCP(mcpServerFromBridgeName(name), bridge); err != nil {
			return fmt.Errorf("register mcp bridge %s: %w", bridge.Name(), err)
		}
	}
	return nil
}

func mcpServerFromBridgeName(name string) string {
	parts := strings.SplitN(name, "__", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

func decideAIDebugToolApproval(ctx context.Context, client llm.LLM, toolName string, input json.RawMessage) (bool, error) {
	if client != nil {
		systemPrompt := "You are a CLI safety gate for Argus aidebug mode. Respond with exactly one token: allow or deny. Default to deny when uncertain. No explanation."
		userPrompt := fmt.Sprintf(
			"Decision target: tool approval\nTool: %s\nInput JSON: %s\nReturn: allow|deny",
			strings.TrimSpace(toolName),
			strings.TrimSpace(string(input)),
		)
		answer, err := runAIDebugDecisionQuery(ctx, client, systemPrompt, userPrompt)
		if err == nil {
			fmt.Fprintf(os.Stderr, "aidebug raw decision for %s: %q\n", toolName, answer)
			if ok, parseErr := parseAIDebugAllowDecision(answer); parseErr == nil {
				return ok, nil
			}
		} else {
			fmt.Fprintf(os.Stderr, "aidebug auto-decision failed for %s: %v\n", toolName, err)
		}
	}

	// LLM이 없거나 결정에 실패한 경우, 터미널 환경이면 사용자에게 직접 질문
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "aidebug: falling back to manual approval for %s\n", toolName)
		return promptToolApprovalWithWriter(os.Stderr, ctx, toolName, input)
	}

	return false, fmt.Errorf("LLM auto-decision failed and no terminal for manual approval")
}

func decideAIDebugPlanStepApproval(ctx context.Context, client llm.LLM, stepIndex, stepTotal int, step query.PlannedStep) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("LLM client is not initialized")
	}
	systemPrompt := "You are a CLI execution gate for Argus aidebug mode. Respond with exactly one token: allow or deny. Default to deny when uncertain. No explanation."
	userPrompt := fmt.Sprintf(
		"Decision target: planned step execution\nStep: %d/%d\nTool: %s\nPrompt: %s\nReturn: allow|deny",
		stepIndex,
		stepTotal,
		strings.TrimSpace(step.Tool),
		strings.TrimSpace(step.Prompt),
	)
	answer, err := runAIDebugDecisionQuery(ctx, client, systemPrompt, userPrompt)
	if err != nil {
		return false, err
	}
	fmt.Fprintf(os.Stderr, "aidebug raw decision for step %d/%d: %q\n", stepIndex, stepTotal, answer)
	return parseAIDebugAllowDecision(answer)
}

func decideAIDebugPromptInput(ctx context.Context, client llm.LLM, prompt string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("llm client is unavailable")
	}
	systemPrompt := "You generate direct terminal input for Argus aidebug automation. Return exactly one line with the input text only. No explanation."
	userPrompt := fmt.Sprintf("Prompt shown to the user:\n%s\n\nReturn the exact input line:", strings.TrimSpace(prompt))
	answer, err := runAIDebugDecisionQuery(ctx, client, systemPrompt, userPrompt)
	if err != nil {
		return "", err
	}
	return normalizeAIDebugInjectedInput(answer), nil
}

func decideAIDebugAskUserResponse(ctx context.Context, client llm.LLM, q *tool.AskUserQuestion) (tool.AskUserResponse, error) {
	if client == nil {
		return tool.AskUserResponse{}, fmt.Errorf("llm client is unavailable")
	}
	if q == nil {
		return tool.AskUserResponse{Canceled: true, Error: "question payload is missing"}, nil
	}
	options := askUserQuestionOptions(q)
	systemPrompt := "You answer CLI ask_user prompts for automation. Return exactly one plain-text answer with no explanation."
	userPrompt := fmt.Sprintf(
		"Question:\n%s\n\nType: %s\nRequired: %t\nOptions:\n%s\n\nReturn one answer only.",
		strings.TrimSpace(q.Question),
		askUserQuestionType(q),
		q.Required,
		formatAskUserOptionsForDecision(options),
	)
	answer, err := runAIDebugDecisionQuery(ctx, client, systemPrompt, userPrompt)
	if err != nil {
		return tool.AskUserResponse{}, err
	}
	clean := normalizeAIDebugInjectedInput(answer)
	if clean == "" {
		clean = strings.TrimSpace(q.Default)
	}
	if askUserQuestionType(q) == "yesno" {
		if v, ok := normalizeYesNoAnswer(clean); ok {
			clean = v
		}
	}
	if clean == "" && q.Required {
		if len(options) > 0 {
			clean = askUserOptionValue(options[0])
		}
		if clean == "" {
			clean = strings.TrimSpace(q.Default)
		}
	}
	return tool.AskUserResponse{Value: clean}, nil
}

func promptAskUserFromReader(reader *bufio.Reader, out io.Writer, q *tool.AskUserQuestion) tool.AskUserResponse {
	if reader == nil {
		reader = bufio.NewReader(os.Stdin)
	}
	if out == nil {
		out = os.Stdout
	}
	if q == nil {
		return tool.AskUserResponse{Canceled: true, Error: "question payload is missing"}
	}

	qType := askUserQuestionType(q)
	switch qType {
	case "text":
		label := strings.TrimSpace(q.Question)
		if label == "" {
			label = "Answer"
		}
		if strings.TrimSpace(q.Default) != "" {
			fmt.Fprintf(out, "%s [%s]: ", label, strings.TrimSpace(q.Default))
		} else {
			fmt.Fprintf(out, "%s: ", label)
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return tool.AskUserResponse{Canceled: true, Error: err.Error()}
		}
		answer := strings.TrimSpace(line)
		if answer == "" {
			answer = strings.TrimSpace(q.Default)
		}
		if q.Required && answer == "" {
			return tool.AskUserResponse{Canceled: true, Error: "answer is required"}
		}
		return tool.AskUserResponse{Value: answer}
	case "yesno":
		label := strings.TrimSpace(q.Question)
		if label == "" {
			label = "Continue?"
		}
		fmt.Fprintf(out, "%s [y/N]: ", label)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return tool.AskUserResponse{Canceled: true, Error: err.Error()}
		}
		if v, ok := normalizeYesNoAnswer(line); ok {
			return tool.AskUserResponse{Value: v}
		}
		return tool.AskUserResponse{Value: "no"}
	default:
		options := askUserQuestionOptions(q)
		if len(options) == 0 {
			return tool.AskUserResponse{Canceled: true, Error: "choice question has no options"}
		}
		fmt.Fprintln(out, strings.TrimSpace(q.Question))
		for i, opt := range options {
			fmt.Fprintf(out, "  %d) %s\n", i+1, askUserOptionLabel(opt))
		}
		if q.MultiSelect {
			fmt.Fprint(out, "Select one or more numbers (comma separated): ")
			line, err := reader.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return tool.AskUserResponse{Canceled: true, Error: err.Error()}
			}
			values := make([]string, 0, len(options))
			for _, tok := range strings.Split(line, ",") {
				n, convErr := strconv.Atoi(strings.TrimSpace(tok))
				if convErr != nil || n < 1 || n > len(options) {
					continue
				}
				values = append(values, askUserOptionValue(options[n-1]))
			}
			if len(values) == 0 && q.Required {
				return tool.AskUserResponse{Canceled: true, Error: "at least one option is required"}
			}
			return tool.AskUserResponse{Value: strings.Join(values, ", ")}
		}
		fmt.Fprint(out, "Select number: ")
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return tool.AskUserResponse{Canceled: true, Error: err.Error()}
		}
		n, convErr := strconv.Atoi(strings.TrimSpace(line))
		if convErr != nil || n < 1 || n > len(options) {
			if q.Required {
				return tool.AskUserResponse{Canceled: true, Error: "invalid selection"}
			}
			return tool.AskUserResponse{}
		}
		return tool.AskUserResponse{Value: askUserOptionValue(options[n-1])}
	}
}

func promptAskUserBatchFromReader(reader *bufio.Reader, out io.Writer, questions []tool.AskUserQuestion) tool.AskUserBatchResponse {
	if len(questions) == 0 {
		return tool.AskUserBatchResponse{Canceled: true, Error: "question payload is missing"}
	}
	answersByIndex := make(map[string]string, len(questions))
	answersByID := make(map[string]string, len(questions))
	for i := range questions {
		resp := promptAskUserFromReader(reader, out, &questions[i])
		if resp.Canceled {
			if strings.TrimSpace(resp.Error) != "" {
				return tool.AskUserBatchResponse{Canceled: true, Error: strings.TrimSpace(resp.Error)}
			}
			return tool.AskUserBatchResponse{Canceled: true}
		}
		value := strings.TrimSpace(resp.Value)
		if value == "" {
			value = strings.TrimSpace(questions[i].Default)
		}
		answersByIndex[strconv.Itoa(i)] = value
		if id := strings.TrimSpace(questions[i].ID); id != "" {
			answersByID[id] = value
		}
	}
	return tool.AskUserBatchResponse{
		AnswersByIndex: answersByIndex,
		AnswersByID:    answersByID,
	}
}

func runAIDebugDecisionQuery(ctx context.Context, client llm.LLM, systemPrompt, userPrompt string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("llm client is unavailable")
	}
	callCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		callCtx, cancel = context.WithTimeout(ctx, 20*time.Second)
	}
	defer cancel()

	req := llm.Request{
		System: []llm.SystemBlock{
			{Type: "text", Text: systemPrompt},
		},
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleUser, userPrompt),
		},
		MaxTokens: 32,
	}
	stream, err := client.Stream(callCtx, req)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	for evt := range stream {
		switch evt.Kind {
		case llm.EventTextDelta:
			out.WriteString(evt.Delta)
		case llm.EventError:
			if evt.Err != nil {
				return "", evt.Err
			}
		}
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return "", fmt.Errorf("empty decision response")
	}
	return text, nil
}

func parseAIDebugAllowDecision(raw string) (bool, error) {
	tokens := aidebugDecisionTokens(raw)
	for _, token := range tokens {
		switch token {
		case "allow", "approve", "approved", "yes", "y", "true":
			return true, nil
		case "deny", "denied", "disallow", "reject", "rejected", "no", "n", "false":
			return false, nil
		}
	}
	return false, fmt.Errorf("unrecognized decision response: %q", strings.TrimSpace(raw))
}

func shouldAIDebugAutoInjectPrompt(prompt string) bool {
	s := strings.ToLower(strings.TrimSpace(prompt))
	if s == "" {
		return false
	}
	for _, keyword := range []string{
		"password",
		"passphrase",
		"otp",
		"pin",
		"secret",
		"token",
		"비밀번호",
	} {
		if strings.Contains(s, keyword) {
			return false
		}
	}
	return true
}

func normalizeAIDebugInjectedInput(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.Trim(s, "`")
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		s = line
		break
	}
	if idx := strings.Index(s, ":"); idx > 0 {
		head := strings.ToLower(strings.TrimSpace(s[:idx]))
		if head == "input" || head == "answer" || head == "response" {
			s = strings.TrimSpace(s[idx+1:])
		}
	}
	s = strings.Trim(s, "\"'`")
	return strings.TrimSpace(s)
}

func askUserQuestionType(q *tool.AskUserQuestion) string {
	if q == nil {
		return "text"
	}
	t := strings.ToLower(strings.TrimSpace(q.Type))
	switch t {
	case "text", "choice", "yesno":
		return t
	default:
		if len(q.Options) > 0 {
			return "choice"
		}
		return "text"
	}
}

func askUserQuestionOptions(q *tool.AskUserQuestion) []tool.AskUserOption {
	if q == nil {
		return nil
	}
	if askUserQuestionType(q) == "yesno" && len(q.Options) == 0 {
		return []tool.AskUserOption{
			{Value: "yes", Label: "Yes"},
			{Value: "no", Label: "No"},
		}
	}
	return q.Options
}

func askUserOptionLabel(opt tool.AskUserOption) string {
	label := strings.TrimSpace(opt.Label)
	if label == "" {
		label = strings.TrimSpace(opt.Value)
	}
	if desc := strings.TrimSpace(opt.Description); desc != "" {
		return label + " - " + desc
	}
	return label
}

func askUserOptionValue(opt tool.AskUserOption) string {
	if v := strings.TrimSpace(opt.Value); v != "" {
		return v
	}
	return strings.TrimSpace(opt.Label)
}

func formatAskUserOptionsForDecision(options []tool.AskUserOption) string {
	if len(options) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(options))
	for i, opt := range options {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, askUserOptionLabel(opt)))
	}
	return strings.Join(lines, "\n")
}

func normalizeYesNoAnswer(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "y", "yes", "true", "1":
		return "yes", true
	case "n", "no", "false", "0":
		return "no", true
	default:
		return "", false
	}
}

func aidebugDecisionTokens(raw string) []string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}, s)
	return strings.Fields(s)
}

func promptToolApproval(ctx context.Context, toolName string, input json.RawMessage) (bool, error) {
	return promptToolApprovalWithWriter(os.Stdout, ctx, toolName, input)
}

func promptToolApprovalWithWriter(w io.Writer, ctx context.Context, toolName string, input json.RawMessage) (bool, error) {
	_ = ctx
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, "\nApprove tool %s with input %s ? [y/N]: ", toolName, presentation.FormatInputJSON(input))
	reader := bufio.NewReader(os.Stdin)
	token := ""
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Non-interactive input (pipe/redirection): consume a token directly.
		if _, err := fmt.Fscan(reader, &token); err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		token = strings.ToLower(strings.TrimSpace(token))
		return token == "y" || token == "yes", nil
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	token = strings.ToLower(strings.TrimSpace(line))
	return token == "y" || token == "yes", nil
}

func promptWorkspacePassword(reg *workspace.Registry, alias, kind, prompt string) (string, error) {
	pwPrompt := workspace.FormatPasswordPrompt(reg, alias, kind, prompt)
	fmt.Fprintf(os.Stdout, "\n%s ", pwPrompt)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Fprintln(os.Stdout)
	return strings.TrimSpace(string(pw)), nil
}
