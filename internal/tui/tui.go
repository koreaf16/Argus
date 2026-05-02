package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/connector"
	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/memdir"
	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/query"
	"github.com/koreaf16/argus/internal/repl/commands"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/lsp"
	"github.com/koreaf16/argus/internal/services/mcp"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/shelljobs"
	"github.com/koreaf16/argus/internal/skills"
	"github.com/koreaf16/argus/internal/state"
	"github.com/koreaf16/argus/internal/todostore"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
	bashutils "github.com/koreaf16/argus/internal/utils/bash"
	"github.com/koreaf16/argus/internal/utils/permissions"
)

type Config struct {
	Engine                 *query.Engine
	Registry               *llm.Registry
	State                  *state.AppState
	ModelPath              string
	SettingsPath           string
	WorkDir                string
	Memory                 *memdir.Store
	Connector              *connector.Manager
	MCP                    *mcp.Manager
	LSP                    *lsp.Manager
	Workspace              *workspace.Manager
	ShellJobs              *shelljobs.Manager
	Skills                 *skills.Registry
	Credentials            *workspace.CredentialStore
	MCPReload              func() error
	ConnectorSearchPrompt  func(ctx context.Context, query string, results []connector.ConnectorSpec) (*connector.ConnectorSpec, error)
	ConnectorInstallPrompt func(ctx context.Context, spec connector.ConnectorSpec) (map[string]string, bool, error)
	Theme                  string
	UI                     UISettings
	AIDebug                bool
	AutoApprove            bool
}

type app struct {
	cfg     Config
	ctx     context.Context
	mu      sync.RWMutex
	program *tea.Program
	parker  *CursorParkWriter

	submitMu     sync.Mutex
	submitCancel context.CancelFunc

	jobInputMu sync.Mutex
	jobInputs  map[string]chan string
}

func (a *app) setSubmitCancel(cancel context.CancelFunc) {
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	if a.submitCancel != nil {
		a.submitCancel()
	}
	a.submitCancel = cancel
}

func (a *app) clearSubmitCancel() {
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	a.submitCancel = nil
}

func (a *app) cancelCurrentSubmit() {
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	if a.submitCancel != nil {
		a.submitCancel()
		a.submitCancel = nil
	}
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Engine == nil {
		return errors.New("tui requires query engine")
	}
	if cfg.Registry == nil {
		return errors.New("tui requires model registry")
	}
	if cfg.State == nil {
		cfg.State = state.NewAppState()
	}
	if cfg.ModelPath == "" {
		cfg.ModelPath = constants.ModelsPath()
	}
	if cfg.SettingsPath == "" {
		cfg.SettingsPath = constants.SettingsPath()
	}
	if cfg.WorkDir == "" {
		if wd, err := os.Getwd(); err == nil {
			cfg.WorkDir = wd
		}
	}
	if err := os.MkdirAll(constants.ConfigDir(), 0o755); err != nil {
		return err
	}

	parker := NewCursorParkWriter(os.Stdout)
	a := &app{
		cfg:       cfg,
		ctx:       ctx,
		parker:    parker,
		jobInputs: make(map[string]chan string),
	}
	a.cfg.Engine.SetApprovalGate(query.ApprovalGate{
		Prompt: a.promptToolApproval,
	})

	if cfg.Workspace != nil {
		cfg.Workspace.SetPromptFunc(func(alias, kind, prompt string) (string, error) {
			p := workspace.FormatPasswordPrompt(cfg.Workspace.Registry(), alias, kind, prompt)
			return a.promptPassword(a.ctx, p)
		})
	}

	model := newUIModel(a, cfg)
	p := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithOutput(parker),
		tea.WithInput(os.Stdin),
	)
	a.setProgram(p)
	defer a.setProgram(nil)

	stopShellPump := a.startShellJobPump()
	defer stopShellPump()

	_, err := p.Run()
	return err
}

func (a *app) setProgram(p *tea.Program) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.program = p
}

func (a *app) send(msg tea.Msg) {
	a.mu.RLock()
	p := a.program
	a.mu.RUnlock()
	if p != nil {
		p.Send(msg)
	}
}

func (a *app) startShellJobPump() func() {
	if a.cfg.ShellJobs == nil {
		return func() {}
	}
	events, cancel := a.cfg.ShellJobs.Subscribe()
	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case evt, ok := <-events:
				if !ok {
					return
				}
				a.handleShellJobEvent(evt)
			}
		}
	}()
	return cancel
}

