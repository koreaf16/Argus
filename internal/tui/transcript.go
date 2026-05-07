package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/tasks"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

// toolCallNoiseRE는 native tool calling을 지원하지 않는 일부 LLM provider가 텍스트 본문에
// 흘려보내는 pseudo tool-call 패턴을 식별한다. assistant 본문에서 단순 strip하여
// raw text 노출을 막는다. 토큰 변형:
//
//	<|tool_call|> / <tool_call|> / <tool_call> / </tool_call>
//	call:<name>{...}
var toolCallNoiseRE = regexp.MustCompile(`(?s)<\|?\s*/?tool_call\|?>|\bcall:[A-Za-z_][A-Za-z0-9_]*\{[^{}]*\}`)

func sanitizeAssistantText(s string) string {
	if s == "" {
		return s
	}
	return toolCallNoiseRE.ReplaceAllString(s, "")
}

// isPlanModeMetaTool은 plan 모드 진입/탈출을 위한 메타 도구를 식별한다.
// 이 도구들은 호출/결과를 ● tool_use entry로 표시하지 않고 ⏸/▶ system 라인으로 대체한다.
func isPlanModeMetaTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "enter_plan_mode", "enterplanmode", "exit_plan_mode", "exitplanmode":
		return true
	}
	return false
}

func planModeMetaText(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "enter_plan_mode", "enterplanmode":
		return "⏸ Plan mode on"
	case "exit_plan_mode", "exitplanmode":
		return "▶ Plan mode off"
	}
	return ""
}

// summarizeBlockedResult는 engine.go가 plan 모드에서 차단한 raw 결과("tool blocked in plan mode: <name>")
// 또는 not-found 결과를 사용자 친화적인 한 줄로 변환한다. 일반 결과는 trimSpace만 거쳐 그대로 반환.
func summarizeBlockedResult(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "tool blocked in plan mode") {
		return "Plan 모드에서 쓰기 도구는 차단됩니다 (ExitPlanMode로 빠져나간 뒤 재시도)."
	}
	if strings.HasPrefix(trimmed, "tool not found:") {
		return trimmed
	}
	return trimmed
}

// refreshActiveTokenKind는 직전 스냅샷 대비 가장 큰 증가가 있었던 카운터를
// activeTokenKind 로 갱신한다. 증가가 전혀 없으면 기존 값을 유지하여 spinner 라인이
// 빈 표시(-)로 떨어지지 않도록 한다.
func (m *uiModel) refreshActiveTokenKind() {
	dIn := m.tokenInputSnap - m.prevTokenInputSnap
	dOut := m.tokenOutputSnap - m.prevTokenOutputSnap
	dTh := m.tokenThinkingSnap - m.prevTokenThinkingSnap

	switch {
	case dTh > 0 && dTh >= dOut && dTh >= dIn:
		m.activeTokenKind = "thinking"
	case dOut > 0 && dOut >= dIn:
		m.activeTokenKind = "output"
	case dIn > 0:
		m.activeTokenKind = "input"
	}

	m.prevTokenInputSnap = m.tokenInputSnap
	m.prevTokenOutputSnap = m.tokenOutputSnap
	m.prevTokenThinkingSnap = m.tokenThinkingSnap
}

