package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourorg/infractl/internal/agent"
	"github.com/yourorg/infractl/internal/config"
	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/mcp"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

// AppModel is the main Bubble Tea application model.
type AppModel struct {
	chat       chatView
	input      inputBar
	statusBar  statusBar
	ag         *agent.Agent
	store      store.ServerStore
	manager    *executor.Manager
	cfg        *config.Config
	ctx        context.Context
	cancel     context.CancelFunc
	reqID      int // 현재 활성 요청 세대 — stale AgentDoneMsg 무시에 사용
	width      int
	height     int
	busy       bool
	ctrlCCount int

	shimmer  shimmerState
	progress *progressTree
	sp       spinner.Model

	activeTools activeToolMap
	queue       inputQueue
	selection   selectionState
	form        formState

	selectHandler    *TUISelectHandler
	formHandler      *TUIFormHandler
	activeServer     *store.Server
	connectorMgr     *connector.Manager
	mcpClients       []*mcp.Client
	sessionStore     store.SessionStore
	execLogStore     store.ExecLogStore
	currentSessionID int64
	yoloMode         bool
	elevatedMode     bool
	planMode         bool
	privilege        privilegePromptState

	knowledgeStore store.KnowledgeStore
	ragSourceStore store.RAGSourceStore
	discoveryStore store.DiscoveryStore
	costTracker    CostTracker
	checkpointMgr  CheckpointManager
	hooksMgr       HooksManager
	scheduleMgr    ScheduleManager
	privCache      *privilege.Cache
	auditStore     store.AuditQueryStore

	thinkingLabel string
	thinkBuf      string // thinking ?ㅽ듃由щ컢 ?꾩쟻 踰꾪띁 (shimmer hint ?낅뜲?댄듃??
	stats         turnStats
	turnCount     int // 吏湲덇퉴吏 ?쒖옉??????(援щ텇??異쒕젰 ?쒖젏 ?먮떒??

	history     toolHistory
	histOverlay toolOverlayState

	box          *ProgramBox
	parker       *CursorParkWriter
	mdRend       *mdRenderer
	streamTokens string
	streamLines  []string
	streamCache  stableCache
	lastStreamAt time.Time
}

// AppOptions configures optional integrations for the app.
type AppOptions struct {
	InitialSessionID int64
	HistoryStore     store.HistoryStore
	ConnectorMgr     *connector.Manager
	MCPClients       []*mcp.Client
	SessionStore     store.SessionStore
	ExecLogStore     store.ExecLogStore
	CursorParker     *CursorParkWriter
	ProgramBox       *ProgramBox
	SelectHandler    *TUISelectHandler
	FormHandler      *TUIFormHandler

	KnowledgeStore store.KnowledgeStore
	RAGSourceStore store.RAGSourceStore
	DiscoveryStore store.DiscoveryStore
	CostTracker    CostTracker
	CheckpointMgr  CheckpointManager
	HooksMgr       HooksManager
	ScheduleMgr    ScheduleManager
	PrivCache      *privilege.Cache
	AuditStore     store.AuditQueryStore
}

// NewApp constructs the base TUI app.
func NewApp(ag *agent.Agent, cfg *config.Config, st store.ServerStore, mgr *executor.Manager) AppModel {
	servers, _ := st.List(context.Background())
	ctx, cancel := context.WithCancel(context.Background())

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = StyleSpinner

	return AppModel{
		chat:      newChatView(80, 20),
		input:     newInputBar(80),
		statusBar: newStatusBar(cfg.LLM.Model, len(servers)),
		shimmer:   newShimmer(),
		progress:  newProgressTree(),
		sp:        sp,
		ag:        ag,
		store:     st,
		manager:   mgr,
		cfg:       cfg,
		ctx:       ctx,
		cancel:    cancel,
		mdRend:    newMdRenderer(74),
	}
}

