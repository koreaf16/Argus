package agent

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

// ContextualPromptLayout separates session-stable prompt sections from
// per-turn dynamic inserts such as task memory and prefetched knowledge.
type ContextualPromptLayout struct {
	prefix           string
	beforeKnowledge  string
	afterKnowledge   string
	hasKnowledgeSlot bool
}

func BuildContextualLayoutAt(
	sections SectionSet,
	toolList []tools.Tool,
	infractlMD string,
	servers []store.Server,
	activeServer *store.Server,
	connectorStates []connector.ConnectorState,
	learnedSystems []store.LearnedSystem,
	ragSources []store.RAGSource,
	knowledgeStats *rag.KnowledgeStats,
	modelName string,
	now time.Time,
	candidatesSection string,
) ContextualPromptLayout {
	var prefix strings.Builder
	var beforeKnowledge strings.Builder
	var afterKnowledge strings.Builder

	isQwen := strings.Contains(strings.ToLower(modelName), "qwen")
	appendContextualCore(&prefix, isQwen, now)
	appendContextualEnvironment(&prefix, activeServer)
	appendAgentBehaviorRules(&prefix, candidatesSection)
	appendContextualPreKnowledgeSections(&beforeKnowledge, sections, toolList, servers, activeServer, isQwen)

	hasKnowledgeSlot := sections.Has(SectionRAG)
	appendContextualPostKnowledgeSections(
		&afterKnowledge,
		sections,
		infractlMD,
		connectorStates,
		learnedSystems,
		ragSources,
		knowledgeStats,
	)

	return ContextualPromptLayout{
		prefix:           prefix.String(),
		beforeKnowledge:  beforeKnowledge.String(),
		afterKnowledge:   afterKnowledge.String(),
		hasKnowledgeSlot: hasKnowledgeSlot,
	}
}

func (l ContextualPromptLayout) Render(taskMemoryContext, knowledgeContext string) string {
	var sb strings.Builder
	sb.WriteString(l.prefix)
	if strings.TrimSpace(taskMemoryContext) != "" {
		sb.WriteString(taskMemoryContext)
		sb.WriteString("\n\n")
	}
	sb.WriteString(l.beforeKnowledge)
	if l.hasKnowledgeSlot && knowledgeContext != "" {
		sb.WriteString(knowledgeContext)
		sb.WriteString("\n\n")
	}
	sb.WriteString(l.afterKnowledge)
	return strings.TrimSpace(sb.String())
}