func (m *uiModel) applyPresentationEvent(evt presentation.Event) {
	switch evt.Kind {
	case presentation.EventState:
		m.footer = evt.Footer
		return
	case presentation.EventViewCleared:
		m.entries = m.entries[:0]
		m.latestTodos = nil
		clear(m.toolEntryByTaskID)
		clear(m.parallelSubLookup)
		m.closeAssistantEntry()
		m.closeThinkingEntry()
		m.toolUseOpen = false
		m.toolUseStreamIdx = -1
		m.assistantFlushedLines = 0
		m.assistantFlushedText = ""
		m.thinkingFlushedLines = 0
		m.thinkingFlushedText = ""
		m.lastPrintedIdx = 0
		m.activeTool = nil
		m.toolFocused = false
		return
	case presentation.EventAssistantDelta:
		// 대화 델타가 올 때만 싱킹 엔트리를 닫음
		m.closeThinkingEntry()
		// LLM이 응답을 시작했으면 열려 있는 tool_group(grep/glob 등)을 닫는다.
		m.finalizeOpenToolGroup()
		m.tokenInputSnap = evt.InputTokens
		m.tokenOutputSnap = evt.OutputTokens
		m.tokenThinkingSnap = evt.ThinkingTokens
		m.refreshActiveTokenKind()
		m.appendAssistantDelta(evt.Text)
		return
	case presentation.EventAssistantDone:
		m.closeAssistantEntry()
		return
	case presentation.EventThinkingDelta:
		m.tokenInputSnap = evt.InputTokens
		m.tokenOutputSnap = evt.OutputTokens
		m.tokenThinkingSnap = evt.ThinkingTokens
		m.refreshActiveTokenKind()
		if m.ui.ViewThinking {
			m.appendThinkingDelta(evt.Text)
		}
		return
	case presentation.EventThinkingDone:
		if m.ui.ViewThinking {
			m.closeThinkingEntry()
		}
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
		// parallel_group 자식 entry 먼저 확인
		if ref, ok := m.parallelSubLookup[evt.TaskID]; ok {
			if ref.parentIdx < len(m.entries) {
				subs := m.entries[ref.parentIdx].SubEntries
				if ref.childIdx < len(subs) {
					if subs[ref.childIdx].Interactive != nil {
						subs[ref.childIdx].Interactive.OnStreamDelta(evt.Text)
						if evt.InputResponse != nil {
							subs[ref.childIdx].Interactive.SetInputResponse(evt.InputResponse)
						}
					}
					m.entries[ref.parentIdx].SubEntries[ref.childIdx].StreamBody += evt.Text
				}
			}
			return
		}
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

		// enter_plan_mode / exit_plan_mode 는 도구 호출이지만 사용자에게는 모드 전환 알림으로만
		// 보여야 한다. tool_use/tool_result entry로 만들지 않고 한 줄 system entry로 대체한다.
		// claude_cli도 동일하게 ⏸/▶ 한 줄만 표시한다.
		if isPlanModeMetaTool(evt.ToolName) {
			m.appendEntry("system", "System", planModeMetaText(evt.ToolName), "")
			return
		}

		// TaskCreate/TaskUpdate: 하단 고정 패널이 진행 상황을 표시하므로
		// 스크롤백에는 별도 tool_use entry를 만들지 않는다. 결과 수신 시 latestTodos를
		// 자동 갱신한다 (EventToolResult 핸들러 참조).
		if isTaskMutationTool(evt.ToolName) {
			return
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
			IsActive:    true,
		}

		if isCollapsibleTool(evt.ToolName) {
			m.addToToolGroup(sub, evt.ToolName, evt.Input)
		} else if isBashTool(evt.ToolName) {
			// 동일 bash/powershell 명령 연속 재시도 → 기존 entry에 카운터만 증가.
			if prev := m.lastBashToolUseIdx(); prev >= 0 && m.entries[prev].Body == body {
				m.entries[prev].BashRepeatCount++
				m.entries[prev].IsActive = true
				m.entries[prev].Failed = false
				m.entries[prev].StreamBody = "" // 이전 실행 스트리밍 출력 초기화
				if taskID != "" {
					m.entries[prev].TaskID = taskID
					m.toolEntryByTaskID[taskID] = prev
				}
				if evt.InputResponse != nil && m.entries[prev].Interactive != nil {
					m.entries[prev].Interactive.SetInputResponse(evt.InputResponse)
				}
				m.toolUseOpen = true
				m.toolUseStreamIdx = prev
			} else {
				m.finalizeOpenToolGroup()
				m.appendEntry("tool_use", "Tool Use: "+evt.ToolName, body, evt.ToolName)
				if idx := len(m.entries) - 1; idx >= 0 {
					m.entries[idx].Interactive = interactive
					m.entries[idx].TaskID = taskID
					m.entries[idx].IsActive = true
					if taskID != "" {
						m.toolEntryByTaskID[taskID] = idx
					}
					if evt.InputResponse != nil && m.entries[idx].Interactive != nil {
						m.entries[idx].Interactive.SetInputResponse(evt.InputResponse)
					}
				}
				m.toolUseOpen = true
				m.toolUseStreamIdx = len(m.entries) - 1
			}
		} else {
			m.finalizeOpenToolGroup()
			m.appendEntry("tool_use", "Tool Use: "+evt.ToolName, body, evt.ToolName)
			if idx := len(m.entries) - 1; idx >= 0 {
				m.entries[idx].Interactive = interactive
				m.entries[idx].TaskID = taskID
				m.entries[idx].IsActive = true
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
		// plan-mode meta 도구의 결과는 사용자 화면에 노출하지 않는다.
		if isPlanModeMetaTool(evt.ToolName) {
			return
		}
		taskID := strings.TrimSpace(evt.TaskID)
		failed := isToolResultFailed(evt.Text)

		// parallel_group 자식 entry 먼저 처리
		if ref, ok := m.parallelSubLookup[taskID]; ok {
			if ref.parentIdx < len(m.entries) {
				subs := m.entries[ref.parentIdx].SubEntries
				if ref.childIdx < len(subs) {
					m.entries[ref.parentIdx].SubEntries[ref.childIdx].IsActive = false
					m.entries[ref.parentIdx].SubEntries[ref.childIdx].Failed = failed
					if subs[ref.childIdx].Interactive != nil {
						subs[ref.childIdx].Interactive.SetFinished(true)
						subs[ref.childIdx].Interactive.SetResult(evt.Text)
					}
					// 결과 텍스트를 sub-entry Body에 저장 (renderer가 반환하는 에러 라인 등)
					if !isEmptySuccessResult(evt.Text) {
						renderer := toolui.GetRenderer(evt.ToolName)
						if renderer != nil {
							if resultRendered := renderer.RenderToolResult(evt.Text, 0, m); resultRendered != "" {
								m.entries[ref.parentIdx].SubEntries[ref.childIdx].Body = resultRendered
							}
						}
					}
				}
				// 모든 sub-entry가 완료되었으면 parallel_group을 비활성화
				allDone := true
				anyFailed := false
				for _, sub := range m.entries[ref.parentIdx].SubEntries {
					if sub.IsActive {
						allDone = false
					}
					if sub.Failed {
						anyFailed = true
					}
				}
				if allDone {
					m.entries[ref.parentIdx].IsActive = false
					m.entries[ref.parentIdx].Failed = anyFailed
					m.entries[ref.parentIdx].EndTime = time.Now()
				}
			}
			delete(m.parallelSubLookup, taskID)
			delete(m.toolEntryByTaskID, taskID)
			return
		}

		if idx := m.resolveToolEntryIndex(taskID); idx >= 0 && idx < len(m.entries) {
			m.entries[idx].IsActive = false
			m.entries[idx].Failed = failed
			if m.entries[idx].Interactive != nil {
				m.entries[idx].Interactive.SetFinished(true)
				m.entries[idx].Interactive.SetResult(evt.Text)
			}
		} else if m.activeTool != nil {
			m.activeTool.SetFinished(true)
		}
		// TaskCreate/TaskUpdate: 하단 고정 패널만 표시하고 스크롤백 결과 entry는
		// 만들지 않는다. latestTodos를 갱신한 뒤 즉시 종료한다.
		if isTaskMutationTool(evt.ToolName) {
			m.latestTodos = tasks.AsTodoItems()
			if taskID != "" {
				delete(m.toolEntryByTaskID, taskID)
			}
			return
		}
		if taskID != "" {
			delete(m.toolEntryByTaskID, taskID)
		}
		if taskID == "" || (m.toolUseStreamIdx >= 0 && m.toolUseStreamIdx < len(m.entries) && m.entries[m.toolUseStreamIdx].TaskID == taskID) {
			m.toolUseOpen = false
			m.toolUseStreamIdx = -1
		}
		if isShellTool(evt.ToolName) {
			m.toolFocused = false
			m.activeTool = nil
		}
		// 성공(exit_code: 0)이고 stdout/stderr가 모두 비어 있으면 결과 entry 자체를 만들지 않는다.
		// ● tool_use 헤드라인이 그린으로 표시되어 성공임을 충분히 알 수 있고,
		// "⎿ exit_code: 0"는 잡음일 뿐이라 claude_cli도 표시하지 않는다.
		if isEmptySuccessResult(evt.Text) {
			return
		}
		// plan-mode 차단 메시지를 한 줄짜리 친절한 본문으로 다듬는다.
		body := summarizeBlockedResult(evt.Text)
		result := transcriptEntry{
			Kind:      "tool_result",
			Title:     "Tool Result: " + emptyFallback(evt.ToolName, "-"),
			Body:      body,
			ToolName:  evt.ToolName,
			TaskID:    taskID,
			Collapsed: true,
		}
		if !m.addResultToToolGroup(result) {
			m.appendEntry("tool_result", "Tool Result: "+emptyFallback(evt.ToolName, "-"), body, evt.ToolName)
		}
	case presentation.EventParallelBatch:
		taskIDs := append([]string(nil), evt.BatchTaskIDs...)
		if len(taskIDs) == 0 && strings.TrimSpace(evt.Input) != "" {
			taskIDs = strings.Split(evt.Input, ",")
		}
		if len(taskIDs) > 0 {
			m.handleParallelBatch(taskIDs)
		}
	case presentation.EventNotice:
		// `executing tool: <name>` notice는 ● tool_use 헤드라인과 중복 정보라 UI에서 숨긴다.
		// (engine.go의 trace 로그는 그대로 남는다.)
		if strings.HasPrefix(evt.Text, "executing tool:") {
			return
		}
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
		m.latestTodos = tasks.AsTodoItems()
		m.appendEntry("plan", "Plan Ready", fmt.Sprintf("approved steps: %d", evt.Count), "")
	case presentation.EventPlanStep:
		m.latestTodos = tasks.AsTodoItems()
		title := fmt.Sprintf("Plan Step %d/%d", evt.StepIndex, evt.StepTotal)
		m.appendEntry("plan", title, fmt.Sprintf("%s: %s", evt.ToolName, evt.Text), evt.ToolName)
	case presentation.EventPlanDecision:
		m.latestTodos = tasks.AsTodoItems()
		title := fmt.Sprintf("Plan Decision %d/%d", evt.StepIndex, evt.StepTotal)
		m.appendEntry("plan", title, evt.Decision, "")
	case presentation.EventPlanResult:
		m.latestTodos = tasks.AsTodoItems()
		title := fmt.Sprintf("Plan Result %d/%d", evt.StepIndex, evt.StepTotal)
		m.appendEntry("plan", title, evt.Text, "")
	}
}

func (m *uiModel) appendAssistantDelta(delta string) {
	delta = sanitizeAssistantText(delta)
	if delta == "" {
		return
	}
	if !m.assistantOpen || m.assistantStreamIdx < 0 || m.assistantStreamIdx >= len(m.entries) {
		m.entries = append(m.entries, transcriptEntry{
			Kind:     "assistant",
			Title:    "Assistant",
			Body:     delta,
			IsActive: true,
		})
		m.assistantOpen = true
		m.assistantStreamIdx = len(m.entries) - 1
		m.assistantLastDelta = time.Now()
		m.assistantFlushedLines = 0
		m.assistantFlushedText = ""
		m.trimEntriesIfNeeded()
		return
	}
	m.entries[m.assistantStreamIdx].Body += delta
	m.assistantLastDelta = time.Now()
}

func (m *uiModel) appendThinkingDelta(delta string) {
	// thinking 본문을 스크롤백 entry로 누적한다. 본문이 비어있을 때도 entry 자체는
	// 생성되어야 한다.
	if !m.thinkingOpen || m.thinkingStreamIdx < 0 || m.thinkingStreamIdx >= len(m.entries) {
		m.entries = append(m.entries, transcriptEntry{
			Kind:      "thinking",
			Title:     "Thinking",
			Body:      delta,
			StartTime: time.Now(),
			IsActive:  true,
		})
		m.thinkingOpen = true
		m.thinkingStreamIdx = len(m.entries) - 1
		m.thinkingLastDelta = time.Now()
		m.thinkingFlushedLines = 0
		m.thinkingFlushedText = ""
		m.trimEntriesIfNeeded()
		return
	}
	if delta != "" {
		m.entries[m.thinkingStreamIdx].Body += delta
		m.thinkingLastDelta = time.Now()
	}
}

func (m *uiModel) closeThinkingEntry() {
	if m.thinkingStreamIdx >= 0 && m.thinkingStreamIdx < len(m.entries) {
		m.entries[m.thinkingStreamIdx].EndTime = time.Now()
		m.entries[m.thinkingStreamIdx].IsActive = false
	}
	m.thinkingOpen = false
	m.thinkingStreamIdx = -1
}

// isTaskMutationTool reports whether the tool name belongs to the task tracker
// suite (TaskCreate / TaskUpdate). After such tools complete, the
// bottom-anchored task panel should refresh its snapshot.
func isTaskMutationTool(name string) bool {
	switch tool.CanonicalName(name) {
	case "task_create", "task_update":
		return true
	}
	return false
}

// isEmptySuccessResult는 tool_result 본문이 성공(exit_code: 0)이고 stdout/stderr가
// 모두 비어 있는 케이스를 식별한다. bash/powershell ExecResult JSON과 distiller 정규화
// 텍스트 두 형식을 모두 지원한다.
func isEmptySuccessResult(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	var execResult struct {
		Code   int    `json:"Code"`
		Stdout string `json:"Stdout"`
		Stderr string `json:"Stderr"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(trimmed), &execResult); err == nil {
		return execResult.Code == 0 &&
			strings.TrimSpace(execResult.Stdout) == "" &&
			strings.TrimSpace(execResult.Stderr) == "" &&
			strings.TrimSpace(execResult.Error) == ""
	}
	if !strings.HasPrefix(trimmed, "exit_code:") {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "exit_code:"))
	nl := strings.IndexAny(rest, "\r\n")
	if nl < 0 {
		return rest == "0"
	}
	if strings.TrimSpace(rest[:nl]) != "0" {
		return false
	}
	for _, line := range strings.Split(rest[nl:], "\n") {
		l := strings.TrimSpace(line)
		if l == "" || l == "stdout:" || l == "stderr:" {
			continue
		}
		return false
	}
	return true
}

// isToolResultFailed는 tool_result 본문(JSON 또는 distiller 정규화 텍스트)을 보고
// 실패 여부를 추정한다. bash/powershell의 exit_code 기반 + JSON에 error 키 존재 여부.
// engine.go가 plan 모드에서 차단한 결과 ("tool blocked in plan mode: ...") 도 실패로 본다.
func isToolResultFailed(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "tool blocked in plan mode") ||
		strings.HasPrefix(trimmed, "tool not found:") {
		return true
	}
	var execResult struct {
		Code  int    `json:"Code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(trimmed), &execResult); err == nil {
		if execResult.Code != 0 {
			return true
		}
		if strings.TrimSpace(execResult.Error) != "" {
			return true
		}
		return false
	}
	// distiller 정규화 텍스트 처리
	if strings.HasPrefix(trimmed, "exit_code:") {
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "exit_code:"))
		if nl := strings.IndexAny(rest, "\r\n"); nl >= 0 {
			rest = rest[:nl]
		}
		rest = strings.TrimSpace(rest)
		if rest != "0" && rest != "" {
			return true
		}
	}
	return false
}

