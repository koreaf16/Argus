package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

type category struct {
	ID           string
	Key          string
	Name         string
	ExampleNames [examplesPerCategory]string
}

type renderContext struct {
	Width    int
	Frame    int
	Theme    demoTheme
	Category category
	Example  int
}

func catalog() []category {
	return []category{
		{
			ID:   "splash",
			Key:  "1",
			Name: "Splash",
			ExampleNames: [examplesPerCategory]string{
				"constellation", "compact-logo", "eye-grid", "creator-ribbon", "minimal-start",
			},
		},
		{
			ID:   "prompt",
			Key:  "2",
			Name: "Prompt Input",
			ExampleNames: [examplesPerCategory]string{
				"default-box", "scanline-box", "focus-glow", "multiline-dense", "yolo-warning",
			},
		},
		{
			ID:   "user",
			Key:  "3",
			Name: "User Bubble",
			ExampleNames: [examplesPerCategory]string{
				"current-box", "compact-line", "right-accent", "muted-card", "command-style",
			},
		},
		{
			ID:   "assistant",
			Key:  "4",
			Name: "Assistant",
			ExampleNames: [examplesPerCategory]string{
				"star-marker", "markdown-rich", "code-heavy", "table-heavy", "long-stream",
			},
		},
		{
			ID:   "thinking",
			Key:  "5",
			Name: "Thinking",
			ExampleNames: [examplesPerCategory]string{
				"eye-pulse", "shimmer-tail", "scanline", "stalled-waiting", "no-motion",
			},
		},
		{
			ID:   "footer",
			Key:  "6",
			Name: "Footer",
			ExampleNames: [examplesPerCategory]string{
				"full-grid", "compact-grid", "mode-first", "context-first", "minimal",
			},
		},
		{
			ID:   "tool_group",
			Key:  "7",
			Name: "Tool Group",
			ExampleNames: [examplesPerCategory]string{
				"search-read-list", "active-scan", "collapsed-summary", "expanded-tree", "finished",
			},
		},
		{
			ID:   "shell",
			Key:  "8",
			Name: "Shell Card",
			ExampleNames: [examplesPerCategory]string{
				"running-output", "focused-pty", "background-job", "error-exit", "compact-done",
			},
		},
		{
			ID:   "search_read",
			Key:  "9",
			Name: "Search/Read",
			ExampleNames: [examplesPerCategory]string{
				"web-search", "file-read", "glob-list", "mcp-resource", "citation-preview",
			},
		},
		{
			ID:   "edit_diff",
			Key:  "0",
			Name: "Edit/Diff",
			ExampleNames: [examplesPerCategory]string{
				"small-diff", "large-diff", "risky-write", "create-file", "patch-summary",
			},
		},
		{
			ID:   "modal",
			Key:  "-",
			Name: "Modal",
			ExampleNames: [examplesPerCategory]string{
				"approval", "destructive-confirm", "password", "ask-select", "ask-batch",
			},
		},
	}
}

func findCategory(cats []category, id string) (category, bool) {
	id = strings.TrimSpace(strings.ToLower(id))
	for _, cat := range cats {
		if strings.ToLower(cat.ID) == id || strings.ToLower(cat.Name) == id {
			return cat, true
		}
	}
	return category{}, false
}

func renderCategoryExample(cat category, idx int, ctx renderContext) string {
	if idx < 0 || idx >= examplesPerCategory {
		return ""
	}
	switch cat.ID {
	case "splash":
		return renderSplash(idx, ctx)
	case "prompt":
		return renderPrompt(idx, ctx)
	case "user":
		return renderUser(idx, ctx)
	case "assistant":
		return renderAssistant(idx, ctx)
	case "thinking":
		return renderThinking(idx, ctx)
	case "footer":
		return renderFooter(idx, ctx)
	case "tool_group":
		return renderToolGroup(idx, ctx)
	case "shell":
		return renderShell(idx, ctx)
	case "search_read":
		return renderSearchRead(idx, ctx)
	case "edit_diff":
		return renderEditDiff(idx, ctx)
	case "modal":
		return renderModal(idx, ctx)
	default:
		return ""
	}
}

