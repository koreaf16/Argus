package toolui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/types"
)

// 인라인 렌더링 공용 헬퍼.
// 도구 UI를 ASCII 전용으로 통일하기 위한 단일 출처.
// 외곽 들여쓰기(2칸)는 호출부(render.go padMultiline)가 적용한다.

// claude_cli MessageResponse 컴포넌트가 `  `+`⎿`+`  ` (2 + 1 + 2 = 5칸) prefix로 결과를 들여쓴다.
// 이 헬퍼들은 동일한 모양을 만들어낸다.
const (
	resultBranchPrefix = "  ⎿  " // 첫 줄 prefix (5칸 폭)
	resultContPrefix   = "     " // 둘째 줄부터의 5칸 정렬 들여쓰기
	hintIndent         = "     " // 키 힌트: claude_cli BackgroundHint paddingLeft=5 와 동일
	defaultMaxArg      = 160
)

// FormatToolCall은 ASCII 헤드라인을 만든다. 예: `Bash(ls -la)`.
// 다중 줄 args는 최대 2줄로 제한하고 초과분은 … 처리. (claude_cli MAX_COMMAND_DISPLAY_LINES=2 방식)
// 색상은 BodyColor (dim 아님 — 강조 약).
func FormatToolCall(toolName, args string, maxChars int, theme ThemeContext) string {
	if maxChars <= 0 {
		maxChars = defaultMaxArg
	}
	titleStyle := theme.Style(theme.BodyColor())
	argsStyle := theme.Style(theme.BodyColor())

	const maxDisplayLines = 2
	rawLines := strings.Split(args, "\n")
	// 뒤쪽 빈 줄 제거
	for len(rawLines) > 0 && strings.TrimSpace(rawLines[len(rawLines)-1]) == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	if len(rawLines) == 0 {
		return titleStyle.Render(toolName)
	}

	truncatedByLines := len(rawLines) > maxDisplayLines
	if truncatedByLines {
		rawLines = rawLines[:maxDisplayLines]
	}

	displayLines := make([]string, len(rawLines))
	for i, line := range rawLines {
		displayLines[i] = strings.TrimRight(strings.ReplaceAll(line, "\t", "  "), " ")
	}

	// 마지막 줄에 글자 수 제한 적용
	lastIdx := len(displayLines) - 1
	truncatedByChars := lipgloss.Width(displayLines[lastIdx]) > maxChars
	if truncatedByChars {
		displayLines[lastIdx] = TruncateToWidth(displayLines[lastIdx], maxChars-1)
	}
	if truncatedByLines || truncatedByChars {
		displayLines[lastIdx] += "…"
	}

	argsText := strings.Join(displayLines, "\n")
	return titleStyle.Render(toolName) + argsStyle.Render("("+argsText+")")
}

// FormatResultLines는 결과 본문을 claude_cli MessageResponse 패턴으로 렌더한다.
//   첫 줄: `  ⎿  content`
//   둘째 줄부터: `     content` (5칸 정렬)
// sameBlock=true: 한 결과 블록의 멀티라인 (bash stdout 패턴) — 첫 줄에만 ⎿.
// sameBlock=false: 각 라인이 독립 결과 (tool_group 자식 요약 패턴) — 모든 라인에 ⎿.
// isError=true이면 본문 색을 ErrorColor로 칠한다.
func FormatResultLines(lines []string, sameBlock, isError bool, theme ThemeContext) string {
	color := theme.BodyColor()
	if isError {
		color = theme.StatusErrorColor()
	}
	bodyStyle := theme.Style(color)
	branchStyle := theme.Style(theme.MutedColor())

	usable := max(theme.Width()-len(resultBranchPrefix), 20)

	var sb strings.Builder
	for i, line := range lines {
		line = strings.ReplaceAll(line, "\t", "    ")
		line = strings.ReplaceAll(line, "\r", "")
		display := TruncateToWidth(line, usable)
		if i > 0 {
			sb.WriteString("\n")
		}
		if i == 0 || !sameBlock {
			sb.WriteString(branchStyle.Render(resultBranchPrefix))
		} else {
			sb.WriteString(resultContPrefix)
		}
		sb.WriteString(bodyStyle.Render(display))
	}
	return sb.String()
}

