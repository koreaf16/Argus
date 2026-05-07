package tui

import (
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/query"
	"github.com/koreaf16/argus/internal/repl/commands"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/state"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tui/markdown"
	"github.com/koreaf16/argus/internal/tui/toolui"
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils/permissions"
	"github.com/mattn/go-runewidth"
)

type submitStartedMsg struct{}
type submitFinishedMsg struct{}
type quitRequestedMsg struct{}

type queryEventMsg struct {
	Event query.UIEvent
}

type presentationEventMsg struct {
	Event presentation.Event
}

type footerStateMsg struct {
	Footer presentation.FooterState
}

type settingsUpdatedMsg struct {
	Settings UISettings
}

type startupInspectDoneMsg struct {
	text string
	cwd  string
}

type approvalScope string

const (
	approvalScopeOnce      approvalScope = "once"
	approvalScopeSession   approvalScope = "session"
	approvalScopePermanent approvalScope = "permanent"
)

type decisionResult struct {
	Allow bool
	Scope approvalScope
	Err   error
}

type passwordResult struct {
	Value string
	Err   error
}

type askUserResult struct {
	Response tool.AskUserResponse
}

type askUserBatchResult struct {
	Response tool.AskUserBatchResponse
}

type serverFormResult struct {
	Result commands.ServerFormResult
	Err    error
}

type serverFormRequestMsg struct {
	EditAlias string // non-empty when editing an existing server
	Response  chan serverFormResult
}

type approvalRequestMsg struct {
	ToolName string
	Input    string
	Response chan decisionResult
}

type confirmRequestMsg struct {
	Prompt    string
	DefaultNo bool
	Response  chan decisionResult
}

type passwordRequestMsg struct {
	Prompt   string
	Response chan passwordResult
}

type askUserRequestMsg struct {
	ToolName string
	Question tool.AskUserQuestion
	Response chan askUserResult
}

type askUserBatchRequestMsg struct {
	ToolName  string
	Questions []tool.AskUserQuestion
	Response  chan askUserBatchResult
}

type transcriptEntry struct {
	Kind       string
	Title      string
	Body       string
	StreamBody string // streaming tool output (bash/powershell stdout)
	ToolName   string
	TaskID     string

	// tool_group 전용 필드
	Collapsed   bool
	SearchCount int
	ReadCount   int
	ListCount   int
	LastHint    string
	SubEntries  []transcriptEntry
	IsActive    bool

	// tool_use 실패 신호 (헤드라인 색을 적색으로 분기)
	Failed bool

	// bash/powershell 반복 실행 횟수
	BashRepeatCount int

	// thinking / parallel_group 전용: 경과시간 표시
	StartTime time.Time
	EndTime   time.Time

	// parallel_group 전용 필드
	ParallelSubByTaskID map[string]int // taskID → SubEntries 인덱스

	Interactive toolui.InteractiveModel
}

type uiModel struct {
	app   *app
	cfg   Config
	theme uiTheme
	ui    UISettings

	width  int
	height int

	input    textarea.Model
	viewport viewport.Model
	footer   presentation.FooterState

	entries []transcriptEntry

	assistantOpen         bool
	assistantStreamIdx    int
	assistantLastDelta    time.Time
	assistantFlushedLines int
	assistantFlushedText  string
	thinkingOpen          bool
	thinkingStreamIdx     int
	thinkingLastDelta     time.Time
	thinkingFlushedLines  int
	thinkingFlushedText   string
	toolUseOpen           bool
	toolUseStreamIdx      int
	busy          bool
	busyStartedAt time.Time
	queuedPrompts []queuedPrompt
	tokenInputSnap        int
	tokenOutputSnap       int
	tokenThinkingSnap     int
	prevTokenInputSnap    int
	prevTokenOutputSnap   int
	prevTokenThinkingSnap int
	// activeTokenKind: 현재 spinner 라인에 표시할 토큰 종류.
	activeTokenKind string

	modalQueue []modalState
	modal      modalState

	slashMatches []slashSuggestion
	slashCursor  int

	inputHistory []inputHistoryEntry
	historyIdx   int
	historyCycle bool
	historyDraft *inputHistoryEntry

	activePastes map[int]pastedText
	pasteSeq     int

	maxEntries int

	lastPrintedIdx int
	transcriptMode bool

	animFrame     int
	animStartedAt time.Time
	ticking       bool
	spinnerVerb   string

	// 최신 태스크 리스트 스냅샷 (Thinking 라인 위에 고정 표시용)
	latestTodos []types.TodoItem

	// activeTool: 현재 활성 상호작용 모델
	activeTool        toolui.InteractiveModel
	toolFocused       bool
	toolEntryByTaskID map[string]int
	parallelSubLookup map[string]parallelSubRef // taskID → {parentIdx, childIdx}
}

// parallelSubRef는 parallel_group 내 자식 entry의 위치를 나타낸다.
type parallelSubRef struct {
	parentIdx int
	childIdx  int
}

func newUIModel(a *app, cfg Config) uiModel {
	resolvedUI := ResolveUISettings(cfg.UI, cfg.Theme, cfg.AIDebug)
	theme := resolveUIThemeWithVariant(resolvedUI.Theme, resolvedUI.Variant, cfg.AIDebug)

	in := textarea.New()
	in.Placeholder = "Type your message or @path/to/file"
	in.Prompt = ""
	in.ShowLineNumbers = false
	in.CharLimit = 0
	in.SetHeight(1)
	in.Focus()
	in.Cursor.SetMode(cursor.CursorHide)

	// Disable internal cursor rendering to avoid conflict with hardware cursor
	focused, blurred := textarea.DefaultStyles()
	focused.Base = lipgloss.NewStyle()
	focused.CursorLine = lipgloss.NewStyle()
	focused.CursorLineNumber = lipgloss.NewStyle()
	focused.EndOfBuffer = lipgloss.NewStyle()
	focused.LineNumber = lipgloss.NewStyle()
	focused.Prompt = lipgloss.NewStyle()
	focused.Text = lipgloss.NewStyle()
	focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.MutedColor))
	blurred = focused
	in.FocusedStyle = focused
	in.BlurredStyle = blurred

	vp := viewport.New(0, 0)

	m := uiModel{
		app:                a,
		cfg:                cfg,
		theme:              theme,
		ui:                 resolvedUI,
		input:              in,
		viewport:           vp,
		entries:            make([]transcriptEntry, 0, 256),
		assistantStreamIdx: -1,
		thinkingStreamIdx:  -1,
		toolUseStreamIdx:   -1,
		footer:             presentation.BuildFooterState(cfg.State, cfg.WorkDir),
		historyIdx:         -1,
		maxEntries:         4000,
		toolEntryByTaskID:  make(map[string]int),
	}
	for _, evt := range presentation.ReplayEvents(cfg.Engine.Messages()) {
		m.applyPresentationEvent(evt)
	}
	m.lastPrintedIdx = len(m.entries)
	m.refreshViewport(true)
	return m
}