func renderSplash(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)

	switch idx {
	case 0:
		top := paint(t.ANSI, "    ·      ∙        ·      ●       ∙      ·      ∙", t.Muted, "", false, false)
		logo := paint(t.ANSI, "█▀▀█ █▀▀█ █▀▀▀ █  █ █▀▀", t.Accent, "", true, false) + "\n" +
			paint(t.ANSI, "█▄▄█ █▄▄▀ █ ▀█ █  █ ▀▀█", t.Accent2, "", true, false) + "\n" +
			paint(t.ANSI, "█  █ █  █ ▀▀▀▀  ▀▀  ▀▀▀", t.Info, "", true, false)
		btm := paint(t.ANSI, " crafted by Jihoon Kwak  · one hundred eyes · never sleeps ", t.Warning, "", false, true)
		return strings.Join([]string{top, "", logo, "", btm}, "\n")
	case 1:
		return accentCard(width, t, t.Accent,
			"⦿ ARGUS",
			paint(t.ANSI, "model: gpt-5.5  provider: openai  effort: high", t.Muted, "", false, false),
			paint(t.ANSI, "workspace: local  cwd: C:\\Dev\\Argus", t.Text, "", false, false),
			paint(t.ANSI, "hint: type /model or /workspace to change context", t.Muted, "", false, false),
		)
	case 2:
		grid := []string{
			"◉ ◌ ◉ ◌ ◉ ◌ ◉ ◌ ◉",
			"◌ ◉ ◌ ◉ ◌ ◉ ◌ ◉ ◌",
			"◉ ◌ ◉ ◎ ◉ ◌ ◉ ◌ ◉",
		}
		for i := range grid {
			grid[i] = paint(t.ANSI, grid[i], t.Accent2, "", i == 1, false)
		}
		return accentCard(width, t, t.Accent2, "Argus Eye Grid", grid...)
	case 3:
		ribbon := paint(t.ANSI, "╾────────────✦", t.Warning, "", false, false) +
			" " + paint(t.ANSI, "crafted by", t.Warning, "", false, true) + " " +
			paint(t.ANSI, "곽 지 훈", t.Warning, "", true, false) + " " +
			paint(t.ANSI, "✦────────────╼", t.Warning, "", false, false)
		return accentCard(width, t, t.Warning, "Creator Ribbon", ribbon)
	default:
		return strings.Join([]string{
			paint(t.ANSI, "ARGUS", t.Text, "", true, false),
			paint(t.ANSI, "local • no sandbox • unconfigured", t.Muted, "", false, false),
			paint(t.ANSI, "type your message and press Enter", t.Muted, "", false, false),
		}, "\n")
	}
}

func renderPrompt(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)

	makePrompt := func(lines ...string) string {
		boxW := width - 2
		if boxW < 36 {
			boxW = 36
		}
		top := paint(t.ANSI, strings.Repeat("▄", boxW), t.PanelBG, "", false, false)
		bottom := paint(t.ANSI, strings.Repeat("▀", boxW), t.PanelBG, "", false, false)
		out := []string{top}
		for i, line := range lines {
			prefix := "  "
			if i == 0 {
				prefix = "> "
			}
			row := panelLine(boxW, t, prefix, line)
			out = append(out, row)
		}
		out = append(out, bottom)
		return strings.Join(out, "\n")
	}

	switch idx {
	case 0:
		return makePrompt("Type your message or @path/to/file")
	case 1:
		return makePrompt("Run security baseline scan on this repository", "with --strict mode and summarize findings")
	case 2:
		return makePrompt("Implement tool renderer fallback for unknown tool name", "focus: stable markdown + no flicker")
	case 3:
		return makePrompt("/workspace connect prod-k8s", "/server inspect prod-k8s", "/model openai/gpt-5.5")
	default:
		badge := paint(t.ANSI, "[YOLO]", t.Danger, "", true, false)
		return makePrompt(badge+" apply patch directly to config and continue without prompt")
	}
}

