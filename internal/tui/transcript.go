package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

func (m *uiModel) applyPresentationEvent(evt presentation.Event) {
	switch evt.Kind {
	case presentation.EventState:
		m.footer = evt.Footer
		return
	case presentation.EventViewCleared:
		m.entries = m.entries[:0]
		clear(m.toolEntryByTaskID)
		m.closeAssistantEntry()
		m.closeThinkingEntry()
		m.toolUseOpen = false
		m.toolUseStreamIdx = -1
		m.assistantFlushedLines = 0
		m.assistantFlushedRenderedLines = 0
		m.lastPrintedIdx = 0
		m.activeTool = nil
		m.toolFocused = false
		return
	case presentation.EventAssistantDelta:
		// 대화 델타가 올 때만 싱킹 엔트리를 닫음
		m.closeThinkingEntry()
		m.appendAssistantDelta(evt.Text)
		return
	case presentation.EventAssistantDone:
		m.closeAssistantEntry()
		return
	case presentation.EventThinkingDelta:
		m.appendThinkingDelta(evt.Text)
		return
	case presentation.EventThinkingDone:
		m.closeThinkingEntry()
		return
	}

	// 중요: System, Notice 등의 일반 이벤트는 현재 진행 중인 스트림(Assistant)을 방해해서는 안 됨.
	// 이전에는 무조건 closeThinkingEntry()와 closeAssistantEntry()를 호출하여 흐름을 끊었음.
	// 이제는 사용자(User)가 직접 말을 걸거나 대화가 끝났을 때만 명시적으로 닫음.

	switch evt.Kind {
	case presentation.EventUser:
		m.closeAssistantEntry()
		m.closeThinkingEntry()
		if !m.toolUseOpen {
			m.activeTool = nil
			m.toolFocused = false
		}
		m.finalizeOpenToolGroup()
		m.appendEntry("user", "You", evt.Text, "")
	case presentation.EventAssistantText:
		m.closeThinkingEntry()
		if !m.toolUseOpen {
			m.activeTool = nil
			m.toolFocused = false
		}
		m.finalizeOpenToolGroup()
		m.appendEntry("assistant", "Assistant", evt.Text, "")
	case presentation.EventToolDelta:
		idx := m.resolveToolEntryIndex(evt.TaskID)
		m.appendToolDelta(evt.Text, evt.TaskID)
		if idx >= 0 && idx < len(m.entries) && m.entries[idx].Interactive != nil {
			m.entries[idx].Interactive.OnStreamDelta(evt.Text)
			if evt.InputResponse != nil {
				m.entries[idx].Interactive.SetInputResponse(evt.InputResponse)
			}
		} else if m.activeTool != nil {
			m.activeTool.OnStreamDelta(evt.Text)
			if evt.InputResponse != nil {
				m.activeTool.SetInputResponse(evt.InputResponse)
			}
		}
	case presentation.EventToolUse:
		body := strings.TrimSpace(evt.Input)
		if body == "" {
			body = "(no input)"
		}
		taskID := strings.TrimSpace(evt.TaskID)
		if taskID == "" {
			taskID = extractTaskIDFromToolInput(evt.Input)
		}

		// 동적 상호작용 모델 생성 시도
		var interactive toolui.InteractiveModel
		if renderer := toolui.GetRenderer(evt.ToolName); renderer != nil {
			var args map[string]any
			_ = json.Unmarshal([]byte(evt.Input), &args)
			if args == nil {
				args = make(map[string]any)
			}
			args["_tool_name"] = evt.ToolName
			args["_active_workspace"] = m.footer.Workspace
			interactive = renderer.CreateInteractiveModel(args, m)
		}

		sub := transcriptEntry{
			Kind:        "tool_use",
			Title:       "Tool Use: " + evt.ToolName,
			Body:        body,
			ToolName:    evt.ToolName,
			TaskID:      taskID,
			Interactive: interactive,
		}

		if isCollapsibleTool(evt.ToolName) {
			m.addToToolGroup(sub, evt.ToolName, evt.Input)
		} else {
			m.finalizeOpenToolGroup()
			m.appendEntry("tool_use", "Tool Use: "+evt.ToolName, body, evt.ToolName)
			if idx := len(m.entries) - 1; idx >= 0 {
				m.entries[idx].Interactive = interactive
				m.entries[idx].TaskID = taskID
				if taskID != "" {
					m.toolEntryByTaskID[taskID] = idx
				}
				if evt.InputResponse != nil && m.entries[idx].Interactive != nil {
					m.entries[idx].Interactive.SetInputResponse(evt.InputResponse)
				}
			}

			if isShellTool(evt.ToolName) {
				m.toolUseOpen = true
				m.toolUseStreamIdx = len(m.entries) - 1
			}
		}
		m.activeTool = interactive
	case presentation.EventToolResult:
		taskID := strings.TrimSpace(evt.TaskID)
		if idx := m.resolveToolEntryIndex(taskID); idx >= 0 && idx < len(m.entries) {
			if m.entries[idx].Interactive != nil {
				m.entries[idx].Interactive.SetFinished(true)
			}
		} else if m.activeTool != nil {
			m.activeTool.SetFinished(true)
		}
		if taskID != "" {
			delete(m.toolEntryByTaskID, taskID)
		}
		if taskID == "" || (m.toolUseStreamIdx >= 0 && m.toolUseStreamIdx < len(m.entries) && m.entries[m.toolUseStreamIdx].TaskID == taskID) {
			m.toolUseOpen = false
			m.toolUseStreamIdx = -1
		}
		result := transcriptEntry{
			Kind:      "tool_result",
			Title:     "Tool Result: " + emptyFallback(evt.ToolName, "-"),
			Body:      strings.TrimSpace(evt.Text),
			ToolName:  evt.ToolName,
			TaskID:    taskID,
			Collapsed: true,
		}
		if !m.addResultToToolGroup(result) {
			m.appendEntry("tool_result", "Tool Result: "+emptyFallback(evt.ToolName, "-"), evt.Text, evt.ToolName)
		}
	case presentation.EventNotice:
		m.appendEntry("notice", "Notice", evt.Text, "")
	case presentation.EventSystem:
		m.appendEntry("system", "System", evt.Text, "")
	case presentation.EventError:
		m.appendEntry("error", "Error", evt.Text, "")
	case presentation.EventApprovalRequest:
		body := "tool: " + evt.ToolName
		if strings.TrimSpace(evt.Input) != "" {
			body += "\n" + evt.Input
		}
		m.appendEntry("approval", "Permission Required", body, evt.ToolName)
	case presentation.EventApprovalDecision:
		m.appendEntry("approval", "Permission Decision", evt.Decision, "")
	case presentation.EventPasswordRequest:
		body := strings.TrimSpace(evt.Prompt)
		if body == "" {
			body = "Password:"
		}
		m.appendEntry("password", "Password Prompt", body, evt.ToolName)
	case presentation.EventAskUserRequest:
		body := strings.TrimSpace(evt.Prompt)
		if body == "" {
			body = "Question"
		}
		if strings.TrimSpace(evt.Input) != "" {
			body += "\n" + strings.TrimSpace(evt.Input)
		}
		m.appendEntry("question", "Question Prompt", body, evt.ToolName)
	case presentation.EventPlanReady:
		m.appendEntry("plan", "Plan Ready", fmt.Sprintf("approved steps: %d", evt.Count), "")
	case presentation.EventPlanStep:
		title := fmt.Sprintf("Plan Step %d/%d", evt.StepIndex, evt.StepTotal)
		m.appendEntry("plan", title, fmt.Sprintf("%s: %s", evt.ToolName, evt.Text), evt.ToolName)
	case presentation.EventPlanDecision:
		title := fmt.Sprintf("Plan Decision %d/%d", evt.StepIndex, evt.StepTotal)
		m.appendEntry("plan", title, evt.Decision, "")
	case presentation.EventPlanResult:
		title := fmt.Sprintf("Plan Result %d/%d", evt.StepIndex, evt.StepTotal)
		m.appendEntry("plan", title, evt.Text, "")
	}
}