type animationTickMsg struct{}

func doTick(intervalMS int) tea.Cmd {
	if intervalMS <= 0 {
		intervalMS = DefaultMotionTick
	}
	return tea.Tick(time.Duration(intervalMS)*time.Millisecond, func(t time.Time) tea.Msg {
		return animationTickMsg{}
	})
}

func (m uiModel) hasActiveEntries() bool {
	for _, e := range m.entries {
		if e.IsActive {
			return true
		}
		for _, sub := range e.SubEntries {
			if sub.IsActive {
				return true
			}
		}
	}
	return false
}

func (m uiModel) tickIntervalMS() int {
	ms := m.ui.Motion.TickMS
	if ms <= 0 {
		ms = DefaultMotionTick
	}
	if !m.ui.Motion.Enabled || m.ui.Motion.Reduced || m.ui.Motion.Level == "static" {
		if ms < 250 {
			ms = 250
		}
	}
	if ms < 20 {
		ms = 20
	}
	if ms > 1000 {
		ms = 1000
	}
	return ms
}

func (m uiModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, tea.ShowCursor)

	// 애니메이션 초기화
	m.animStartedAt = time.Now()
	m.ticking = true
	cmds = append(cmds, doTick(m.tickIntervalMS()))

	cmds = append(cmds, tea.Printf("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l"))

	cmds = append(cmds, tea.Println(renderLogoBlock(m.cfg)))

	if m.cfg.Workspace != nil {
		activeAlias := workspace.LocalAlias
		if m.cfg.State != nil {
			m.cfg.State.SetActiveWorkspace(activeAlias)
			m.cfg.State.SetActiveTarget(state.ActiveTarget{Alias: activeAlias, CWD: m.cfg.WorkDir})
		}

		if m.app != nil && m.app.ctx != nil {
			cmds = append(cmds, func() tea.Msg {
				m.cfg.Workspace.CollectLocalSystemHeader(m.app.ctx)
				return nil
			})
		}

		cmds = append(cmds, func() tea.Msg {
			return footerStateMsg{
				Footer: presentation.BuildFooterState(m.cfg.State, m.cfg.WorkDir),
			}
		})
	}

	for _, e := range m.entries {
		cmds = append(cmds, tea.Println(m.renderTranscriptEntryAt(-1, e)))
	}
	return tea.Batch(cmds...)
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.MouseMsg); ok {
		return m, nil
	}

	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.resize()
		return m, nil
	case quitRequestedMsg:
		return m, tea.Quit
	case submitStartedMsg:
		m.busy = true
		m.busyStartedAt = time.Now()
		verbs := constants.GetSpinnerVerbs()
		m.spinnerVerb = verbs[time.Now().UnixNano()%int64(len(verbs))]
		m.app.cfg.Engine.ResetBudget()
		m.tokenInputSnap = 0
		m.tokenOutputSnap = 0
		m.animStartedAt = time.Now()
		m.ticking = true
		return m, doTick(m.tickIntervalMS())
	case submitFinishedMsg:
		m.busy = false
		m.busyStartedAt = time.Time{}
		footerCmd := func() tea.Msg { return m.app.buildFooterMsg() }
		if flushCmd := m.dequeueAndFlush(); flushCmd != nil {
			return m, tea.Batch(footerCmd, flushCmd)
		}
		return m, footerCmd
	case queryEventMsg:
		if evt, ok := presentation.FromUIEvent(v.Event); ok {
			prevIdx := m.assistantStreamIdx
			prevThinkingIdx := m.thinkingStreamIdx
			m.applyPresentationEvent(evt)
			var tickCmd tea.Cmd
			if evt.Kind == presentation.EventToolUse && !m.ticking {
				m.animStartedAt = time.Now()
				m.ticking = true
				tickCmd = doTick(m.tickIntervalMS())
			}
			return m, tea.Batch(m.handleEventFinalization(evt), m.scrollbackCmd(evt, prevIdx, prevThinkingIdx), tickCmd)
		}
		return m, nil
	case presentationEventMsg:
		prevIdx := m.assistantStreamIdx
		prevThinkingIdx := m.thinkingStreamIdx
		m.applyPresentationEvent(v.Event)
		var tickCmd tea.Cmd
		if v.Event.Kind == presentation.EventToolUse && !m.ticking {
			m.animStartedAt = time.Now()
			m.ticking = true
			tickCmd = doTick(m.tickIntervalMS())
		}
		return m, tea.Batch(m.handleEventFinalization(v.Event), m.scrollbackCmd(v.Event, prevIdx, prevThinkingIdx), tickCmd)
	case footerStateMsg:
		m.footer = v.Footer
		return m, nil
	case settingsUpdatedMsg:
		m.UpdateUISettings(v.Settings)
		return m, nil
	case startupInspectDoneMsg:
		evt := presentation.Event{Kind: presentation.EventSystem, Text: v.text}
		m.applyPresentationEvent(evt)
		footerCmd := func() tea.Msg { return m.app.buildFooterMsg() }
		return m, footerCmd
	case animationTickMsg:
		// 화면에 표시할 스트리밍 데이터가 남아있거나 활성 작업이 있다면 틱을 유지합니다.
		hasStreamingData := m.lastPrintedIdx < len(m.entries)
		if !m.busy && m.activeTool == nil && !m.hasActiveEntries() && !hasStreamingData {
			m.ticking = false
			return m, nil
		}

		m.ticking = true
		if m.animStartedAt.IsZero() {
			m.animStartedAt = time.Now()
		}
		if m.ui.Motion.Enabled {
			// 시간 기반 프레임 계산으로 속도 안정화
			elapsed := time.Since(m.animStartedAt)
			interval := int64(m.tickIntervalMS())
			m.animFrame = int(elapsed.Milliseconds() / interval)
		}
		if m.busy {
			m.tokenInputSnap, m.tokenOutputSnap, m.tokenThinkingSnap = m.app.cfg.Engine.TokenSnapshot()
		}

		// 모든 활성 InteractiveModel에 TickMsg 전파
		tickMsg := spinner.TickMsg{Time: time.Now()}

		if m.activeTool != nil && m.ui.Motion.Enabled {
			m.activeTool, _ = m.activeTool.Update(tickMsg)
		}

		// Transcript 내의 모든 활성 도구 업데이트 (병렬 그룹 및 중첩 구조 포함)
		for i := range m.entries {
			e := &m.entries[i]
			if e.IsActive && e.Interactive != nil {
				e.Interactive, _ = e.Interactive.Update(tickMsg)
			}
			for j := range e.SubEntries {
				sub := &e.SubEntries[j]
				if sub.IsActive && sub.Interactive != nil {
					sub.Interactive, _ = sub.Interactive.Update(tickMsg)
				}
			}
		}

		return m, tea.Batch(doTick(m.tickIntervalMS()))
	case approvalRequestMsg:
		m.enqueueModal(modalState{
			Kind:      modalApproval,
			Title:     "Tool Permission",
			Prompt:    "Allow this tool execution?",
			ToolName:  v.ToolName,
			InputJSON: v.Input,
			DecisionC: v.Response,
		})
		return m, nil
	case confirmRequestMsg:
		cursor := 0
		if v.DefaultNo {
			cursor = 1
		}
		m.enqueueModal(modalState{
			Kind:      modalConfirm,
			Title:     "Confirm",
			Prompt:    v.Prompt,
			DefaultNo: v.DefaultNo,
			AskCursor: cursor,
			DecisionC: v.Response,
		})
		return m, nil
	case passwordRequestMsg:
		m.enqueueModal(modalState{
			Kind:      modalPassword,
			Title:     "Password Required",
			Prompt:    v.Prompt,
			PasswordC: v.Response,
		})
		return m, nil
	case askUserRequestMsg:
		m.enqueueModal(modalState{
			Kind:     modalAskUser,
			Title:    "Question",
			ToolName: v.ToolName,
			Question: &v.Question,
			AskUserC: v.Response,
		})
		return m, nil
	case askUserBatchRequestMsg:
		m.enqueueModal(modalState{
			Kind:          modalAskUserBatch,
			Title:         "Questions",
			ToolName:      v.ToolName,
			Questions:     append([]tool.AskUserQuestion(nil), v.Questions...),
			AskUserBatchC: v.Response,
		})
		return m, nil
	case serverFormRequestMsg:
		sf := &serverFormState{PortStr: "22"}
		title := "Add Server"
		if v.EditAlias != "" {
			title = "Edit Server"
			if m.app.cfg.Workspace != nil {
				if entry, ok := m.app.cfg.Workspace.Registry().Get(v.EditAlias); ok {
					sf = serverFormStateFromEntry(entry, m.app.cfg.Workspace)
				}
			}
		}
		m.enqueueModal(modalState{
			Kind:        modalServerForm,
			Title:       title,
			ServerForm:  sf,
			ServerFormC: v.Response,
		})
		return m, nil
	case serverListRequestMsg:
		var entries []workspace.StatusEntry
		if m.app.cfg.Workspace != nil {
			entries = m.app.cfg.Workspace.Status()
		}
		m.enqueueModal(modalState{
			Kind:  modalServerList,
			Title: "Servers",
			ServerList: &serverListState{
				Entries: entries,
				Cursor:  0,
			},
			ServerListC: v.Response,
		})
		return m, nil
	case modelListRequestMsg:
		var entries []modelListEntry
		active := ""
		if m.app.cfg.Registry != nil {
			list := m.app.cfg.Registry.List()
			active = m.app.cfg.Registry.ActiveAlias()
			for _, e := range list {
				entries = append(entries, modelListEntry{
					Alias:    e.Alias,
					Provider: string(e.Provider),
					ModelID:  e.ModelID,
					Status:   m.app.cfg.Registry.EntryStatus(e),
					Active:   e.Alias == active,
				})
			}
		}
		cursor := 0
		for i, e := range entries {
			if e.Active {
				cursor = i
				break
			}
		}
		m.enqueueModal(modalState{
			Kind:       modalModelList,
			Title:      "Model Selection",
			ModelList:  &modelListState{Entries: entries, Cursor: cursor},
			ModelListC: v.Response,
		})
		return m, nil
	case connectorSearchRequestMsg:
		m.enqueueModal(modalState{
			Kind: modalConnectorSearch,
			ConnectorSearch: &connectorSearchState{
				Query:   v.Query,
				Results: v.Results,
			},
			ConnectorSearchC: v.Response,
		})
		return m, nil
	case connectorInstallRequestMsg:
		envValues := make([]string, len(v.Spec.EnvPrompts))
		m.enqueueModal(modalState{
			Kind: modalConnectorInstall,
			ConnectorInstall: &connectorInstallState{
				Spec:      v.Spec,
				EnvValues: envValues,
			},
			ConnectorInstallC: v.Response,
		})
		return m, nil
	case serverListRefreshMsg:
		if m.modal.Kind == modalServerList {
			m.modal.ServerList.ErrorMsg = v.ErrorMsg
			m.refreshServerListEntries()
		}
		return m, nil
	case tea.KeyMsg:
		if m.modal.Kind != modalNone {
			m.handleModalKey(v)
			return m, nil
		}

		if m.toolFocused && m.activeTool != nil {
			switch v.String() {
			case "tab", "esc":
				m.toolFocused = false
				m.activeTool.SetFocus(false)
				m.input.Focus()
				return m, nil
			case "ctrl+o":
				m.activeTool.SetExpanded(!m.activeTool.IsExpanded())
				return m, nil
			default:
				var cmd tea.Cmd
				m.activeTool, cmd = m.activeTool.Update(msg)
				return m, cmd
			}
		}

		if v.Paste && len(v.Runes) > 0 {
			m.insertPastedText(string(v.Runes))
			return m, nil
		}

		switch v.String() {
		case "esc":
			if strings.TrimSpace(m.input.Value()) == "" && len(m.queuedPrompts) > 0 {
				var parts []string
				for _, q := range m.queuedPrompts {
					parts = append(parts, q.Submission.Display)
				}
				m.input.SetValue(strings.Join(parts, "\n\n"))
				m.input.SetCursor(len(m.input.Value()))
				m.queuedPrompts = nil
				m.refreshSlashSuggestions()
				m.resize()
				return m, nil
			}
			if m.busy {
				m.app.cancelCurrentSubmit()
			}
			return m, nil
		case "ctrl+d":
			if m.busy && m.activeTool != nil {
				var cmd tea.Cmd
				m.activeTool, cmd = m.activeTool.Update(v)
				return m, cmd
			}
		case "ctrl+o":
			if m.activeTool != nil {
				m.activeTool.SetExpanded(!m.activeTool.IsExpanded())
			} else {
				m.toggleLastToolGroup()
			}
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+r":
			m.cycleInputHistoryBackward()
			m.refreshSlashSuggestions()
			m.resize()
			return m, nil
		case "tab":
			if m.applySlashSuggestion() {
				m.resize()
				return m, nil
			}
			if m.activeTool != nil {
				m.toolFocused = true
				m.activeTool.SetFocus(true)
				m.input.Blur()
				return m, nil
			}
		case "ctrl+n":
			if m.moveSlashSuggestion(1) {
				return m, nil
			}
		case "ctrl+p":
			if m.moveSlashSuggestion(-1) {
				return m, nil
			}
		case "up":
			if m.shouldCycleInputHistoryBackwardOnUp() {
				m.cycleInputHistoryBackward()
				m.refreshSlashSuggestions()
				m.resize()
				return m, nil
			}
		case "down":
			if m.shouldCycleInputHistoryForwardOnDown() {
				m.cycleInputHistoryForward()
				m.refreshSlashSuggestions()
				m.resize()
				return m, nil
			}
		case "enter":
			raw := m.input.Value()
			if strings.TrimSpace(raw) == "" {
				return m, nil
			}
			submission := m.buildPromptSubmission(raw)
			isSlash := strings.HasPrefix(submission.Display, "/")
			if m.busy {
				m.queuedPrompts = append(m.queuedPrompts, queuedPrompt{
					Submission: submission,
					IsSlash:    isSlash,
					EnqueuedAt: time.Now(),
				})
				m.pushInputHistory(raw)
				m.resetHistoryCycle()
				m.input.SetValue("")
				m.input.SetCursor(0)
				m.activePastes = nil
				m.refreshSlashSuggestions()
				m.resize()
				return m, nil
			}
			m.pushInputHistory(raw)
			m.resetHistoryCycle()
			m.input.SetValue("")
			m.input.SetCursor(0)
			m.activePastes = nil
			m.refreshSlashSuggestions()
			m.resize()
			m.app.submitPrompt(submission)
			return m, nil
		case "ctrl+j", "alt+enter":
			m.resetHistoryCycle()
			m.input.InsertRune('\n')
			m.refreshSlashSuggestions()
			m.resize()
			return m, nil
		case "shift+tab":
			m.cyclePermissionMode()
			return m, nil
		case "ctrl+y":
			m.toggleYoroMode()
			return m, nil
		case "ctrl+v":
			content, err := clipboard.ReadAll()
			if err == nil && m.insertPastedText(content) {
				return m, nil
			}
		}

		var vpCmd, inCmd tea.Cmd
		oldVal := m.input.Value()
		m.input, inCmd = m.input.Update(msg)
		newVal := m.input.Value()

		if len(v.Runes) > 0 || v.String() == "backspace" || v.String() == "delete" {
			m.resetHistoryCycle()
		}

		if oldVal != newVal {
			m.refreshSlashSuggestions()
		}

		if strings.Count(oldVal, "\n") != strings.Count(newVal, "\n") {
			m.resize()
		}

		return m, tea.Batch(vpCmd, inCmd)
	}
	return m, nil
}

