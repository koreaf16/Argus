// Package tools
// File: knowledge_search.go
// Description: FTS5 기반 로컬 지식 베이스 검색 도구
// Responsibility: knowledge_base에서 과거 에러 패턴 및 지식을 키워드 검색하여 반환

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

// KnowledgeSearchTool은 로컬 knowledge_base에서 FTS5 검색을 수행하는 도구이다.
type KnowledgeSearchTool struct {
	Store store.KnowledgeStore
}

func (t *KnowledgeSearchTool) Name() string         { return "knowledge_search" }
func (t *KnowledgeSearchTool) IsReadOnly() bool     { return true }
func (t *KnowledgeSearchTool) IsEnabled() bool      { return true }

func (t *KnowledgeSearchTool) Description() string {
	return "Search the local knowledge base for previously learned error patterns, solutions, and tips. Always check this BEFORE using web_search to avoid redundant lookups. Returns matching entries with situation and resolution."
}

func (t *KnowledgeSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Keywords to search for (e.g. error code, tool name, system name)",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Optional filter: 'error_pattern', 'system_knowledge', 'procedure', or 'tip'",
				"enum":        []string{"error_pattern", "system_knowledge", "procedure", "tip"},
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum results (default: 5)",
				"default":     5,
			},
		},
		"required": []string{"query"},
	}
}

func (t *KnowledgeSearchTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	query, err := argString(args, "query", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	limit := argInt(args, "limit", 5)

	entries, err := t.Store.SearchKnowledge(ctx, query, limit)
	if err != nil {
		return ToolOutcome{}, fmt.Errorf("knowledge search %q: %w", query, err)
	}
	if len(entries) == 0 {
		return ToolOutcome{Content: fmt.Sprintf("No knowledge found for: %q\nConsider using web_search to find information online.", query), Success: true}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d knowledge entries for: %q\n\n", len(entries), query))
	for i, e := range entries {
		sb.WriteString(fmt.Sprintf("--- [%d] %s (%s) ---\n", i+1, e.Title, e.Category))
		if e.Situation != "" {
			sb.WriteString(fmt.Sprintf("Situation: %s\n", e.Situation))
		}
		sb.WriteString(fmt.Sprintf("Resolution: %s\n", e.Resolution))
		if e.SuccessCommand != "" {
			sb.WriteString(fmt.Sprintf("Command: %s\n", e.SuccessCommand))
		}
		sb.WriteString(fmt.Sprintf("(confidence: %.1f, used %d times)\n\n", e.Confidence, e.UseCount))

		// 사용 카운트 증가 (비동기, 에러 무시)
		go t.Store.IncrementUseCount(context.Background(), e.ID) //nolint: errcheck
	}
	return ToolOutcome{Content: sb.String(), Success: true}, nil
}
