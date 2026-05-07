package serverconnect

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

func init() {
	toolui.Register("server_connect", &ServerConnectRenderer{})
}

// branchPrefix / contPrefix는 toolui.inline.go의 unexported 상수와 동일한 값.
const (
	scBranchPrefix = "  ⎿  "
	scContPrefix   = "     "
)

type serverConnectPhase int

const (
	phaseConnecting        serverConnectPhase = iota
	phaseInspecting                           // inspect 결과 수신 중
	phaseInventoryScanning                    // 인벤토리 스캔 중 (header 수신 전)
	phaseInventoryHeader                      // system_header 수신 완료, 나머지 스캔 중
	phaseInventoryReady                       // 인벤토리 전체 완료
	phaseInventoryFailed                      // 인벤토리 실패
)

// Stream markers used by the serverconnect tool and renderer.
// These strings must stay in sync with the constants used in modal_server_list.go.
const (
	inventoryScanningMarker = "[ARGUS_INVENTORY_SCANNING]"
	inventoryHeaderMarker   = "[ARGUS_INVENTORY_HEADER]"
	// inventoryReadyPrefix — 형식: [ARGUS_INVENTORY_READY:<durationMs>]
	inventoryReadyPrefix  = "[ARGUS_INVENTORY_READY:"
	inventoryFailedMarker = "[ARGUS_INVENTORY_FAILED]"
)

type ServerConnectInteractiveModel struct {
	alias      string
	theme      toolui.ThemeContext
	sp         spinner.Model
	phase      serverConnectPhase
	details    string
	errResult  string
	isFinished bool

	// 인벤토리 페이즈 데이터
	inventoryLines      []string
	inventoryDurationMs int64
	inventoryScanStart  time.Time
	inventoryErr        string // phaseInventoryFailed 시 에러 메시지
}

func (m *ServerConnectInteractiveModel) Init() tea.Cmd { return m.sp.Tick }

func (m *ServerConnectInteractiveModel) Update(msg tea.Msg) (toolui.InteractiveModel, tea.Cmd) {
	if tick, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(tick)
		return m, cmd
	}
	return m, nil
}

func (m *ServerConnectInteractiveModel) View() string {
	headline := toolui.FormatToolCall("ServerConnect", m.alias, 80, m.theme)
	branchStyle := m.theme.Style(m.theme.MutedColor())
	branch := branchStyle.Render(scBranchPrefix)

	var body strings.Builder

	errorStyle := m.theme.Style(m.theme.StatusErrorColor())

	if m.isFinished {
		if m.errResult != "" {
			body.WriteString(branch)
			body.WriteString(errorStyle.Render("✘ " + m.errResult))
		} else {
			m.renderConnectedSection(&body, branch)
			switch m.phase {
			case phaseInventoryReady:
				m.renderInventoryReadySection(&body, branch)
			case phaseInventoryFailed:
				m.renderInventoryFailedSection(&body, branch)
			}
		}
	} else {
		switch m.phase {
		case phaseInspecting:
			body.WriteString(branch)
			body.WriteString(fmt.Sprintf("%s Inspecting...", m.sp.View()))
		case phaseInventoryScanning:
			m.renderConnectedSection(&body, branch)
			body.WriteString("\n")
			body.WriteString(branch)
			body.WriteString(branchStyle.Render(fmt.Sprintf("%s inventory scanning...", m.sp.View())))
		case phaseInventoryHeader:
			m.renderConnectedSection(&body, branch)
			m.renderInventoryHeaderSection(&body, branch)
			body.WriteString("\n")
			body.WriteString(scContPrefix)
			body.WriteString(branchStyle.Render(fmt.Sprintf("  %s scanning services...", m.sp.View())))
		case phaseInventoryReady:
			m.renderConnectedSection(&body, branch)
			m.renderInventoryReadySection(&body, branch)
		case phaseInventoryFailed:
			m.renderConnectedSection(&body, branch)
			m.renderInventoryFailedSection(&body, branch)
		default:
			body.WriteString(branch)
			body.WriteString(fmt.Sprintf("%s Connecting to %s...", m.sp.View(), m.alias))
		}
	}

	return headline + "\n" + body.String()
}

func (m *ServerConnectInteractiveModel) renderConnectedSection(b *strings.Builder, branch string) {
	successStyle := m.theme.Style(m.theme.StatusSuccessColor())
	b.WriteString(branch)
	b.WriteString(successStyle.Render("✔ Connected to " + m.alias))
	if m.details != "" {
		b.WriteString("\n")
		b.WriteString(scContPrefix)
		b.WriteString(m.theme.Style(m.theme.MutedColor()).Render(m.details))
	}
}