func (m *uiModel) appendAssistantDelta(delta string) {
	if !m.assistantOpen || m.assistantStreamIdx < 0 || m.assistantStreamIdx >= len(m.entries) {
		m.entries = append(m.entries, transcriptEntry{
			Kind:  "assistant",
			Title: "Assistant",
			Body:  delta,
		})
		m.assistantOpen = true
		m.assistantStreamIdx = len(m.entries) - 1
		m.assistantLastDelta = time.Now()
		m.assistantFlushedLines = 0
		m.assistantFlushedRenderedLines = 0
		m.trimEntriesIfNeeded()
		return
	}
	m.entries[m.assistantStreamIdx].Body += delta
	m.assistantLastDelta = time.Now()
}

func (m *uiModel) appendThinkingDelta(delta string) {
	if !m.thinkingOpen || m.thinkingStreamIdx < 0 || m.thinkingStreamIdx >= len(m.entries) {
		m.entries = append(m.entries, transcriptEntry{
			Kind:  "thinking",
			Title: "Thinking",
			Body:  delta,
		})
		m.thinkingOpen = true
		m.thinkingStreamIdx = len(m.entries) - 1
		m.thinkingLastDelta = time.Now()
		m.thinkingFlushedLines = 0
		m.trimEntriesIfNeeded()
		return
	}
	m.entries[m.thinkingStreamIdx].Body += delta
	m.thinkingLastDelta = time.Now()
}