func (m *uiModel) handleEventFinalization(evt presentation.Event) tea.Cmd {
	switch evt.Kind {
	case presentation.EventToolUse:
		if len(m.entries) == 0 {
			return nil
		}
		last := m.entries[len(m.entries)-1]
		if last.Kind == "tool_group" {
			return nil
		}
		if m.toolUseOpen && m.toolUseStreamIdx >= 0 {
			return nil
		}

		// [수정] 활성 상태인 도구 사용은 tea.Println으로 출력하지 않고 View() 영역에 둡니다.
		if last.IsActive {
			return nil
		}

		var cmds []tea.Cmd
		if len(m.entries) >= 2 {
			prev := m.entries[len(m.entries)-2]
			if prev.Kind == "tool_group" && !prev.IsActive {
				cmds = append(cmds, tea.Println("\n"+m.renderTranscriptEntryAt(-1, prev)))
			}
		}
		cmds = append(cmds, tea.Println("\n"+m.renderTranscriptEntryAt(-1, last)))
		return tea.Batch(cmds...)
	case presentation.EventToolResult:
		return nil

	case presentation.EventAssistantDone:
		return nil
	case presentation.EventUser, presentation.EventSystem, presentation.EventError,
		presentation.EventNotice,
		presentation.EventApprovalDecision, presentation.EventPlanResult, presentation.EventPasswordRequest,
		presentation.EventAskUserRequest:
		return nil
	case presentation.EventViewCleared:
		return tea.ClearScreen
	}
	return nil
}