func renderUser(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)
	bodyColor := t.Text
	if idx == 3 {
		bodyColor = t.Muted
	}
	if idx == 4 {
		bodyColor = t.Warning
	}

	head := paint(t.ANSI, strings.Repeat("▄", width-2), t.PanelBG, "", false, false)
	foot := paint(t.ANSI, strings.Repeat("▀", width-2), t.PanelBG, "", false, false)

	body := []string{}
	switch idx {
	case 0:
		body = append(body, paint(t.ANSI, "> 우리 시스템 UI 방향을 signature 중심으로 잡자", bodyColor, t.PanelBG, true, false))
	case 1:
		body = append(body, paint(t.ANSI, "> 데모 먼저 만들고 코어 반영하자", bodyColor, t.PanelBG, true, false))
	case 2:
		body = append(body,
			paint(t.ANSI, "> 작업하자", bodyColor, t.PanelBG, true, false),
			paint(t.ANSI, "  카테고리별 예제 5개씩 넣어줘", bodyColor, t.PanelBG, false, false),
		)
	case 3:
		body = append(body, paint(t.ANSI, "> snapshot 출력 위주로 확인할게", bodyColor, t.PanelBG, false, false))
	default:
		body = append(body, paint(t.ANSI, "> /model openai/gpt-5.5", bodyColor, t.PanelBG, true, false))
	}

	return strings.Join(append([]string{head}, append(body, foot)...), "\n")
}

func renderAssistant(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)

	prefix := paint(t.ANSI, "✦ ", t.Accent, "", true, false)
	switch idx {
	case 0:
		return prefix + paint(t.ANSI, "데모 우선으로 진행하고 코어는 이후 반영하겠습니다.", t.Text, "", false, false)
	case 1:
		body := []string{
			prefix + paint(t.ANSI, "## Demo Output Plan", t.Accent2, "", true, false),
			"  " + paint(t.ANSI, "- category 11개", t.Text, "", false, false),
			"  " + paint(t.ANSI, "- category당 example 5개", t.Text, "", false, false),
			"  " + paint(t.ANSI, "- snapshot 모드/interactive 모드 동시 지원", t.Text, "", false, false),
		}
		return strings.Join(body, "\n")
	case 2:
		code := "[ code:go ]\n" +
			"  go build -o argus_demo.exe ./cmd/ui_demo\n" +
			"  argus_demo.exe --snapshot all --no-ansi\n" +
			"[ end ]"
		return prefix + "\n  " + paint(t.ANSI, code, t.Muted, "", false, false)
	case 3:
		table := "  category      examples\n" +
			"  splash        5\n" +
			"  prompt        5\n" +
			"  assistant     5\n" +
			"  ...           ..."
		return prefix + "\n" + paint(t.ANSI, table, t.Text, "", false, false)
	default:
		line := "Stable stream demo: rendering plain text immediately, markdown blocks at stable boundaries."
		return prefix + paint(t.ANSI, trimToWidth(wordwrap.String(line, width-4), width-2), t.Text, "", false, false)
	}
}

func renderThinking(idx int, ctx renderContext) string {
	t := ctx.Theme
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := frames[(ctx.Frame/2)%len(frames)]
	if !t.ANSI {
		frame = "|/-\\"[(ctx.Frame/2)%4 : (ctx.Frame/2)%4+1]
	}

	switch idx {
	case 0:
		return paint(t.ANSI, frame, t.Accent, "", true, false) + " " + paint(t.ANSI, "eye-pulse: synthesizing layout decisions...", t.Muted, "", false, true)
	case 1:
		return paint(t.ANSI, frame, t.Accent2, "", true, false) + " " + paint(t.ANSI, "shimmer-tail: matching prompt and user bubble grammar...", t.Muted, "", false, true)
	case 2:
		return paint(t.ANSI, "⦿", t.Info, "", true, false) + " " + paint(t.ANSI, "scanline: stabilizing markdown line flush window...", t.Muted, "", false, true)
	case 3:
		return paint(t.ANSI, frame, t.Warning, "", true, false) + " " + paint(t.ANSI, "waiting for output... (stalled > 3s)", t.Warning, "", false, false)
	default:
		return paint(t.ANSI, "·", t.Muted, "", false, false) + " " + paint(t.ANSI, "reduced motion: static status only", t.Muted, "", false, true)
	}
}