func (m *ServerConnectInteractiveModel) renderInventoryReadySection(b *strings.Builder, branch string) {
	successStyle := m.theme.Style(m.theme.StatusSuccessColor())
	catStyle := m.theme.Style(m.theme.ToolUseColor())
	mutedStyle := m.theme.Style(m.theme.MutedColor())

	b.WriteString("\n")
	b.WriteString(branch)
	dur := fmt.Sprintf("%.1fs", float64(m.inventoryDurationMs)/1000)
	b.WriteString(successStyle.Render(fmt.Sprintf("✔ inventory ready (%s)", dur)))

	for _, line := range m.inventoryLines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(scContPrefix)
		if strings.HasPrefix(line, " ") {
			// 연속 행 (카테고리 없음)
			b.WriteString(mutedStyle.Render("  " + strings.TrimSpace(line)))
			continue
		}
		// "category   value" 형식
		if idx := strings.Index(line, "   "); idx >= 0 {
			cat := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+3:])
			b.WriteString(catStyle.Render("▸ " + cat))
			b.WriteString("  ")
			b.WriteString(mutedStyle.Render(val))
		} else {
			b.WriteString(mutedStyle.Render("  " + line))
		}
	}
}

// renderInventoryHeaderSection shows partial inventory lines during the header phase.
func (m *ServerConnectInteractiveModel) renderInventoryHeaderSection(b *strings.Builder, branch string) {
	catStyle := m.theme.Style(m.theme.ToolUseColor())
	mutedStyle := m.theme.Style(m.theme.MutedColor())
	b.WriteString("\n")
	b.WriteString(branch)
	b.WriteString(mutedStyle.Render("inventory scanning..."))
	for _, line := range m.inventoryLines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(scContPrefix)
		if idx := strings.Index(line, "   "); idx >= 0 {
			cat := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+3:])
			b.WriteString(catStyle.Render("▸ " + cat))
			b.WriteString("  ")
			b.WriteString(mutedStyle.Render(val))
		} else {
			b.WriteString(mutedStyle.Render("  " + line))
		}
	}
}

// renderInventoryFailedSection shows the inventory failure indicator.
func (m *ServerConnectInteractiveModel) renderInventoryFailedSection(b *strings.Builder, branch string) {
	errorStyle := m.theme.Style(m.theme.StatusErrorColor())
	mutedStyle := m.theme.Style(m.theme.MutedColor())
	b.WriteString("\n")
	b.WriteString(branch)
	b.WriteString(errorStyle.Render("✘ inventory failed"))
	if m.inventoryErr != "" {
		b.WriteString("\n")
		b.WriteString(scContPrefix)
		b.WriteString(mutedStyle.Render(m.inventoryErr))
	}
}

func (m *ServerConnectInteractiveModel) OnStreamDelta(delta string) {
	if strings.Contains(delta, "[ARGUS_SERVER_CONNECT:connected]") {
		m.phase = phaseInspecting
		return
	}
	if strings.HasPrefix(delta, inventoryScanningMarker) {
		m.phase = phaseInventoryScanning
		m.inventoryScanStart = time.Now()
		return
	}
	if strings.HasPrefix(delta, inventoryHeaderMarker) {
		m.phase = phaseInventoryHeader
		rest := delta[len(inventoryHeaderMarker):]
		rest = strings.TrimPrefix(rest, "\n")
		if strings.TrimSpace(rest) != "" {
			m.inventoryLines = strings.Split(rest, "\n")
		}
		return
	}
	if strings.HasPrefix(delta, inventoryReadyPrefix) {
		m.phase = phaseInventoryReady
		// 형식: [ARGUS_INVENTORY_READY:<ms>]\n<lines...>
		endBracket := strings.Index(delta, "]")
		if endBracket >= 0 {
			durationStr := delta[len(inventoryReadyPrefix):endBracket]
			if ms, err := strconv.ParseInt(durationStr, 10, 64); err == nil {
				m.inventoryDurationMs = ms
			} else if !m.inventoryScanStart.IsZero() {
				m.inventoryDurationMs = time.Since(m.inventoryScanStart).Milliseconds()
			}
			rest := delta[endBracket+1:]
			rest = strings.TrimPrefix(rest, "\n")
			if strings.TrimSpace(rest) != "" {
				m.inventoryLines = strings.Split(rest, "\n")
			}
		}
		return
	}
	if strings.HasPrefix(delta, inventoryFailedMarker) {
		m.phase = phaseInventoryFailed
		rest := delta[len(inventoryFailedMarker):]
		rest = strings.TrimPrefix(rest, "\n")
		m.inventoryErr = strings.TrimSpace(rest)
		return
	}
	if m.phase == phaseInspecting {
		if d := strings.TrimSpace(delta); d != "" {
			m.details = d
		}
	}
}