func (m *uiModel) scrollbackCmd(evt presentation.Event, prevAssistantIdx, prevThinkingIdx int) tea.Cmd {
	switch evt.Kind {
	case presentation.EventAssistantDelta:
		var cmds []tea.Cmd
		if prevThinkingIdx >= 0 {
			if finalized := m.finalizeThinkingStream(prevThinkingIdx); finalized != nil {
				cmds = append(cmds, finalized)
			}
		}
		m.refreshViewport(true)
		cmds = append(cmds, m.flushStreamingLines(m.assistantStreamIdx, &m.assistantFlushedLines))
		return tea.Batch(cmds...)

	case presentation.EventThinkingDelta:
		if !m.ui.ViewThinking {
			return nil
		}
		m.refreshViewport(true)
		return m.flushStreamingLines(m.thinkingStreamIdx, &m.thinkingFlushedLines)

	case presentation.EventThinkingDone:
		if m.ui.ViewThinking {
			return m.finalizeThinkingStream(prevThinkingIdx)
		}
		m.refreshViewport(true)
		return nil

	case presentation.EventState, presentation.EventViewCleared:
		m.refreshViewport(true)
		return nil

	case presentation.EventAssistantDone:
		return m.finalizeAssistantStream(prevAssistantIdx)

	case presentation.EventToolUse:
		var cmds []tea.Cmd
		if prevThinkingIdx >= 0 {
			if finalized := m.finalizeThinkingStream(prevThinkingIdx); finalized != nil {
				cmds = append(cmds, finalized)
			}
		}

		if m.lastPrintedIdx < len(m.entries) {
			newLastPrinted := m.lastPrintedIdx
			for i := m.lastPrintedIdx; i < len(m.entries); i++ {
				entry := m.entries[i]
				if entry.IsActive || (entry.Kind == "tool_use" && m.toolUseOpen && m.toolUseStreamIdx == i) {
					break
				}
				skip := entry.Kind == "notice" || entry.Kind == "tool_use"
				if !skip {
					cmds = append(cmds, tea.Println("\n"+m.renderTranscriptEntryAt(-1, entry)))
				}
				newLastPrinted = i + 1
			}
			m.lastPrintedIdx = newLastPrinted
		}

		m.refreshViewport(true)
		return tea.Batch(cmds...)

	case presentation.EventToolResult:
		if m.lastPrintedIdx < len(m.entries) {
			var cmds []tea.Cmd
			newLastPrinted := m.lastPrintedIdx
			for i := m.lastPrintedIdx; i < len(m.entries); i++ {
				entry := m.entries[i]
				// [수정] 활성 상태인 엔트리가 있다면 출력을 멈추고 루프를 중단합니다.
				if entry.IsActive {
					break
				}
				rendered := m.renderTranscriptEntryAt(-1, entry)
				if rendered != "" {
					cmds = append(cmds, tea.Println("\n"+rendered))
				}
				newLastPrinted = i + 1
			}
			m.lastPrintedIdx = newLastPrinted
			m.refreshViewport(true)
			switch len(cmds) {
			case 0:
				return nil
			case 1:
				return cmds[0]
			default:
				return tea.Sequence(cmds...)
			}
		}
		m.refreshViewport(true)
		return nil
	default:
		var cmds []tea.Cmd
		if finalized := m.finalizeAssistantStream(prevAssistantIdx); finalized != nil {
			cmds = append(cmds, finalized)
		}

		if m.lastPrintedIdx < len(m.entries) {
			newLastPrinted := m.lastPrintedIdx
			for i := m.lastPrintedIdx; i < len(m.entries); i++ {
				entry := m.entries[i]
				// [수정] 활성 상태이거나 스트리밍 중인 도구 사용은 View() 영역에 보존합니다.
				if entry.IsActive || (entry.Kind == "tool_use" && m.toolUseOpen && m.toolUseStreamIdx == i) {
					break
				}
				skip := entry.Kind == "notice" || entry.Kind == "tool_use"
				if !skip {
					cmds = append(cmds, tea.Println("\n"+m.renderTranscriptEntryAt(-1, entry)))
				}
				newLastPrinted = i + 1
			}
			m.lastPrintedIdx = newLastPrinted
		}

		m.refreshViewport(true)

		switch len(cmds) {
		case 0:
			return nil
		case 1:
			return cmds[0]
		default:
			return tea.Sequence(cmds...)
		}
	}
}