func renderFooter(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)

	label := paint(t.ANSI, " workspace (/directory)  sandbox       /model         context       memory      session      diff    tokens ", t.Muted, "", false, false)
	row := paint(t.ANSI, " [local] /Argus          no sandbox    gpt-5.5        23% used      251 MB      a1b2c3d4     +7/-2   13.2k  ", t.Text, "", false, false)

	switch idx {
	case 0:
		return strings.Join([]string{fitLine(label, width, t), fitLine(row, width, t)}, "\n")
	case 1:
		return paint(t.ANSI, " local /Argus   no sandbox   gpt-5.5   23% ctx   +7/-2   13.2k", t.Text, "", false, false)
	case 2:
		return paint(t.ANSI, " mode: plan  |  shift+tab: cycle permission mode  |  active tools: 3", t.Accent2, "", true, false)
	case 3:
		gauge := "[" + paint(t.ANSI, strings.Repeat("█", 3), t.Warning, "", false, false) + paint(t.ANSI, strings.Repeat("░", 7), t.Muted, "", false, false) + "] 30%"
		return paint(t.ANSI, " context ", t.Text, "", true, false) + gauge
	default:
		return paint(t.ANSI, " local • gpt-5.5 • 23% context • +7/-2", t.Muted, "", false, false)
	}
}

func renderToolGroup(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)
	frames := []string{"|", "/", "-", "\\"}
	spin := frames[(ctx.Frame/2)%len(frames)]

	switch idx {
	case 0:
		return accentCard(width, t, t.Accent,
			spin+" 2 search, 3 read, 1 list...",
			paint(t.ANSI, "› \"bubbletea markdown flicker\" in render.go", t.Muted, "", false, false),
		)
	case 1:
		return accentCard(width, t, t.Info,
			spin+" active-scan",
			paint(t.ANSI, "grep  /renderThinkingRow/", t.Text, "", false, false),
			paint(t.ANSI, "fileread internal/tui/render.go", t.Text, "", false, false),
		)
	case 2:
		return accentCard(width, t, t.Muted, "✓ 8 operations done (ctrl+o to expand)")
	case 3:
		return accentCard(width, t, t.Accent2,
			"expanded-tree",
			paint(t.ANSI, "  ├─ web_search \"argus cli design\"", t.Text, "", false, false),
			paint(t.ANSI, "  ├─ fileread cmd/ui_demo/main.go", t.Text, "", false, false),
			paint(t.ANSI, "  └─ grep \"renderCategoryExample\"", t.Text, "", false, false),
		)
	default:
		return accentCard(width, t, t.Success,
			"✓ finished",
			paint(t.ANSI, "summary: 3 search, 6 read, 1 list", t.Muted, "", false, false),
		)
	}
}

func renderShell(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴"}
	spin := frames[(ctx.Frame/2)%len(frames)]

	switch idx {
	case 0:
		return accentCard(width, t, t.Accent,
			spin+" bash npm run build",
			paint(t.ANSI, "   $ npm run build", t.Text, "", false, false),
			paint(t.ANSI, "   ✓ bundled in 2.3s", t.Success, "", false, false),
		)
	case 1:
		return accentCard(width, t, t.Warning,
			spin+" powershell interactive",
			paint(t.ANSI, "   [TAB to defocus] [Ctrl+D background]", t.Warning, "", true, false),
		)
	case 2:
		return accentCard(width, t, t.Info,
			spin+" bash background task",
			paint(t.ANSI, "   task_id: job-29f7  target: prod-k8s", t.Muted, "", false, false),
		)
	case 3:
		return accentCard(width, t, t.Danger,
			"✕ exit code 1",
			paint(t.ANSI, "stderr: permission denied while opening cache", t.Danger, "", false, false),
		)
	default:
		return accentCard(width, t, t.Success, "✓ done in 184ms")
	}
}

func renderSearchRead(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)

	switch idx {
	case 0:
		return accentCard(width, t, t.Info,
			"⦿ web_search",
			paint(t.ANSI, `query: "bubbletea streaming markdown stable line"`, t.Text, "", false, false),
			paint(t.ANSI, "results: 10  duration: 2.1s", t.Muted, "", false, false),
		)
	case 1:
		return accentCard(width, t, t.Accent,
			"⦿ fileread",
			paint(t.ANSI, "path: internal/tui/render.go", t.Text, "", false, false),
			paint(t.ANSI, "lines: 1..260", t.Muted, "", false, false),
		)
	case 2:
		return accentCard(width, t, t.Accent2,
			"⦿ glob/list",
			paint(t.ANSI, "pattern: internal/tui/*.go", t.Text, "", false, false),
			paint(t.ANSI, "matches: 27", t.Muted, "", false, false),
		)
	case 3:
		return accentCard(width, t, t.Warning,
			"⦿ mcp resource read",
			paint(t.ANSI, "server: openai-docs", t.Text, "", false, false),
			paint(t.ANSI, "uri: docs://models/latest", t.Muted, "", false, false),
		)
	default:
		return accentCard(width, t, t.Success,
			"citation preview",
			paint(t.ANSI, "• https://charm.sh/bubbletea", t.Info, "", false, false),
			paint(t.ANSI, "• https://github.com/charmbracelet/lipgloss", t.Info, "", false, false),
		)
	}
}

