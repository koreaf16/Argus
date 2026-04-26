// Package todo defines TodoWrite and TodoRead tools for the agent.
package todo

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// WriteToolName is the LLM-facing TodoWrite tool name.
const WriteToolName = "TodoWrite"

// ReadToolName is the LLM-facing TodoRead tool name.
const ReadToolName = "TodoRead"

// WriteTool writes or updates the session todo list.
type WriteTool struct {
	Tracker *Tracker
}

func (t *WriteTool) Name() string { return WriteToolName }
func (t *WriteTool) Description() string {
	return "Write or update the todo list for a multi-step task. Use this before mutation tools when the task has several steps."
}
func (t *WriteTool) IsReadOnly() bool { return false }
func (t *WriteTool) IsEnabled() bool  { return true }

func (t *WriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"markdown_list": map[string]interface{}{
				"type": "string",
				"description": "Provide the entire todo list as a markdown checklist. Use '- [ ]' for pending, '- [>]' for in_progress, and '- [x]' for completed. Begin each item with a unique ID (e.g., '1.', '2.'). Example:\n- [x] 1. Backup DB\n- [>] 2. Download Patch\n- [ ] 3. Apply RU",
			},
		},
		"required": []string{"markdown_list"},
	}
}

func (t *WriteTool) Execute(_ context.Context, args map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	rawMarkdown, ok := args["markdown_list"].(string)
	if !ok {
		return tools.ToolOutcome{}, fmt.Errorf("todo write: expected string for markdown_list")
	}

	// Clean up markdown code blocks if present
	rawMarkdown = strings.TrimSpace(rawMarkdown)
	if strings.HasPrefix(rawMarkdown, "```") {
		lines := strings.Split(rawMarkdown, "\n")
		if len(lines) > 2 {
			// Remove first and last line if they are backticks
			rawMarkdown = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	lines := strings.Split(rawMarkdown, "\n")
	deduped := make(map[string]TodoItem)
	idOrder := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}

		// Normalize line: remove leading list markers like '-', '*', '1.'
		origLine := line
		line = strings.TrimLeft(line, "-* \t")
		// Also trim leading numbers like "1. "
		if idx := strings.Index(line, " "); idx > 0 {
			prefix := line[:idx]
			if strings.HasSuffix(prefix, ".") || strings.HasSuffix(prefix, ":") {
				// Potential ID, keep it for now but trim from main content
				line = strings.TrimSpace(line[idx:])
			}
		}

		status := StatusPending
		content := ""

		// Check for checkboxes with more flexibility
		lowerLine := strings.ToLower(line)
		if strings.HasPrefix(lowerLine, "[x]") {
			status = StatusCompleted
			content = line[3:]
		} else if strings.HasPrefix(line, "[>]") {
			status = StatusInProgress
			content = line[3:]
		} else if strings.HasPrefix(line, "[ ]") {
			status = StatusPending
			content = line[3:]
		} else {
			// No checkbox found, default to pending and use stripped line
			status = StatusPending
			content = line
		}

		content = strings.TrimSpace(content)
		// Strip leading "N." or "N:" prefix from content (e.g., "1. Task" → "Task")
		if spIdx := strings.Index(content, " "); spIdx > 0 && spIdx < 5 {
			pfx := content[:spIdx]
			if strings.HasSuffix(pfx, ".") || strings.HasSuffix(pfx, ":") {
				allDigits := true
				for _, r := range pfx[:len(pfx)-1] {
					if r < '0' || r > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					content = strings.TrimSpace(content[spIdx:])
				}
			}
		}
		if content == "" {
			continue
		}

		// Try to extract ID again from the normalized content or use index
		id := fmt.Sprintf("%d", len(idOrder)+1)
		// If the original line had a pattern like "1. Task", try to use "1"
		trimmedOrig := strings.TrimLeft(origLine, "-* \t")
		if dotIdx := strings.Index(trimmedOrig, "."); dotIdx > 0 && dotIdx < 5 {
			possibleID := trimmedOrig[:dotIdx]
			isNum := true
			for _, r := range possibleID {
				if r < '0' || r > '9' {
					isNum = false
					break
				}
			}
			if isNum {
				id = possibleID
			}
		}

		item := TodoItem{
			ID:      id,
			Content: content,
			Status:  status,
		}
		if _, exists := deduped[id]; !exists {
			idOrder = append(idOrder, id)
		}
		deduped[id] = item
	}

	if len(idOrder) == 0 {
		err := fmt.Errorf("todo write: no valid checklist items found in markdown_list")
		return tools.ToolOutcome{ErrorMessage: err.Error()}, err
	}

	// First pass: Add new items and update status for anything that is NOT moving to in_progress.
	for _, id := range idOrder {
		item := deduped[id]
		existing, exists := t.Tracker.Store().Get(id)
		if !exists {
			tempItem := item
			if item.Status == StatusInProgress {
				tempItem.Status = StatusPending
			}
			if err := t.Tracker.Add(tempItem); err != nil {
				return tools.ToolOutcome{ErrorMessage: err.Error()}, fmt.Errorf("todo write: add: %w", err)
			}
			existing = tempItem
		}

		// Update non-in_progress status changes.
		if item.Status != StatusInProgress && item.Status != existing.Status {
			switch item.Status {
			case StatusCompleted:
				if err := t.Tracker.SetCompleted(id); err != nil {
					return tools.ToolOutcome{ErrorMessage: err.Error()}, fmt.Errorf("todo write: set completed: %w", err)
				}
			default:
				if err := t.Tracker.Store().Update(id, item.Status); err != nil {
					return tools.ToolOutcome{ErrorMessage: err.Error()}, fmt.Errorf("todo write: update: %w", err)
				}
			}
		}
	}

	// Second pass: Update in_progress status.
	// Ensure only one item is set to in_progress at a time.
	var lastInProgressID string
	for _, id := range idOrder {
		if deduped[id].Status == StatusInProgress {
			lastInProgressID = id
		}
	}

	if lastInProgressID != "" {
		existing, _ := t.Tracker.Store().Get(lastInProgressID)
		if existing.Status != StatusInProgress {
			// If someone else is in progress, they must be set to pending first.
			for _, it := range t.Tracker.Store().List() {
				if it.Status == StatusInProgress && it.ID != lastInProgressID {
					t.Tracker.Store().Update(it.ID, StatusPending)
				}
			}
			if err := t.Tracker.SetInProgress(lastInProgressID); err != nil {
				return tools.ToolOutcome{ErrorMessage: err.Error()}, fmt.Errorf("todo write: set in_progress: %w", err)
			}
		}
	}

	return tools.ToolOutcome{
		Content: Render(t.Tracker.Store().List()),
		Success: true,
	}, nil
}

// ReadTool returns the current todo list.
type ReadTool struct {
	Store *Store
}

func (t *ReadTool) Name() string        { return ReadToolName }
func (t *ReadTool) Description() string { return "Read the current todo list." }
func (t *ReadTool) IsReadOnly() bool    { return true }
func (t *ReadTool) IsEnabled() bool     { return true }

func (t *ReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *ReadTool) Execute(_ context.Context, _ map[string]interface{}, _ executor.Executor) (tools.ToolOutcome, error) {
	return tools.ToolOutcome{
		Content: Render(t.Store.List()),
		Success: true,
	}, nil
}
