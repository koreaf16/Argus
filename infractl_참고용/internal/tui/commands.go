package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourorg/infractl/internal/store"
)

// handleSlashCommand handles slash commands entered in the input bar.
func (m AppModel) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return m, nil
	}

	println := func(s string) { m.box.Println(s) }
	printErr := func(s string) { m.box.Println(renderErrorLine(fmt.Errorf("%s", s))) }

	switch parts[0] {
	case "/quit", "/exit", "/q":
		m.cancel()
		return m, tea.Quit
	case "/clear":
		m.ag.ClearHistory()
		println(renderSystemLine("Cleared conversation history."))
	case "/model":
		println(renderSystemLine(fmt.Sprintf("Current model: %s (%s)", m.cfg.LLM.Model, m.cfg.LLM.Endpoint)))
	case "/help":
		println(renderSystemLine(helpText()))
	case "/tools":
		tools := m.ag.Tools()
		var sb strings.Builder
		sb.WriteString("Registered tools:\n")
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("  %-20s - %s\n", t.Name(), t.Description()))
		}
		println(renderSystemLine(sb.String()))
	case "/servers", "/workspaces":
		if len(parts) >= 2 && parts[1] == "remove" {
			if len(parts) < 3 {
				printErr("Usage: /servers remove <name>")
				return m, nil
			}
			return m, m.removeServerCmd(parts[2])
		}
		servers, _ := m.store.List(context.Background())
		println(buildServerTable(servers, m.activeServer))
	case "/server", "/workspace":
		if len(parts) >= 2 {
			name := strings.TrimSpace(parts[1])
			switch strings.ToLower(name) {
			case "clear", "none", "localhost", "local":
				if m.activeServer != nil && m.manager != nil && m.manager.Has(m.activeServer.Name) {
					if err := m.manager.Remove(m.activeServer.Name); err != nil {
						printErr(fmt.Sprintf("Failed to disconnect active server %q: %v", m.activeServer.Name, err))
						return m, nil
					}
				}
				m.ag.ClearActiveServer()
				println(renderSystemLine("Active server cleared and SSH session disconnected (default target: local server)."))
				return m, nil
			}
			servers, _ := m.store.List(context.Background())
			for _, s := range servers {
				if strings.EqualFold(s.Name, name) {
					srv := s
					m.ag.SetActiveServer(srv)
					println(renderSystemLine("Active server: " + srv.Name + " (" + srv.Host + ", dir: " + srv.WorkspaceDir + ")"))
					if briefing := m.renderAuditBriefing(srv.Name); briefing != "" {
						println(briefing)
					}
					return m, nil
				}
			}
			printErr("Server not found: " + name)
		} else {
			if m.selectHandler == nil {
				printErr("/server selection UI is unavailable")
				return m, nil
			}
			return m, m.serverFocusCmd()
		}
	case "/services":
		serverFilter := ""
		if len(parts) >= 2 {
			serverFilter = strings.TrimSpace(parts[1])
		}
		println(renderSystemLine(m.buildServicesView(context.Background(), serverFilter)))
	case "/connectors":
		println(renderSystemLine(m.formatConnectors()))
	case "/mcp":
		if len(parts) >= 3 && parts[1] == "reconnect" {
			return m, m.reconnectMCPCmd(parts[2])
		}
		println(renderSystemLine(m.formatMCPStatus()))
	case "/sessions":
		if len(parts) >= 3 && parts[1] == "restore" {
			n, err := strconv.Atoi(parts[2])
			if err == nil {
				return m, m.restoreSessionCmd(n)
			}
			printErr("Usage: /sessions restore <number>")
		} else {
			println(renderSystemLine(m.formatSessions()))
		}
	case "/history":
		println(renderSystemLine(m.formatHistory()))
	case "/knowledge":
		println(renderSystemLine(m.handleKnowledge(parts)))
	case "/rag":
		msg, cmd := m.handleRAG(parts)
		println(renderSystemLine(msg))
		if cmd != nil {
			return m, cmd
		}
	case "/cost":
		println(renderSystemLine(m.handleCost(parts)))
	case "/checkpoints":
		println(renderSystemLine(m.handleCheckpoints(parts)))
	case "/hooks":
		msg, cmd := m.handleHooks(parts)
		println(renderSystemLine(msg))
		if cmd != nil {
			return m, cmd
		}
	case "/schedules":
		msg, cmd := m.handleSchedules(parts)
		println(renderSystemLine(msg))
		if cmd != nil {
			return m, cmd
		}
	case "/ospermission", "/osessions":
		println(renderSystemLine(m.formatOSPermissions()))
	case "/yoro":
		active := m.ag.ToggleYOROMode()
		m.yoloMode = active
		m.statusBar.setYoloMode(active)
		if active {
			println(renderSystemLine("YORO mode enabled. Confirmation prompts are skipped."))
		} else {
			println(renderSystemLine("YORO mode disabled. Risky actions require confirmation."))
		}
	case "/elevate":
		active := m.ag.ToggleElevatedMode()
		m.elevatedMode = active
		m.statusBar.setElevatedMode(active)
		if active {
			println(renderSystemLine("Elevated mode enabled. Managed sudo/su execution is allowed."))
		} else {
			println(renderSystemLine("Elevated mode disabled. sudo/su execution requires /elevate."))
		}
	case "/audit":
		sub := "list"
		if len(parts) >= 2 {
			sub = parts[1]
		}
		switch sub {
		case "show":
			if len(parts) < 3 {
				printErr("Usage: /audit show <run_id>")
				return m, nil
			}
			println(m.renderAuditShow(parts[2]))
		default:
			println(m.renderAuditList(""))
		}
	default:
		printErr(fmt.Sprintf("Unknown command: %s", parts[0]))
	}
	return m, nil
}