func (a *app) handleShellJobEvent(evt shelljobs.Event) {
	if strings.TrimSpace(evt.JobID) == "" {
		return
	}
	switch evt.Kind {
	case shelljobs.EventStart:
		inputCh := make(chan string, 64)
		a.setJobInputChannel(evt.JobID, inputCh)
		go func(jobID string, ch <-chan string) {
			for in := range ch {
				if a.cfg.ShellJobs != nil {
					_ = a.cfg.ShellJobs.SendInput(jobID, in)
				}
			}
		}(evt.JobID, inputCh)

		inputJSON := buildShellJobToolUseInput(evt)
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind:          presentation.EventToolUse,
				ToolName:      evt.ToolName,
				Input:         inputJSON,
				TaskID:        evt.JobID,
				InputResponse: inputCh,
			},
		})
	case shelljobs.EventOutput:
		if strings.TrimSpace(evt.Output) == "" {
			return
		}
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind:     presentation.EventToolDelta,
				ToolName: evt.ToolName,
				Text:     evt.Output,
				TaskID:   evt.JobID,
			},
		})
	case shelljobs.EventDone:
		if ch := a.popJobInputChannel(evt.JobID); ch != nil {
			close(ch)
		}
		raw, _ := json.Marshal(map[string]any{
			"Stdout":           "",
			"Stderr":           strings.TrimSpace(evt.Error),
			"Code":             evt.ExitCode,
			"BackgroundTaskID": evt.JobID,
			"OutputTaskID":     evt.JobID,
		})
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind:     presentation.EventToolResult,
				ToolName: evt.ToolName,
				Text:     string(raw),
				TaskID:   evt.JobID,
			},
		})
		a.emitFooter()
	}
}

func buildShellJobToolUseInput(evt shelljobs.Event) string {
	payload := map[string]any{
		"command":    evt.Command,
		"background": true,
		"task_id":    evt.JobID,
	}
	if alias := strings.TrimSpace(evt.TargetAlias); alias != "" {
		payload["server"] = alias
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func (a *app) setJobInputChannel(jobID string, ch chan string) {
	a.jobInputMu.Lock()
	defer a.jobInputMu.Unlock()
	if old := a.jobInputs[jobID]; old != nil {
		close(old)
	}
	a.jobInputs[jobID] = ch
}

func (a *app) popJobInputChannel(jobID string) chan string {
	a.jobInputMu.Lock()
	defer a.jobInputMu.Unlock()
	ch := a.jobInputs[jobID]
	delete(a.jobInputs, jobID)
	return ch
}

func (a *app) submit(input string) {
	a.submitPrompt(promptSubmission{Display: input, Expanded: input})
}

func (a *app) submitPrompt(input promptSubmission) {
	go a.runSubmit(input)
}

func (a *app) runSubmit(input promptSubmission) {
	submitCtx, cancel := context.WithCancel(a.ctx)
	a.setSubmitCancel(cancel)
	defer a.clearSubmitCancel()

	a.send(submitStartedMsg{})
	defer a.send(submitFinishedMsg{})

	display := strings.TrimSpace(input.Display)
	expanded := strings.TrimSpace(input.Expanded)
	if expanded == "" {
		a.emitFooter()
		return
	}
	if display == "" {
		display = expanded
	}

	if strings.HasPrefix(display, "/") {
		quit, loaded, err := a.handleSlashCommand(submitCtx, expanded)
		if err != nil {
			a.send(presentationEventMsg{
				Event: presentation.Event{
					Kind: presentation.EventError,
					Text: err.Error(),
				},
			})
		}
		if loaded {
			a.send(presentationEventMsg{Event: presentation.Event{Kind: presentation.EventViewCleared}})
			a.replayTranscript()
		}
		a.emitFooter()
		if quit {
			a.send(quitRequestedMsg{})
		}
		return
	}

	a.send(presentationEventMsg{
		Event: presentation.Event{
			Kind: presentation.EventUser,
			Text: display,
		},
	})

	stream, err := a.cfg.Engine.SubmitMessage(submitCtx, expanded)
	if err != nil {
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind: presentation.EventError,
				Text: err.Error(),
			},
		})
		a.emitFooter()
		return
	}

	var (
		plannedSteps []query.PlannedStep
		streamFailed bool
	)

	for evt := range stream {
		// 중복 방지를 위해 엔진 이벤트를 그대로 전달하되,
		// 필요한 메타데이터 처리가 끝난 후 단 한 번만 a.send() 호출.
		a.send(queryEventMsg{Event: evt})

		switch evt.Kind {
		case query.UIEventPasswordPrompt:
			pwPrompt := strings.TrimSpace(evt.Prompt)
			if pwPrompt == "" {
				pwPrompt = "Password:"
			}
			if !strings.HasSuffix(pwPrompt, " ") {
				pwPrompt += " "
			}
			pw, pwErr := a.promptPassword(submitCtx, pwPrompt)
			if evt.PasswordResponse != nil {
				if pwErr != nil {
					evt.PasswordResponse <- ""
				} else {
					evt.PasswordResponse <- pw
				}
			}
		case query.UIEventAskUserPrompt:
			response := a.promptAskUser(submitCtx, evt.ToolName, evt.Question)
			if evt.AskUserResponse != nil {
				evt.AskUserResponse <- response
			}
		case query.UIEventAskUserBatchPrompt:
			response := a.promptAskUserBatch(submitCtx, evt.ToolName, evt.Questions)
			if evt.AskUserBatchResponse != nil {
				evt.AskUserBatchResponse <- response
			}
		case query.UIEventPlanExecutionReady:
			plannedSteps = append([]query.PlannedStep(nil), evt.PlanSteps...)
		case query.UIEventError:
			streamFailed = true
		}
	}

	// AI 응답 스트림이 완료되었으므로 'Processing...' 상태 해제
	a.send(submitFinishedMsg{})

	if len(plannedSteps) > 0 && !streamFailed {
		a.runPlannedSteps(submitCtx, plannedSteps)
	}
	a.emitFooter()
}