func (m *uiModel) finalizeThinkingStream(streamIdx int) tea.Cmd {
	if streamIdx < 0 || streamIdx >= len(m.entries) {
		return nil
	}
	e := m.entries[streamIdx]
	if e.Kind != "thinking" {
		return nil
	}
	if m.lastPrintedIdx > streamIdx {
		return nil
	}
	m.lastPrintedIdx = streamIdx + 1
	m.refreshViewport(true)

	prevFlushedText := m.thinkingFlushedText
	m.thinkingFlushedLines = 0
	m.thinkingFlushedText = ""

	source := strings.TrimSpace(e.Body)
	if source == "" {
		return nil
	}

	termW := m.width - 3
	if termW < 20 {
		termW = 20
	}
	mdRendered := markdown.Render(source, termW, m.thinkingPalette())
	fullFormatted := m.renderAssistantEntry(strings.TrimRight(mdRendered, "\n"), termW)
	formattedLines := strings.Split(fullFormatted, "\n")

	var startIdx int
	if prevFlushedText != "" {
		prevMd := markdown.Render(prevFlushedText, termW, m.thinkingPalette())
		prevFormatted := m.renderAssistantEntry(strings.TrimRight(prevMd, "\n"), termW)
		startIdx = len(strings.Split(prevFormatted, "\n"))
	}

	if startIdx > len(formattedLines) {
		startIdx = len(formattedLines)
	}
	if startIdx >= len(formattedLines) {
		return nil
	}
	tail := strings.Join(formattedLines[startIdx:], "\n")
	if strings.TrimSpace(stripANSI(tail)) == "" {
		return nil
	}
	if startIdx == 0 {
		tail = prefixStreamingBlock(tail)
	}
	return tea.Println(tail)
}

