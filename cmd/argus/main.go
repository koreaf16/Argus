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
	"os/signal"
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
	"github.com/koreaf16/argus/internal/tasks"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"

	"github.com/koreaf16/argus/internal/tools/accountshell"
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
	"github.com/koreaf16/argus/internal/tools/toolsearch"
	"github.com/koreaf16/argus/internal/tools/webfetch"
	"github.com/koreaf16/argus/internal/tools/websearch"
	"github.com/koreaf16/argus/internal/tui"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

type parsedFlags struct {
	help          bool
	version       bool
	init          bool
	model         string
	print         string
	resume        string
	aidebug       bool
	aidebugTrace  bool // mirror + 풀 NDJSON 트레이스 (혼합)
	aidebugRaw    bool // mirror 끄고 풀 NDJSON 트레이스만 (legacy 외부 자동화 호환)
	autoOK        bool
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
	fs.BoolVar(&out.aidebug, "aidebug", false, "headless mirror mode: stdout shows TUI ANSI rendering, session file gets full NDJSON")
	fs.BoolVar(&out.aidebugTrace, "aidebug-trace", false, "with --aidebug: also stream NDJSON trace to stderr (mirror + trace)")
	fs.BoolVar(&out.aidebugRaw, "aidebug-trace-only", false, "with --aidebug: skip mirror, emit NDJSON trace only (legacy)")
	fs.BoolVar(&out.autoOK, "auto-approve", false, "auto-approve tool permission prompts")
	fs.BoolVar(&out.autoOK, "yolo", false, "alias for --auto-approve")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	// -r 옵션 뒤에 세션 ID가 오지 않고 다른 플래그가 오는 경우 (예: -r -p "...") 처리
	if strings.HasPrefix(out.resume, "-") {
		out.resume = ""
	}

	return out, nil
}

