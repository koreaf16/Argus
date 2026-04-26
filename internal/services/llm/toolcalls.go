package llm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type StreamAccumulator struct {
	openAITools map[int]*openAIToolAccum
}

type openAIToolAccum struct {
	id   string
	name string
	args strings.Builder
}

func NewStreamAccumulator() *StreamAccumulator {
	return &StreamAccumulator{
		openAITools: make(map[int]*openAIToolAccum),
	}
}

func (a *StreamAccumulator) AppendOpenAIToolDelta(index int, id, name, argsPart string) {
	acc, ok := a.openAITools[index]
	if !ok {
		acc = &openAIToolAccum{}
		a.openAITools[index] = acc
	}
	if id != "" {
		acc.id = id
	}
	if name != "" {
		acc.name = name
	}
	if argsPart != "" {
		acc.args.WriteString(argsPart)
	}
}

func (a *StreamAccumulator) FlushOpenAIToolUses() []ToolUseStart {
	out := make([]ToolUseStart, 0, len(a.openAITools))
	keys := make([]int, 0, len(a.openAITools))
	for idx := range a.openAITools {
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	for _, idx := range keys {
		acc := a.openAITools[idx]
		argRaw := strings.TrimSpace(acc.args.String())
		if argRaw == "" {
			argRaw = "{}"
		}
		if !json.Valid([]byte(argRaw)) {
			if wrapped, err := json.Marshal(map[string]string{"raw": argRaw}); err == nil {
				argRaw = string(wrapped)
			} else {
				argRaw = "{}"
			}
		}
		out = append(out, ToolUseStart{
			ID:    acc.id,
			Name:  acc.name,
			Input: json.RawMessage(argRaw),
		})
	}
	a.openAITools = make(map[int]*openAIToolAccum)
	return out
}

func BuildToolResultMessage(callID, toolName, payload string, isError bool) Message {
	return Message{
		Role: RoleUser,
		Content: []ContentBlock{
			{
				Type:      ContentToolResult,
				ToolUseID: callID,
				Name:      toolName,
				Text:      payload,
				IsError:   isError,
			},
		},
	}
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type OpenAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type GeminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

func AnthropicToToolCall(id, name string, input json.RawMessage) ToolCall {
	return ToolCall{
		ID:        id,
		Name:      name,
		Arguments: cloneRawJSON(input),
	}
}

func ToolCallToAnthropicBlock(call ToolCall) map[string]any {
	var input any = map[string]any{}
	if len(call.Arguments) > 0 {
		_ = json.Unmarshal(call.Arguments, &input)
	}
	return map[string]any{
		"type":  "tool_use",
		"id":    call.ID,
		"name":  call.Name,
		"input": input,
	}
}

func OpenAIToToolCalls(calls []OpenAIToolCall) ([]ToolCall, error) {
	out := make([]ToolCall, 0, len(calls))
	for _, c := range calls {
		args := strings.TrimSpace(c.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		if !json.Valid([]byte(args)) {
			return nil, fmt.Errorf("invalid openai tool arguments for %s", c.Function.Name)
		}
		out = append(out, ToolCall{
			ID:        c.ID,
			Name:      c.Function.Name,
			Arguments: json.RawMessage(args),
		})
	}
	return out, nil
}

func ToolCallsToOpenAI(calls []ToolCall) ([]OpenAIToolCall, error) {
	out := make([]OpenAIToolCall, 0, len(calls))
	for _, c := range calls {
		arg := "{}"
		if len(c.Arguments) > 0 {
			if !json.Valid(c.Arguments) {
				return nil, fmt.Errorf("invalid tool arguments for %s", c.Name)
			}
			arg = string(c.Arguments)
		}
		var oc OpenAIToolCall
		oc.ID = c.ID
		oc.Type = "function"
		oc.Function.Name = c.Name
		oc.Function.Arguments = arg
		out = append(out, oc)
	}
	return out, nil
}

func GeminiToToolCall(call GeminiFunctionCall) (ToolCall, error) {
	args, err := json.Marshal(call.Args)
	if err != nil {
		return ToolCall{}, err
	}
	return ToolCall{
		Name:      call.Name,
		Arguments: args,
	}, nil
}

func ToolCallToGemini(call ToolCall) (GeminiFunctionCall, error) {
	var args map[string]any
	if len(call.Arguments) == 0 {
		args = map[string]any{}
	} else if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return GeminiFunctionCall{}, err
	}
	return GeminiFunctionCall{
		Name: call.Name,
		Args: args,
	}, nil
}

func NormalizeSchemaForGemini(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	out := make(map[string]any, len(schema))
	for k, v := range schema {
		if k == "additionalProperties" {
			continue
		}
		switch typed := v.(type) {
		case map[string]any:
			out[k] = NormalizeSchemaForGemini(typed)
		case []any:
			next := make([]any, 0, len(typed))
			for _, item := range typed {
				if m, ok := item.(map[string]any); ok {
					next = append(next, NormalizeSchemaForGemini(m))
				} else {
					next = append(next, item)
				}
			}
			out[k] = next
		default:
			out[k] = v
		}
	}
	return out
}

func cloneRawJSON(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}
