package connectortool

import (
	"testing"

	tool "github.com/koreaf16/argus/internal/tools"
)

func TestConnectorSuggestToolHiddenFromToolSpecs(t *testing.T) {
	registry := tool.NewRegistry()
	if err := registry.Register(New(nil)); err != nil {
		t.Fatalf("register connector_suggest: %v", err)
	}

	if _, ok := registry.Lookup("connector_suggest"); !ok {
		t.Fatalf("connector_suggest should remain callable by explicit lookup")
	}

	for _, spec := range registry.ToolSpecs(tool.Context{}) {
		if spec.Name == "connector_suggest" {
			t.Fatalf("connector_suggest should not be exposed to LLM tool specs")
		}
	}
}