func (m *uiModel) finalizeAssistantStream(streamIdx int) tea.Cmd {
	if streamIdx < 0 || streamIdx >= len(m.entries) {
		return nil
	}
	e := m.entries[streamIdx]
	if e.Kind != "assistant" {
		return nil
	}
	if m.lastPrintedIdx > streamIdx {
		return nil
	}
	m.lastPrintedIdx = streamIdx + 1
	m.refreshViewport(true)

	prevFlushedText := m.assistantFlushedText
	m.assistantFlushedLines = 0
	m.assistantFlushedText = ""

	source := strings.TrimSpace(e.Body)
	if source == "" {
		return nil
	}

	termW := m.width - 3
	if termW < 20 {
		termW = 20
	}
	mdRendered := markdown.Render(source, termW, m.llmOutputPalette())
	fullFormatted := m.renderAssistantEntry(strings.TrimRight(mdRendered, "\n"), termW)
	formattedLines := strings.Split(fullFormatted, "\n")

	var startIdx int
	if prevFlushedText != "" {
		prevMd := markdown.Render(prevFlushedText, termW, m.llmOutputPalette())
		prevFormatted := m.renderAssistantEntry(strings.TrimRight(prevMd, "\n"), termW)
		startIdx = len(strings.Split(prevFormatted, "\n"))
	}

	if startIdx > len(formattedLines) {
		startIdx = len(formattedLines)
	}
	if startIdx >= len(formattedLines) {
		return nil
	}
	tail := strings.Join(formattedLines[startIdx:], "\n")
	if strings.TrimSpace(tail) == "" {
		return nil
	}

	if startIdx == 0 {
		tail = prefixStreamingBlock(tail)
	}
	return tea.Println(tail)
}

func (m *uiModel) cyclePermissionMode() {
	if m.cfg.State == nil {
		return
	}
	permCtx := permissions.NewDefaultPermissionContext()
	permCtx.Mode = m.cfg.State.GetPermissionMode()
	next, _ := permissions.CyclePermissionMode(permCtx)
	m.cfg.State.SetPermissionMode(next)
	if next == types.PermissionModePlan {
		m.cfg.State.SetMode("plan")
	} else if m.cfg.State.GetMode() == "plan" {
		m.cfg.State.SetMode("normal")
	}
	// Shift+Tab 으로 모드를 떠날 때 YORO 복원 컨텍스트는 의미가 없으므로 정리한다.
	m.cfg.State.ClearPreYoroMode()
	m.footer = presentation.BuildFooterState(m.cfg.State, m.cfg.WorkDir)
}

// toggleYoroMode 는 Ctrl+Y 로 YORO(bypassPermissions) 모드를 즉시 토글한다.
// 진입 시 직전 모드를 저장하고, 해제 시 저장된 모드로 복원한다.
// PLAN 모드에서 진입한 경우 해제 시 PLAN 으로 돌아간다.
func (m *uiModel) toggleYoroMode() {
	if m.cfg.State == nil {
		return
	}
	current := m.cfg.State.GetPermissionMode()
	if current == types.PermissionModeBypassPermissions {
		// 해제: 직전 모드 복원 (없으면 default)
		restore := m.cfg.State.PreYoroMode()
		if restore == "" || restore == types.PermissionModeBypassPermissions {
			restore = types.PermissionModeDefault
		}
		m.cfg.State.SetPermissionMode(restore)
		if restore == types.PermissionModePlan {
			m.cfg.State.SetMode("plan")
		} else {
			m.cfg.State.SetMode("normal")
		}
		m.cfg.State.ClearPreYoroMode()
	} else {
		// 진입: 직전 모드 저장 후 YORO
		m.cfg.State.SetPreYoroMode(current)
		m.cfg.State.SetPermissionMode(types.PermissionModeBypassPermissions)
		// YORO 는 전역 plan 모드와 무관하므로 normal 로 정리한다.
		m.cfg.State.SetMode("normal")
	}
	m.footer = presentation.BuildFooterState(m.cfg.State, m.cfg.WorkDir)
}

func (m *uiModel) resize() {
	if m.width <= 0 {
		return
	}

	inputWidth := m.width - 5
	if inputWidth < 12 {
		inputWidth = 12
	}
	m.input.SetWidth(inputWidth)

	maxInputH := 8
	lineCount := strings.Count(m.input.Value(), "\n") + 1
	m.input.SetHeight(max(1, min(maxInputH, lineCount)))
	m.input.MaxHeight = maxInputH

	m.viewport.Width = m.width
}

type renderedView struct {
	body          string
	modal         string
	modalOverlayY int
}

const streamingLeadingBlankLines = 2

func prefixStreamingBlock(s string) string {
	return strings.Repeat("\n", streamingLeadingBlankLines) + s
}

func prependStreamingBlankLines(lines []string) []string {
	prefixed := make([]string, 0, streamingLeadingBlankLines+len(lines))
	for i := 0; i < streamingLeadingBlankLines; i++ {
		prefixed = append(prefixed, "")
	}
	return append(prefixed, lines...)
}

func (m uiModel) liveEntryLeadingBlankLines(idx int, e transcriptEntry) int {
	switch {
	case e.Kind == "assistant" && m.assistantOpen && idx == m.assistantStreamIdx:
		if m.assistantFlushedText == "" {
			return streamingLeadingBlankLines
		}
		return 0
	case e.Kind == "thinking" && m.thinkingOpen && idx == m.thinkingStreamIdx:
		if m.thinkingFlushedText == "" {
			return streamingLeadingBlankLines
		}
		return 0
	default:
		return 1
	}
}