// FormatStatusLine은 "Running...", "Done", "(No output)" 등 muted 마무리 라인을 렌더한다.
// 첫 라인이 다른 본문 뒤에 붙는 경우와 단독 첫 라인 두 케이스 모두 동일하게 ⎿ branch를 사용.
func FormatStatusLine(text string, theme ThemeContext) string {
	branchStyle := theme.Style(theme.MutedColor())
	textStyle := theme.Style(theme.MutedColor()).Italic(true)
	return branchStyle.Render(resultBranchPrefix) + textStyle.Render(text)
}

// FormatHiddenLinesHint는 "... +N lines (ctrl+o to expand)" 축약 힌트 라인을 렌더한다.
// tool_result 3줄 제한 초과 시 continuation prefix(5칸)와 함께 muted 색상으로 표시한다.
func FormatHiddenLinesHint(n int, theme ThemeContext) string {
	mutedStyle := theme.Style(theme.MutedColor())
	text := fmt.Sprintf("... +%d lines (ctrl+o to expand)", n)
	return resultContPrefix + mutedStyle.Render(text)
}

// FormatHintLine은 "[TAB to focus] [Ctrl+D to background]" 같은 키 힌트 라인을 렌더한다.
// focused=true면 Warning Bold, 아니면 muted.
func FormatHintLine(text string, focused bool, theme ThemeContext) string {
	if focused {
		return hintIndent + theme.Style(theme.StatusWarningColor()).Bold(true).Render(text)
	}
	return hintIndent + theme.Style(theme.MutedColor()).Render(text)
}

// NormalizeToolName은 내부 도구명을 사용자 표시용 이름으로 변환한다.
func NormalizeToolName(internal string) string {
	switch strings.ToLower(strings.TrimSpace(internal)) {
	case "bash":
		return "Bash"
	case "powershell":
		return "PowerShell"
	case "webfetch", "web_fetch":
		return "WebFetch"
	case "websearch", "web_search":
		return "WebSearch"
	case "fileread", "file_read", "read":
		return "Read"
	case "filewrite", "file_write", "write":
		return "Write"
	case "fileedit", "file_edit", "edit":
		return "Edit"
	case "grep":
		return "Grep"
	case "glob":
		return "Glob"
	case "todowrite":
		return "TodoWrite"
	case "todoread":
		return "TodoRead"
	case "lsp":
		return "LSP"
	case "server_connect":
		return "ServerConnect"
	case "server_inspect":
		return "ServerInspect"
	case "server_metrics":
		return "ServerMetrics"
	case "server_tunnel":
		return "ServerTunnel"
	case "server_copy":
		return "ServerCopy"
	case "":
		return "Tool"
	default:
		return strings.TrimSpace(internal)
	}
}

// TruncateToWidth는 lipgloss 폭 기준으로 문자열을 cap한다. 끝에 ...는 호출자 책임.
func TruncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	res := ""
	for _, r := range runes {
		if lipgloss.Width(res+string(r)) > width {
			break
		}
		res += string(r)
	}
	return res
}

// RenderTodoMarker는 단일 todo 항목 한 줄을 색상+마커로 반환한다.
// 마커: in_progress=▪ (squareSmallFilled), completed=✓ (tick), pending=□ (squareSmall).
// claude_cli TaskListV2와 동일한 figures 글리프를 사용한다.
// 들여쓰기는 호출부에서 추가한다.
func RenderTodoMarker(status types.TodoStatus, content string, theme ThemeContext) string {
	var marker, color string
	bold := false
	strike := false
	faint := false
	switch status {
	case types.TodoStatusInProgress:
		marker = "▪"
		color = theme.ClaudeAccentColor()
		bold = true
	case types.TodoStatusCompleted:
		marker = "✓"
		color = theme.StatusSuccessColor()
		strike = true
		faint = true
	default:
		marker = "□"
		color = theme.MutedColor()
	}
	markerStyle := theme.Style(color)
	if bold {
		markerStyle = markerStyle.Bold(true)
	}
	contentStyle := markerStyle
	if strike {
		contentStyle = contentStyle.Strikethrough(true)
	}
	if faint {
		contentStyle = contentStyle.Faint(true)
	}
	return markerStyle.Render(marker) + " " + contentStyle.Render(content)
}