func renderEditDiff(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)

	switch idx {
	case 0:
		return accentCard(width, t, t.Accent,
			"small diff",
			paint(t.ANSI, "- return renderLegacy()", t.Danger, "", false, false),
			paint(t.ANSI, "+ return renderFrame()", t.Success, "", false, false),
		)
	case 1:
		lines := []string{
			paint(t.ANSI, "- old spinner frames = [|/-\\]", t.Danger, "", false, false),
			paint(t.ANSI, "+ new spinner frames = [⠋⠙⠹⠸⠼⠴]", t.Success, "", false, false),
			paint(t.ANSI, "+ reduced motion fallback added", t.Success, "", false, false),
		}
		return accentCard(width, t, t.Accent2, "large diff", lines...)
	case 2:
		return accentCard(width, t, t.Warning,
			"risky write warning",
			paint(t.ANSI, "target: internal/tui/render.go", t.Text, "", false, false),
			paint(t.ANSI, "scope: animation and cursor behavior", t.Warning, "", false, false),
		)
	case 3:
		return accentCard(width, t, t.Info,
			"create file",
			paint(t.ANSI, "+ cmd/ui_demo/render.go", t.Success, "", false, false),
			paint(t.ANSI, "+ cmd/ui_demo/theme.go", t.Success, "", false, false),
		)
	default:
		return accentCard(width, t, t.Muted,
			"patch summary",
			paint(t.ANSI, "files: 3  insertions: 900+  deletions: 120", t.Text, "", false, false),
		)
	}
}

func renderModal(idx int, ctx renderContext) string {
	t := ctx.Theme
	width := maxInt(72, ctx.Width)
	if width > 96 {
		width = 96
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(width)
	if t.ANSI {
		box = box.BorderForeground(lipgloss.Color(t.Accent))
	}

	var body string
	switch idx {
	case 0:
		body = strings.Join([]string{
			paint(t.ANSI, "Tool Permission", t.Accent, "", true, false),
			"Allow this tool execution?",
			"",
			paint(t.ANSI, "tool: shell_command", t.Muted, "", false, false),
			paint(t.ANSI, `command: go build -o argus_demo.exe ./cmd/ui_demo`, t.Text, "", false, false),
			"",
			paint(t.ANSI, "[allow] [deny]", t.Success, "", true, false),
		}, "\n")
	case 1:
		body = strings.Join([]string{
			paint(t.ANSI, "Destructive Action", t.Danger, "", true, false),
			"Delete 42 generated files from demo snapshots?",
			"",
			paint(t.ANSI, "[YES DELETE] [cancel]", t.Danger, "", true, false),
		}, "\n")
	case 2:
		body = strings.Join([]string{
			paint(t.ANSI, "Password Required", t.Warning, "", true, false),
			"Enter ssh password for prod-k8s:",
			"",
			paint(t.ANSI, "********", t.Text, t.PanelBG, false, false),
		}, "\n")
	case 3:
		body = strings.Join([]string{
			paint(t.ANSI, "Question", t.Accent2, "", true, false),
			"Select rollout mode",
			"",
			paint(t.ANSI, "> canary", t.Accent2, "", true, false),
			"  blue-green",
			"  immediate",
		}, "\n")
	default:
		body = strings.Join([]string{
			paint(t.ANSI, "Questions", t.Info, "", true, false),
			"1) Environment: prod",
			"2) Risk level: medium",
			"3) Notify channel: #ops",
			"",
			paint(t.ANSI, "[submit]", t.Success, "", true, false),
		}, "\n")
	}
	return box.Render(body)
}