func (m *uiModel) appendEntry(kind, title, body, toolName string) {
	// 중복 시스템 메시지 필터링 (예: Plan mode on/off가 여러 경로로 중복 유입되는 경우 방지)
	if kind == "system" && len(m.entries) > 0 {
		last := m.entries[len(m.entries)-1]
		if last.Kind == "system" && strings.TrimSpace(last.Body) == strings.TrimSpace(body) {
			return
		}
	}

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
	if m.assistantStreamIdx >= 0 && m.assistantStreamIdx < len(m.entries) {
		m.entries[m.assistantStreamIdx].IsActive = false
	}
	m.assistantOpen = false
	m.assistantStreamIdx = -1
	// assistantFlushedLines는 여기서 리셋하지 않는다. EventAssistantDone 처리 시
	// closeAssistantEntry가 먼저 실행된 뒤 scrollbackCmd에서 finalizeAssistantStream이
	// 호출되는데, 거기서 flushed 만큼 잔여 부분만 println해야 중복 출력이 없다.
	// 새 assistant stream 시작 시 appendAssistantDelta가 어차피 0으로 리셋한다.
}

// isCollapsibleTool returns true for read/search operations that should be grouped.
// webfetch / web_search 는 자체 ToolRenderer(WebFetchRenderer / WebSearchRenderer)
// 가 등록되어 있으므로 그룹화에서 제외해 개별 UI 로 표시한다.
func isCollapsibleTool(toolName string) bool {
	tn := tool.CanonicalName(toolName)
	switch tn {
	case "grep", "glob", "fileread", "lsp", "read_mcp_resource", "list_mcp_resources":
		return true
	}
	return false
}

