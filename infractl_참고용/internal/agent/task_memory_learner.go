package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
)

const taskMemoryLearnTimeout = 10 * time.Second

// TaskMemoryLearner persists task-level success/failure memory so similar future runs can reuse it.
type TaskMemoryLearner struct {
	knowledgeStore store.KnowledgeStore
	execLogStore   store.ExecLogStore
	memory         *rag.MemoryService
}

func NewTaskMemoryLearner(ks store.KnowledgeStore, es store.ExecLogStore, memory *rag.MemoryService) *TaskMemoryLearner {
	return &TaskMemoryLearner{
		knowledgeStore: ks,
		execLogStore:   es,
		memory:         memory,
	}
}

// TriggerRecord stores success/failure task memory asynchronously from a newly written execution log.
func (t *TaskMemoryLearner) TriggerRecord(ctx context.Context, logID int64) {
	if t == nil || logID == 0 || t.knowledgeStore == nil || t.execLogStore == nil {
		return
	}
	go func() {
		recordCtx, cancel := context.WithTimeout(context.Background(), taskMemoryLearnTimeout)
		defer cancel()
		if err := t.recordFromLog(recordCtx, logID); err != nil {
			slog.Debug("task memory record skipped", "log_id", logID, "err", err)
		}
	}()
}

func (t *TaskMemoryLearner) recordFromLog(ctx context.Context, logID int64) error {
	logEntry, err := t.execLogStore.GetExecutionLogByID(ctx, logID)
	if err != nil {
		return fmt.Errorf("get execution log %d: %w", logID, err)
	}
	if strings.TrimSpace(logEntry.TaskKey) == "" || strings.TrimSpace(logEntry.CommandKey) == "" {
		return nil
	}

	command := bestCommandText(logEntry)
	server := strings.TrimSpace(logEntry.TargetServer)
	if server == "" {
		server = "localhost"
	}

	category := "task_failure"
	title := fmt.Sprintf("Avoid failed pattern: %s", logEntry.CommandKey)
	situation := fmt.Sprintf("Task=%s server=%s tool=%s", logEntry.TaskKey, server, logEntry.ToolName)
	resolution := "Avoid this failed approach and try an alternative command path."
	errorPattern := strings.TrimSpace(logEntry.FailureSig)
	successCommand := ""
	confidence := 0.85

	if logEntry.Success {
		category = "task_success"
		title = fmt.Sprintf("Successful approach: %s", logEntry.CommandKey)
		resolution = "Reuse this sequence first for similar tasks on the same target."
		errorPattern = ""
		successCommand = command
		confidence = 0.95
	}

	workflowJSON := buildWorkflowJSON(ctx, t.knowledgeStore, server, logEntry.TaskKey, logEntry.CommandKey, command, logEntry.Success)

	entry := store.KnowledgeEntry{
		Category:       category,
		Title:          title,
		Situation:      situation,
		Resolution:     resolution,
		ToolName:       logEntry.ToolName,
		ErrorPattern:   errorPattern,
		SuccessCommand: successCommand,
		ServerName:     server,
		TaskKey:        logEntry.TaskKey,
		CommandKey:     logEntry.CommandKey,
		WorkflowJSON:   workflowJSON,
		Confidence:     confidence,
	}

	id, err := t.knowledgeStore.UpsertTaskMemory(ctx, entry)
	if err != nil {
		return fmt.Errorf("upsert task memory: %w", err)
	}
	entry.ID = id

	if t.memory != nil {
		if err := t.memory.IndexKnowledge(ctx, entry); err != nil {
			slog.Warn("index task memory into local memory", "id", id, "err", err)
		}
	}
	return nil
}

func bestCommandText(logEntry store.ExecutionLog) string {
	if logEntry.ToolName != "shell_exec" {
		return logEntry.CommandKey
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(logEntry.InputParams), &args); err != nil {
		return logEntry.CommandKey
	}
	cmd, _ := args["command"].(string)
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return logEntry.CommandKey
	}
	return cmd
}

func buildWorkflowJSON(
	ctx context.Context,
	ks store.KnowledgeStore,
	serverName string,
	taskKey string,
	commandKey string,
	commandText string,
	success bool,
) string {
	items := make([]string, 0, 5)
	if success && strings.TrimSpace(commandText) != "" {
		items = append(items, commandText)
	}

	existing, err := ks.SearchTaskMemories(ctx, "task_success", serverName, taskKey, "", 5)
	if err == nil {
		for _, e := range existing {
			v := strings.TrimSpace(e.SuccessCommand)
			if v == "" {
				continue
			}
			if containsString(items, v) {
				continue
			}
			items = append(items, v)
			if len(items) >= 5 {
				break
			}
		}
	}
	if !success {
		if strings.TrimSpace(commandKey) != "" && !containsString(items, commandKey) {
			items = append(items, "avoid:"+commandKey)
		}
	}
	if len(items) == 0 {
		return "[]"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