func buildMinimalChatAt(infractlMD string, now time.Time) string {
	var sb strings.Builder
	sb.WriteString("You are infractl, an infrastructure operations AI agent.\n")
	appendCurrentDateContext(&sb, now)
	sb.WriteString("Rules:\n")
	sb.WriteString("- Respond in the user's language.\n")
	sb.WriteString("- Keep responses concise and actionable.\n")
	sb.WriteString("- If infrastructure context is required (commands, logs, status), ask for the minimum required input.\n")
	sb.WriteString("- Do not call tools for greetings or small talk.\n")
	if infractlMD != "" {
		sb.WriteString("\n## Project-Specific Instructions\n")
		sb.WriteString(infractlMD)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}
func appendContextualCore(sb *strings.Builder, isQwen bool, now time.Time) {
	sb.WriteString("You are infractl, an infrastructure operations AI agent.\n")
	sb.WriteString("You manage systems by selecting the correct server and using the minimum necessary tools.\n\n")

	sb.WriteString("## Core Rules\n")
	sb.WriteString("1. A server is the execution context. Local server is the directory where infractl was launched; its state lives in `.infractl`. SSH servers run from the configured remote server directory, default `~/.infractl/workspace`.\n")
	sb.WriteString("2. If a tool target is omitted, run in the current server. If no SSH server is active, the current server is local.\n")
	sb.WriteString("3. Only call `server_focus` when the user explicitly asks to connect or switch to a named server. Never call `server_focus` just because a software product name matches a server name.\n")
	sb.WriteString("4. A server name is not a database, service, SID, PDB, instance name, or software product/version. Use connector tools for database/service access inside a server.\n")
	sb.WriteString("5. Use the fewest tools needed. When you have enough evidence, answer directly.\n")
	sb.WriteString("6. Keep database checks non-interactive unless the user explicitly asks for an interactive session. Prefer quick checks such as `SELECT 1` and exit cleanly.\n\n")

	if isQwen {
		tools.AppendQwenToolGuideline(sb)
	}
	appendCurrentDateContext(sb, now)
}
func appendContextualEnvironment(sb *strings.Builder, activeServer *store.Server) {
	if activeServer != nil {
		appendActiveServerContext(sb, activeServer)
		return
	}

	hostname, _ := os.Hostname()
	cwd, _ := os.Getwd()
	appendLocalControllerContext(sb, runtime.GOOS, runtime.GOARCH, hostname, cwd)
}

// appendAgentBehaviorRules는 tier 판단·disambiguation·propose_action·memory_save 규칙을 주입한다.
// general tier가 기본 진입점이므로 모든 공통 판단 규칙을 여기서 명시한다.
func appendAgentBehaviorRules(sb *strings.Builder, candidatesSection string) {
	sb.WriteString("## Agent Behavior Rules\n")
	sb.WriteString("- **Tier escalation**: If the request requires multi-step planning, root-cause analysis, migration, or architecture design, call `escalate_to_reasoning` as the FIRST tool call.\n")
	sb.WriteString("- **Disambiguation**: If the user's message could refer to multiple entities (server vs. service, or multiple servers), call `ask_user_question` with the candidate list BEFORE taking any action. Never guess or proceed with the first match.\n")
	sb.WriteString("- **Command execution**: Always propose shell/SSH/DB commands via `propose_action`. Never output commands as plain code blocks only.\n")
	sb.WriteString("- **Memory save**: If the user says '기억해', '저장해', 'remember this', or similar, call `knowledge_add` immediately.\n")
	sb.WriteString("- **Server switch**: If the user explicitly asks to connect or switch to a named server, call `server_focus`. Do NOT auto-switch based on a software product name.\n\n")
	if candidatesSection != "" {
		sb.WriteString(candidatesSection)
	}
}

func appendContextualPreKnowledgeSections(
	sb *strings.Builder,
	sections SectionSet,
	toolList []tools.Tool,
	servers []store.Server,
	activeServer *store.Server,
	isQwen bool,
) {
	if sections.Has(SectionTools) && len(toolList) > 0 && isQwen {
		tools.AppendQwenToolIndex(sb, toolList)
	}

	if sections.Has(SectionServers) && len(servers) > 0 {
		sb.WriteString("## Registered Servers (SSH)\n")
		sb.WriteString("These are SSH-backed servers. A server name is not a database, service, SID, instance name, or software product name.\n")
		sb.WriteString("Remote commands run from the server directory configured for that SSH account.\n\n")
		for _, srv := range servers {
			sb.WriteString(formatKnownServerLine(srv))
		}
		sb.WriteString("\n")

		sb.WriteString("CRITICAL: Do NOT use a server name as a tool `target` just because a software product or service name appears in the user's message. Software install/setup requests stay in the current server.\n")
		if activeServer == nil {
			sb.WriteString("If the user explicitly asks to switch to one of these servers, call `server_focus server=<name>`. If no active server is selected and the user did not specify one, omitted target means the current local server.\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("Active server is **%s**. When the user says \"this server\", \"here\", \"로컬\", \"이 PC\", or similar, it always means **%s**.\n", activeServer.Name, activeServer.Name))
			sb.WriteString(fmt.Sprintf("ALL shell_exec / file_transfer / command-running tools run on **%s** by default. NEVER pass `target: \"localhost\"` — the dispatcher will reject it while a server is active. Omit the `target` field instead.\n\n", activeServer.Name))
		}
	}
	if sections.Has(SectionGuardrails) {
		appendContextGuardrails(sb, activeServer == nil)
	}

	if sections.Has(SectionServerFocus) {
		appendServerFocusSection(sb, activeServer, servers)
	}
}
func appendContextualPostKnowledgeSections(
	sb *strings.Builder,
	sections SectionSet,
	infractlMD string,
	connectorStates []connector.ConnectorState,
	learnedSystems []store.LearnedSystem,
	ragSources []store.RAGSource,
	knowledgeStats *rag.KnowledgeStats,
) {
	if sections.Has(SectionRAG) {
		appendRAGSection(sb, ragSources, knowledgeStats)
	}

	if sections.Has(SectionLearnedSystems) && len(learnedSystems) > 0 {
		appendLearnedSystemsSection(sb, learnedSystems)
	}

	if sections.Has(SectionConnectors) && len(connectorStates) > 0 {
		appendConnectorsSection(sb, connectorStates)
	}

	if sections.Has(SectionConnectorFirst) {
		appendConnectorFirstRule(sb)
	}

	if sections.Has(SectionSafety) {
		appendSafetyRules(sb)
	}

	if sections.Has(SectionToolPriority) {
		appendDedicatedToolPriority(sb)
	}

	if sections.Has(SectionToolSelection) {
		appendToolSelectionGuidelines(sb)
	}

	if sections.Has(SectionErrorRecovery) {
		appendErrorRecoveryProtocol(sb)
	}

	if sections.Has(SectionDiscovery) {
		appendDiscoveryFlow(sb)
	}

	if sections.Has(SectionTaskCompletion) {
		appendTaskCompletionRules(sb)
	}

	if sections.Has(SectionGrounding) {
		appendGroundingRules(sb)
	}

	if sections.Has(SectionBehavior) {
		appendBehaviorRules(sb)
	}

	if sections.Has(SectionPhasePlanning) {
		appendPhasePlanning(sb)
	}

	if sections.Has(SectionInfractlMD) && infractlMD != "" {
		sb.WriteString("## Project-Specific Instructions\n")
		sb.WriteString(infractlMD)
		sb.WriteString("\n\n")
	}
}
func formatKnownServerLine(srv store.Server) string {
	line := fmt.Sprintf("- **%s** (%s@%s:%d)", srv.Name, srv.User, srv.Host, srv.Port)
	if srv.WorkspaceDir != "" {
		line += fmt.Sprintf(" | server dir: `%s`", srv.WorkspaceDir)
	}
	if srv.OS != "" {
		line += fmt.Sprintf(" | OS: %s", srv.OS)
	}
	return line + "\n"
}
