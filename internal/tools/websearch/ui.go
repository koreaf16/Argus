package websearch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

// WebSearchRenderer provides custom UI for the web_search tool.
type WebSearchRenderer struct{}

func init() {
	toolui.Register("web_search", &WebSearchRenderer{})
}

// WebSearchInteractiveModel implements toolui.InteractiveModel for web_search.
// Renders like the Bash tool: collapsible result list with "... N hidden (Ctrl+O)" pattern.
type WebSearchInteractiveModel struct {
	query      string
	allowed    []string
	blocked    []string
	theme      toolui.ThemeContext
	spinner    spinner.Model
	results    []SearchResult
	messages   []string
	duration   float64
	isExpanded bool
	isFinished bool
}

func (m *WebSearchInteractiveModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *WebSearchInteractiveModel) Update(msg tea.Msg) (toolui.InteractiveModel, tea.Cmd) {
	switch v := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(v)
		return m, cmd
	}
	return m, nil
}

func (m *WebSearchInteractiveModel) View() string {
	argsText := fmt.Sprintf("\"%s\"", m.query)
	if len(m.allowed) > 0 {
		argsText += fmt.Sprintf(", only allowing domains: %s", strings.Join(m.allowed, ", "))
	}
	if len(m.blocked) > 0 {
		argsText += fmt.Sprintf(", blocking domains: %s", strings.Join(m.blocked, ", "))
	}
	headline := toolui.FormatToolCall("WebSearch", argsText, 160, m.theme)

	var bodyLines []string
	switch {
	case !m.isFinished:
		bodyLines = []string{m.spinner.View() + " Searching..."}
	case len(m.results) == 0:
		if len(m.messages) > 0 {
			bodyLines = append(bodyLines, m.messages...)
		} else {
			bodyLines = []string{"No results found"}
		}
	default:
		timeStr := fmt.Sprintf("%.0fs", m.duration)
		if m.duration < 1 {
			timeStr = fmt.Sprintf("%.0fms", m.duration*1000)
		}
		bodyLines = append(bodyLines, fmt.Sprintf("Found %d results in %s", len(m.results), timeStr))

		resultLines := formatWebSearchResults(m.results)
		maxShow := 4
		if m.isExpanded {
			maxShow = len(resultLines)
		}
		if len(resultLines) > maxShow {
			bodyLines = append(bodyLines, resultLines[:maxShow]...)
			hidden := len(resultLines) - maxShow
			hint := "Ctrl+O to show"
			if m.isExpanded {
				hint = "Ctrl+O to collapse"
			}
			bodyLines = append(bodyLines, fmt.Sprintf("... %d results hidden (%s) ...", hidden, hint))
		} else {
			bodyLines = append(bodyLines, resultLines...)
		}
	}

	return headline + "\n" + toolui.FormatResultLines(bodyLines, true, false, m.theme)
}

func (m *WebSearchInteractiveModel) SetFocus(_ bool)                {}
func (m *WebSearchInteractiveModel) IsFocused() bool                { return false }
func (m *WebSearchInteractiveModel) SetExpanded(e bool)             { m.isExpanded = e }
func (m *WebSearchInteractiveModel) IsExpanded() bool               { return m.isExpanded }
func (m *WebSearchInteractiveModel) OnStreamDelta(_ string)         {}
func (m *WebSearchInteractiveModel) SetInputResponse(_ chan string) {}
func (m *WebSearchInteractiveModel) SetFinished(finished bool)      { m.isFinished = finished }

func (m *WebSearchInteractiveModel) SetResult(text string) {
	m.results, m.messages, m.duration = parseSearchOutput(text)
}

func parseSearchOutput(text string) (results []SearchResult, messages []string, durationSeconds float64) {
	var out SearchOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return
	}
	durationSeconds = out.DurationSeconds
	for _, r := range out.Results {
		if msg, ok := r.(string); ok {
			msg = strings.TrimSpace(msg)
			if msg != "" && !strings.HasPrefix(strings.ToLower(msg), "found ") {
				messages = append(messages, msg)
			}
			continue
		}
		obj, ok := r.(map[string]any)
		if !ok {
			continue
		}
		content, ok := obj["content"].([]any)
		if !ok {
			continue
		}
		for _, item := range content {
			b, err := json.Marshal(item)
			if err != nil {
				continue
			}
			var sr SearchResult
			if json.Unmarshal(b, &sr) == nil && sr.Title != "" {
				results = append(results, sr)
			}
		}
	}
	return
}

// formatWebSearchResults formats results as title + indented snippet lines.
func formatWebSearchResults(results []SearchResult) []string {
	var lines []string
	for _, r := range results {
		lines = append(lines, r.Title)
		if r.Snippet != "" {
			snip := r.Snippet
			runes := []rune(snip)
			if len(runes) > 80 {
				snip = string(runes[:77]) + "..."
			}
			lines = append(lines, "  "+snip)
		}
	}
	return lines
}

func (r *WebSearchRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	query, _ := args["query"].(string)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.Style(theme.StatusWarningColor())

	return &WebSearchInteractiveModel{
		query:   strings.TrimSpace(query),
		allowed: toStringSlice(args["allowed_domains"]),
		blocked: toStringSlice(args["blocked_domains"]),
		theme:   theme,
		spinner: s,
	}
}

func (r *WebSearchRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		query = strings.TrimSpace(fmt.Sprintf("%v", args["query"]))
	}
	allowed := toStringSlice(args["allowed_domains"])
	blocked := toStringSlice(args["blocked_domains"])

	argsText := fmt.Sprintf("\"%s\"", query)
	if len(allowed) > 0 {
		argsText += fmt.Sprintf(", only allowing domains: %s", strings.Join(allowed, ", "))
	}
	if len(blocked) > 0 {
		argsText += fmt.Sprintf(", blocking domains: %s", strings.Join(blocked, ", "))
	}
	return toolui.FormatToolCall("WebSearch", argsText, 160, theme)
}

func (r *WebSearchRenderer) RenderToolResult(text string, _ int64, theme toolui.ThemeContext) string {
	results, messages, durationSec := parseSearchOutput(text)
	var bodyLines []string
	if len(results) == 0 {
		if len(messages) > 0 {
			bodyLines = append(bodyLines, messages...)
		} else {
			bodyLines = []string{"No results found"}
		}
	} else {
		timeStr := fmt.Sprintf("%.0fs", durationSec)
		if durationSec < 1 {
			timeStr = fmt.Sprintf("%.0fms", durationSec*1000)
		}
		bodyLines = append(bodyLines, fmt.Sprintf("Found %d results in %s", len(results), timeStr))
		resultLines := formatWebSearchResults(results)
		const maxShow = 4
		if len(resultLines) > maxShow {
			bodyLines = append(bodyLines, resultLines[:maxShow]...)
			bodyLines = append(bodyLines, fmt.Sprintf("... %d results hidden (Ctrl+O to show) ...", len(resultLines)-maxShow))
		} else {
			bodyLines = append(bodyLines, resultLines...)
		}
	}
	return toolui.FormatResultLines(bodyLines, true, false, theme)
}

func toStringSlice(v any) []string {
	switch raw := v.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, 0, len(raw))
		for _, s := range raw {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