// renderAuditList builds a one-line-per-run table for the active server.
func (m AppModel) renderAuditList(serverFilter string) string {
	if m.auditStore == nil {
		return renderSystemLine("Audit store not available.")
	}
	since := time.Now().AddDate(0, 0, -7)
	summaries, err := m.auditStore.ListAuditRunSummaries(context.Background(), serverFilter, since, 20)
	if err != nil {
		return renderSystemLine("audit list error: " + err.Error())
	}
	if len(summaries) == 0 {
		return renderSystemLine("감사 이력이 없습니다 (최근 7일).")
	}
	var sb strings.Builder
	sb.WriteString("감사 이력 (최근 7일):\n")
	for _, rs := range summaries {
		title := rs.Title
		if title == "" {
			title = "(제목 없음)"
		}
		if len([]rune(title)) > 40 {
			title = string([]rune(title)[:37]) + "..."
		}
		elapsed := briefElapsed(rs.StartedAt)
		sb.WriteString(fmt.Sprintf("  [%s] %-40s  %s  %d/%d step  %s\n",
			briefStatus(rs.Status), title, rs.ServerName, rs.StepDone, rs.StepTotal, elapsed))
	}
	return renderSystemLine(sb.String())
}

// renderAuditShow renders a brief summary for a specific run.
func (m AppModel) renderAuditShow(runID string) string {
	if m.auditStore == nil {
		return renderSystemLine("Audit store not available.")
	}
	detail, err := m.auditStore.DescribeAuditRun(context.Background(), runID)
	if err != nil {
		return renderSystemLine("audit show error: " + err.Error())
	}
	run := detail.Run
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Run: %s\n서버: %s | 상태: %s\n시작: %s\n",
		run.Title, run.ServerName, run.Status, run.StartedAt.Format("2006-01-02 15:04")))
	if run.SummaryMD != "" {
		sb.WriteString("요약: " + run.SummaryMD + "\n")
	}
	if len(detail.Steps) > 0 {
		sb.WriteString(fmt.Sprintf("스텝 (%d개):\n", len(detail.Steps)))
		for _, st := range detail.Steps {
			sb.WriteString(fmt.Sprintf("  %d. [%s] %s\n", st.Seq, st.Status, st.Label))
		}
	}
	return renderSystemLine(sb.String())
}