func (m uiModel) renderFrame() renderedView {
	footerView := m.renderFooter()
	inputView := m.renderInput()
	queuedPreview := m.renderQueuedPreview()
	taskListRow := m.renderTaskListRow()
	thinkingRow := m.renderThinkingRow()
	modeStatus := m.renderModeStatus()
	modeDivider := m.renderModeDivider()
	modalView := ""
	modalPartIndex := -1
	if m.modal.Kind != modalNone {
		modalView = m.renderModal()
	}

	bottomParts := make([]string, 0, 12)
	bottomParts = append(bottomParts, "", "")
	if taskListRow != "" {
		bottomParts = append(bottomParts, taskListRow)
		if thinkingRow != "" {
			bottomParts = append(bottomParts, "")
		}
	}
	if thinkingRow != "" {
		bottomParts = append(bottomParts, thinkingRow)
	}
	bottomParts = append(bottomParts, modeDivider)

	// anchorBaseParts excludes the modal so maxStreamH is not reduced when a modal appears.
	anchorBaseParts := append([]string(nil), bottomParts...)
	if queuedPreview != "" {
		anchorBaseParts = append(anchorBaseParts, modeStatus, inputView, queuedPreview, footerView)
	} else {
		anchorBaseParts = append(anchorBaseParts, modeStatus, inputView, footerView)
	}

	if modalView != "" {
		modalPartIndex = len(bottomParts)
		bottomParts = append(bottomParts, modalView, "")
	}
	if queuedPreview != "" {
		bottomParts = append(bottomParts, modeStatus, inputView, queuedPreview, footerView)
	} else {
		bottomParts = append(bottomParts, modeStatus, inputView, footerView)
	}

	var streamLines []string
	for i := m.lastPrintedIdx; i < len(m.entries); i++ {
		rendered := m.renderTranscriptEntryAt(i, m.entries[i])
		if rendered == "" {
			continue
		}
		for blank := 0; blank < m.liveEntryLeadingBlankLines(i, m.entries[i]); blank++ {
			streamLines = append(streamLines, "")
		}
		streamLines = append(streamLines, strings.Split(rendered, "\n")...)
	}

	anchorH := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, anchorBaseParts...))
	maxStreamH := m.height - anchorH
	if maxStreamH < 0 {
		maxStreamH = 0
	}
	if len(streamLines) > maxStreamH {
		streamLines = streamLines[len(streamLines)-maxStreamH:]
	}

	finalParts := make([]string, 0, len(streamLines)+len(bottomParts))
	if len(streamLines) > 0 {
		finalParts = append(finalParts, strings.Join(streamLines, "\n"))
	}
	finalParts = append(finalParts, bottomParts...)

	modalOverlayY := -1
	if modalPartIndex >= 0 {
		modalOverlayY = len(streamLines)
		for i := 0; i < modalPartIndex; i++ {
			modalOverlayY += lipgloss.Height(bottomParts[i])
		}
	}

	return renderedView{
		body:          lipgloss.JoinVertical(lipgloss.Left, finalParts...),
		modal:         modalView,
		modalOverlayY: modalOverlayY,
	}
}

func (m uiModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	rendered := m.renderFrame()
	if m.app != nil && m.app.parker != nil {
		m.updateCursorTargetForRendered(rendered)
	}

	return rendered.body
}
func (m uiModel) updateCursorTargetForRendered(rendered renderedView) {
	if m.app == nil || m.app.parker == nil {
		return
	}

	if m.modal.Kind == modalPassword {
		lineIdx, col, ok := passwordModalCursorPosition(rendered.modal)
		if !ok || rendered.modalOverlayY < 0 {
			m.app.parker.SetTarget(0, 0, false)
			return
		}

		targetLine := rendered.modalOverlayY + lineIdx
		linesUp := lipgloss.Height(rendered.body) - 1 - targetLine
		if linesUp < 0 {
			linesUp = 0
		}
		if col < 1 {
			col = 1
		}
		m.app.parker.SetTarget(linesUp, col, true)
		return
	}

	if m.modal.Kind != modalNone {
		m.app.parker.SetTarget(0, 0, false)
		return
	}

	footerH := lipgloss.Height(m.renderFooter())
	queuePreviewH := 0
	if qp := m.renderQueuedPreview(); qp != "" {
		queuePreviewH = lipgloss.Height(qp)
	}

	linesUp := footerH + queuePreviewH + 1

	textareaRow := m.input.Line()
	contentLinesCount := m.input.Height()

	if textareaRow < contentLinesCount {
		linesUp += (contentLinesCount - 1 - textareaRow)
	}

	prefixWidth := 2
	lineVal := m.input.Value()
	lines := strings.Split(lineVal, "\n")
	visualCol := prefixWidth

	if textareaRow < len(lines) {
		rowText := lines[textareaRow]
		colIdx := m.input.LineInfo().CharOffset
		runes := []rune(rowText)
		if colIdx > len(runes) {
			colIdx = len(runes)
		}
		visualCol += runewidth.StringWidth(string(runes[:colIdx]))
	}

	m.app.parker.SetTarget(linesUp, visualCol+1, true)
}

func passwordModalCursorPosition(modal string) (line, col int, ok bool) {
	lines := strings.Split(modal, "\n")
	for i, row := range lines {
		plain := stripANSI(row)
		runes := []rune(plain)
		for j, r := range runes {
			if r != '*' {
				continue
			}
			return i, runewidth.StringWidth(string(runes[:j])) + 1, true
		}
	}
	return 0, 0, false
}

func (m uiModel) updateCursorTarget() {
	if m.app == nil || m.app.parker == nil {
		return
	}
	m.updateCursorTargetForRendered(m.renderFrame())
}