func (a *app) replayTranscript() {
	for _, evt := range presentation.ReplayEvents(a.cfg.Engine.Messages()) {
		a.send(presentationEventMsg{Event: evt})
	}
}

func (a *app) buildFooterMsg() footerStateMsg {
	cwd := a.cfg.WorkDir
	if ws := a.cfg.Workspace; ws != nil {
		activeAlias := ws.ActiveAlias()
		if strings.TrimSpace(activeAlias) == "" {
			activeAlias = workspace.LocalAlias
		}
		if a.cfg.State != nil && a.cfg.State.ActiveWorkspace() != activeAlias {
			a.cfg.State.SetActiveWorkspace(activeAlias)
		}
		if snap, ok := ws.GetInspectSnapshot(activeAlias); ok && snap.CWD != "" {
			cwd = snap.CWD
		}
	}
	footer := presentation.BuildFooterState(a.cfg.State, cwd)
	if a.cfg.Engine != nil {
		in, out, thinking := a.cfg.Engine.CumulativeTokenSnapshot()
		footer.TokensUsed = in + out + thinking
		footer.TokensIn = in
		footer.TokensOut = out
		footer.TokensThinking = thinking
	}
	footer.DiffAdded, footer.DiffRemoved = gitDiffStat(cwd)
	return footerStateMsg{Footer: footer}
}

func (a *app) emitFooter() {
	a.send(a.buildFooterMsg())
}

func (a *app) promptToolApproval(ctx context.Context, toolName string, input json.RawMessage) (bool, error) {
	a.send(presentationEventMsg{
		Event: presentation.Event{
			Kind:     presentation.EventApprovalRequest,
			ToolName: toolName,
			Input:    presentation.FormatInputJSON(input),
		},
	})
	if a.cfg.AutoApprove {
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind:     presentation.EventApprovalDecision,
				Decision: "allow",
			},
		})
		return true, nil
	}

	respCh := make(chan decisionResult, 1)
	a.send(approvalRequestMsg{
		ToolName: toolName,
		Input:    presentation.FormatInputJSON(input),
		Response: respCh,
	})

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case resp := <-respCh:
		if resp.Allow {
			if err := a.applyApprovalRule(toolName, input, resp.Scope); err != nil {
				a.send(presentationEventMsg{
					Event: presentation.Event{
						Kind: presentation.EventNotice,
						Text: fmt.Sprintf("failed to persist permission rule: %v", err),
					},
				})
			}
		}
		decision := "deny"
		if resp.Allow {
			decision = "allow"
		}
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind:     presentation.EventApprovalDecision,
				Decision: decision,
			},
		})
		return resp.Allow, resp.Err
	}
}