func main() {
	// 한글/한자/일본어 등 동아시아 문자를 터미널 표시 폭(2)으로 계산하도록 강제한다.
	// 미설정 시 Windows 기본 환경에서 EastAsianWidth=false로 동작하여 마크다운 표
	// 셀이 헤더와 어긋나게 정렬되는 문제가 있다.
	runewidth.DefaultCondition.EastAsianWidth = true

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 세션 자동 저장 defer (비정상 종료나 타임아웃 대응)
	defer func() {
		if err := persistSessionSnapshot(config.Memory, config.State, engine.Messages()); err != nil {
			fmt.Fprintf(os.Stderr, "failed to persist session snapshot on exit: %v\n", err)
		}
	}()

	// 시그널 핸들러 추가
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	if config.DebugSink == nil {
		return fmt.Errorf("aidebug sink is not configured")
	}
	defer config.DebugSink.Close()
	var decisionLLM llm.LLM
	if client, err := reg.Build(); err == nil {
		decisionLLM = client
	}

	// mirror 모드 활성화: --aidebug-trace-only가 명시되지 않은 경우 stdout에
	// TUI ANSI 미러 출력을 흘린다. trace-only 모드에서는 nil로 두어 기존 동작 유지.
	var mirror *tui.HeadlessMirror
	if !flags.aidebugRaw {
		mirror = tui.NewHeadlessMirror(tui.Config{
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
			AutoApprove:  flags.autoOK,
		}, os.Stdout)
	}

	reader := bufio.NewReader(os.Stdin)

	engine.SetApprovalGate(query.ApprovalGate{
		Prompt: func(ctx context.Context, toolName string, input json.RawMessage) (bool, error) {
			engine.EmitTrace("aidebug.tool_approval_request", "", map[string]any{
				"tool":  toolName,
				"input": string(input),
			})

			if flags.autoOK {
				engine.EmitTrace("aidebug.decision", "", map[string]any{
					"phase":      "tool_approval",
					"tool":       toolName,
					"decision":   "allow",
					"handled_by": "auto-approve",
				})
				return true, nil
			}

			// -p (단일 프롬프트) 모드: stdin에 외부 컨트롤러가 데이터를 보내지 않으면
			// reader.Peek 자체가 hang. JSON 주입/수동 입력 경로 모두 진입하지 않고
			// 즉시 auto-deny로 안전하게 종료시킨다. 외부 컨트롤러 사용 시에는
			// --auto-approve(--yolo)를 명시적으로 지정해야 한다.
			if strings.TrimSpace(flags.print) != "" {
				engine.EmitTrace("aidebug.decision", "", map[string]any{
					"phase":      "tool_approval",
					"tool":       toolName,
					"decision":   "deny",
					"handled_by": "auto-deny-single-prompt",
					"reason":     "single-prompt mode requires --auto-approve/--yolo for tool approvals",
				})
				return false, nil
			}

			// JSON 주입(Injection) 지원 — REPL 모드에서만 동작
			if peek, err := reader.Peek(1); err == nil && len(peek) > 0 && peek[0] == '{' {
				line, _ := reader.ReadString('\n')
				var inject struct {
					Allow bool `json:"allow"`
				}
				if err := json.Unmarshal([]byte(line), &inject); err == nil {
					decision := "deny"
					if inject.Allow {
						decision = "allow"
					}
					engine.EmitTrace("aidebug.decision", "", map[string]any{
						"phase":      "tool_approval",
						"tool":       toolName,
						"decision":   decision,
						"handled_by": "json",
					})
					return inject.Allow, nil
				}
			}

			// 수동 입력(Fallback) — REPL 모드 전용
			fmt.Fprintf(os.Stderr, "Allow tool %s? [y/N]: ", toolName)
			line, _ := reader.ReadString('\n')
			allow := strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y")

			decision := "deny"
			if allow {
				decision = "allow"
			}
			engine.EmitTrace("aidebug.decision", "", map[string]any{
				"phase":      "tool_approval",
				"tool":       toolName,
				"decision":   decision,
				"handled_by": "human",
			})
			return allow, nil
		},
	})

	if config.Workspace != nil {
		config.Workspace.SetPromptFunc(func(alias, kind, prompt string) (string, error) {
			pwPrompt := workspace.FormatPasswordPrompt(config.Workspace.Registry(), alias, kind, prompt)

			fmt.Fprint(os.Stderr, pwPrompt+" ")

			// -p (단일 프롬프트) 모드: reader.Peek 자체가 stdin에서 hang하므로
			// REPL 진입 전에 즉시 에러로 종료시킨다.
			if strings.TrimSpace(flags.print) != "" {
				engine.EmitTrace("aidebug.decision", "", map[string]any{
					"phase":      "workspace_prompt",
					"prompt":     strings.TrimSpace(pwPrompt),
					"decision":   "abort",
					"handled_by": "auto-abort-single-prompt",
					"reason":     "single-prompt mode does not support stdin credential prompts",
				})
				return "", fmt.Errorf("aidebug single-prompt mode: credential required for %s/%s", alias, kind)
			}

			if peek, err := reader.Peek(1); err == nil && len(peek) > 0 && peek[0] == '{' {
				line, _ := reader.ReadString('\n')
				var inject map[string]string
				if err := json.Unmarshal([]byte(line), &inject); err == nil {
					if val, ok := inject["value"]; ok {
						engine.EmitTrace("aidebug.decision", "", map[string]any{
							"phase":  "workspace_prompt",
							"prompt": strings.TrimSpace(pwPrompt),
							"value":  val,
						})
						return val, nil
					}
				}
			}

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
			cmdName := ""
			if parts := strings.Fields(strings.TrimSpace(input)); len(parts) > 0 {
				cmdName = strings.TrimPrefix(parts[0], "/")
			}
			_, err := commands.Dispatch(input, ctx)
			engine.EmitTrace("slash_command", "", map[string]any{
				"name":     cmdName,
				"is_error": err != nil,
			})
			// 설정 변경 가능성이 있는 명령 후 UI 설정 동기화
			if cmdName == "viewthink" || cmdName == "config" {
				config.UI = tui.LoadUISettings(constants.SettingsPath())
				if mirror != nil {
					mirror.UpdateUISettings(config.UI)
				}
			}
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
			if mirror != nil {
				mirror.Apply(event)
			}
			if event.Kind == query.UIEventAssistantDelta {
				// 텍스트 자체는 mirror가 ANSI로 그려준다.
			}
			if event.Kind == query.UIEventToolUse {
				// tool.call.start trace는 engine이 이미 발행함 — 중복 출력 불필요
			}
			if event.Kind == query.UIEventPasswordPrompt {
				pwPrompt := strings.TrimSpace(event.Prompt)
				if pwPrompt == "" {
					pwPrompt = "Password:"
				}
				if !strings.HasSuffix(pwPrompt, " ") {
					pwPrompt += " "
				}
				fmt.Fprint(os.Stderr, pwPrompt)

				handledByJSON := false
				injected := ""
				if peek, err := reader.Peek(1); err == nil && len(peek) > 0 && peek[0] == '{' {
					line, _ := reader.ReadString('\n')
					var inject map[string]string
					if err := json.Unmarshal([]byte(line), &inject); err == nil {
						if val, ok := inject["value"]; ok {
							injected = val
							handledByJSON = true
							engine.EmitTrace("aidebug.decision", "", map[string]any{
								"phase":  "password_prompt",
								"prompt": strings.TrimSpace(pwPrompt),
								"value":  val,
							})
						}
					}
				}

				if event.PasswordResponse != nil {
					if handledByJSON {
						event.PasswordResponse <- injected
					} else {
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
				var resp tool.AskUserResponse
				if flags.autoOK {
					qType := askUserQuestionType(event.Question)
					switch qType {
					case "yesno":
						resp = tool.AskUserResponse{Value: "yes"}
					case "choice":
						opts := askUserQuestionOptions(event.Question)
						if len(opts) > 0 {
							resp = tool.AskUserResponse{Value: askUserOptionValue(opts[0])}
						} else {
							resp = tool.AskUserResponse{Value: "yes"}
						}
					default:
						resp = tool.AskUserResponse{Value: "yes"}
					}
					engine.EmitTrace("aidebug.decision", event.TaskID, map[string]any{
						"phase":      "ask_user",
						"tool":       event.ToolName,
						"handled_by": "auto-approve",
						"value":      resp.Value,
					})
				} else {
					resp = promptAskUserFromReader(reader, os.Stderr, event.Question)
					engine.EmitTrace("aidebug.decision", event.TaskID, map[string]any{
						"phase":      "ask_user",
						"tool":       event.ToolName,
						"handled_by": "human",
						"value":      resp.Value,
						"canceled":   resp.Canceled,
					})
				}
				if event.AskUserResponse != nil {
					event.AskUserResponse <- resp
				}
			}
			if event.Kind == query.UIEventAskUserBatchPrompt {
				var resp tool.AskUserBatchResponse
				if flags.autoOK {
					answersByIndex := make(map[string]string)
					answersByID := make(map[string]string)
					for i, q := range event.Questions {
						val := "yes"
						qType := askUserQuestionType(&q)
						if qType == "choice" {
							opts := askUserQuestionOptions(&q)
							if len(opts) > 0 {
								val = askUserOptionValue(opts[0])
							}
						}
						answersByIndex[strconv.Itoa(i)] = val
						if q.ID != "" {
							answersByID[q.ID] = val
						}
					}
					resp = tool.AskUserBatchResponse{
						AnswersByIndex: answersByIndex,
						AnswersByID:    answersByID,
					}
					engine.EmitTrace("aidebug.decision", event.TaskID, map[string]any{
						"phase":          "ask_user_batch",
						"tool":           event.ToolName,
						"handled_by":     "auto-approve",
						"question_count": len(event.Questions),
					})
				} else {
					resp = promptAskUserBatchFromReader(reader, os.Stderr, event.Questions)
					engine.EmitTrace("aidebug.decision", event.TaskID, map[string]any{
						"phase":          "ask_user_batch",
						"tool":           event.ToolName,
						"handled_by":     "human",
						"question_count": len(event.Questions),
						"canceled":       resp.Canceled,
					})
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
		if mirror != nil {
			mirror.FlushAll()
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
			engine.EmitTrace("plan.step", "", map[string]any{
				"index":    i + 1,
				"total":    len(steps),
				"tool":     step.Tool,
				"prompt":   step.Prompt,
				"decision": "error",
				"error":    err.Error(),
			})
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "aidebug denied plan step %d/%d: %s: %s\n", i+1, len(steps), step.Tool, step.Prompt)
			engine.EmitTrace("plan.step", "", map[string]any{
				"index":    i + 1,
				"total":    len(steps),
				"tool":     step.Tool,
				"prompt":   step.Prompt,
				"decision": "deny",
			})
			return nil
		}
		fmt.Fprintf(os.Stderr, "aidebug approved plan step %d/%d: %s: %s\n", i+1, len(steps), step.Tool, step.Prompt)
		engine.EmitTrace("plan.step", "", map[string]any{
			"index":    i + 1,
			"total":    len(steps),
			"tool":     step.Tool,
			"prompt":   step.Prompt,
			"decision": "allow",
		})
		out, execErr := engine.ExecutePlannedStep(ctx, step)
		if execErr != nil {
			return fmt.Errorf("execute planned step %d/%d: %w", i+1, len(steps), execErr)
		}
		fmt.Fprintf(os.Stderr, "aidebug step %d/%d completed: %s\n", i+1, len(steps), truncateAIDebugOutput(out))
		engine.EmitTrace("plan.step", "", map[string]any{
			"index":    i + 1,
			"total":    len(steps),
			"tool":     step.Tool,
			"prompt":   step.Prompt,
			"decision": "completed",
			"output":   truncateAIDebugOutput(out),
		})
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
		// 외부 writer는 항상 stderr (mirror ANSI는 stdout으로 분리).
		// 모드별 필터:
		//   --aidebug-trace-only → 풀 NDJSON (legacy 동작)
		//   --aidebug-trace      → 풀 NDJSON + mirror 동시
		//   --aidebug 단독       → mirror만 (NDJSON은 세션 파일에만)
		mode := aidebug.FilterDropAll
		switch {
		case flags.aidebugRaw:
			mode = aidebug.FilterFull
		case flags.aidebugTrace:
			mode = aidebug.FilterMirror
		}
		debugSink, err = aidebug.NewSinkWithMode(os.Stderr, tracePath, mode)
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

	// 세션 재개 시 기존 활성 워크스페이스 복원
	if resumedSnapshot != nil && resumedSnapshot.Metadata != nil {
		if ws, ok := resumedSnapshot.Metadata["active_workspace"].(string); ok && ws != "" {
			_ = workspaceRegistry.SetActive(ws)
		}
	}
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
		toolsearch.New(),
		enterplanmode.NewEnterPlanModeTool(),
		exitplanmode.NewExitPlanModeTool(),
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
		connectortool.New(connectorManager),
		serverinspect.NewServerInspectTool(),
		servermetrics.NewServerMetricsTool(),
		servertunnel.NewServerTunnelTool(),
		accountshell.NewAccountShellTool(),
		shelljob.NewShellJobTool(),
		shelljobcontrol.NewShellJobControlTool(),
		skilltool.NewSkillTool(skillRegistry),
		&snitptool.SnipTool{},
		task.NewTaskCreateTool(),
		task.NewTaskUpdateTool(),
		askuser.NewAskUserQuestionTool(),
		enterworktree.NewEnterWorktreeTool(),
		exitworktree.NewExitWorktreeTool(),
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

	var mcpReloadEngine *query.Engine
	mcpReload := func() error {
		if err := mcpManager.Load(); err != nil {
			if mcpReloadEngine != nil {
				mcpReloadEngine.EmitTrace("mcp.server", "", map[string]any{
					"phase": "reload",
					"error": err.Error(),
				})
			}
			return err
		}
		appState.SetActiveMCPServers(mcpManager.ServerNames())
		if mcpReloadEngine != nil {
			mcpReloadEngine.EmitTrace("mcp.server", "", map[string]any{
				"phase":   "reload",
				"servers": mcpManager.ServerNames(),
			})
		}
		return registerMCPBridgeTools(toolRegistry, mcpManager)
	}

	engine := query.NewEngine(client, toolRegistry, appState, query.DefaultSystemPrompt)
	mcpReloadEngine = engine
	engineCfg := query.DefaultConfig()
	engineCfg.DebugTools = false
	engine.SetConfig(engineCfg)
	if resumedSnapshot != nil {
		engine.ReplaceMessages(resumedSnapshot.Messages)
	}

	if flags != nil && flags.autoOK {
		appState.SetPermissionMode(types.PermissionModeBypassPermissions)
	}

	engine.AddStopHook(func(_ context.Context, _ query.StopSummary) {
		if err := persistSessionSnapshot(memStore, appState, engine.Messages()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: session autosave failed: %v\n", err)
		}
	})
	workDir := ""
	if wd, err := os.Getwd(); err == nil {
		workDir = wd
		approvalPrompt := func(ctx context.Context, toolName string, input json.RawMessage) (bool, error) {
			return promptToolApprovalWithWriter(os.Stdout, ctx, toolName, input, appState)
		}
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

	if debugSink != nil {
		engine.EmitTrace("mcp.server", "", map[string]any{
			"phase":   "load",
			"servers": mcpManager.ServerNames(),
		})
		appState.SetStateChangeHook(func(field, before, after string) {
			engine.EmitTrace("state.change", "", map[string]any{
				"field":  field,
				"before": before,
				"after":  after,
			})
		})
	}

	uiSettings := tui.LoadUISettings(constants.SettingsPath())

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

		// AppState 필드 복원
		if snap.Metadata != nil {
			for k, v := range snap.Metadata {
				appState.SetMetadata(k, v)
			}
		}
		if snap.Mode != "" {
			appState.SetMode(snap.Mode)
		}

		return &snap, nil
	}
	id, err := session.NewID()
	if err != nil {
		return nil, fmt.Errorf("create session id: %w", err)
	}
	appState.SetSessionID(id)

	// 새로운 세션 시작 시 기존 Task 초기화
	_ = tasks.ClearAllTasks()

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
		Mode:     appState.GetMode(),
		Metadata: appState.MetadataSnapshot(),
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
		{alias: "EnterWorktreeTool", target: "enter_worktree"},
		{alias: "ExitWorktreeTool", target: "exit_worktree"},
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
			bridge := mcp.NewBridgeTool(server, cfg.Name, cfg.Description, cfg.InputSchema, manager)
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
	return parseAIDebugAllowDecision(answer)
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

	// JSON 주입(Injection) 지원: 첫 글자가 '{' 이면 JSON으로 파싱 시도
	if peek, err := reader.Peek(1); err == nil && len(peek) > 0 && peek[0] == '{' {
		line, _ := reader.ReadString('\n')
		var resp tool.AskUserResponse
		if err := json.Unmarshal([]byte(line), &resp); err == nil {
			return resp
		}
		// 파싱 실패 시 일반 텍스트 모드로 폴백하기 위해 reader를 다시 준비해야 할 수도 있으나,
		// 보통 JSON 주입 시도는 명확한 의도이므로 실패 시 에러 반환이 안전함.
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
	if reader == nil {
		reader = bufio.NewReader(os.Stdin)
	}
	if len(questions) == 0 {
		return tool.AskUserBatchResponse{Canceled: true, Error: "question payload is missing"}
	}

	// JSON 주입(Injection) 지원: 첫 글자가 '{' 이면 JSON으로 파싱 시도
	if peek, err := reader.Peek(1); err == nil && len(peek) > 0 && peek[0] == '{' {
		line, _ := reader.ReadString('\n')
		var resp tool.AskUserBatchResponse
		if err := json.Unmarshal([]byte(line), &resp); err == nil {
			return resp
		}
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
		callCtx, cancel = context.WithTimeout(ctx, 60*time.Second)
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

func promptToolApprovalWithWriter(w io.Writer, ctx context.Context, toolName string, input json.RawMessage, appState *state.AppState) (bool, error) {
	_ = ctx
	if w == nil {
		w = os.Stdout
	}

	if toolName == constants.ExitPlanModeToolName || toolName == constants.LegacyExitPlanModeToolName {
		return promptPlanApproval(w, input, appState)
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

func promptPlanApproval(w io.Writer, input json.RawMessage, appState *state.AppState) (bool, error) {
	var req struct {
		Plan           string `json:"plan"`
		PlanText       string `json:"planText"`
		AllowedPrompts []struct {
			Tool   string `json:"tool"`
			Prompt string `json:"prompt"`
		} `json:"allowedPrompts"`
	}
	_ = json.Unmarshal(input, &req)

	plan := strings.TrimSpace(req.Plan)
	if plan == "" {
		plan = strings.TrimSpace(req.PlanText)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "┌──────────────────────────────────────────────────────────────────────────────┐")
	fmt.Fprintln(w, "│  📋 Plan Approval Request                                                    │")
	fmt.Fprintln(w, "├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Fprintln(w, "│                                                                              │")
	fmt.Fprintln(w, "│  The agent has completed the plan. Please review it before proceeding.       │")
	fmt.Fprintln(w, "│                                                                              │")
	if plan != "" {
		fmt.Fprintln(w, "│  [Plan Content]                                                              │")
		for _, line := range strings.Split(plan, "\n") {
			line = strings.TrimRight(line, "\r")
			for len(line) > 74 {
				fmt.Fprintf(w, "│  %-74s  │\n", line[:74])
				line = line[74:]
			}
			fmt.Fprintf(w, "│  %-74s  │\n", line)
		}
		fmt.Fprintln(w, "│                                                                              │")
	}
	if len(req.AllowedPrompts) > 0 {
		fmt.Fprintln(w, "│  [Requested Permissions]                                                     │")
		for _, p := range req.AllowedPrompts {
			msg := fmt.Sprintf("• %s: %s", p.Tool, p.Prompt)
			for len(msg) > 74 {
				fmt.Fprintf(w, "│  %-74s  │\n", msg[:74])
				msg = msg[74:]
			}
			fmt.Fprintf(w, "│  %-74s  │\n", msg)
		}
		fmt.Fprintln(w, "│                                                                              │")
	}
	fmt.Fprintln(w, "├──────────────────────────────────────────────────────────────────────────────┤")
	fmt.Fprintf(w, "│  Approve this plan and start execution? (y/n/yolo) [y]: ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(w, "│")
		fmt.Fprintln(w, "└──────────────────────────────────────────────────────────────────────────────┘")
		return false, err
	}
	fmt.Fprintln(w, "└──────────────────────────────────────────────────────────────────────────────┘")

	token := strings.ToLower(strings.TrimSpace(line))
	if token == "" {
		token = "y"
	}

	if token == "yolo" {
		if appState != nil {
			appState.SetPermissionMode(types.PermissionModeBypassPermissions)
		}
		return true, nil
	}

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