func (m *uiModel) flushStreamingLines(entryIdx int, flushedCount *int) tea.Cmd {
	if entryIdx < 0 || entryIdx >= len(m.entries) {
		return nil
	}
	e := m.entries[entryIdx]
	body := e.Body
	switch e.Kind {
	case "assistant":
		body = stableStreamingMarkdownSource(body)
		body = markdown.StableStreamingPrefix(body)
	case "thinking":
		body = thinkingStreamSource(body)
	}
	lines := strings.Split(body, "\n")
	if len(lines) <= 1 {
		return nil
	}

	completeLines := lines[:len(lines)-1]
	newLinesCount := len(completeLines) - *flushedCount

	if newLinesCount <= 0 {
		return nil
	}

	if e.Kind == "assistant" || e.Kind == "thinking" {
		termW := m.width - 3
		if termW < 20 {
			termW = 20
		}
		allCompleteText := strings.Join(completeLines, "\n")
		palette := m.llmOutputPalette()
		prevFlushedText := m.assistantFlushedText
		if e.Kind == "thinking" {
			palette = m.thinkingPalette()
			prevFlushedText = m.thinkingFlushedText
		}

		// 이전에 flush한 raw text를 지금 시점의 termW로 재렌더링해서 startIdx 계산.
		// ensureBlankBefore/After가 문맥에 따라 빈 줄을 삽입하므로, 줄 수(int)를
		// 저장하면 재렌더링 시 어긋날 수 있다. raw text 재렌더링으로 항상 정확한
		// 커트라인을 확보한다.
		var startIdx int
		if prevFlushedText != "" {
			prevMd := markdown.Render(prevFlushedText, termW, palette)
			prevFormatted := m.renderAssistantEntry(strings.TrimRight(prevMd, "\n"), termW)
			startIdx = len(strings.Split(prevFormatted, "\n"))
		}

		mdRendered := markdown.Render(allCompleteText, termW, palette)
		fullFormatted := m.renderAssistantEntry(strings.TrimRight(mdRendered, "\n"), termW)
		formattedLines := strings.Split(fullFormatted, "\n")

		if startIdx > len(formattedLines) {
			startIdx = len(formattedLines)
		}
		newRendered := formattedLines[startIdx:]
		*flushedCount = len(completeLines)
		if len(newRendered) == 0 {
			return nil
		}

		if e.Kind == "thinking" {
			m.thinkingFlushedText = allCompleteText
		} else {
			m.assistantFlushedText = allCompleteText
		}

		if startIdx == 0 {
			newRendered = prependStreamingBlankLines(newRendered)
		}

		return tea.Println(strings.Join(newRendered, "\n"))
	}

	startFlushCount := *flushedCount
	var toPrint []string
	for i := *flushedCount; i < len(completeLines); i++ {
		line := completeLines[i]
		rendered := m.renderTranscriptLine(e.Kind, line, i)
		toPrint = append(toPrint, rendered)
	}

	*flushedCount = len(completeLines)
	rendered := strings.Join(toPrint, "\n")
	if e.Kind == "thinking" && startFlushCount == 0 {
		rendered = "\n" + rendered
	}
	return tea.Println(rendered)
}

func (m *uiModel) renderTranscriptLine(kind, line string, lineIdx int) string {
	prefix := "  "
	if kind == "assistant" && lineIdx == 0 {
		if m.theme.DisableANSI {
			prefix = "* "
		} else {
			prefix = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.AssistantMarkerColor)).Render("* ")
		}
	}

	color := m.theme.BodyColor
	if kind == "thinking" {
		color = m.theme.ThinkingTitleColor
	}
	style := m.theme.style(color)
	return prefix + style.Render(line)
}

func serverFormStateFromEntry(e workspace.ServerEntry, m *workspace.Manager) *serverFormState {
	portStr := "22"
	if e.Port > 0 {
		portStr = strconv.Itoa(e.Port)
	}
	sf := &serverFormState{
		EditAlias:  e.Alias,
		Alias:      e.Alias,
		Host:       e.Host,
		PortStr:    portStr,
		User:       e.User,
		DefaultCWD: e.DefaultCWD,
	}
	switch {
	case e.Auth.UseAgent:
		sf.AuthMode = serverFormAuthAgent
	case e.Auth.IdentityFile != "":
		sf.AuthMode = serverFormAuthKey
	case e.Auth.AllowPassword:
		sf.AuthMode = serverFormAuthPassword
		if m != nil && m.GetPassword(e.Alias, "ssh") != "" {
			sf.PasswordRegistered = true
		}
	}

	elev := e.Elevation
	sf.AllowElevation = elev.Allowed

	sf.ElevationMethod = "su"
	if elev.Mode == "reuse_login" || elev.Mode == "sudo" {
		sf.ElevationMethod = "sudo"
	}

	var targets []targetUserState
	for _, u := range elev.TargetUsers {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if u == "root" {
			if m != nil && m.GetSudoPassword(e.Alias, "root") != "" {
				sf.RootPwRegistered = true
			}
			continue
		}

		tState := targetUserState{User: u}
		if m != nil && m.GetSudoPassword(e.Alias, u) != "" {
			tState.PwRegistered = true
		}
		targets = append(targets, tState)
	}
	if m != nil && m.Registry() != nil {
		seen := make(map[string]bool, len(targets))
		for _, t := range targets {
			seen[strings.TrimSpace(t.User)] = true
		}
		for _, entry := range m.Registry().List() {
			if entry.Kind != workspace.ServerKindAccount || entry.ParentAlias != e.Alias {
				continue
			}
			u := strings.TrimSpace(entry.User)
			if u == "" || seen[u] {
				continue
			}
			tState := targetUserState{User: u}
			if m.GetPasswordForTarget(e.Alias, entry.SwitchMethod, u) != "" {
				tState.PwRegistered = true
			}
			targets = append(targets, tState)
			seen[u] = true
		}
	}
	sf.Targets = targets

	if len(elev.TargetUsers) == 0 && elev.Allowed {
		if m != nil && m.GetSudoPassword(e.Alias, "") != "" {
			sf.RootPwRegistered = true
		}
	}

	return sf
}

func (m *uiModel) refreshViewport(scrollToBottom bool) {
	var parts []string

	for i, e := range m.entries {
		parts = append(parts, m.renderTranscriptEntryAt(i, e))
	}

	content := strings.Join(parts, "\n\n")
	m.viewport.SetContent(content)

	if scrollToBottom {
		m.viewport.GotoBottom()
	}
}

func (m *uiModel) UpdateUISettings(ui UISettings) {
	resolved := ResolveUISettings(ui, m.theme.Name, m.cfg.AIDebug)
	m.ui = resolved
	m.theme = resolveUIThemeWithVariant(resolved.Theme, resolved.Variant, m.cfg.AIDebug)
	m.resize()
}

func (m *uiModel) dequeueAndFlush() tea.Cmd {
	if len(m.queuedPrompts) == 0 {
		return nil
	}
	head := m.queuedPrompts[0]
	if head.IsSlash {
		m.queuedPrompts = m.queuedPrompts[1:]
		sub := head.Submission
		return func() tea.Msg {
			m.app.submitPrompt(sub)
			return nil
		}
	}
	var batch []promptSubmission
	cut := 0
	for i, q := range m.queuedPrompts {
		if q.IsSlash {
			break
		}
		batch = append(batch, q.Submission)
		cut = i + 1
	}
	m.queuedPrompts = m.queuedPrompts[cut:]
	return func() tea.Msg {
		m.app.submitMultiple(batch)
		return nil
	}
}