func (m *ServerConnectInteractiveModel) SetFinished(finished bool) { m.isFinished = finished }

func (m *ServerConnectInteractiveModel) SetResult(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// 성공/스캔 마커 제거
	text = strings.ReplaceAll(text, "[ARGUS_SERVER_CONNECT:connected]", "")
	text = strings.ReplaceAll(text, inventoryScanningMarker, "")
	text = strings.ReplaceAll(text, inventoryHeaderMarker, "")
	text = strings.ReplaceAll(text, inventoryFailedMarker, "")
	// inventoryReadyPrefix 마커 제거 ([ARGUS_INVENTORY_READY:<ms>])
	if idx := strings.Index(text, inventoryReadyPrefix); idx >= 0 {
		endBracket := strings.Index(text[idx:], "]")
		if endBracket >= 0 {
			text = text[:idx] + text[idx+endBracket+1:]
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// 에러 판별: "failed", "error", "연결 실패" 접두사
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "failed") || strings.HasPrefix(lower, "error") || strings.HasPrefix(lower, "연결 실패") {
		m.errResult = text
		return
	}
	// inspect 요약 라인 추출 (아직 details가 없을 때 폴백)
	if m.details == "" {
		for _, line := range strings.Split(text, "\n") {
			l := strings.TrimSpace(line)
			if strings.Contains(l, "OS:") || strings.Contains(l, "user:") || strings.Contains(l, "shell:") {
				m.details = l
				break
			}
		}
	}
}

func (m *ServerConnectInteractiveModel) SetFocus(bool)               {}
func (m *ServerConnectInteractiveModel) IsFocused() bool             { return false }
func (m *ServerConnectInteractiveModel) SetExpanded(bool)            {}
func (m *ServerConnectInteractiveModel) IsExpanded() bool            { return false }
func (m *ServerConnectInteractiveModel) SetInputResponse(chan string) {}

// ── Renderer ─────────────────────────────────────────────────────────────────

type ServerConnectRenderer struct{}

func (r *ServerConnectRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	alias, _ := args["server"].(string)
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "?"
	}
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.Style(theme.ClaudeAccentColor())
	return &ServerConnectInteractiveModel{alias: alias, theme: theme, sp: s}
}

func (r *ServerConnectRenderer) RenderToolUse(args map[string]any, streamBody string, theme toolui.ThemeContext) string {
	alias, _ := args["server"].(string)
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "?"
	}
	headline := toolui.FormatToolCall("ServerConnect", alias, 80, theme)

	streamBody = strings.TrimSpace(streamBody)
	if streamBody == "" {
		return headline
	}

	var lines []string
	if strings.Contains(streamBody, "[ARGUS_SERVER_CONNECT:connected]") {
		lines = append(lines, "✔ "+alias+"에 연결됨")
		for _, line := range strings.Split(streamBody, "\n") {
			l := strings.TrimSpace(line)
			if strings.Contains(l, "OS:") || strings.Contains(l, "user:") {
				lines = append(lines, l)
				break
			}
		}
	} else {
		for _, line := range strings.Split(streamBody, "\n") {
			l := strings.TrimSpace(line)
			if l != "" && l != "[ARGUS_SERVER_CONNECT:connected]" &&
				!strings.HasPrefix(l, inventoryScanningMarker) &&
				!strings.HasPrefix(l, inventoryReadyPrefix) {
				lines = append(lines, l)
			}
		}
	}
	if len(lines) == 0 {
		return headline
	}
	return headline + "\n" + toolui.FormatResultLines(lines, true, false, theme)
}

func (r *ServerConnectRenderer) RenderToolResult(resultText string, _ int64, theme toolui.ThemeContext) string {
	resultText = strings.TrimSpace(resultText)
	if resultText == "" {
		return ""
	}
	// 성공 마커가 있으면 InteractiveModel이 이미 표시했으므로 생략.
	if strings.Contains(resultText, "[ARGUS_SERVER_CONNECT:connected]") {
		return ""
	}
	// 에러 결과
	var lines []string
	for _, line := range strings.Split(resultText, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			lines = append(lines, l)
		}
	}
	return toolui.FormatResultLines(lines, true, true, theme)
}