// isBashTool returns true for shell execution tools that support repeat grouping.
func isBashTool(toolName string) bool {
	tn := tool.CanonicalName(toolName)
	return tn == "bash" || tn == "powershell"
}

// lastBashToolUseIdx returns the index of the most recent bash/powershell tool_use
// entry, scanning backwards through system/tool_result entries. Returns -1 if the
// last non-trivial entry is something else (assistant text, thinking, tool_group).
func (m *uiModel) lastBashToolUseIdx() int {
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		switch e.Kind {
		case "tool_use":
			if isBashTool(e.ToolName) {
				return i
			}
			return -1
		case "tool_result", "system":
			continue
		default:
			return -1
		}
	}
	return -1
}

// classifyTool returns search/read/list classification for a tool.
func classifyTool(toolName string) (isSearch, isRead, isList bool) {
	tn := tool.CanonicalName(toolName)
	switch tn {
	case "grep":
		return true, false, false
	case "glob", "list_mcp_resources":
		return false, false, true
	case "fileread", "lsp", "read_mcp_resource":
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
		if len(q) > 40 {
			return fmt.Sprintf(`"%s…"`, q[:40])
		}
		return fmt.Sprintf(`"%s"`, q)
	}
	if p, ok := inp["pattern"].(string); ok && p != "" {
		const maxPattern = 30
		if len(p) > maxPattern {
			p = p[:maxPattern] + "…"
		}
		hint := fmt.Sprintf(`"%s"`, p)
		if path, ok := inp["path"].(string); ok && path != "" {
			suffix := " in " + filepath.Base(path)
			if len(hint)+len(suffix) > 50 {
				return hint
			}
			hint += suffix
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
	clear(m.parallelSubLookup)
	for idx := range m.entries {
		switch m.entries[idx].Kind {
		case "tool_use":
			taskID := strings.TrimSpace(m.entries[idx].TaskID)
			if taskID == "" {
				continue
			}
			m.toolEntryByTaskID[taskID] = idx
		case "parallel_group":
			for taskID, childIdx := range m.entries[idx].ParallelSubByTaskID {
				taskID = strings.TrimSpace(taskID)
				if taskID == "" {
					continue
				}
				m.toolEntryByTaskID[taskID] = idx
				m.parallelSubLookup[taskID] = parallelSubRef{
					parentIdx: idx,
					childIdx:  childIdx,
				}
			}
		default:
			continue
		}
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

// handleParallelBatch merges existing top-level tool_use entries (identified by taskIDs)
// into a single parallel_group entry for visual grouping.
func (m *uiModel) handleParallelBatch(taskIDs []string) {
	if len(taskIDs) < 2 {
		return
	}

	type indexedEntry struct {
		originalIdx int
		entry       transcriptEntry
		taskID      string
	}
	var found []indexedEntry
	for _, taskID := range taskIDs {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		if idx, ok := m.toolEntryByTaskID[taskID]; ok && idx < len(m.entries) {
			found = append(found, indexedEntry{
				originalIdx: idx,
				entry:       m.entries[idx],
				taskID:      taskID,
			})
		}
	}
	if len(found) < 2 {
		return
	}

	// Sort by original index to preserve display order
	sort.Slice(found, func(i, j int) bool { return found[i].originalIdx < found[j].originalIdx })

	// Build SubEntries in the taskID order given by the caller (which reflects batch order)
	subEntries := make([]transcriptEntry, 0, len(taskIDs))
	subByTaskID := make(map[string]int)
	for _, taskID := range taskIDs {
		taskID = strings.TrimSpace(taskID)
		for _, fe := range found {
			if fe.taskID == taskID {
				subByTaskID[taskID] = len(subEntries)
				subEntries = append(subEntries, fe.entry)
				break
			}
		}
	}

	group := transcriptEntry{
		Kind:                "parallel_group",
		IsActive:            true,
		SubEntries:          subEntries,
		ParallelSubByTaskID: subByTaskID,
		StartTime:           time.Now(),
	}

	// Build set of original indices to remove
	removeSet := make(map[int]bool, len(found))
	for _, fe := range found {
		removeSet[fe.originalIdx] = true
	}
	firstIdx := found[0].originalIdx

	// Rebuild m.entries, replacing the individual entries with the parallel_group
	newEntries := make([]transcriptEntry, 0, len(m.entries)-len(found)+1)
	inserted := false
	for i, e := range m.entries {
		if removeSet[i] {
			if !inserted {
				newEntries = append(newEntries, group)
				inserted = true
			}
		} else {
			newEntries = append(newEntries, e)
		}
	}
	m.entries = newEntries

	// The parallel_group ends up at firstIdx (since entries before it are preserved)
	parallelGroupIdx := firstIdx

	// Update lookups
	if m.parallelSubLookup == nil {
		m.parallelSubLookup = make(map[string]parallelSubRef)
	}
	if m.toolEntryByTaskID == nil {
		m.toolEntryByTaskID = make(map[string]int)
	}
	for taskID, childIdx := range subByTaskID {
		m.parallelSubLookup[taskID] = parallelSubRef{
			parentIdx: parallelGroupIdx,
			childIdx:  childIdx,
		}
		m.toolEntryByTaskID[taskID] = parallelGroupIdx
	}

	// Rewind lastPrintedIdx so the parallel_group gets flushed when it becomes inactive
	if m.lastPrintedIdx > parallelGroupIdx {
		m.lastPrintedIdx = parallelGroupIdx
	}

	// Disable individual streaming — each sub-entry manages its own Interactive model
	m.toolUseOpen = false
	m.toolUseStreamIdx = -1
}