func (a *app) promptConfirm(ctx context.Context, prompt string, defaultNo bool) (bool, error) {
	if a.cfg.AutoApprove {
		return true, nil
	}
	respCh := make(chan decisionResult, 1)
	a.send(confirmRequestMsg{
		Prompt:    prompt,
		DefaultNo: defaultNo,
		Response:  respCh,
	})
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case resp := <-respCh:
		return resp.Allow, resp.Err
	}
}

func (a *app) promptPassword(ctx context.Context, prompt string) (string, error) {
	respCh := make(chan passwordResult, 1)
	a.send(passwordRequestMsg{
		Prompt:   prompt,
		Response: respCh,
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case resp := <-respCh:
		return resp.Value, resp.Err
	}
}

func (a *app) promptConnectorSearch(ctx context.Context, query string, results []connector.ConnectorSpec) (*connector.ConnectorSpec, error) {
	respCh := make(chan connectorSearchResult, 1)
	a.send(connectorSearchRequestMsg{Query: query, Results: results, Response: respCh})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		return resp.Spec, nil
	}
}

func (a *app) promptConnectorInstall(ctx context.Context, spec connector.ConnectorSpec) (map[string]string, bool, error) {
	respCh := make(chan connectorInstallResult, 1)
	a.send(connectorInstallRequestMsg{Spec: spec, Response: respCh})
	select {
	case <-ctx.Done():
		return nil, true, ctx.Err()
	case resp := <-respCh:
		return resp.EnvAnswers, resp.Cancelled, nil
	}
}

func (a *app) promptServerForm(ctx context.Context, req commands.ServerFormRequest) (commands.ServerFormResult, error) {
	respCh := make(chan serverFormResult, 1)
	a.send(serverFormRequestMsg{EditAlias: req.EditAlias, Response: respCh})
	select {
	case <-ctx.Done():
		return commands.ServerFormResult{}, ctx.Err()
	case resp := <-respCh:
		return resp.Result, resp.Err
	}
}

func (a *app) promptServerList(ctx context.Context) (commands.ServerListResult, error) {
	respCh := make(chan serverListResult, 1)
	a.send(serverListRequestMsg{Response: respCh})
	select {
	case <-ctx.Done():
		return commands.ServerListResult{}, ctx.Err()
	case resp := <-respCh:
		return commands.ServerListResult{
			Action:    resp.Action,
			EditAlias: resp.EditAlias,
		}, resp.Err
	}
}

func (a *app) promptModelList(ctx context.Context) (string, error) {
	respCh := make(chan modelListResult, 1)
	a.send(modelListRequestMsg{Response: respCh})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case resp := <-respCh:
		return resp.Alias, resp.Err
	}
}

func (a *app) promptAskUser(ctx context.Context, toolName string, question *tool.AskUserQuestion) tool.AskUserResponse {
	if question == nil {
		return tool.AskUserResponse{Canceled: true, Error: "question payload is missing"}
	}
	respCh := make(chan askUserResult, 1)
	a.send(askUserRequestMsg{
		ToolName: toolName,
		Question: *question,
		Response: respCh,
	})
	select {
	case <-ctx.Done():
		return tool.AskUserResponse{Canceled: true, Error: ctx.Err().Error()}
	case resp := <-respCh:
		return resp.Response
	}
}

func (a *app) promptAskUserBatch(ctx context.Context, toolName string, questions []tool.AskUserQuestion) tool.AskUserBatchResponse {
	if len(questions) == 0 {
		return tool.AskUserBatchResponse{Canceled: true, Error: "question payload is missing"}
	}
	respCh := make(chan askUserBatchResult, 1)
	a.send(askUserBatchRequestMsg{
		ToolName:  toolName,
		Questions: append([]tool.AskUserQuestion(nil), questions...),
		Response:  respCh,
	})
	select {
	case <-ctx.Done():
		return tool.AskUserBatchResponse{Canceled: true, Error: ctx.Err().Error()}
	case resp := <-respCh:
		return resp.Response
	}
}

