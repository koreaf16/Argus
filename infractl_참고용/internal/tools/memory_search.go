// Package tools
// File: memory_search.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
)

// MemorySearchTool inspects internal memory status or performs a hybrid search.
type MemorySearchTool struct {
	Memory *rag.MemoryService
}

func (t *MemorySearchTool) Name() string         { return "memory_search" }
func (t *MemorySearchTool) IsReadOnly() bool     { return true }
func (t *MemorySearchTool) IsEnabled() bool      { return true }

func (t *MemorySearchTool) Description() string {
	return "Inspect internal memory index status or run an internal hybrid search over knowledge, learned systems, and past conversations."
}

func (t *MemorySearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Optional query. When omitted, returns internal memory status.",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return when query is set.",
				"default":     5,
			},
		},
	}
}

func (t *MemorySearchTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	if t.Memory == nil {
		return ToolOutcome{Content: "internal memory service is not configured", Success: true}, nil
	}

	query, _ := argString(args, "query", false)
	if strings.TrimSpace(query) == "" {
		stats, err := t.Memory.Stats(ctx)
		if err != nil {
			return ToolOutcome{}, fmt.Errorf("memory stats: %w", err)
		}
		return ToolOutcome{Content: formatMemoryStats(stats, t.Memory.EmbedderStatus()), Success: true}, nil
	}

	topK := argInt(args, "max_results", 5)
	results, err := t.Memory.Search(ctx, query, topK, 0.05)
	if err != nil {
		return ToolOutcome{}, fmt.Errorf("memory search: %w", err)
	}
	if len(results) == 0 {
		return ToolOutcome{Content: fmt.Sprintf("No internal memory found for: %q", query), Success: true}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Internal memory search results for %q\n\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s", i+1, r.Document.SourceType, r.Document.Title))
		if r.Document.ServerName != "" {
			sb.WriteString(fmt.Sprintf(" @ %s", r.Document.ServerName))
		}
		sb.WriteString(fmt.Sprintf(" (score: %.3f)\n", r.Score))
		sb.WriteString(truncateForTool(r.Document.Content, 320))
		sb.WriteString("\n\n")
	}
	return ToolOutcome{Content: strings.TrimSpace(sb.String()), Success: true}, nil
}

func formatMemoryStats(stats store.MemoryStats, embedderStatus string) string {
	var keys []string
	for key := range stats.CountBySource {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("Internal memory status\n")
	sb.WriteString(fmt.Sprintf("Total docs: %d\n", stats.TotalDocs))
	sb.WriteString(fmt.Sprintf("With vectors: %d\n", stats.TotalWithVector))
	if strings.TrimSpace(embedderStatus) != "" {
		sb.WriteString(embedderStatus + "\n")
	}
	for _, key := range keys {
		sb.WriteString(fmt.Sprintf("- %s: %d\n", key, stats.CountBySource[key]))
	}
	return strings.TrimSpace(sb.String())
}

func truncateForTool(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
