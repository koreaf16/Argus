package query

import (
	"encoding/json"

	"github.com/koreaf16/argus/internal/services/llm"
)

func newUserTextMessage(text string) llm.Message {
	return llm.TextMessage(llm.RoleUser, text)
}

func newAssistantMessage(text string, toolCalls []llm.ToolUseStart) llm.Message {
	blocks := make([]llm.ContentBlock, 0, 1+len(toolCalls))
	if text != "" {
		blocks = append(blocks, llm.ContentBlock{
			Type: llm.ContentText,
			Text: text,
		})
	}
	for _, tc := range toolCalls {
		blocks = append(blocks, llm.ContentBlock{
			Type:  llm.ContentToolUse,
			ID:    tc.ID,
			Name:  tc.Name,
			Input: cloneJSON(tc.Input),
		})
	}
	return llm.Message{
		Role:    llm.RoleAssistant,
		Content: blocks,
	}
}

func newToolResultMessage(toolUseID, toolName, text string, isError bool) llm.Message {
	return llm.Message{
		Role: llm.RoleUser,
		Content: []llm.ContentBlock{
			{
				Type:      llm.ContentToolResult,
				Name:      toolName,
				ToolUseID: toolUseID,
				Text:      text,
				IsError:   isError,
			},
		},
	}
}

func cloneMessages(messages []llm.Message) []llm.Message {
	out := make([]llm.Message, len(messages))
	for i := range messages {
		out[i].Role = messages[i].Role
		out[i].Content = make([]llm.ContentBlock, len(messages[i].Content))
		for j := range messages[i].Content {
			out[i].Content[j] = messages[i].Content[j]
			out[i].Content[j].Input = cloneJSON(messages[i].Content[j].Input)
		}
	}
	return out
}

func cloneJSON(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}
