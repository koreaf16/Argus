package query

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type scriptedLLM struct {
	mu        sync.Mutex
	sequences [][]llm.Event
	index     int
}

func (s *scriptedLLM) Stream(context.Context, llm.Request) (<-chan llm.Event, error) {
	s.mu.Lock()
	idx := s.index
	s.index++
	s.mu.Unlock()

	events := []llm.Event{}
	if idx < len(s.sequences) {
		events = s.sequences[idx]
	} else {
		stop := llm.StopReasonEndTurn
		events = []llm.Event{{Kind: llm.EventStop, Stop: &stop}}
	}

	ch := make(chan llm.Event, len(events))
	for _, evt := range events {
		ch <- evt
	}
	close(ch)
	return ch, nil
}

func (s *scriptedLLM) CountTokens(context.Context, llm.Request) (int, error) {
	return 30, nil
}

func (s *scriptedLLM) Capabilities() llm.Caps {
	return llm.Caps{Tools: true}
}

func (s *scriptedLLM) Provider() string {
	return "scripted"
}

type scriptedTool struct {
	name       string
	output     string
	outputFn   func(json.RawMessage) string
	isError    bool
	permission types.PermissionBehavior
	writable   bool
}

func (t *scriptedTool) Name() string { return t.name }
func (t *scriptedTool) Description(tool.Context) string {
	return "scripted tool"
}
func (t *scriptedTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{"type": "object"}
}
func (t *scriptedTool) IsReadOnly() bool {
	return !t.writable
}
func (t *scriptedTool) MaxResultSizeChars() int {
	return 0
}
func (t *scriptedTool) CheckPermission(tool.Context, json.RawMessage) (tool.PermissionResult, error) {
	return tool.PermissionResult{Behavior: t.permission}, nil
}
func (t *scriptedTool) Call(_ tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	output := t.output
	if t.outputFn != nil {
		output = t.outputFn(input)
	}
	ch := make(chan tool.ToolEvent, 2)
	if t.isError {
		ch <- tool.NewErrorEvent(errors.New(output))
	} else {
		ch <- tool.NewOutputEvent(output)
	}
	ch <- tool.NewDoneEvent()
	close(ch)
	return ch, nil
}

func mustRawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return b
}