func (a *app) applyApprovalRule(toolName string, input json.RawMessage, scope approvalScope) error {
	if scope != approvalScopeSession && scope != approvalScopePermanent {
		return nil
	}
	ruleValue, ok := buildApprovalRuleValue(toolName, input)
	if !ok {
		return nil
	}

	if a.cfg.State != nil {
		a.cfg.State.AddSessionPermissionRule(types.PermissionRule{
			Source:       types.RuleSourceSession,
			RuleBehavior: types.BehaviorAllow,
			RuleValue:    ruleValue,
		})
	}
	if scope == approvalScopePermanent {
		if err := permissions.AddPermissionRulesToSettings([]types.PermissionRuleValue{ruleValue}, types.BehaviorAllow); err != nil {
			return err
		}
		if a.cfg.Engine != nil {
			a.cfg.Engine.InvalidatePermissionRuleCache()
		}
	}
	return nil
}

func buildApprovalRuleValue(toolName string, input json.RawMessage) (types.PermissionRuleValue, bool) {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if toolName == "" {
		return types.PermissionRuleValue{}, false
	}
	rule := types.PermissionRuleValue{ToolName: toolName}
	if toolName == "bash" || toolName == "powershell" {
		command := tool.ExtractStringInput(input, "command")
		prefix := extractShellRulePrefix(toolName, command)
		if prefix == "" {
			return types.PermissionRuleValue{}, false
		}
		rule.RuleContent = scopedShellRuleContent(input, prefix+":*")
	}
	return rule, true
}

func scopedShellRuleContent(input json.RawMessage, commandRule string) string {
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return commandRule
	}
	var scopes []string
	for _, key := range []string{"server", "role", "channel", "as_user", "privilege_method"} {
		value, _ := obj[key].(string)
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		scopes = append(scopes, key+"="+strings.ReplaceAll(value, ";", "_"))
	}
	if len(scopes) == 0 {
		return commandRule
	}
	return "ctx[" + strings.Join(scopes, ";") + "]:" + commandRule
}

