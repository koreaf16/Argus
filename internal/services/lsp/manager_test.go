package lsp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type fakeRPCClient struct {
	results    map[string]any
	lastMethod string
	lastParams any
}

func (f *fakeRPCClient) Call(method string, params any, out any) error {
	f.lastMethod = method
	f.lastParams = params
	value, ok := f.results[method]
	if !ok || out == nil {
		return nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func (f *fakeRPCClient) Notify(string, any) error { return nil }
func (f *fakeRPCClient) Close()                   {}
func (f *fakeRPCClient) DiagnosticsMap() map[string][]Diagnostic {
	return nil
}

func TestManagerDefinitionParsesLocationAndLocationLink(t *testing.T) {
	m := NewManager()
	client := &fakeRPCClient{
		results: map[string]any{
			"textDocument/definition": []any{
				map[string]any{
					"uri": "file:///a.go",
					"range": map[string]any{
						"start": map[string]any{"line": 1, "character": 2},
						"end":   map[string]any{"line": 1, "character": 5},
					},
				},
				map[string]any{
					"targetUri": "file:///b.go",
					"targetSelectionRange": map[string]any{
						"start": map[string]any{"line": 3, "character": 4},
						"end":   map[string]any{"line": 3, "character": 7},
					},
				},
			},
		},
	}
	m.servers["go"] = &serverState{client: client, status: "running"}

	got, err := m.Definition("go", "file:///main.go", 0, 0)
	if err != nil {
		t.Fatalf("definition failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 locations, got %d", len(got))
	}
	if got[0].URI != "file:///a.go" {
		t.Fatalf("unexpected first location uri: %s", got[0].URI)
	}
	if got[1].URI != "file:///b.go" {
		t.Fatalf("unexpected second location uri: %s", got[1].URI)
	}
}

func TestManagerReferencesPassesIncludeDeclaration(t *testing.T) {
	m := NewManager()
	client := &fakeRPCClient{
		results: map[string]any{
			"textDocument/references": []any{},
		},
	}
	m.servers["go"] = &serverState{client: client, status: "running"}

	_, err := m.References("go", "file:///main.go", 10, 11, true)
	if err != nil {
		t.Fatalf("references failed: %v", err)
	}
	if client.lastMethod != "textDocument/references" {
		t.Fatalf("unexpected method: %s", client.lastMethod)
	}
	params, ok := client.lastParams.(map[string]any)
	if !ok {
		t.Fatalf("unexpected params type: %T", client.lastParams)
	}
	ctx, ok := params["context"].(map[string]any)
	if !ok {
		t.Fatalf("missing context in params: %v", params)
	}
	include, ok := ctx["includeDeclaration"].(bool)
	if !ok || !include {
		t.Fatalf("expected includeDeclaration=true, got %v", ctx["includeDeclaration"])
	}
}

func TestManagerDocumentSymbolsParsesBothShapes(t *testing.T) {
	m := NewManager()
	client := &fakeRPCClient{
		results: map[string]any{
			"textDocument/documentSymbol": []any{
				map[string]any{
					"name": "FromSymbolInfo",
					"kind": 12,
					"location": map[string]any{
						"uri": "file:///a.go",
						"range": map[string]any{
							"start": map[string]any{"line": 1, "character": 1},
							"end":   map[string]any{"line": 1, "character": 2},
						},
					},
				},
				map[string]any{
					"name": "Parent",
					"kind": 5,
					"range": map[string]any{
						"start": map[string]any{"line": 2, "character": 1},
						"end":   map[string]any{"line": 6, "character": 1},
					},
					"selectionRange": map[string]any{
						"start": map[string]any{"line": 2, "character": 3},
						"end":   map[string]any{"line": 2, "character": 8},
					},
					"children": []any{
						map[string]any{
							"name": "Child",
							"kind": 6,
							"range": map[string]any{
								"start": map[string]any{"line": 3, "character": 1},
								"end":   map[string]any{"line": 3, "character": 7},
							},
							"selectionRange": map[string]any{
								"start": map[string]any{"line": 3, "character": 1},
								"end":   map[string]any{"line": 3, "character": 7},
							},
						},
					},
				},
			},
		},
	}
	m.servers["go"] = &serverState{client: client, status: "running"}

	got, err := m.DocumentSymbols("go", "file:///main.go")
	if err != nil {
		t.Fatalf("document symbols failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(got))
	}
	if got[0].Name != "FromSymbolInfo" {
		t.Fatalf("unexpected first symbol: %+v", got[0])
	}
	if got[1].Name != "Parent" || len(got[1].Children) != 1 || got[1].Children[0].Name != "Child" {
		t.Fatalf("unexpected nested symbols: %+v", got[1])
	}
}

func TestManagerDefinitionRequiresRunningServer(t *testing.T) {
	m := NewManager()
	_, err := m.Definition("go", "file:///main.go", 0, 0)
	if err == nil {
		t.Fatal("expected error for missing server")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNumberToIntRejectsUnsupportedType(t *testing.T) {
	_, ok := numberToInt(fmt.Sprintf("%d", 1))
	if ok {
		t.Fatal("expected conversion to fail")
	}
}

func TestPathToFileURI(t *testing.T) {
	if got := pathToFileURI(""); got != "" {
		t.Fatalf("expected empty uri for empty input, got %q", got)
	}

	uri := pathToFileURI(t.TempDir())
	if !strings.HasPrefix(uri, "file://") {
		t.Fatalf("expected file uri, got %q", uri)
	}
}
