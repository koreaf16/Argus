// Package tools
// File: knowledge_add.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
)

// KnowledgeAddTool stores user-provided knowledge in the local knowledge base.
type KnowledgeAddTool struct {
	Store  store.KnowledgeStore
	Memory *rag.MemoryService
}

func (t *KnowledgeAddTool) Name() string         { return "knowledge_add" }
func (t *KnowledgeAddTool) IsReadOnly() bool     { return false }
func (t *KnowledgeAddTool) IsEnabled() bool      { return true }

func (t *KnowledgeAddTool) Description() string {
	return "Save knowledge to the local knowledge base. Saved entries are indexed into internal memory hybrid search and can be retrieved by knowledge_search and rag_search."
}

func (t *KnowledgeAddTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Knowledge category",
				"enum":        []string{"error_pattern", "system_knowledge", "procedure", "tip"},
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Brief title for this knowledge entry",
			},
			"situation": map[string]interface{}{
				"type":        "string",
				"description": "When/where this knowledge applies (context description)",
			},
			"resolution": map[string]interface{}{
				"type":        "string",
				"description": "The solution, rule, or procedure to remember",
			},
			"error_pattern": map[string]interface{}{
				"type":        "string",
				"description": "Optional: error code or keyword pattern (e.g. 'ORA-00942')",
			},
			"tool_name": map[string]interface{}{
				"type":        "string",
				"description": "Optional: related tool name (e.g. 'shell_exec')",
			},
			"success_command": map[string]interface{}{
				"type":        "string",
				"description": "Optional: the command or query that succeeded",
			},
		},
		"required": []string{"category", "title", "resolution"},
	}
}

func (t *KnowledgeAddTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	category, err := argString(args, "category", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	title, err := argString(args, "title", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	resolution, err := argString(args, "resolution", true)
	if err != nil {
		return ToolOutcome{}, err
	}
	situation, _ := argString(args, "situation", false)
	errorPattern, _ := argString(args, "error_pattern", false)
	toolName, _ := argString(args, "tool_name", false)
	successCmd, _ := argString(args, "success_command", false)

	entry := store.KnowledgeEntry{
		Category:       category,
		Title:          title,
		Situation:      situation,
		Resolution:     resolution,
		ToolName:       toolName,
		ErrorPattern:   errorPattern,
		SuccessCommand: successCmd,
		Confidence:     1.0,
	}

	id, err := t.Store.SaveKnowledge(ctx, entry)
	if err != nil {
		return ToolOutcome{}, fmt.Errorf("save knowledge: %w", err)
	}
	entry.ID = id
	if t.Memory != nil {
		if err := t.Memory.IndexKnowledge(ctx, entry); err != nil {
			slog.Warn("index knowledge_add into memory", "id", id, "err", err)
		}
	}

	return ToolOutcome{Content: fmt.Sprintf("Saved knowledge entry (ID: %d)\nCategory: %s\nTitle: %s", id, category, title), Success: true}, nil
}