// renderAuditBriefing renders attach-time run cards for a given server.
func (m AppModel) renderAuditBriefing(serverName string) string {
	if m.auditStore == nil {
		return ""
	}
	since := time.Now().AddDate(0, 0, -7)
	summaries, err := m.auditStore.ListAuditRunSummaries(context.Background(), serverName, since, 5)
	if err != nil || len(summaries) == 0 {
		return ""
	}
	// Only show planning/running/failed — skip completed runs unless very recent.
	var cards []store.AuditRunSummary
	for _, rs := range summaries {
		switch rs.Status {
		case store.AuditRunStatusPlanning, store.AuditRunStatusRunning:
			cards = append(cards, rs)
		case store.AuditRunStatusFailed:
			cards = append(cards, rs)
		}
	}
	if len(cards) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("─── 이전 작업 이력 ───\n")
	for _, rs := range cards {
		title := rs.Title
		if title == "" {
			title = "(제목 없음)"
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s  %d/%d step  %s\n",
			briefStatus(rs.Status), title, rs.StepDone, rs.StepTotal, briefElapsed(rs.StartedAt)))
	}
	sb.WriteString("(`/audit show <id>`로 상세 조회 가능)\n")
	return renderSystemLine(sb.String())
}

func briefStatus(s store.AuditRunStatus) string {
	switch s {
	case store.AuditRunStatusPlanning:
		return "planning"
	case store.AuditRunStatusRunning:
		return "running "
	case store.AuditRunStatusSucceeded:
		return "done    "
	case store.AuditRunStatusFailed:
		return "FAIL    "
	case store.AuditRunStatusAborted:
		return "aborted "
	}
	return string(s)
}