// NewAppWithOptions constructs the app with optional integrations enabled.
func NewAppWithOptions(
	ag *agent.Agent,
	cfg *config.Config,
	st store.ServerStore,
	mgr *executor.Manager,
	opts AppOptions,
) AppModel {
	m := NewApp(ag, cfg, st, mgr)
	m.currentSessionID = opts.InitialSessionID
	m.input.SetHistoryStore(opts.HistoryStore, opts.InitialSessionID)
	if opts.HistoryStore != nil {
		if entries, err := opts.HistoryStore.ListPromptHistory(context.Background(), 200); err == nil {
			m.input.LoadHistory(entries)
		}
	}
	m.connectorMgr = opts.ConnectorMgr
	m.mcpClients = opts.MCPClients
	m.sessionStore = opts.SessionStore
	m.execLogStore = opts.ExecLogStore
	m.parker = opts.CursorParker
	m.box = opts.ProgramBox
	m.selectHandler = opts.SelectHandler
	m.formHandler = opts.FormHandler
	m.knowledgeStore = opts.KnowledgeStore
	m.ragSourceStore = opts.RAGSourceStore
	m.discoveryStore = opts.DiscoveryStore
	m.costTracker = opts.CostTracker
	m.checkpointMgr = opts.CheckpointMgr
	m.hooksMgr = opts.HooksMgr
	m.scheduleMgr = opts.ScheduleMgr
	m.privCache = opts.PrivCache
	m.auditStore = opts.AuditStore
	m.yoloMode = ag.IsYOROMode()
	m.elevatedMode = ag.IsElevatedMode()
	m.statusBar.setYoloMode(m.yoloMode)
	m.statusBar.setElevatedMode(m.elevatedMode)

	// Agent???먯떊(QuestionHandler 援ы쁽泥????깅줉?⑸땲??
	m.ag.SetQuestionHandler(m)

	return m
}

