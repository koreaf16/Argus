// Package agent
// File: prompt.go
// Description: Build and compose system prompts with workspace context and behavior rules.`n// Responsibility: BuildContextual prompt assembly and INFRACTL.md loading with @include support.
package agent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
	"github.com/yourorg/infractl/internal/serverenv"
)

// BuildContextual assembles the contextual system prompt from enabled sections.
func BuildContextual(
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
	knowledgeContext string,
	taskMemoryContext string,
) string {
	layout := BuildContextualLayoutAt(
		sections,
		toolList,
		infractlMD,
		servers,
		activeServer,
		connectorStates,
		learnedSystems,
		ragSources,
		knowledgeStats,
		modelName,
		time.Now(),
		"",
	)
	return layout.Render(taskMemoryContext, knowledgeContext)
}

// appendActiveServerContext renders the active remote server and the local workspace fallback.
func appendActiveServerContext(sb *strings.Builder, srv *store.Server) {
	sb.WriteString("## Active Server Context\n")
	sb.WriteString(fmt.Sprintf("The current active server is **%s**. When a tool target is omitted, execute in this workspace unless the user explicitly asks for the local workspace.\n", srv.Name))
	sb.WriteString(fmt.Sprintf("- SSH: %s@%s:%d\n", srv.User, srv.Host, srv.Port))
	if srv.WorkspaceDir != "" {
		sb.WriteString(fmt.Sprintf("- Workspace directory: `%s`\n", srv.WorkspaceDir))
	}
	if srv.OS != "" {
		sb.WriteString(fmt.Sprintf("- OS/distribution: %s\n", srv.OS))
	}
	if srv.EnvProfile != "" {
		sb.WriteString(fmt.Sprintf("- Environment profile: %s\n", srv.EnvProfile))
	}
	sb.WriteString("- Use `target: \"localhost\"` only when the user asks for this local controller or local files.\n")
	sb.WriteString("- `localhost` means the machine running infractl and can be Windows, Linux, or macOS.\n\n")

	hostname, _ := os.Hostname()
	cwd, _ := os.Getwd()
	sb.WriteString("## Local Workspace (Always Available)\n")
	sb.WriteString(fmt.Sprintf("- Local OS: %s (%s)\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("- Local hostname: %s\n", hostname))
	sb.WriteString(fmt.Sprintf("- Local workspace root: %s\n", cwd))
	switch runtime.GOOS {
	case "windows":
		sb.WriteString("- Local shell: PowerShell\n")
		sb.WriteString("- Local Windows paths such as `C:\\...` can be inspected with PowerShell and `target: \"localhost\"`.\n")
	default:
		sb.WriteString("- Local shell: bash/sh\n")
		sb.WriteString("- Windows-style paths such as `C:\\...` are not readable from this local controller unless mounted or transferred first.\n")
	}
	sb.WriteString("Local execution is always possible through `target: \"localhost\"`. Do not claim local access is unavailable without trying the local workspace.\n\n")
}

// appendServerFocusSection guides the model to select the current server when needed.
func appendServerFocusSection(sb *strings.Builder, activeServer *store.Server, servers []store.Server) {
	if activeServer != nil {
		return
	}
	sb.WriteString("## Server Focus\n")
	switch {
	case len(servers) == 1:
		sb.WriteString(fmt.Sprintf("One SSH server is registered: **%s**. Call `workspace_focus` (alias: `server_focus`) to make it the active server when the user wants remote execution.\n", servers[0].Name))
	case len(servers) > 1:
		sb.WriteString("Multiple SSH servers are registered. Rules:\n")
		sb.WriteString("- If the user names a workspace, call `server_focus server=<name>` before omitting target.\n")
		sb.WriteString("- If it is unclear which workspace to use, call `workspace_focus` with no arguments so the user can choose.\n")
		sb.WriteString("- After a workspace is active, omit target for tools that should run there. Use `target: \"localhost\"` only for the local workspace.\n")
	default:
		sb.WriteString("No SSH servers are registered. The current server is the local workspace.\n")
	}
	sb.WriteString("\n")
}

// appendLearnedSystemsSection renders learned systems discovered in previous sessions.
func appendLearnedSystemsSection(sb *strings.Builder, learnedSystems []store.LearnedSystem) {
	sb.WriteString("## Learned Systems\n")
	sb.WriteString("The entries below were learned from previous sessions.\n")
	const maxDisplay = 10
	for i, sys := range learnedSystems {
		if i >= maxDisplay {
			sb.WriteString(fmt.Sprintf("- ... and %d more entries\n", len(learnedSystems)-maxDisplay))
			break
		}
		sb.WriteString(fmt.Sprintf("- **%s** on `%s`", sys.ServiceType, sys.ServerName))
		if sys.CLIPath != "" {
			sb.WriteString(fmt.Sprintf(" | CLI: `%s`", sys.CLIPath))
		}
		if sys.ConfigPath != "" {
			sb.WriteString(fmt.Sprintf(" | Config: `%s`", sys.ConfigPath))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Reuse these known paths first before running broad discovery.\n\n")
}

// appendConnectorsSection renders active connector state.
func appendConnectorsSection(sb *strings.Builder, connectorStates []connector.ConnectorState) {
	sb.WriteString("## Active Connectors\n")
	sb.WriteString("Currently connected database/service connectors are listed below.\n")
	sb.WriteString("**Note:** these are service connectors inside the active server, not registered SSH workspaces.\n")
	sb.WriteString("Use connector-specific tools for interactions (for example, `oracle_ORCL.tablespace`).\n\n")
	const maxDisplay = 15
	for i, cs := range connectorStates {
		if i >= maxDisplay {
			sb.WriteString(fmt.Sprintf("- ... and %d more connectors\n", len(connectorStates)-maxDisplay))
			break
		}
		icon := connectorIcon(string(cs.Status))
		sb.WriteString(fmt.Sprintf("%s %s/%s/%s -> %s (%d tools)\n",
			icon, cs.ServerName, cs.Type, cs.ServiceName, cs.Status, len(cs.Tools)))
	}
	sb.WriteString("\nSee **## Connector-First Rule** below for the mandatory activation sequence.\n\n")
}

// appendBehaviorRules renders global behavioral constraints.
func appendBehaviorRules(sb *strings.Builder) {
	sb.WriteString("## Behavior Rules\n")
	sb.WriteString("1. Confirm command execution targets when ambiguity exists.\n")
	sb.WriteString("   - If an active server is set and the user mentions another name, verify before running commands.\n")
	sb.WriteString("   - Do not guess when target scope is unclear.\n")
	sb.WriteString("2. Present results in concise, readable format.\n")
	sb.WriteString("3. Include a short reason in tool-call `description` fields.\n")
	sb.WriteString("4. If a tool fails, analyze output first and avoid blind retries.\n")
	sb.WriteString("5. Clearly distinguish \"not found by current search\" vs \"does not exist\".\n")
	sb.WriteString("6. Respond in the same language used by the user.\n")
	sb.WriteString("7. On repeated tool failures, use `rag_search` first; if unresolved, explain current state and options.\n")
	sb.WriteString("8. Unless explicitly requested, limit broad directory listing and discovery loops.\n")
	sb.WriteString("9. Keep final replies focused on conclusions and evidence, not exhaustive step logs.\n")
	sb.WriteString("10. For file lookup tasks without a specific filename, list directory contents first before filtering.\n")
	sb.WriteString("11. Use `TodoWrite` only for complex multi-step mutation tasks (install/deploy/config change), not read-only checks.\n")
	sb.WriteString("12. Never include special tokens like `<|` or `|>` or invalid escapes in tool-call JSON.\n")
	sb.WriteString("13. For existence/status probes, prefer safe commands that return exit code 0 (for example `... || echo \"not_found\"`) so expected misses are not surfaced as failures.\n\n")
}

// LoadInfractlMD loads ~/.infractl/INFRACTL.md with include expansion.
type infractlMDFileState struct {
	Path        string
	Size        int64
	ModUnixNano int64
}

func LoadInfractlMD() string {
	content, _ := loadInfractlMDData()
	return content
}

func loadInfractlMDData() (string, []infractlMDFileState) {
	stateDir, err := serverenv.StateDir()
	if err != nil {
		slog.Debug("get workspace state dir for INFRACTL.md", "err", err)
		return "", nil
	}

	path := filepath.Join(stateDir, "INFRACTL.md")
	info, err := os.Stat(path)
	if err != nil {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}

	slog.Debug("loaded INFRACTL.md", "path", path)
	baseDir := filepath.Dir(path)
	content, states := processIncludesWithState(string(data), baseDir, 0)
	root := infractlMDFileState{
		Path:        path,
		Size:        info.Size(),
		ModUnixNano: info.ModTime().UnixNano(),
	}
	return content, append([]infractlMDFileState{root}, states...)
}

// BuildMinimalChat builds a compact chat-only system prompt for no-tools flows.
func BuildMinimalChat(infractlMD string) string {
	return buildMinimalChatAt(infractlMD, time.Now())
}
func appendCurrentDateContext(sb *strings.Builder, now time.Time) {
	sb.WriteString(fmt.Sprintf("## Current Date\n**%s** (UTC%s)\n\n",
		now.Format("2006-01-02 (Mon)"),
		now.Format("-07:00"),
	))
}

func processIncludes(content string, baseDir string, depth int) string {
	rendered, _ := processIncludesWithState(content, baseDir, depth)
	return rendered
}

func processIncludesWithState(content string, baseDir string, depth int) (string, []infractlMDFileState) {
	if depth >= 5 {
		return content, nil
	}

	var result strings.Builder
	var states []infractlMDFileState
	totalSize := 0
	const maxTotalMDSize = 50000 // max 50KB

	for _, line := range strings.Split(content, "\n") {
		if totalSize > maxTotalMDSize {
			result.WriteString("\n[... INFRACTL.md include limit reached ...]\n")
			break
		}

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@include ") {
			includePath := strings.TrimSpace(trimmed[len("@include "):])
			if !filepath.IsAbs(includePath) {
				includePath = filepath.Join(baseDir, includePath)
			}
			info, statErr := os.Stat(includePath)
			if statErr == nil {
				states = append(states, infractlMDFileState{
					Path:        includePath,
					Size:        info.Size(),
					ModUnixNano: info.ModTime().UnixNano(),
				})
			}
			data, err := os.ReadFile(includePath)
			if err != nil {
				slog.Debug("@include file not found", "path", includePath, "err", err)
				result.WriteString(line + "\n")
				totalSize += len(line) + 1
				continue
			}
			slog.Debug("@include processed", "path", includePath)
			includeBaseDir := filepath.Dir(includePath)
			included, nestedStates := processIncludesWithState(string(data), includeBaseDir, depth+1)
			states = append(states, nestedStates...)
			result.WriteString(included)
			totalSize += len(included)
			if !strings.HasSuffix(included, "\n") {
				result.WriteString("\n")
				totalSize += 1
			}
			continue
		}
		result.WriteString(line + "\n")
		totalSize += len(line) + 1
	}
	return strings.TrimSuffix(result.String(), "\n"), states
}