func (m *uiModel) closeThinkingEntry() {
	if !m.thinkingOpen || m.thinkingStreamIdx < 0 || m.thinkingStreamIdx >= len(m.entries) {
		m.thinkingOpen = false
		m.thinkingStreamIdx = -1
		m.thinkingFlushedLines = 0
		return
	}
	m.removeEntryAt(m.thinkingStreamIdx)
	m.thinkingOpen = false
	m.thinkingStreamIdx = -1
	m.thinkingFlushedLines = 0
}

func (m *uiModel) removeEntryAt(idx int) {
	if idx < 0 || idx >= len(m.entries) {
		return
	}
	m.entries = append(m.entries[:idx], m.entries[idx+1:]...)
	if m.assistantStreamIdx > idx {
		m.assistantStreamIdx--
	} else if m.assistantStreamIdx == idx {
		m.closeAssistantEntry()
	}
	if m.thinkingStreamIdx > idx {
		m.thinkingStreamIdx--
	} else if m.thinkingStreamIdx == idx {
		m.thinkingOpen = false
		m.thinkingStreamIdx = -1
		m.thinkingFlushedLines = 0
	}
	m.rebuildToolEntryByTaskID()
}

func (m *uiModel) appendEntry(kind, title, body, toolName string) {
	m.entries = append(m.entries, transcriptEntry{
		Kind:     kind,
		Title:    title,
		Body:     strings.TrimSpace(body),
		ToolName: toolName,
	})
	m.trimEntriesIfNeeded()
}

func (m *uiModel) trimEntriesIfNeeded() {
	if m.maxEntries <= 0 || len(m.entries) <= m.maxEntries {
		m.rebuildToolEntryByTaskID()
		return
	}
	drop := len(m.entries) - m.maxEntries
	m.entries = append([]transcriptEntry(nil), m.entries[drop:]...)

	if m.assistantStreamIdx >= 0 {
		m.assistantStreamIdx -= drop
		if m.assistantStreamIdx < 0 {
			m.closeAssistantEntry()
		}
	}
	if m.thinkingStreamIdx >= 0 {
		m.thinkingStreamIdx -= drop
		if m.thinkingStreamIdx < 0 {
			m.thinkingOpen = false
			m.thinkingStreamIdx = -1
		}
	}
	m.rebuildToolEntryByTaskID()
}

func (m *uiModel) closeAssistantEntry() {
	m.assistantOpen = false
	m.assistantStreamIdx = -1
	// assistantFlushedLines는 여기서 리셋하지 않는다. EventAssistantDone 처리 시
	// closeAssistantEntry가 먼저 실행된 뒤 scrollbackCmd에서 finalizeAssistantStream이
	// 호출되는데, 거기서 flushed 만큼 잔여 부분만 println해야 중복 출력이 없다.
	// 새 assistant stream 시작 시 appendAssistantDelta가 어차피 0으로 리셋한다.
}

// isCollapsibleTool returns true for read/search operations that should be grouped.
func isCollapsibleTool(toolName string) bool {
	tn := strings.ToLower(toolName)
	switch tn {
	case "grep", "glob", "fileread", "web_search", "web_fetch", "lsp", "todoread", "read_mcp_resource", "list_mcp_resources":
		return true
	}
	return false
}

// classifyTool returns search/read/list classification for a tool.
func classifyTool(toolName string) (isSearch, isRead, isList bool) {
	tn := strings.ToLower(toolName)
	switch tn {
	case "grep", "web_search":
		return true, false, false
	case "glob", "list_mcp_resources":
		return false, false, true
	case "fileread", "web_fetch", "lsp", "todoread", "read_mcp_resource":
		return false, true, false
	}
	return false, false, false
}

