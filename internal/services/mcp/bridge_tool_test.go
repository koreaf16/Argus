package mcp

import "testing"

func TestBridgeToolUsesDiscoveredInputSchema(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}
	bridge := NewBridgeTool("alpha", "search", "Search", schema, nil)
	got := bridge.InputSchema()
	if got["type"] != "object" {
		t.Fatalf("unexpected schema: %#v", got)
	}
	if _, ok := got["properties"].(map[string]any); !ok {
		t.Fatalf("expected properties to be preserved: %#v", got)
	}
}
