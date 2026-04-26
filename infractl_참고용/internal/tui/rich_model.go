// Package tui
// File: rich_model.go
// Description: 에이전트 실행 중 리치 렌더링을 담당하는 임시 BubbleTea 모델
// Responsibility: shimmer, progress tree, cmdbox, 스트리밍 마크다운, statusbar 렌더링

package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// RichModel은 에이전트 실행 중에만 동작하는 inline BubbleTea 모델이다.
// readline이 입력을 담당하므로 입력 처리가 없다.
type RichModel struct {
	chat     chatView
	shimmer  shimmerState
	progress *progressTree
	status   statusBar
	state    *SessionState

	width, height int
	busy          bool
	sp            spinner.Model
	currentTool   string
	currentTarget string
	thinkingLabel string // tier+model shimmer 레이블
	thinkBuf      string

	ctx    context.Context
	cancel context.CancelFunc
}

// NewRichModel은 새 RichModel을 생성한다.
func NewRichModel(state *SessionState, ctx context.Context, cancel context.CancelFunc) RichModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = StyleSpinner

	w := state.Width
	if w < 40 {
		w = 80
	}

	return RichModel{
		chat:     newChatView(w, 50),
		shimmer:  newShimmer(),
		progress: newProgressTree(),
		status: statusBar{
			model:        state.Model,
			serverCount:  state.ServerCount,
			width:        w,
			totalCostUSD: state.TotalCostUSD,
			inputTokens:  state.InputTokens,
			outputTokens: state.OutputTokens,
		},
		state:  state,
		width:  w,
		busy:   true,
		sp:     sp,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (m RichModel) Init() tea.Cmd {
	shimmerCmd := m.shimmer.Start("thinking...")
	return tea.Batch(
		tea.EnableMouseCellMotion,
		m.sp.Tick,
		shimmerCmd,
	)
}

func (m RichModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chat.setSize(m.width, m.height-2) // statusbar(1) + shimmer(1)
		m.status.setWidth(m.width)
		m.shimmer.width = m.width
		m.progress.width = m.width

	case tea.KeyMsg:
		// 실행 중 Ctrl+C/Esc만 처리 (에이전트 중단)
		if msg.Type == tea.KeyCtrlC || msg.String() == "esc" {
			m.cancel()
			m.busy = false
			m.shimmer.Stop()
			m.chat.AddError("사용자에 의해 중단되었습니다.")
			return m, tea.Quit
		}

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.chat.ScrollUp(3)
		case tea.MouseButtonWheelDown:
			m.chat.ScrollDown(3)
		}
		return m, nil

	case spinner.TickMsg:
		if m.busy {
			var cmd tea.Cmd
			m.sp, cmd = m.sp.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case ShimmerTickMsg:
		if m.busy {
			cmd := m.shimmer.Tick()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case UsageUpdateMsg:
		m.status.updateUsage(msg)
		m.state.AddUsage(msg.InputTokens, msg.OutputTokens, msg.CostUSD)
		return m, nil

	case TokenMsg:
		m.shimmer.RecordActivity()
		m.chat.AppendToken(string(msg))

	case ThinkingTokenMsg:
		m.shimmer.RecordActivity()
		m.thinkBuf += string(msg)
		m.shimmer.SetHint(thinkHint(m.thinkBuf, 60))
		m.chat.AppendThinkingToken(string(msg))

	case ToolStartMsg:
		m.currentTool = msg.Name
		m.currentTarget = msg.Target
		desc, _ := msg.Args["description"].(string)
		if phaseID, phaseName, ok := parsePhaseFromDescription(desc); ok {
			m.progress.AddToolWithPhase(msg.ToolID, msg.Name, msg.Target, phaseID, phaseName, msg.Args)
		} else {
			m.progress.AddTool(msg.ToolID, msg.Name, msg.Target, msg.Args)
		}
		cmd := m.chat.AddCmdBox(msg.Name, msg.Target, msg.Args)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case ShellOutputMsg:
		m.shimmer.RecordActivity()
		m.chat.AppendBoxOutput(msg.Line)
		m.progress.SetOutput(msg.ToolID, msg.Line)

	case ThinkingStartMsg:
		m.thinkBuf = ""
		m.shimmer.SetHint("")
		m.thinkingLabel = ThinkingLabel(msg.Tier, msg.Model)
		m.shimmer.SetText(m.thinkingLabel)

	case ToolEndMsg:
		m.currentTool = ""
		m.currentTarget = ""
		m.progress.CompleteTool(msg.ToolID, msg.Duration, msg.Success, msg.MetadataJSON)
		if msg.Name == "verify_complete" {
			if meta, ok := parseTaskProgressMetadata(msg.MetadataJSON); ok && strings.TrimSpace(meta.VerifiedByToolID) != "" {
				m.progress.UpdateTaskProgress(meta.VerifiedByToolID, msg.MetadataJSON)
			}
		}
		label := m.thinkingLabel
		if label == "" {
			label = "thinking..."
		}
		m.shimmer.SetText(label)
		m.chat.CompleteLastBox(msg.Result, msg.Duration, msg.Success, msg.MetadataJSON)

	case ResponseDoneMsg:
		m.thinkBuf = ""
		m.shimmer.SetHint("")
		m.shimmer.RecordActivity()
		m.chat.FinalizeAssistant(string(msg))

	case ErrorMsg:
		slog.Error("agent error", "err", msg.Err)
		m.chat.AddError(msg.Err.Error())
		m.busy = false
		m.shimmer.Stop()

	case AgentDoneMsg:
		m.busy = false
		m.shimmer.Stop()
		m.progress.Reset()
		return m, tea.Quit

	}

	// chatView 스피너/박스 업데이트
	chatCmd := m.chat.Update(msg)
	if chatCmd != nil {
		cmds = append(cmds, chatCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m RichModel) View() string {
	if m.width == 0 {
		return ""
	}

	parts := []string{m.chat.View()}

	// shimmer (busy일 때, chatView 바로 아래에 밀착)
	if m.busy && m.shimmer.active {
		parts = append(parts, m.shimmer.View())
	}

	// progress tree (도구 실행 중)
	if !m.progress.IsEmpty() {
		parts = append(parts, m.progress.View())
	}

	// 상태바 (마지막 줄)
	parts = append(parts, m.status.View())

	result := strings.Join(parts, "\n")
	return strings.TrimRight(result, "\n") + "\n"
}

// syncStateBack은 RichModel 종료 시 누적 상태를 SessionState에 기록한다.
func (m *RichModel) syncStateBack() {
	m.state.TotalCostUSD = m.status.totalCostUSD
	m.state.InputTokens = m.status.inputTokens
	m.state.OutputTokens = m.status.outputTokens
}

// StatusLine은 현재 상태바를 한 줄 문자열로 반환한다 (외부 사용).
func StatusLine(state *SessionState) string {
	sb := statusBar{
		model:        state.Model,
		serverCount:  state.ServerCount,
		width:        state.Width,
		totalCostUSD: state.TotalCostUSD,
		inputTokens:  state.InputTokens,
		outputTokens: state.OutputTokens,
	}
	return sb.View()
}

// PrintUserHeader는 사용자 입력을 Claude CLI 스타일로 출력한다.
func PrintUserHeader(input string) {
	header := StyleInputPrompt.Render("❯ ") + StyleCmdBoxToolName.Render("You")
	body := StyleUserMsg.Render("  " + input)
	fmt.Println("\n" + header)
	fmt.Println(body)
}

// PrintBanner는 시작 배너를 출력한다.
func PrintBanner(state *SessionState) {
	title := StyleBannerTitle.Render("Infractl") + " " + StyleBannerInfo.Render("v1.0.0")
	info := StyleInfoBarDim.Render(
		fmt.Sprintf("model: %s | %d workspaces", state.Model, state.ServerCount))
	fmt.Printf("\n  %s\n  %s\n\n", title, info)
}

// PrintInputBorderTop은 readline 입력 전 상단 border를 출력한다.
// ANSI escape 없이 순수 유니코드만 사용 (Windows readline 호환).
func PrintInputBorderTop(width int) {
	fmt.Println(strings.Repeat("─", max(width, 0)))
}

// PrintInputBorderBottom은 readline 입력 후 하단 border를 출력한다.
func PrintInputBorderBottom(width int) {
	fmt.Println(strings.Repeat("─", max(width, 0)))
}
