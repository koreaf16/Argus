package tui

import (
	"context"
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
	StreamBody string // streaming tool output (bash/powershell ??????딅? ??stdout ?????밸븶??
	ToolName   string
	TaskID     string

	// tool_group ?????밸븶??
	Collapsed   bool
	SearchCount int
	ReadCount   int
	ListCount   int
	LastHint    string
	SubEntries  []transcriptEntry
	IsActive    bool

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

	assistantOpen                bool
	assistantStreamIdx           int
	assistantLastDelta           time.Time
	assistantFlushedLines        int
	assistantFlushedRenderedLines int
	thinkingOpen          bool
	thinkingStreamIdx     int
	thinkingLastDelta     time.Time
	thinkingFlushedLines  int
	toolUseOpen           bool
	toolUseStreamIdx      int
	busy                  bool
	busyStartedAt         time.Time
	tokenInputSnap        int
	tokenOutputSnap       int

	modalQueue []modalState
	modal      modalState

	slashMatches []slashSuggestion
	slashCursor  int

	inputHistory []string
	historyIdx   int
	historyCycle bool

	maxEntries int

	lastPrintedIdx int

	animFrame   int
	spinnerVerb string

	// ????繹먮끏援?????嚥??????????袁ㅻ쇀??
	activeTool        toolui.InteractiveModel
	toolFocused       bool
	toolEntryByTaskID map[string]int
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
	cmds = append(cmds, doTick(m.tickIntervalMS()))
	cmds = append(cmds, tea.Printf("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l"))

	cmds = append(cmds, tea.Println(renderLogoBlock(m.cfg)))

	if m.cfg.Workspace != nil {
		// 시작 시에는 무조건 로컬 환경을 기본 워크스페이스로 강제 고정
		activeAlias := workspace.LocalAlias
		if m.cfg.State != nil {
			m.cfg.State.SetActiveWorkspace(activeAlias)
		}

		ws := m.cfg.Workspace
		cmds = append(cmds, func() tea.Msg {
			snap, _ := ws.RunInspect(context.Background(), activeAlias)
			// 결과를 화면(transcript)에 찍지 않고 하단 상태바만 업데이트하도록 메시지 변경
			return footerStateMsg{
				Footer: presentation.BuildFooterState(m.cfg.State, snap.CWD),
			}
		})
	}

	for _, e := range m.entries {
		cmds = append(cmds, tea.Println(m.renderTranscriptEntryAt(-1, e)))
	}
	return tea.Batch(cmds...)
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Filter out all mouse messages at the beginning
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
		return m, doTick(m.tickIntervalMS())
	case submitFinishedMsg:
		m.busy = false
		m.busyStartedAt = time.Time{}
		footerCmd := func() tea.Msg { return m.app.buildFooterMsg() }
		return m, footerCmd
	case queryEventMsg:
		if evt, ok := presentation.FromUIEvent(v.Event); ok {
			prevIdx := m.assistantStreamIdx
			m.applyPresentationEvent(evt)
			return m, tea.Batch(m.handleEventFinalization(evt), m.scrollbackCmd(evt, prevIdx))
		}
		return m, nil
	case presentationEventMsg:
		prevIdx := m.assistantStreamIdx
		m.applyPresentationEvent(v.Event)
		return m, tea.Batch(m.handleEventFinalization(v.Event), m.scrollbackCmd(v.Event, prevIdx))
	case footerStateMsg:
		m.footer = v.Footer
		return m, nil
	case startupInspectDoneMsg:
		evt := presentation.Event{Kind: presentation.EventSystem, Text: v.text}
		m.applyPresentationEvent(evt)
		footerCmd := func() tea.Msg { return m.app.buildFooterMsg() }
		// ???????????????쇨덧??????釉먮빱???뽯굵????????띻콣?????썹땟???(handleEventFinalization ??꿔꺂????????怨뚰뇠???떐?
		return m, footerCmd
	case animationTickMsg:
		if !m.busy && m.activeTool == nil {
			return m, nil
		}

		if m.ui.Motion.Enabled {
			step := 1
			if m.ui.Motion.Reduced || m.ui.Motion.Level == "restrained" {
				step = 1
			}
			m.animFrame += step
		}
		if m.busy {
			m.tokenInputSnap, m.tokenOutputSnap = m.app.cfg.Engine.TokenSnapshot()
		}

		var toolCmd tea.Cmd
		if m.activeTool != nil && m.ui.Motion.Enabled {
			// BashInteractiveModel ??μ떜媛?걫?繹먭껫????????됱춻????????띻콣?????썹땟??????ш끽維뽳쭩???????????삳뜪??????????轅붽틓???????? ????? ?????⑤뜪?????꿔꺂??틝???놁뗄???????밸븶???
			m.activeTool, toolCmd = m.activeTool.Update(spinner.TickMsg{
				Time: time.Now(),
				// ID??0????????????숈?????棺堉?댆洹ⓦ럹????? ?轅붽틓??熬곥끇釉???????????????筌먦끆?????棺堉?댆?????????異??				// BashInteractiveModel.Update???ル봿?? TickMsg ???????ㅿ폁????ш끽維뽳쭩?????ル∥堉????筌먦끆?????????????띻콣?????썹땟?????棺堉?댆??????? ui.go????癲ル슢???????濚밸Ŧ援??
			})
		}
		return m, tea.Batch(doTick(m.tickIntervalMS()), toolCmd)
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
					sf = serverFormStateFromEntry(entry)
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

		// ????袁ㅻ쇀????????? ??????뀀땽 ??β뼯援????????袁ㅻ쇀???源낅츐????????????밸븶???
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

		switch v.String() {
		case "esc":
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
			// ?????????살퓢癲????????쇨덧?筌먦렜逾?????袁ㅻ쇀???源낅츐??????????袁ｋ쨨????癲ル슢????
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
			if m.input.Value() == "" {
				m.cycleInputHistoryBackward()
				m.resize()
				return m, nil
			}
		case "down":
			if m.input.Value() == "" {
				m.cycleInputHistoryForward()
				m.resize()
				return m, nil
			}
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			if m.busy {
				m.applyPresentationEvent(presentation.Event{
					Kind: presentation.EventNotice,
					Text: "Previous request is still running.",
				})
				return m, m.handleEventFinalization(presentation.Event{
					Kind: presentation.EventNotice,
					Text: "Previous request is still running.",
				})
			}
			m.pushInputHistory(text)
			m.resetHistoryCycle()
			m.input.SetValue("")
			m.input.SetCursor(0)
			m.refreshSlashSuggestions()
			m.resize()
			m.app.submit(text)
			return m, nil
		case "ctrl+j":
			m.input.InsertRune('\n')
			m.resize()
			return m, nil
		case "shift+tab":
			m.cyclePermissionMode()
			return m, nil
		case "ctrl+v":
			content, err := clipboard.ReadAll()
			if err == nil {
				m.input.InsertString(content)
				m.resize()
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

		// ????ш끽維뽳쭩??????ш끽維뽳쭩?좊쐪筌먲퐢?????嶺뚮쮳釉띿땋???????⑤챷竊??????쇨덧????????댟??モ뵛??뗫뼀???????????ㅼ뒧?????濚밸Ŧ援???????resize ??꿔꺂?????
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
		// default scrollbackCmd??????轅붽틓??影?뽧걤????棺堉?댆????????밸븶??ｋ뜦?
		return nil

	case presentation.EventAssistantDone:
		return nil
	case presentation.EventUser, presentation.EventSystem, presentation.EventError,
		presentation.EventNotice,
		presentation.EventApprovalDecision, presentation.EventPlanResult, presentation.EventPasswordRequest,
		presentation.EventAskUserRequest:
		// default scrollbackCmd??????轅붽틓??影?뽧걤????棺堉?댆????????밸븶??ｋ뜦?
		return nil
	case presentation.EventViewCleared:
		return tea.ClearScreen
	}
	return nil
}

func (m *uiModel) scrollbackCmd(evt presentation.Event, prevStreamIdx int) tea.Cmd {
	switch evt.Kind {
	case presentation.EventAssistantDelta:
		// assistant streaming도 thinking과 동일하게 완성된 줄은 즉시 tea.Println으로
		// 스크롤백에 flush한다. 그러면 View()의 streamLines에는 마지막 미완성 줄만
		// 남아 prompt가 출력 끝 + 빈 줄 2개 위치에서 흔들리지 않고 유지된다.
		m.refreshViewport(true)
		return m.flushStreamingLines(m.assistantStreamIdx, &m.assistantFlushedLines)
	case presentation.EventThinkingDelta:
		m.refreshViewport(true)
		return m.flushStreamingLines(m.thinkingStreamIdx, &m.thinkingFlushedLines)
	case presentation.EventThinkingDone, presentation.EventState, presentation.EventViewCleared:
		m.refreshViewport(true)
		return nil
	case presentation.EventAssistantDone:
		return m.finalizeAssistantStream(prevStreamIdx)
	case presentation.EventToolResult:
		// tool_use(박스) → tool_result 순으로 스크롤백에 출력
		if m.lastPrintedIdx < len(m.entries) {
			var cmds []tea.Cmd
			for i := m.lastPrintedIdx; i < len(m.entries); i++ {
				rendered := m.renderTranscriptEntryAt(-1, m.entries[i])
				if rendered != "" {
					cmds = append(cmds, tea.Println("\n"+rendered))
				}
			}
			m.lastPrintedIdx = len(m.entries)
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
		if finalized := m.finalizeAssistantStream(prevStreamIdx); finalized != nil {
			cmds = append(cmds, finalized)
		}

		if m.lastPrintedIdx < len(m.entries) {
			newLastPrinted := m.lastPrintedIdx
			for i := m.lastPrintedIdx; i < len(m.entries); i++ {
				entry := m.entries[i]
				// 스트리밍 중인 shell tool_use는 결과가 올 때까지 라이브 뷰에 유지
				if entry.Kind == "tool_use" && m.toolUseOpen && m.toolUseStreamIdx == i {
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

	flushedRendered := m.assistantFlushedRenderedLines
	m.assistantFlushedLines = 0
	m.assistantFlushedRenderedLines = 0

	source := strings.TrimSpace(e.Body)
	if source == "" {
		return nil
	}

	termW := m.width - 3
	if termW < 20 {
		termW = 20
	}
	mdRendered := markdown.Render(source, termW, m.themeToPalette())
	fullFormatted := m.renderAssistantEntry(strings.TrimRight(mdRendered, "\n"), termW)
	formattedLines := strings.Split(fullFormatted, "\n")

	if flushedRendered >= len(formattedLines) {
		return nil
	}
	tail := strings.Join(formattedLines[flushedRendered:], "\n")
	if strings.TrimSpace(tail) == "" {
		return nil
	}

	return tea.Println("\n" + tail)
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
	m.footer = presentation.BuildFooterState(m.cfg.State, m.cfg.WorkDir)
}

func (m *uiModel) resize() {
	if m.width <= 0 {
		return
	}

	// 1. ?????밸븶?????袁ｋ쨨營???????⑤챷竊????????됲룈 ???濚밸Ŧ???
	inputWidth := m.width - 5
	if inputWidth < 12 {
		inputWidth = 12
	}
	m.input.SetWidth(inputWidth)

	// ?????⑤챷竊??????붺몭?⑸쨨???(?????쇨덧????????곕춴?????ル봿?????ㅼ뒧??, ?轅붽틓????彛? 8??
	maxInputH := 8
	lineCount := strings.Count(m.input.Value(), "\n") + 1
	m.input.SetHeight(max(1, min(maxInputH, lineCount)))
	m.input.MaxHeight = maxInputH

	// 2. ???????????????濚밸Ŧ???
	// View()?????JoinVertical?????곗뒭????????????????꿔꺂????釉띯뵛??轅붽틓??筌뚮챶夷???????붺몭?⑸쨨??????곗뒭??????????밸븶????????濚밸Ŧ援??
	m.viewport.Width = m.width
}

type renderedView struct {
	body          string
	modal         string
	modalOverlayY int
}

func (m uiModel) renderFrame() renderedView {
	footerView := m.renderFooter()
	inputView := m.renderInput()
	thinkingRow := m.renderThinkingRow()
	modeStatus := m.renderModeStatus()
	modeDivider := m.renderModeDivider()
	modalView := ""
	modalPartIndex := -1
	if m.modal.Kind != modalNone {
		modalView = m.renderModal()
	}

	// Keep the live status/input/footer block anchored at the terminal bottom.
	// LLM 출력 끝과 prompt 사이에 항상 빈 줄 2개를 보호 영역으로 둔다.
	// thinkingRow(처리 중 표시)가 있을 때는 그 위가 보호 영역, 없을 때는
	// divider 위가 보호 영역이 된다 — 어느 경우든 출력과 prompt 사이 여백 2줄 보장.
	bottomParts := make([]string, 0, 8)
	bottomParts = append(bottomParts, "", "")
	if thinkingRow != "" {
		bottomParts = append(bottomParts, thinkingRow)
	}
	bottomParts = append(bottomParts, modeDivider)

	anchorParts := append([]string(nil), bottomParts...)
	anchorParts = append(anchorParts, modeStatus, inputView, footerView)

	if modalView != "" {
		modalPartIndex = len(bottomParts)
		bottomParts = append(bottomParts, modalView)
	} else {
		bottomParts = append(bottomParts, modeStatus, inputView, footerView)
	}

	// Dynamic Inline Rendering: 이미 스크롤백에 tea.Println으로 인쇄된 엔트리
	// (m.lastPrintedIdx 미만)는 다시 그리지 않는다. 미인쇄 엔트리만 anchor 위에
	// 표시하고, anchor가 흔들리지 않도록 위쪽을 빈 줄로 padding 한다.
	var streamLines []string
	for i := m.lastPrintedIdx; i < len(m.entries); i++ {
		rendered := m.renderTranscriptEntryAt(i, m.entries[i])
		if rendered == "" {
			continue
		}
		// 스크롤백(tea.Println("\n"+rendered))과 일치시키기 위해 각 엔트리 앞에 빈 줄 추가
		streamLines = append(streamLines, "")
		streamLines = append(streamLines, strings.Split(rendered, "\n")...)
	}

	fixedH := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, bottomParts...))
	anchorH := fixedH
	if modalView != "" {
		// A modal overlays the lower panel. Keep transcript clipping anchored to
		// the normal non-modal panel so opening a modal does not shift output.
		anchorH = lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, anchorParts...))
	}
	maxStreamH := m.height - anchorH
	if maxStreamH < 0 {
		maxStreamH = 0
	}
	if len(streamLines) > maxStreamH {
		streamLines = streamLines[len(streamLines)-maxStreamH:]
	}

	// Dynamic Inline Rendering: View() 출력은 짧게 유지해야 한다. 빈 줄로 화면을
	// padding 하면 인라인 모드에서 tea.Println이 누적해 둔 스크롤백을 위로 밀어
	// 가려 버린다. anchor는 자연스럽게 출력 길이만큼만 차지하도록 둔다.
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

	// Bubble Tea 프레임 렌더링. \x1b[J (Erase In Display)를 사용하면 
	// 인라인 모드에서 이전 대화 내역이 지워질 수 있으므로 제거함.
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

	// footer(footerH) + input_bottom_border(1)
	linesUp := footerH + 1

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
	if e.Kind == "assistant" {
		body = stableStreamingMarkdownSource(body)
		// 진행 중인 마크다운 표/코드블록은 새 줄이 추가될수록 렌더된 라인 인덱스가
		// 밀리기 때문에(예: 표 하단 경계선이 아래로 이동), 라인 차분 기반 flush가
		// 같은 경계선을 반복 출력하게 된다. 해당 블록이 끝날 때까지 본문에서 제외한다.
		body = markdown.StableStreamingPrefix(body)
	}
	lines := strings.Split(body, "\n")
	if len(lines) <= 1 {
		return nil
	}

	// ?轅붽틓???????μ떝?띄몭??←솒??? ?????밸븶??볧돯???????깅렰???숆강?붺춯??쳱???μ떝?띄몭??袁㏉떋???????濚밸Ŧ寃㎩쳞????遺뱁떐?????癲ル슢???????롪퍓肉?? ?????μ떝?띄몭??←솒???????뿥??Flush
	completeLines := lines[:len(lines)-1]
	newLinesCount := len(completeLines) - *flushedCount

	if newLinesCount <= 0 {
		return nil
	}

	if e.Kind == "assistant" {
		termW := m.width - 3
		if termW < 20 {
			termW = 20
		}
		allCompleteText := strings.Join(completeLines, "\n")
		mdRendered := markdown.Render(allCompleteText, termW, m.themeToPalette())
		fullFormatted := m.renderAssistantEntry(strings.TrimRight(mdRendered, "\n"), termW)
		formattedLines := strings.Split(fullFormatted, "\n")

		startIdx := m.assistantFlushedRenderedLines
		if startIdx > len(formattedLines) {
			startIdx = len(formattedLines)
		}
		newRendered := formattedLines[startIdx:]
		*flushedCount = len(completeLines)
		if len(newRendered) == 0 {
			m.assistantFlushedRenderedLines = startIdx
			return nil
		}
		m.assistantFlushedRenderedLines = len(formattedLines)
		return tea.Println(strings.Join(newRendered, "\n"))
	}

	var toPrint []string
	for i := *flushedCount; i < len(completeLines); i++ {
		line := completeLines[i]
		rendered := m.renderTranscriptLine(e.Kind, line, i)
		toPrint = append(toPrint, rendered)
	}

	*flushedCount = len(completeLines)
	return tea.Println(strings.Join(toPrint, "\n"))
}

func (m *uiModel) renderTranscriptLine(kind, line string, lineIdx int) string {
	prefix := "  "
	if kind == "assistant" && lineIdx == 0 {
		if m.theme.DisableANSI {
			prefix = "✦ "
		} else {
			prefix = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.AssistantMarkerColor)).Render("✦ ")
		}
	}

	color := m.theme.BodyColor
	if kind == "thinking" {
		color = m.theme.ThinkingTitleColor
	}
	style := m.theme.style(color)
	return prefix + style.Render(line)
}

// serverFormStateFromEntry creates a serverFormState pre-filled with existing server data.
func serverFormStateFromEntry(e workspace.ServerEntry) *serverFormState {
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
		sf.IdentityFile = e.Auth.IdentityFile
	case e.Auth.AllowPassword:
		sf.AuthMode = serverFormAuthPassword
	}
	return sf
}

func (m *uiModel) refreshViewport(scrollToBottom bool) {
	var parts []string

	// ?黎??筌??????嚥?(???ル봿?????????밸븶???????⑤뜤癲???꿔꺂????????롪퍓肉????癲ル슢????癒λ돗?????살퓢??)
	// parts = append(parts, renderLogoBlock(m.cfg))

	for i, e := range m.entries {
		parts = append(parts, m.renderTranscriptEntryAt(i, e))
	}

	content := strings.Join(parts, "\n\n")
	m.viewport.SetContent(content)

	if scrollToBottom {
		m.viewport.GotoBottom()
	}
}