// extractHint extracts a short display hint from tool input JSON.
func extractHint(toolName, input string) string {
	var inp map[string]any
	if err := json.Unmarshal([]byte(input), &inp); err != nil {
		return ""
	}

	tn := strings.ToLower(toolName)

	// Shell tools: show first few words of the command
	if tn == "bash" || tn == "powershell" {
		if cmd, ok := inp["command"].(string); ok && cmd != "" {
			fields := strings.Fields(strings.TrimSpace(cmd))
			if len(fields) > 0 {
				hint := fields[0]
				if len(fields) > 1 && !strings.HasPrefix(fields[1], "-") {
					hint += " " + fields[1]
				}
				if len(hint) > 40 {
					return hint[:40] + "…"
				}
				return hint + "…"
			}
		}
	}

	if q, ok := inp["query"].(string); ok && q != "" {
		return fmt.Sprintf(`"%s"`, q)
	}
	if p, ok := inp["pattern"].(string); ok && p != "" {
		hint := fmt.Sprintf(`"%s"`, p)
		if path, ok := inp["path"].(string); ok && path != "" {
			hint += " in " + filepath.Base(path)
		}
		return hint
	}
	if path, ok := inp["path"].(string); ok && path != "" {
		return filepath.Base(path)
	}
	if u, ok := inp["url"].(string); ok && u != "" {
		if len(u) > 50 {
			return u[:50] + "…"
		}
		return u
	}
	if uri, ok := inp["uri"].(string); ok && uri != "" {
		if len(uri) > 50 {
			return "..." + uri[len(uri)-47:]
		}
		return uri
	}
	return ""
}

// lastOpenToolGroup returns a pointer to the last entry if it's an active tool_group.
func (m *uiModel) lastOpenToolGroup() *transcriptEntry {
	if len(m.entries) == 0 {
		return nil
	}
	last := &m.entries[len(m.entries)-1]
	if last.Kind == "tool_group" && last.IsActive {
		return last
	}
	return nil
}

// addToToolGroup adds a tool_use sub-entry to the current group (or creates a new one).
func (m *uiModel) addToToolGroup(sub transcriptEntry, toolName, input string) {
	isSearch, isRead, isList := classifyTool(toolName)
	hint := extractHint(toolName, input)

	if last := m.lastOpenToolGroup(); last != nil {
		last.SubEntries = append(last.SubEntries, sub)
		if isSearch {
			last.SearchCount++
		}
		if isRead {
			last.ReadCount++
		}
		if isList {
			last.ListCount++
		}
		if hint != "" {
			last.LastHint = hint
		}
		return
	}

	g := transcriptEntry{
		Kind:       "tool_group",
		Collapsed:  true,
		IsActive:   true,
		SubEntries: []transcriptEntry{sub},
	}
	if isSearch {
		g.SearchCount++
	}
	if isRead {
		g.ReadCount++
	}
	if isList {
		g.ListCount++
	}
	if hint != "" {
		g.LastHint = hint
	}
	m.entries = append(m.entries, g)
	m.trimEntriesIfNeeded()
}

// addResultToToolGroup adds a tool_result sub-entry to the active group.
func (m *uiModel) addResultToToolGroup(result transcriptEntry) bool {
	if g := m.lastOpenToolGroup(); g != nil {
		g.SubEntries = append(g.SubEntries, result)
		return true
	}
	return false
}

// finalizeOpenToolGroup marks the current open tool_group as inactive.
func (m *uiModel) finalizeOpenToolGroup() {
	if g := m.lastOpenToolGroup(); g != nil {
		g.IsActive = false
	}
}

// appendToolDelta appends streaming stdout to a task-specific tool_use entry.
func (m *uiModel) appendToolDelta(delta, taskID string) {
	idx := m.resolveToolEntryIndex(taskID)
	if idx < 0 || idx >= len(m.entries) {
		return
	}
	m.entries[idx].StreamBody += delta
}

func (m *uiModel) resolveToolEntryIndex(taskID string) int {
	if taskID != "" {
		if idx, ok := m.toolEntryByTaskID[taskID]; ok {
			return idx
		}
	}
	if m.toolUseOpen && m.toolUseStreamIdx >= 0 && m.toolUseStreamIdx < len(m.entries) {
		return m.toolUseStreamIdx
	}
	return -1
}

func (m *uiModel) rebuildToolEntryByTaskID() {
	clear(m.toolEntryByTaskID)
	for idx := range m.entries {
		if m.entries[idx].Kind != "tool_use" {
			continue
		}
		taskID := strings.TrimSpace(m.entries[idx].TaskID)
		if taskID == "" {
			continue
		}
		m.toolEntryByTaskID[taskID] = idx
	}
}

func extractTaskIDFromToolInput(input string) string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return ""
	}
	if v, ok := raw["task_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	if v, ok := raw["job_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// isShellTool returns true for tools that stream stdout in real-time.
func isShellTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "bash", "powershell":
		return true
	default:
		return false
	}
}

// toggleLastToolGroup toggles the collapsed state of the most recent tool_group.
func (m *uiModel) toggleLastToolGroup() {
	for i := len(m.entries) - 1; i >= 0; i-- {
		if m.entries[i].Kind == "tool_group" {
			m.entries[i].Collapsed = !m.entries[i].Collapsed
			return
		}
	}
}