func briefElapsed(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// serverFocusCmd returns a Cmd that asks user to select an active workspace.
func (m AppModel) serverFocusCmd() tea.Cmd {
	return func() tea.Msg {
		servers, err := m.store.List(context.Background())
		if err != nil || len(servers) == 0 {
			return SystemMsg("No registered workspaces.")
		}
		if len(servers) == 1 {
			srv := servers[0]
			m.ag.SetActiveServer(srv)
			return SystemMsg(fmt.Sprintf("Active workspace: %s (%s, dir: %s)", srv.Name, srv.Host, srv.WorkspaceDir))
		}

		opts := make([]SelectOption, len(servers))
		for i, s := range servers {
			desc := fmt.Sprintf("%s@%s:%d", s.User, s.Host, s.Port)
			if s.WorkspaceDir != "" {
				desc += " | " + s.WorkspaceDir
			}
			if s.OS != "" {
				desc += " | " + s.OS
			}
			opts[i] = SelectOption{Label: s.Name, Description: desc, HideOther: true}
		}

		result, err := m.selectHandler.RequestSelect("Select active workspace", opts)
		if err != nil || result.Index < 0 || result.Index >= len(servers) {
			return nil
		}
		srv := servers[result.Index]
		m.ag.SetActiveServer(srv)
		return SystemMsg(fmt.Sprintf("Active workspace: %s (%s, dir: %s)", srv.Name, srv.Host, srv.WorkspaceDir))
	}
}

func helpText() string {
	return "Available commands:\n" +
		"  /help                         - show this help\n" +
		"  /tools                        - list registered tools\n" +
		"  /workspaces                   - list registered workspaces\n" +
		"  /workspaces remove <name>     - remove a workspace\n" +
		"  /workspace [name|clear]       - set or clear the active workspace\n" +
		"  /servers, /server             - aliases for /workspaces and /workspace\n" +
		"  /services [workspace]         - show discovered services\n" +
		"  /connectors                   - show active connectors\n" +
		"  /mcp                          - show MCP status\n" +
		"  /mcp reconnect <name>         - reconnect MCP\n" +
		"  /sessions                     - list recent sessions\n" +
		"  /sessions restore <N>         - restore a session\n" +
		"  /history                      - show tool execution history\n" +
		"  /knowledge                    - list knowledge\n" +
		"  /knowledge search <query>     - search knowledge\n" +
		"  /knowledge delete <ID>        - delete knowledge\n" +
		"  /rag                          - list RAG sources\n" +
		"  /rag delete <ID>              - delete a RAG source\n" +
		"  /rag priority <ID> <N>        - change RAG source priority\n" +
		"  /cost                         - show cost summary\n" +
		"  /cost week                    - show recent 7-day cost\n" +
		"  /cost detail                  - show detailed cost\n" +
		"  /checkpoints [workspace]      - list checkpoints\n" +
		"  /hooks                        - list hooks\n" +
		"  /hooks enable/disable/delete <id>\n" +
		"  /schedules                    - list schedules\n" +
		"  /schedules enable/disable/delete <id>\n" +
		"  /osessions                    - show OS permission/session cache\n" +
		"  /yoro                         - toggle YORO mode\n" +
		"  /elevate                      - toggle elevated sudo/su mode\n" +
		"  /clear                        - clear conversation history\n" +
		"  /model                        - show current model\n" +
		"  /quit                         - quit"
}

func connectorStatusIcon(status string) string {
	switch status {
	case "connected":
		return "+"
	case "connecting":
		return "~"
	case "error":
		return "!"
	default:
		return " "
	}
}

func (m AppModel) formatConnectors() string {
	if m.connectorMgr == nil {
		return "Connector manager is not initialized."
	}
	states := m.connectorMgr.States()
	if len(states) == 0 {
		return "No active connectors."
	}
	var sb strings.Builder
	sb.WriteString("Active connectors:\n")
	for _, s := range states {
		icon := connectorStatusIcon(string(s.Status))
		sb.WriteString(fmt.Sprintf("  %s %-10s %-12s %-15s - %s (%d tools)\n",
			icon, s.Type, s.ServiceName, s.ServerName, s.Status, len(s.Tools)))
	}
	return sb.String()
}

func (m AppModel) formatMCPStatus() string {
	if len(m.mcpClients) == 0 {
		return "No configured MCP servers."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MCP server status (%d):\n", len(m.mcpClients)))
	for _, c := range m.mcpClients {
		icon := connectorStatusIcon(string(c.Status))
		sb.WriteString(fmt.Sprintf("  %s %-15s - %s\n", icon, c.Name, c.Status))
	}
	return sb.String()
}

func (m AppModel) reconnectMCPCmd(name string) tea.Cmd {
	return func() tea.Msg {
		for _, c := range m.mcpClients {
			if c.Name == name {
				if err := c.Reconnect(m.ctx); err != nil {
					return ErrorMsg{Err: fmt.Errorf("MCP %q reconnect failed: %w", name, err)}
				}
				return SystemMsg(fmt.Sprintf("MCP %q reconnected", name))
			}
		}
		return ErrorMsg{Err: fmt.Errorf("MCP server not found: %s", name)}
	}
}

func (m AppModel) formatSessions() string {
	if m.sessionStore == nil {
		return "Session store is not initialized."
	}
	convs, err := m.sessionStore.ListConversations(m.ctx, 20)
	if err != nil {
		return fmt.Sprintf("Failed to load sessions: %s", err)
	}
	if len(convs) == 0 {
		return "No saved sessions."
	}

	var sb strings.Builder
	sb.WriteString("Recent sessions:\n")
	sb.WriteString(fmt.Sprintf("  %-4s %-40s %-20s\n", "No", "Title", "Updated"))
	sb.WriteString("  " + strings.Repeat("-", 68) + "\n")
	for i, c := range convs {
		title := c.Title
		if len(title) > 38 {
			title = title[:38] + ".."
		}
		sb.WriteString(fmt.Sprintf("  %-4d %-40s %-20s\n", i+1, title, c.UpdatedAt.Format("01/02 15:04")))
	}
	sb.WriteString("\nUse /sessions restore <No> to restore a session.")
	return sb.String()
}

func (m AppModel) restoreSessionCmd(n int) tea.Cmd {
	return func() tea.Msg {
		convs, err := m.sessionStore.ListConversations(m.ctx, 20)
		if err != nil || n < 1 || n > len(convs) {
			return ErrorMsg{Err: fmt.Errorf("invalid session number: %d", n)}
		}
		conv := convs[n-1]
		if err := m.ag.RestoreSession(m.ctx, conv.ID); err != nil {
			return ErrorMsg{Err: fmt.Errorf("restore session failed: %w", err)}
		}
		m.ag.SetSessionID(conv.ID)
		return SystemMsg(fmt.Sprintf("Restored session %q", conv.Title))
	}
}

func (m AppModel) formatHistory() string {
	if m.execLogStore == nil {
		return "Execution log store is not initialized."
	}
	logs, err := m.execLogStore.ListExecutionLogs(m.ctx, 0, 20)
	if err != nil {
		return fmt.Sprintf("Failed to load execution logs: %s", err)
	}
	if len(logs) == 0 {
		return "No execution history."
	}

	var sb strings.Builder
	sb.WriteString("Recent tool execution logs:\n")
	for i, l := range logs {
		result := "+"
		if !l.Success {
			result = "!"
		}
		sb.WriteString(fmt.Sprintf("  %d. %s %-20s %s  %s\n",
			i+1, result, l.ToolName, l.TargetServer, l.Timestamp.Format(time.Kitchen)))
	}
	return sb.String()
}

// removeServerCmd returns a Cmd that removes a registered workspace.
func (m AppModel) removeServerCmd(name string) tea.Cmd {
	return func() tea.Msg {
		canonical, err := m.resolveRegisteredServerName(strings.TrimSpace(name))
		if err != nil {
			return ErrorMsg{Err: err}
		}
		if m.manager != nil && m.manager.Has(canonical) {
			if err := m.manager.Remove(canonical); err != nil {
				return SystemMsg(fmt.Sprintf("Manager removal failed (ignored): %s", err))
			}
		}
		if err := m.store.Remove(context.Background(), canonical); err != nil {
			return ErrorMsg{Err: fmt.Errorf("workspace removal failed: %w", err)}
		}
		return SystemMsg(fmt.Sprintf("Workspace %q removed.", canonical))
	}
}

func (m AppModel) resolveRegisteredServerName(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("server name is required")
	}
	if m.store == nil {
		return "", fmt.Errorf("server not found: %s", name)
	}
	servers, err := m.store.List(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to list servers: %w", err)
	}
	for _, srv := range servers {
		if strings.EqualFold(strings.TrimSpace(srv.Name), name) {
			return srv.Name, nil
		}
	}
	return "", fmt.Errorf("server not found: %s", name)
}

func (m AppModel) handleKnowledge(parts []string) string {
	if m.knowledgeStore == nil {
		return "Knowledge store is not initialized."
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	switch sub {
	case "search":
		if len(parts) < 3 {
			return "Usage: /knowledge search <query>"
		}
		query := strings.Join(parts[2:], " ")
		entries, err := m.knowledgeStore.SearchKnowledge(m.ctx, query, 10)
		if err != nil {
			return fmt.Sprintf("Search failed: %s", err)
		}
		if len(entries) == 0 {
			return fmt.Sprintf("No results for %q", query)
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Knowledge results for %q (%d):\n\n", query, len(entries)))
		for _, e := range entries {
			sb.WriteString(fmt.Sprintf("  [#%d] %s (%s)\n", e.ID, e.Title, e.Category))
			if e.Situation != "" {
				sb.WriteString(fmt.Sprintf("  Situation: %s\n", e.Situation))
			}
			sb.WriteString(fmt.Sprintf("  Resolution: %s\n\n", e.Resolution))
		}
		return sb.String()
	case "delete":
		if len(parts) < 3 {
			return "Usage: /knowledge delete <ID>"
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return fmt.Sprintf("Invalid ID: %s", parts[2])
		}
		if err := m.knowledgeStore.DeleteKnowledge(m.ctx, id); err != nil {
			return fmt.Sprintf("Delete failed: %s", err)
		}
		return fmt.Sprintf("Knowledge entry #%d deleted.", id)
	default:
		entries, err := m.knowledgeStore.ListKnowledge(m.ctx, "", 20)
		if err != nil {
			return fmt.Sprintf("Failed to list knowledge: %s", err)
		}
		if len(entries) == 0 {
			return "No saved knowledge."
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Saved knowledge (%d):\n", len(entries)))
		sb.WriteString(fmt.Sprintf("  %-6s %-16s %-35s %-8s\n", "ID", "Category", "Title", "Uses"))
		sb.WriteString("  " + strings.Repeat("-", 70) + "\n")
		for _, e := range entries {
			title := e.Title
			if len(title) > 33 {
				title = title[:33] + ".."
			}
			sb.WriteString(fmt.Sprintf("  %-6d %-16s %-35s %d\n", e.ID, e.Category, title, e.UseCount))
		}
		sb.WriteString("\nDelete: /knowledge delete <ID> | Search: /knowledge search <query>")
		return sb.String()
	}
}

func (m AppModel) handleRAG(parts []string) (string, tea.Cmd) {
	if m.ragSourceStore == nil {
		return "RAG source store is not initialized.", nil
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	switch sub {
	case "delete":
		if len(parts) < 3 {
			return "Usage: /rag delete <ID>", nil
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return fmt.Sprintf("Invalid ID: %s", parts[2]), nil
		}
		return "", func() tea.Msg {
			if err := m.ragSourceStore.DeleteRAGSource(m.ctx, id); err != nil {
				return ErrorMsg{Err: fmt.Errorf("RAG source delete failed: %w", err)}
			}
			return SystemMsg(fmt.Sprintf("RAG source %d deleted.", id))
		}
	case "priority":
		if len(parts) < 4 {
			return "Usage: /rag priority <ID> <priority>", nil
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return fmt.Sprintf("Invalid ID: %s", parts[2]), nil
		}
		priority, err := strconv.Atoi(parts[3])
		if err != nil || priority < 1 {
			return fmt.Sprintf("Invalid priority: %s (must be >= 1)", parts[3]), nil
		}
		return "", func() tea.Msg {
			if err := m.ragSourceStore.UpdateRAGSourcePriority(m.ctx, id, priority); err != nil {
				return ErrorMsg{Err: fmt.Errorf("priority update failed: %w", err)}
			}
			return SystemMsg(fmt.Sprintf("RAG source %d priority set to %d.", id, priority))
		}
	default:
		sources, err := m.ragSourceStore.ListRAGSources(m.ctx)
		if err != nil {
			return fmt.Sprintf("Failed to list RAG sources: %s", err), nil
		}
		if len(sources) == 0 {
			return "No registered RAG sources.", nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("RAG sources (%d):\n", len(sources)))
		sb.WriteString(fmt.Sprintf("  %-4s %-20s %-10s %-15s %-4s\n", "ID", "Name", "DB", "Workspace", "Prio"))
		sb.WriteString("  " + strings.Repeat("-", 58) + "\n")
		for _, src := range sources {
			sb.WriteString(fmt.Sprintf("  %-4d %-20s %-10s %-15s %d\n",
				src.ID, src.Name, src.DBType, src.ServerName, src.Priority))
		}
		sb.WriteString("\nCommands: /rag delete <ID> | /rag priority <ID> <number>")
		return sb.String(), nil
	}
}

func (m AppModel) handleCost(parts []string) string {
	if m.costTracker == nil {
		return "Cost tracker is not configured."
	}
	sub := ""
	if len(parts) >= 2 {
		sub = parts[1]
	}
	switch sub {
	case "week":
		return m.formatCostSummary(7, "recent 7 days")
	case "detail":
		return m.formatCostDetail(30)
	default:
		return m.formatCostSummary(30, "this month (30 days)")
	}
}

func (m AppModel) formatCostSummary(days int, label string) string {
	s, err := m.costTracker.Summary(m.ctx, days)
	if err != nil {
		return fmt.Sprintf("Failed to load cost summary: %s", err)
	}
	return fmt.Sprintf("Cost summary for %s\n  Total input tokens:  %s\n  Total output tokens: %s\n  Total calls:         %d\n  Estimated cost:      $%.4f",
		label, formatTokens(s.TotalInputTokens), formatTokens(s.TotalOutputTokens), s.CallCount, s.TotalCost)
}

func (m AppModel) formatCostDetail(days int) string {
	daily, err := m.costTracker.DailyCosts(m.ctx, days)
	if err != nil {
		return fmt.Sprintf("Failed to load daily costs: %s", err)
	}
	if len(daily) == 0 {
		return "No usage records."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Daily cost detail for recent %d days:\n", days))
	sb.WriteString(fmt.Sprintf("  %-12s  %12s  %12s  %8s  %8s\n", "Date", "Input", "Output", "Calls", "Cost($)"))
	sb.WriteString("  " + strings.Repeat("-", 60) + "\n")
	for _, d := range daily {
		sb.WriteString(fmt.Sprintf("  %-12s  %12s  %12s  %8d  %8.4f\n",
			d.Date, formatTokens(d.InputTokens), formatTokens(d.OutputTokens), d.CallCount, d.EstimatedCost))
	}
	return sb.String()
}

func (m AppModel) handleCheckpoints(parts []string) string {
	if m.checkpointMgr == nil {
		return "Checkpoint manager is not initialized."
	}
	workspaceName := ""
	if len(parts) >= 2 {
		workspaceName = parts[1]
	}
	cps, err := m.checkpointMgr.List(m.ctx, workspaceName, 20)
	if err != nil {
		return fmt.Sprintf("Failed to list checkpoints: %s", err)
	}
	if len(cps) == 0 {
		if workspaceName != "" {
			return fmt.Sprintf("No checkpoints for workspace %q.", workspaceName)
		}
		return "No saved checkpoints."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Checkpoints (recent %d):\n", len(cps)))
	sb.WriteString(fmt.Sprintf("  %-4s %-12s %-20s %s\n", "ID", "Workspace", "Created", "Description"))
	sb.WriteString("  " + strings.Repeat("-", 60) + "\n")
	for _, cp := range cps {
		ts := cp.CreatedAt.Format("01/02 15:04:05")
		desc := cp.Description
		if len(desc) > 30 {
			desc = desc[:28] + ".."
		}
		rollback := ""
		if cp.Snapshot.RollbackCommand != "" {
			rollback = " [rollback available]"
		}
		sb.WriteString(fmt.Sprintf("  %-4d %-12s %-20s %s%s\n", cp.ID, cp.Server, ts, desc, rollback))
	}
	sb.WriteString("\nRollback: ask to roll back checkpoint #N.")
	return sb.String()
}

func (m AppModel) handleHooks(parts []string) (string, tea.Cmd) {
	if m.hooksMgr == nil {
		return "Hook manager is not initialized.", nil
	}
	if len(parts) >= 3 {
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return fmt.Sprintf("Invalid hook ID: %s", parts[2]), nil
		}
		switch parts[1] {
		case "enable":
			return "", func() tea.Msg {
				if err := m.hooksMgr.SetEnabled(m.ctx, id, true); err != nil {
					return ErrorMsg{Err: fmt.Errorf("enable hook failed: %w", err)}
				}
				return SystemMsg(fmt.Sprintf("Hook #%d enabled.", id))
			}
		case "disable":
			return "", func() tea.Msg {
				if err := m.hooksMgr.SetEnabled(m.ctx, id, false); err != nil {
					return ErrorMsg{Err: fmt.Errorf("disable hook failed: %w", err)}
				}
				return SystemMsg(fmt.Sprintf("Hook #%d disabled.", id))
			}
		case "delete":
			return "", func() tea.Msg {
				if err := m.hooksMgr.Delete(m.ctx, id); err != nil {
					return ErrorMsg{Err: fmt.Errorf("delete hook failed: %w", err)}
				}
				return SystemMsg(fmt.Sprintf("Hook #%d deleted.", id))
			}
		default:
			return fmt.Sprintf("Unknown /hooks subcommand: %s", parts[1]), nil
		}
	}
	hooks, err := m.hooksMgr.List(m.ctx)
	if err != nil {
		return fmt.Sprintf("Failed to list hooks: %s", err), nil
	}
	if len(hooks) == 0 {
		return "No registered hooks.", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Registered hooks (%d):\n", len(hooks)))
	sb.WriteString(fmt.Sprintf("  %-4s %-18s %-6s %s\n", "ID", "Event", "On", "Script"))
	sb.WriteString("  " + strings.Repeat("-", 60) + "\n")
	for _, h := range hooks {
		enabled := "+"
		if !h.Enabled {
			enabled = "-"
		}
		script := h.ScriptPath
		if len(script) > 35 {
			script = "..." + script[len(script)-32:]
		}
		sb.WriteString(fmt.Sprintf("  %-4d %-18s %-6s %s\n", h.ID, h.Event, enabled, script))
	}
	sb.WriteString("\nDisable: /hooks disable <id> | Enable: /hooks enable <id> | Delete: /hooks delete <id>")
	return sb.String(), nil
}

func (m AppModel) handleSchedules(parts []string) (string, tea.Cmd) {
	if m.scheduleMgr == nil {
		return "Schedule manager is not initialized.", nil
	}
	if len(parts) >= 3 {
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return fmt.Sprintf("Invalid schedule ID: %s", parts[2]), nil
		}
		switch parts[1] {
		case "enable":
			return "", func() tea.Msg {
				if err := m.scheduleMgr.SetEnabled(m.ctx, id, true); err != nil {
					return ErrorMsg{Err: fmt.Errorf("enable schedule failed: %w", err)}
				}
				return SystemMsg(fmt.Sprintf("Schedule #%d enabled.", id))
			}
		case "disable":
			return "", func() tea.Msg {
				if err := m.scheduleMgr.SetEnabled(m.ctx, id, false); err != nil {
					return ErrorMsg{Err: fmt.Errorf("disable schedule failed: %w", err)}
				}
				return SystemMsg(fmt.Sprintf("Schedule #%d disabled.", id))
			}
		case "delete":
			return "", func() tea.Msg {
				if err := m.scheduleMgr.Delete(m.ctx, id); err != nil {
					return ErrorMsg{Err: fmt.Errorf("delete schedule failed: %w", err)}
				}
				return SystemMsg(fmt.Sprintf("Schedule #%d deleted.", id))
			}
		default:
			return fmt.Sprintf("Unknown /schedules subcommand: %s", parts[1]), nil
		}
	}
	schedules, err := m.scheduleMgr.List(m.ctx)
	if err != nil {
		return fmt.Sprintf("Failed to list schedules: %s", err), nil
	}
	if len(schedules) == 0 {
		return "No registered schedules.", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Registered schedules (%d):\n", len(schedules)))
	sb.WriteString(fmt.Sprintf("  %-4s %-6s %-20s %-15s %s\n", "ID", "On", "Name", "Cron", "Last Run"))
	sb.WriteString("  " + strings.Repeat("-", 65) + "\n")
	for _, s := range schedules {
		enabled := "+"
		if !s.Enabled {
			enabled = "-"
		}
		lastRun := "never"
		if s.LastRun != nil {
			lastRun = s.LastRun.Format("01/02 15:04")
		}
		name := s.Name
		if len(name) > 18 {
			name = name[:16] + ".."
		}
		sb.WriteString(fmt.Sprintf("  %-4d %-6s %-20s %-15s %s\n", s.ID, enabled, name, s.CronExpr, lastRun))
	}
	return sb.String(), nil
}

func (m AppModel) formatOSPermissions() string {
	var sb strings.Builder
	sb.WriteString("Acquired OS permissions:\n\n")

	var hasSessions bool
	if m.manager != nil {
		sessions := m.manager.ListPersistentSessions()
		serverNames := make([]string, 0, len(sessions))
		for name := range sessions {
			serverNames = append(serverNames, name)
		}
		sort.Strings(serverNames)
		for _, serverName := range serverNames {
			for _, si := range sessions[serverName] {
				hasSessions = true
				alive := "alive"
				if !si.Alive {
					alive = "dead"
				}
				sb.WriteString(fmt.Sprintf("  [session] %-15s  session=%-10s  user=%-10s  dir=%s  (%s)\n",
					serverName, si.SessionID, si.CurrentUser, si.CurrentDir, alive))
			}
		}
	}
	if !hasSessions {
		sb.WriteString("  (no active persistent sessions)\n")
	}
	sb.WriteString("\n")

	var hasCache bool
	if m.privCache != nil {
		keys := m.privCache.List()
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].Target != keys[j].Target {
				return keys[i].Target < keys[j].Target
			}
			return keys[i].User < keys[j].User
		})
		for _, k := range keys {
			hasCache = true
			sb.WriteString(fmt.Sprintf("  [cache] target=%-15s  method=%-6s  user=%s\n", k.Target, k.Method, k.User))
		}
	}
	if !hasCache {
		sb.WriteString("  (no privilege cache entries)\n")
	}
	return sb.String()
}
