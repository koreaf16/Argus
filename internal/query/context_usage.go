package query

import (
	"encoding/json"
	"strings"

	ctxpkg "github.com/koreaf16/argus/internal/context"
	"github.com/koreaf16/argus/internal/services/llm"
)

func buildContextUsageBreakdown(system []llm.SystemBlock, messages []llm.Message, tools []llm.ToolSpec, totalEstimate int) map[string]any {
	systemChars := 0
	for _, block := range system {
		systemChars += len(block.Text)
	}

	messageChars := 0
	toolResultChars := 0
	clearedResults := 0
	artifactRefs := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			messageChars += len(block.Text) + len(block.Name)
			if block.Type != llm.ContentToolResult {
				continue
			}
			toolResultChars += len(block.Text)
			if strings.Contains(block.Text, ctxpkg.OldToolResultClearedPlaceholder) {
				clearedResults++
			}
			if strings.Contains(block.Text, "[Full output:") || strings.Contains(block.Text, "[전체 출력:") {
				artifactRefs++
			}
		}
	}

	toolSchemaChars := 0
	if payload, err := json.Marshal(tools); err == nil {
		toolSchemaChars = len(payload)
	}

	return map[string]any{
		"estimated_total_tokens":     totalEstimate,
		"system_tokens":              systemChars / 4,
		"messages_tokens":            messageChars / 4,
		"tool_schema_tokens":         toolSchemaChars / 4,
		"tool_result_tokens":         toolResultChars / 4,
		"cleared_tool_results_count": clearedResults,
		"artifact_ref_count":         artifactRefs,
	}
}
