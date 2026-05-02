package toolsearch

import (
	"context"
	"encoding/json"
	"testing"

	tool "github.com/koreaf16/argus/internal/tools"
)

type testTool struct {
	name     string
	desc     string
	readOnly bool
}

func (t testTool) Name() string { return t.name }
func (t testTool) Description(tool.Context) string {
	return t.desc
}
func (t testTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{"type": "object"}
}
func (t testTool) IsReadOnly() bool { return t.readOnly }
func (t testTool) Call(tool.Context, json.RawMessage) (<-chan tool.ToolEvent, error) {
	ch := make(chan tool.ToolEvent, 1)
	ch <- tool.NewDoneEvent()
	close(ch)
	return ch, nil
}
func (t testTool) CheckPermission(tool.Context, json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}
func (t testTool) MaxResultSizeChars() int { return 0 }

func TestToolSearchFindsByQueryAndCategory(t *testing.T) {
	reg := tool.NewRegistry()
	if err := reg.Register(testTool{name: "web_search", desc: "Search the web", readOnly: true}); err != nil {
		t.Fatalf("register web_search: %v", err)
	}
	if err := reg.Register(testTool{name: "filewrite", desc: "Write files", readOnly: false}); err != nil {
		t.Fatalf("register filewrite: %v", err)
	}

	searchTool := New()
	input, err := json.Marshal(map[string]any{
		"query":               "current web docs",
		"evidence_categories": []string{tool.EvidenceExternalFresh},
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	events, err := searchTool.Call(tool.Context{Context: context.Background(), Registry: reg}, input)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var output string
	for evt := range events {
		if evt.Kind == tool.ToolEventOutput {
			output = evt.Output
		}
	}
	var payload response
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("unmarshal output: %v; raw=%q", err, output)
	}
	if len(payload.Candidates) != 1 || payload.Candidates[0].Name != "web_search" {
		t.Fatalf("unexpected candidates: %+v", payload.Candidates)
	}
}