func extractShellRulePrefix(toolName, command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	if toolName == "bash" {
		if pfx, err := bashutils.GetBashCommandPrefix(context.Background(), command); err == nil && strings.TrimSpace(pfx) != "" {
			return strings.TrimSpace(pfx)
		}
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	subcommandCLI := map[string]bool{
		"git": true, "npm": true, "yarn": true, "pnpm": true, "cargo": true,
		"go": true, "docker": true, "kubectl": true, "apt": true, "apt-get": true,
		"brew": true, "python": true, "python3": true, "pip": true, "pip3": true,
		"sudo": true, "su": true, "pwsh": true, "powershell": true,
	}
	if subcommandCLI[strings.ToLower(fields[0])] && len(fields) >= 2 {
		return strings.TrimSpace(fields[0] + " " + fields[1])
	}
	return strings.TrimSpace(fields[0])
}

func formatAskUserPromptOptions(options []tool.AskUserOption) string {
	if len(options) == 0 {
		return ""
	}
	lines := make([]string, 0, len(options))
	for i, opt := range options {
		label := strings.TrimSpace(opt.Label)
		if label == "" {
			label = strings.TrimSpace(opt.Value)
		}
		if label == "" {
			continue
		}
		line := fmt.Sprintf("%d. %s", i+1, label)
		if desc := strings.TrimSpace(opt.Description); desc != "" {
			line += " - " + desc
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (a *app) handleSlashCommand(ctx context.Context, line string) (quit bool, loadedSession bool, err error) {
	if strings.EqualFold(strings.TrimSpace(line), "/clear") {
		a.send(presentationEventMsg{Event: presentation.Event{Kind: presentation.EventViewCleared}})
		if a.cfg.Engine != nil {
			a.cfg.Engine.EmitTrace("slash_command", "", map[string]any{"name": "clear", "is_error": false})
			a.cfg.Engine.EmitTrace("view.cleared", "", map[string]any{"reason": "slash_command"})
		}
		return false, false, nil
	}

	sink := &lineCaptureWriter{
		onLine: func(s string) {
			s = strings.TrimSpace(s)
			if s == "" {
				return
			}
			a.send(presentationEventMsg{
				Event: presentation.Event{
					Kind: presentation.EventSystem,
					Text: s,
				},
			})
		},
	}
	cmdCtx := commands.CommandContext{
		Context:                ctx,
		Stdout:                 sink,
		State:                  a.cfg.State,
		Registry:               a.cfg.Registry,
		Engine:                 a.cfg.Engine,
		ModelPath:              a.cfg.ModelPath,
		SettingsPath:           a.cfg.SettingsPath,
		WorkDir:                a.cfg.WorkDir,
		Memory:                 a.cfg.Memory,
		MCP:                    a.cfg.MCP,
		LSP:                    a.cfg.LSP,
		Workspace:              a.cfg.Workspace,
		Skills:                 a.cfg.Skills,
		Connector:              a.cfg.Connector,
		Credentials:            a.cfg.Credentials,
		MCPReload:              a.cfg.MCPReload,
		ModelHandler:           a.handleModelCommand,
		ServerFormPrompt:       a.promptServerForm,
		ServerListPrompt:       a.promptServerList,
		ConnectorSearchPrompt:  a.promptConnectorSearch,
		ConnectorInstallPrompt: a.promptConnectorInstall,
	}
	quit, err = commands.Dispatch(line, cmdCtx)
	sink.Flush()
	if a.cfg.Engine != nil {
		cmdName := ""
		if parts := strings.Fields(strings.TrimSpace(line)); len(parts) > 0 {
			cmdName = strings.TrimPrefix(parts[0], "/")
		}
		a.cfg.Engine.EmitTrace("slash_command", "", map[string]any{
			"name":     cmdName,
			"is_error": err != nil,
		})
	}
	if err != nil {
		return quit, false, err
	}
	loadedSession = isSessionLoadCommand(line)
	return quit, loadedSession, nil
}

func (a *app) handleModelCommand(args []string) error {
	if len(args) == 0 {
		alias, err := a.promptModelList(a.ctx)
		if err != nil {
			return err
		}
		if alias != "" {
			if err := a.cfg.Registry.SetActive(alias); err != nil {
				return err
			}
			if err := a.cfg.Registry.Save(a.cfg.ModelPath); err != nil {
				return err
			}
			if err := a.refreshEngineLLM(); err != nil {
				return err
			}
			a.send(presentationEventMsg{
				Event: presentation.Event{
					Kind: presentation.EventSystem,
					Text: fmt.Sprintf("active model switched to: %s", alias),
				},
			})
		}
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: /model show <alias>")
		}
		info, err := a.cfg.Registry.Show(args[1])
		if err != nil {
			return err
		}
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind: presentation.EventSystem,
				Text: info,
			},
		})
		return nil
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: /model rm <alias>")
		}
		if err := a.cfg.Registry.Remove(args[1]); err != nil {
			return err
		}
		if err := a.cfg.Registry.Save(a.cfg.ModelPath); err != nil {
			return err
		}
		return a.refreshEngineLLM()
	case "add":
		entry, err := parseModelAddArgs(args)
		if err != nil {
			return err
		}
		if err := a.cfg.Registry.Add(entry); err != nil {
			return err
		}
		if err := a.cfg.Registry.Save(a.cfg.ModelPath); err != nil {
			return err
		}
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind: presentation.EventSystem,
				Text: fmt.Sprintf("added model alias: %s", entry.Alias),
			},
		})
		return nil
	default:
		target := args[0]
		if _, err := strconv.Atoi(target); err != nil {
			// ResolveSwitchToken handles alias lookup.
		}
		entry, err := a.cfg.Registry.ResolveSwitchToken(target)
		if err != nil {
			return err
		}
		if err := a.cfg.Registry.SetActive(entry.Alias); err != nil {
			return err
		}
		if err := a.cfg.Registry.Save(a.cfg.ModelPath); err != nil {
			return err
		}
		if err := a.refreshEngineLLM(); err != nil {
			return err
		}
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind: presentation.EventSystem,
				Text: fmt.Sprintf("active model: %s (%s)", entry.Alias, entry.Display),
			},
		})
		return nil
	}
}