func (m AppModel) Init() tea.Cmd {
	// 시작 배너 출력 (ProgramBox가 설정될 때까지 잠시 대기 후 출력)
	go func() {
		for i := 0; i < 10; i++ {
			if m.box != nil && m.box.p != nil {
				servers, _ := m.store.List(context.Background())
				m.box.Println(welcomeBanner(m.cfg.LLM.Model, len(servers)))
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	return tea.Batch(
		tea.EnableBracketedPaste,
		tea.ShowCursor,
		initialWindowSizeCmd(m.parker),
		m.input.Init(),
	)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.busy {
			var cmd tea.Cmd
			m.sp, cmd = m.sp.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case ShimmerTickMsg:
		if m.busy {
			if cmd := m.shimmer.Tick(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case UsageUpdateMsg:
		m.statusBar.updateUsage(msg)
		m.stats.AddTokens(msg.InputTokens, msg.OutputTokens)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.mdRend = newMdRenderer(m.width - 6)
		m.streamCache.Reset()
		if m.streamTokens != "" {
			m.streamLines = renderStreamingPreview(m.streamTokens, m.mdRend, &m.streamCache, 0)
		}
		m = m.resize()
		return m, nil

	case tea.MouseMsg:
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case SubmitMsg:
		// 폼 모드 활성 시: 입력값을 폼 필드에 반영하고 agent로 보내지 않는다.
		if m.form.active {
			m.form.AcceptValue(msg.DisplayInput)
			m.input.Reset()
			if m.form.AdvanceToNext() {
				// 모든 필드 완료 → 즉시 제출 (Review 없음)
				return m.Update(FormResponseMsg{
					Result:  m.form.BuildResult(),
					ReplyCh: m.form.replyCh,
				})
			}
			m.input.ti.Placeholder = m.form.CurrentPlaceholder()
			return m, nil
		}

		displayInput := msg.DisplayInput
		expandedInput := msg.ExpandedInput
		if IsSlashCommand(displayInput) {
			return m.handleSlashCommand(displayInput)
		}
		if m.busy {
			m.queue.Enqueue(displayInput, expandedInput)
			m.statusBar.setQueueLen(m.queue.Len())
			if m.box != nil {
				m.box.Println(renderSystemLine(
					fmt.Sprintf("queued [%d]: %s", m.queue.Len(), truncateForQueue(displayInput, 60))))
			}
			return m, nil
		}
		if m.sessionStore != nil && m.currentSessionID == 0 {
			cmds = append(cmds, m.createSessionCmd(displayInput))
		} else {
			m.ag.UpdateSessionTitle(m.ctx, displayInput)
		}
		m.busy = true
		m.input.SetBusy(true)
		m.reqID++ // 새 요청 세대 — stale AgentDoneMsg 무시에 사용
		m.activeTools.Clear()
		m.progress.Reset()
		m.stats.Start()
		if m.box != nil {
			if m.turnCount > 0 {
				m.box.Println(renderTurnSeparator(m.width))
			}
			m.box.Println(renderUserInputLine(displayInput, m.width))
		}
		m.turnCount++
		label := m.thinkingLabel
		if label == "" {
			label = "thinking..."
		}
		cmds = append(cmds, m.runAgent(expandedInput), m.sp.Tick, m.shimmer.Start(label))
		return m, tea.Batch(cmds...)

	case SystemMsg:
		if m.box != nil {
			m.box.Println(renderSystemLine(string(msg)))
		}
		return m, nil

	case TokenMsg:
		m.shimmer.RecordActivity()
		m.streamTokens += string(msg)
		if time.Since(m.lastStreamAt) > 150*time.Millisecond {
			m.streamLines = renderStreamingPreview(m.streamTokens, m.mdRend, &m.streamCache, 0)
			m.lastStreamAt = time.Now()
		}
		return m, nil

	case ThinkingTokenMsg:
		m.shimmer.RecordActivity()
		m.thinkBuf += string(msg)
		m.shimmer.SetHint(thinkHint(m.thinkBuf, 60))
		return m, nil

	case ThinkingStartMsg:
		m.thinkingLabel = ThinkingLabel(msg.Tier, m.cfg.LLM.Model) // cfg?먯꽌 紐⑤뜽紐낆쓣 媛?몄샃?덈떎.
		m.shimmer.SetText(m.thinkingLabel)
		// ??LLM ?몄텧 ???댁쟾 thinking 踰꾪띁/?뚰듃 珥덇린??
		m.thinkBuf = ""
		m.shimmer.SetHint("")
		return m, nil

	case ErrorMsg:
		m.busy = false
		m.shimmer.Stop()
		if m.box != nil {
			m.box.Println(renderSystemLine(fmt.Sprintf("ERROR: %v", msg.Err)))
		}
		// ?먯뿉 ?湲?以묒씤 ?낅젰???덈떎硫??ㅼ쓬 ?낅젰??泥섎━?⑸땲??
		if entry, ok := m.queue.Dequeue(); ok {
			return m, tea.Sequence(
				NewSystemCmd("queued [%d]: %s", m.queue.Len()+1, truncateForQueue(entry.displayInput, 60)),
				NewSubmitCmd(entry.displayInput, entry.expandedInput),
			)
		}
		return m, nil
	}

	if next, cmd, handled := m.handleSystemMsg(msg); handled {
		return next, cmd
	}
	if next, cmd, handled := m.handleToolMsg(msg); handled {
		return next, cmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	m = m.resize()
	return m, tea.Batch(cmds...)
}

// RequestQuestion? QuestionHandler ?명꽣?섏씠?ㅻ? 援ы쁽?⑸땲??
func (m AppModel) RequestQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error) {
	if m.selectHandler == nil {
		return tools.QuestionResponse{SelectedIndex: -1}, fmt.Errorf("selection handler unavailable")
	}

	allowCustomInput := req.AllowCustomInput || len(req.Options) == 0
	opts := make([]SelectOption, len(req.Options))
	for i, opt := range req.Options {
		opts[i] = SelectOption{
			Label:       opt.Label,
			Description: opt.Description,
			HideOther:   !allowCustomInput,
		}
	}

	res, err := m.selectHandler.RequestSelectCtxWithHeader(ctx, req.Question, opts, req.Header)
	if err != nil {
		return tools.QuestionResponse{SelectedIndex: -1}, err
	}

	return tools.QuestionResponse{
		SelectedIndex: res.Index,
		SelectedLabel: res.Label,
		UserInput:     res.Label,
		IsOther:       res.IsOther,
	}, nil
}

// RequestForm은 QuestionHandler 인터페이스를 구현한다.
func (m AppModel) RequestForm(ctx context.Context, req tools.FormRequest) (tools.FormResponse, error) {
	if m.formHandler == nil {
		return tools.FormResponse{Cancelled: true}, fmt.Errorf("form handler unavailable")
	}
	return m.formHandler.RequestForm(ctx, req)
}

// PendingInputSource는 대기 중인 입력을 제공한다.
func (m AppModel) PendingInputSource() interface{} {
	return m.queue
}



func (m AppModel) resize() AppModel {
	m.input.setWidth(m.width)
	maxInputH := max(1, min(m.height-5, max(3, m.height/3)))
	m.input.setMaxHeight(maxInputH)
	m.statusBar.setWidth(m.width)
	m.shimmer.width = m.width
	return m
}