// RenderTodoChecklist는 todo 목록을 캡한 후 묶음으로 렌더한다.
// 첫 줄(⎿ branch)은 카운트 헤더, 이후 5칸 정렬로 항목을 출력한다.
// 입력 순서를 그대로 유지한다. cap=8, 초과분은 "… +N pending, M completed".
// claude_cli TaskListV2 (src/components/TaskListV2.tsx) 와 동일한 형식.
func RenderTodoChecklist(items []types.TodoItem, theme ThemeContext) string {
	if len(items) == 0 {
		return ""
	}

	var inProgress, pending, completed int
	for _, it := range items {
		switch it.Status {
		case types.TodoStatusInProgress:
			inProgress++
		case types.TodoStatusCompleted:
			completed++
		default:
			pending++
		}
	}

	ordered := items

	const cap = 8
	overflowPending := 0
	overflowCompleted := 0
	if len(ordered) > cap {
		for _, it := range ordered[cap:] {
			switch it.Status {
			case types.TodoStatusCompleted:
				overflowCompleted++
			default:
				overflowPending++
			}
		}
		ordered = ordered[:cap]
	}

	usable := max(theme.Width()-len(resultBranchPrefix), 20)
	branchStyle := theme.Style(theme.MutedColor())
	headerDim := theme.Style(theme.MutedColor())
	headerNum := theme.Style(theme.BodyColor()).Bold(true)

	header := buildTodoHeader(len(items), completed, inProgress, pending, headerDim, headerNum)

	var sb strings.Builder
	sb.WriteString(branchStyle.Render(resultBranchPrefix))
	sb.WriteString(header)
	for _, it := range ordered {
		content := strings.TrimSpace(it.Content)
		if content == "" {
			content = strings.TrimSpace(it.ActiveForm)
		}
		// 글리프(1) + 공백(1) = 2칸 차감.
		content = TruncateToWidth(content, usable-2)
		sb.WriteString("\n")
		sb.WriteString(resultContPrefix)
		sb.WriteString(RenderTodoMarker(it.Status, content, theme))
	}

	if overflowPending > 0 || overflowCompleted > 0 {
		mutedStyle := theme.Style(theme.MutedColor()).Italic(true)
		sb.WriteString("\n")
		sb.WriteString(resultContPrefix)
		sb.WriteString(mutedStyle.Render(formatOverflow(overflowPending, overflowCompleted)))
	}

	return sb.String()
}

// buildTodoHeader는 "5 tasks (2 done, 1 in progress, 2 open)" 형식 헤더를 만든다.
// 숫자만 bold로 강조하고 나머지는 muted dim. 0인 카운트는 표시하지 않는다.
func buildTodoHeader(total, done, inProgress, open int, dim, num lipgloss.Style) string {
	noun := "tasks"
	if total == 1 {
		noun = "task"
	}
	parts := []string{}
	if done > 0 {
		parts = append(parts, num.Render(fmt.Sprintf("%d", done))+dim.Render(" done"))
	}
	if inProgress > 0 {
		parts = append(parts, num.Render(fmt.Sprintf("%d", inProgress))+dim.Render(" in progress"))
	}
	if open > 0 {
		parts = append(parts, num.Render(fmt.Sprintf("%d", open))+dim.Render(" open"))
	}
	head := num.Render(fmt.Sprintf("%d", total)) + dim.Render(" "+noun)
	if len(parts) == 0 {
		return head
	}
	return head + dim.Render(" (") + strings.Join(parts, dim.Render(", ")) + dim.Render(")")
}

func formatOverflow(pending, completed int) string {
	switch {
	case pending > 0 && completed > 0:
		return fmt.Sprintf("… +%d pending, %d completed", pending, completed)
	case pending > 0:
		return fmt.Sprintf("… +%d pending", pending)
	case completed > 0:
		return fmt.Sprintf("… +%d completed", completed)
	default:
		return ""
	}
}
