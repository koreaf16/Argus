// Package agent
// File: knowledge.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
)

const learnTimeout = 30 * time.Second

// KnowledgeLearner extracts reusable error patterns from retry chains.
type KnowledgeLearner struct {
	knowledgeStore store.KnowledgeStore
	execLogStore   store.ExecLogStore
	llmClient      llm.Client
	memory         *rag.MemoryService
}

// NewKnowledgeLearner creates a knowledge learner.
func NewKnowledgeLearner(ks store.KnowledgeStore, es store.ExecLogStore, lc llm.Client, memory *rag.MemoryService) *KnowledgeLearner {
	return &KnowledgeLearner{
		knowledgeStore: ks,
		execLogStore:   es,
		llmClient:      lc,
		memory:         memory,
	}
}

// TriggerLearn launches background learning for a successful retry.
func (kl *KnowledgeLearner) TriggerLearn(ctx context.Context, successLogID int64) {
	if successLogID == 0 {
		return
	}
	go func() {
		learnCtx, cancel := context.WithTimeout(context.Background(), learnTimeout)
		defer cancel()
		if err := kl.learnFromLog(learnCtx, successLogID); err != nil {
			slog.Debug("knowledge learn skipped", "log_id", successLogID, "err", err)
		}
	}()
}

func (kl *KnowledgeLearner) learnFromLog(ctx context.Context, successLogID int64) error {
	successLog, err := kl.execLogStore.GetExecutionLogByID(ctx, successLogID)
	if err != nil {
		return fmt.Errorf("get success log %d: %w", successLogID, err)
	}
	if successLog.RetryOf == nil {
		return nil
	}

	failedLog, err := kl.execLogStore.GetExecutionLogByID(ctx, *successLog.RetryOf)
	if err != nil {
		return fmt.Errorf("get failed log %d: %w", *successLog.RetryOf, err)
	}
	if failedLog.ErrorMessage == "" {
		return nil
	}

	existing, _ := kl.knowledgeStore.SearchKnowledge(ctx, failedLog.ErrorMessage[:min(len(failedLog.ErrorMessage), 50)], 1)
	for _, e := range existing {
		if e.SourceExecutionID != nil && *e.SourceExecutionID == failedLog.ID {
			return nil
		}
	}

	entry, err := kl.extractPattern(ctx, failedLog, successLog)
	if err != nil {
		return fmt.Errorf("extract pattern: %w", err)
	}

	id, err := kl.knowledgeStore.SaveKnowledge(ctx, entry)
	if err != nil {
		return fmt.Errorf("save knowledge: %w", err)
	}
	entry.ID = id
	if kl.memory != nil {
		if err := kl.memory.IndexKnowledge(ctx, entry); err != nil {
			slog.Warn("index learned knowledge into memory", "id", id, "err", err)
		}
	}

	slog.Info("knowledge learned from retry chain",
		"id", id, "tool", failedLog.ToolName, "pattern", entry.ErrorPattern)
	return nil
}

func (kl *KnowledgeLearner) extractPattern(ctx context.Context, failed, success store.ExecutionLog) (store.KnowledgeEntry, error) {
	prompt := fmt.Sprintf(`Analyze this tool execution retry pair and extract a concise error pattern.

FAILED execution:
- Tool: %s
- Input: %s
- Error: %s

SUCCESSFUL retry:
- Tool: %s
- Input: %s
- Output (first 500 chars): %s

Respond in this exact JSON format (no markdown, no explanation):
{
  "title": "brief title of the error and fix",
  "situation": "when/where this error occurs (1-2 sentences)",
  "resolution": "how to fix it (1-2 sentences)",
  "error_pattern": "key error keyword or code (e.g. ORA-00942, permission denied)",
  "success_command": "the successful command or key change made"
}`,
		failed.ToolName, failed.InputParams, failed.ErrorMessage,
		success.ToolName, success.InputParams, truncate(success.Output, 500),
	)

	resp, err := kl.llmClient.Chat(ctx, []llm.Message{
		{Role: llm.RoleSystem, Content: "You extract structured error patterns from tool execution logs. Always respond with valid JSON only."},
		{Role: llm.RoleUser, Content: prompt},
	}, nil, nil)
	if err != nil {
		return store.KnowledgeEntry{}, fmt.Errorf("llm extract pattern: %w", err)
	}

	parsed := parsePatternJSON(resp.Content)
	srcID := failed.ID
	return store.KnowledgeEntry{
		Category:          "error_pattern",
		Title:             parsed["title"],
		Situation:         parsed["situation"],
		Resolution:        parsed["resolution"],
		ToolName:          failed.ToolName,
		ErrorPattern:      parsed["error_pattern"],
		SuccessCommand:    parsed["success_command"],
		SourceExecutionID: &srcID,
		Confidence:        0.9,
	}, nil
}

func parsePatternJSON(content string) map[string]string {
	result := map[string]string{
		"title": "Auto-learned error pattern", "situation": "", "resolution": content,
		"error_pattern": "", "success_command": "",
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(content), &parsed); err == nil {
		for k, v := range parsed {
			if v != "" {
				result[k] = v
			}
		}
	}
	return result
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
