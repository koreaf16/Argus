package tui

import (
	"fmt"
	"sort"
	"strings"
)

type slashCommandDef struct {
	Name        string
	Aliases     []string
	Usage       string
	Description string
}

var slashCommandRegistry = []slashCommandDef{
	{Name: "/help", Description: "show command help"},
	{Name: "/tools", Description: "list registered tools"},
	{Name: "/workspaces", Aliases: []string{"/servers"}, Description: "list registered servers"},
	{Name: "/workspace", Aliases: []string{"/server"}, Usage: "/workspace [name|clear]", Description: "set the active server"},
	{Name: "/services", Usage: "/services [workspace]", Description: "show services for a workspace"},
	{Name: "/connectors", Description: "show active connectors"},
	{Name: "/mcp", Usage: "/mcp [reconnect <name>]", Description: "show MCP status"},
	{Name: "/sessions", Usage: "/sessions [restore <N>]", Description: "show recent sessions"},
	{Name: "/history", Description: "show prompt history"},
	{Name: "/context", Description: "show prompt/context cache stats"},
	{Name: "/knowledge", Description: "search and manage knowledge"},
	{Name: "/rag", Description: "manage RAG sources"},
	{Name: "/checkpoints", Description: "show checkpoints"},
	{Name: "/hooks", Description: "manage hooks"},
	{Name: "/schedules", Description: "manage schedules"},
	{Name: "/ospermission", Aliases: []string{"/osessions"}, Description: "show OS permission cache"},
	{Name: "/yoro", Description: "toggle YORO mode"},
	{Name: "/elevate", Description: "toggle elevated sudo/su mode"},
	{Name: "/clear", Description: "clear conversation history"},
	{Name: "/model", Description: "show current model"},
	{Name: "/quit", Aliases: []string{"/exit", "/q"}, Description: "quit the app"},
	{Name: "/audit", Usage: "/audit [list|show <id>]", Description: "show audit run history"},
}

func slashCommandNames(def slashCommandDef) []string {
	names := make([]string, 0, 1+len(def.Aliases))
	names = append(names, def.Name)
	names = append(names, def.Aliases...)
	return names
}

func slashCommandDisplayName(def slashCommandDef) string {
	return def.Name
}

func slashCommandHelpLabel(def slashCommandDef) string {
	label := slashCommandUsage(def)
	if len(def.Aliases) > 0 {
		label = fmt.Sprintf("%s (%s)", label, strings.Join(def.Aliases, ", "))
	}
	return label
}

func slashCommandUsage(def slashCommandDef) string {
	if def.Usage != "" {
		return def.Usage
	}
	return def.Name
}

func resolveSlashCommand(name string) (slashCommandDef, bool) {
	needle := strings.ToLower(strings.TrimSpace(name))
	for _, def := range slashCommandRegistry {
		for _, candidate := range slashCommandNames(def) {
			if strings.EqualFold(candidate, needle) {
				return def, true
			}
		}
	}
	return slashCommandDef{}, false
}

func slashCommandMatchesPrefix(prefix string) []slashCommandDef {
	needle := strings.ToLower(strings.TrimSpace(prefix))
	if needle == "/" {
		needle = ""
	}
	if strings.HasPrefix(needle, "/") {
		needle = strings.TrimPrefix(needle, "/")
	}

	matches := make([]slashCommandDef, 0, len(slashCommandRegistry))
	for _, def := range slashCommandRegistry {
		for _, candidate := range slashCommandNames(def) {
			candidate = strings.ToLower(strings.TrimPrefix(candidate, "/"))
			if needle == "" || strings.HasPrefix(candidate, needle) {
				matches = append(matches, def)
				break
			}
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		left := slashCommandDisplayName(matches[i])
		right := slashCommandDisplayName(matches[j])
		if len(left) != len(right) {
			return len(left) < len(right)
		}
		return left < right
	})
	return matches
}

func slashCommandHelpText() string {
	var sb strings.Builder
	sections := []struct {
		title string
		names []string
	}{
		{title: "Core", names: []string{"/help", "/clear", "/model", "/quit"}},
		{title: "Navigation", names: []string{"/tools", "/workspaces", "/workspace", "/sessions", "/history", "/context"}},
		{title: "Integrations", names: []string{"/connectors", "/mcp", "/knowledge", "/rag"}},
		{title: "Automation", names: []string{"/checkpoints", "/hooks", "/schedules", "/ospermission", "/yoro", "/elevate"}},
	}

	defByName := make(map[string]slashCommandDef, len(slashCommandRegistry))
	for _, def := range slashCommandRegistry {
		defByName[def.Name] = def
		for _, alias := range def.Aliases {
			defByName[alias] = def
		}
	}

	sb.WriteString("Slash commands:\n")
	for _, section := range sections {
		sb.WriteString("\n")
		sb.WriteString(section.title)
		sb.WriteString(":\n")
		for _, name := range section.names {
			def, ok := defByName[name]
			if !ok {
				continue
			}
			sb.WriteString(fmt.Sprintf("  %-28s %s\n", slashCommandHelpLabel(def), def.Description))
		}
	}
	sb.WriteString("\nExamples:\n")
	sb.WriteString("  /workspace clear\n")
	sb.WriteString("  /sessions restore 1\n")
	sb.WriteString("  /context\n")
	sb.WriteString("  /mcp reconnect <name>\n")
	sb.WriteString("  /knowledge search <query>\n")
	sb.WriteString("  /rag delete <id>\n")
	return sb.String()
}