func parseModelAddArgs(args []string) (llm.ModelEntry, error) {
	if len(args) < 4 {
		return llm.ModelEntry{}, fmt.Errorf("usage: /model add <provider> <alias> <model-id> [base-url] [api-key-env] [display] [context-window]")
	}
	provider := llm.Provider(strings.TrimSpace(args[1]))
	if provider != llm.ProviderAnthropic && provider != llm.ProviderGemini && provider != llm.ProviderOpenAICompat {
		return llm.ModelEntry{}, fmt.Errorf("invalid provider: %s", args[1])
	}
	alias := strings.TrimSpace(args[2])
	modelID := strings.TrimSpace(args[3])
	if alias == "" || modelID == "" {
		return llm.ModelEntry{}, fmt.Errorf("alias and model-id are required")
	}

	var (
		baseURL   string
		apiKeyEnv string
		display   string
		ctxWin    int
	)
	if provider == llm.ProviderOpenAICompat {
		baseURL = "https://api.openai.com/v1"
	}
	switch provider {
	case llm.ProviderAnthropic:
		apiKeyEnv = "ANTHROPIC_API_KEY"
	case llm.ProviderGemini:
		apiKeyEnv = "GEMINI_API_KEY"
	}
	display = modelID

	rest := args[4:]
	if len(rest) > 0 {
		if provider == llm.ProviderOpenAICompat {
			baseURL = strings.TrimSpace(rest[0])
			rest = rest[1:]
		}
	}
	if len(rest) > 0 {
		apiKeyEnv = strings.TrimSpace(rest[0])
		rest = rest[1:]
	}
	if len(rest) > 0 {
		display = strings.TrimSpace(rest[0])
		rest = rest[1:]
	}
	if len(rest) > 0 {
		n, err := strconv.Atoi(strings.TrimSpace(rest[0]))
		if err != nil {
			return llm.ModelEntry{}, fmt.Errorf("invalid context window: %s", rest[0])
		}
		ctxWin = n
	}
	return llm.ModelEntry{
		Alias:      alias,
		ModelID:    modelID,
		Provider:   provider,
		BaseURL:    baseURL,
		APIKeyEnv:  apiKeyEnv,
		Display:    display,
		ContextWin: ctxWin,
		Caps:       llm.DefaultCapsForProvider(provider),
	}, nil
}

func (a *app) refreshEngineLLM() error {
	client, err := a.cfg.Registry.Build()
	if err != nil {
		a.cfg.Engine.SetLLM(llm.NewErrorLLM("registry", llm.Caps{}, err))
		return err
	}
	a.cfg.Engine.SetLLM(client)
	if active, ok := a.cfg.Registry.ActiveEntry(); ok && a.cfg.State != nil {
		a.cfg.State.SetActiveModel(active.Alias, active.Display, active.ContextWin)
		a.cfg.State.SetActiveModelProvider(string(active.Provider))
	}
	a.emitFooter()
	return nil
}

func (a *app) runPlannedSteps(ctx context.Context, steps []query.PlannedStep) {
	if len(steps) == 0 || a.cfg.Engine == nil {
		return
	}
	sessionID := todostore.SessionID("")
	if a.cfg.State != nil {
		sessionID = todostore.SessionID(a.cfg.State.SessionID())
	}

	existing, _ := todostore.Load(sessionID)
	if a.cfg.State != nil {
		stateTodos := a.cfg.State.Todos(sessionID)
		if len(stateTodos) > 0 {
			existing = stateTodos
		}
	}
	todos := todostore.SyncForSteps(existing, toTodoSteps(steps))
	_ = persistSessionTodos(a.cfg, sessionID, todos)

	for i, step := range steps {
		idx := i + 1
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind:      presentation.EventPlanStep,
				StepIndex: idx,
				StepTotal: len(steps),
				ToolName:  step.Tool,
				Text:      step.Prompt,
			},
		})

		ok, err := a.promptConfirm(ctx, fmt.Sprintf("run step %d/%d? [y/N]", idx, len(steps)), true)
		decision := "deny"
		if ok {
			decision = "allow"
		}
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind:      presentation.EventPlanDecision,
				StepIndex: idx,
				StepTotal: len(steps),
				Decision:  decision,
			},
		})
		if err != nil {
			a.send(presentationEventMsg{
				Event: presentation.Event{
					Kind: presentation.EventError,
					Text: fmt.Sprintf("step approval failed: %v", err),
				},
			})
			return
		}
		if !ok {
			a.send(presentationEventMsg{
				Event: presentation.Event{
					Kind: presentation.EventNotice,
					Text: fmt.Sprintf("plan execution stopped at step %d/%d", idx, len(steps)),
				},
			})
			_ = persistSessionTodos(a.cfg, sessionID, todos)
			return
		}

		todos[i].Status = types.TodoStatusInProgress
		_ = persistSessionTodos(a.cfg, sessionID, todos)

		result, err := a.cfg.Engine.ExecutePlannedStep(ctx, step)
		if err != nil {
			todos[i].Status = types.TodoStatusPending
			_ = persistSessionTodos(a.cfg, sessionID, todos)
			a.send(presentationEventMsg{
				Event: presentation.Event{
					Kind:      presentation.EventPlanResult,
					StepIndex: idx,
					StepTotal: len(steps),
					Text:      "failed: " + err.Error(),
				},
			})
			return
		}

		todos[i].Status = types.TodoStatusCompleted
		_ = persistSessionTodos(a.cfg, sessionID, todos)
		a.send(presentationEventMsg{
			Event: presentation.Event{
				Kind:      presentation.EventPlanResult,
				StepIndex: idx,
				StepTotal: len(steps),
				Text:      "completed: " + truncatePlanStepOutput(result),
			},
		})
	}
	a.send(presentationEventMsg{
		Event: presentation.Event{
			Kind: presentation.EventNotice,
			Text: fmt.Sprintf("plan execution completed (%d/%d)", len(steps), len(steps)),
		},
	})
}

