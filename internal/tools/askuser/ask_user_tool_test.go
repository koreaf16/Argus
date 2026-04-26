package askuser

import (
	"context"
	"encoding/json"
	"testing"

	tools "github.com/koreaf16/argus/internal/tools"
)

func TestAskUserToolSingleQuestion(t *testing.T) {
	input := json.RawMessage(`{
		"questions": [
			{
				"id": "project",
				"question": "Project type?",
				"type": "choice",
				"options": [
					{"value":"web","label":"Web"},
					{"value":"cli","label":"CLI"}
				]
			}
		]
	}`)

	ch, err := NewAskUserTool().Call(tools.NewContext(context.Background()), input)
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	first := <-ch
	if first.Kind != tools.ToolEventAskUserBatchPrompt {
		t.Fatalf("expected ask user batch prompt event, got %s", first.Kind)
	}
	if first.AskUserBatchPrompt == nil || len(first.AskUserBatchPrompt.Questions) != 1 {
		t.Fatalf("unexpected question payload: %#v", first.AskUserBatchPrompt)
	}
	if first.AskUserBatchPrompt.Questions[0].Question != "Project type?" {
		t.Fatalf("unexpected question payload: %#v", first.AskUserBatchPrompt.Questions[0])
	}
	if first.AskUserBatchResponse == nil {
		t.Fatalf("expected response channel")
	}
	first.AskUserBatchResponse <- tools.AskUserBatchResponse{
		AnswersByIndex: map[string]string{"0": "web"},
		AnswersByID:    map[string]string{"project": "web"},
	}

	second := <-ch
	if second.Kind != tools.ToolEventOutput {
		t.Fatalf("expected output event, got %s", second.Kind)
	}
	var payload struct {
		Canceled bool              `json:"canceled"`
		Answers  map[string]string `json:"answers"`
		ByID     map[string]string `json:"answers_by_id"`
	}
	if err := json.Unmarshal([]byte(second.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.Canceled {
		t.Fatalf("expected non-canceled result")
	}
	if payload.Answers["0"] != "web" {
		t.Fatalf("unexpected index answer: %#v", payload.Answers)
	}
	if payload.ByID["project"] != "web" {
		t.Fatalf("unexpected id answer: %#v", payload.ByID)
	}

	third := <-ch
	if third.Kind != tools.ToolEventDone {
		t.Fatalf("expected done event, got %s", third.Kind)
	}
}

func TestAskUserToolCanceled(t *testing.T) {
	input := json.RawMessage(`{"question":"Continue?","type":"yesno"}`)
	ch, err := NewAskUserTool().Call(tools.NewContext(context.Background()), input)
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	prompt := <-ch
	if prompt.Kind != tools.ToolEventAskUserBatchPrompt {
		t.Fatalf("expected ask user batch prompt event, got %s", prompt.Kind)
	}
	prompt.AskUserBatchResponse <- tools.AskUserBatchResponse{Canceled: true}

	out := <-ch
	if out.Kind != tools.ToolEventOutput {
		t.Fatalf("expected output event, got %s", out.Kind)
	}
	var payload struct {
		Canceled bool `json:"canceled"`
	}
	if err := json.Unmarshal([]byte(out.Output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !payload.Canceled {
		t.Fatalf("expected canceled output")
	}
}