func toTodoSteps(steps []query.PlannedStep) []todostore.Step {
	out := make([]todostore.Step, 0, len(steps))
	for _, step := range steps {
		out = append(out, todostore.Step{
			Tool:   step.Tool,
			Prompt: step.Prompt,
		})
	}
	return out
}

func persistSessionTodos(cfg Config, sessionID string, todos []types.TodoItem) error {
	if err := todostore.Save(sessionID, todos); err != nil {
		return err
	}
	if cfg.State != nil {
		cfg.State.SetTodos(sessionID, todostore.NormalizeForStorage(todos))
	}
	return nil
}

func truncatePlanStepOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500] + " ... (truncated)"
	}
	if s == "" {
		return "(no output)"
	}
	return s
}

func isSessionLoadCommand(line string) bool {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "/"))
	return len(fields) >= 3 && strings.EqualFold(fields[0], "session") && strings.EqualFold(fields[1], "load")
}

type lineCaptureWriter struct {
	mu     sync.Mutex
	buf    strings.Builder
	onLine func(string)
}

func (w *lineCaptureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, b := range p {
		if b == '\n' {
			w.flushLineLocked()
			continue
		}
		w.buf.WriteByte(b)
	}
	return len(p), nil
}

func (w *lineCaptureWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushLineLocked()
}

func (w *lineCaptureWriter) flushLineLocked() {
	if w.onLine == nil {
		w.buf.Reset()
		return
	}
	line := strings.TrimRight(w.buf.String(), "\r")
	w.buf.Reset()
	if line == "" {
		return
	}
	w.onLine(line)
}

// formatModelTable renders the model registry as a color-highlighted table.
// The active model row gets accent color and a ● marker.
func formatModelTable(reg *llm.Registry, theme uiTheme) string {
	if theme.DisableANSI {
		return reg.FormatTable()
	}

	list := reg.List()
	active := reg.ActiveAlias()

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ToolUseTitleColor)).Bold(true)
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.AssistantMarkerColor)).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.BodyColor))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.MutedColor))

	header := fmt.Sprintf("%-4s %-2s %-22s %-13s %-28s %-12s", "#", " ", "Alias", "Provider", "ModelID", "Status")
	var sb strings.Builder
	sb.WriteString(headerStyle.Render(header))
	sb.WriteString("\n")

	for i, entry := range list {
		status := reg.EntryStatus(entry)
		if entry.Alias == active {
			row := fmt.Sprintf("%-4d %-2s %-22s %-13s %-28s %-12s",
				i+1, "●", entry.Alias, entry.Provider, entry.ModelID, status)
			sb.WriteString(activeStyle.Render(row))
		} else {
			mark := "○"
			row := fmt.Sprintf("%-4d %-2s %-22s %-13s %-28s %-12s",
				i+1, mark, entry.Alias, entry.Provider, entry.ModelID, status)
			style := bodyStyle
			if status != "ready" {
				style = mutedStyle
			}
			sb.WriteString(style.Render(row))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
