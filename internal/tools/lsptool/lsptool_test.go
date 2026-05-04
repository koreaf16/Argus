package lsptool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/lsp"
	tools "github.com/koreaf16/argus/internal/tools"
)

type fakeManager struct {
	status               map[string]string
	diagnostics          map[string][]lsp.Diagnostic
	definition           []lsp.Location
	references           []lsp.Location
	symbols              []lsp.DocumentSymbol
	referenceIncludeDecl bool
	referencesCalled     bool
	startCalled          bool
	startWorkspaceDir    string
}

func (f *fakeManager) Start(language, command string, args []string, workspaceDir string) error {
	f.startCalled = true
	f.startWorkspaceDir = workspaceDir
	return nil
}
func (f *fakeManager) Stop(language string) error { return nil }
func (f *fakeManager) Hover(language, uri string, line, character int) (string, error) {
	return "hover", nil
}
func (f *fakeManager) Diagnostics(language string) map[string][]lsp.Diagnostic { return f.diagnostics }
func (f *fakeManager) Status() map[string]string                               { return f.status }
func (f *fakeManager) Definition(language, uri string, line, character int) ([]lsp.Location, error) {
	return f.definition, nil
}
func (f *fakeManager) References(language, uri string, line, character int, includeDeclaration bool) ([]lsp.Location, error) {
	f.referencesCalled = true
	f.referenceIncludeDecl = includeDeclaration
	return f.references, nil
}
func (f *fakeManager) DocumentSymbols(language, uri string) ([]lsp.DocumentSymbol, error) {
	return f.symbols, nil
}

func TestLSPToolStatusActionSortsLanguages(t *testing.T) {
	fm := &fakeManager{
		status: map[string]string{
			"z": "running",
			"a": "stopped",
		},
	}
	SetManager(fm)
	defer SetManager(lsp.NewManager())

	events := callTool(t, map[string]any{"action": "status"})
	output := firstOutput(t, events)
	if output != "LSP 서버: a=stopped, z=running" {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestLSPToolDefinitionFormatsLocations(t *testing.T) {
	fm := &fakeManager{
		definition: []lsp.Location{
			{
				URI: "file:///b.go",
				Range: lsp.Range{
					Start: lsp.Position{Line: 7, Character: 1},
				},
			},
			{
				URI: "file:///a.go",
				Range: lsp.Range{
					Start: lsp.Position{Line: 0, Character: 0},
				},
			},
		},
	}
	SetManager(fm)
	defer SetManager(lsp.NewManager())

	events := callTool(t, map[string]any{
		"action":    "definition",
		"language":  "go",
		"uri":       "file:///main.go",
		"line":      0,
		"character": 0,
	})
	output := firstOutput(t, events)
	expected := strings.Join([]string{
		"file:///a.go:1:1",
		"file:///b.go:8:2",
	}, "\n")
	if output != expected {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestLSPToolReferencesUsesIncludeDeclaration(t *testing.T) {
	fm := &fakeManager{
		references: []lsp.Location{},
	}
	SetManager(fm)
	defer SetManager(lsp.NewManager())

	events := callTool(t, map[string]any{
		"action":              "references",
		"language":            "go",
		"uri":                 "file:///main.go",
		"line":                1,
		"character":           2,
		"include_declaration": true,
	})
	output := firstOutput(t, events)
	if output != "위치 정보 없음" {
		t.Fatalf("unexpected output: %s", output)
	}
	if !fm.referencesCalled {
		t.Fatal("expected references call")
	}
	if !fm.referenceIncludeDecl {
		t.Fatal("expected include_declaration=true")
	}
}

func TestLSPToolSymbolsFormatsHierarchy(t *testing.T) {
	fm := &fakeManager{
		symbols: []lsp.DocumentSymbol{
			{
				Name: "Parent",
				Kind: 5,
				SelectionRange: lsp.Range{
					Start: lsp.Position{Line: 0, Character: 2},
				},
				Children: []lsp.DocumentSymbol{
					{
						Name: "Child",
						Kind: 6,
						SelectionRange: lsp.Range{
							Start: lsp.Position{Line: 1, Character: 1},
						},
					},
				},
			},
		},
	}
	SetManager(fm)
	defer SetManager(lsp.NewManager())

	events := callTool(t, map[string]any{
		"action":   "symbols",
		"language": "go",
		"uri":      "file:///main.go",
	})
	output := firstOutput(t, events)
	expected := strings.Join([]string{
		"Parent (5) 1:3",
		"  Child (6) 2:2",
	}, "\n")
	if output != expected {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestLSPToolUnsupportedActionReturnsError(t *testing.T) {
	SetManager(&fakeManager{})
	defer SetManager(lsp.NewManager())

	events := callTool(t, map[string]any{"action": "nope"})
	errMsg := firstError(t, events)
	if !strings.Contains(errMsg, "지원되지 않는 LSP 작업입니다") {
		t.Fatalf("unexpected error: %s", errMsg)
	}
}

func TestLSPToolStartUsesContextWorkingDir(t *testing.T) {
	fm := &fakeManager{}
	SetManager(fm)
	defer SetManager(lsp.NewManager())

	events := callToolWithContext(t, map[string]any{
		"action":   "start",
		"language": "go",
		"command":  "gopls",
	}, tools.Context{
		Context:    context.Background(),
		WorkingDir: "/tmp/workspace",
	})
	_ = firstOutput(t, events)
	if !fm.startCalled {
		t.Fatal("expected start to be called")
	}
	if fm.startWorkspaceDir != "/tmp/workspace" {
		t.Fatalf("expected workspace dir from context, got %q", fm.startWorkspaceDir)
	}
}

func TestLSPToolStartPrefersExplicitWorkdir(t *testing.T) {
	fm := &fakeManager{}
	SetManager(fm)
	defer SetManager(lsp.NewManager())

	events := callToolWithContext(t, map[string]any{
		"action":   "start",
		"language": "go",
		"command":  "gopls",
		"workdir":  "/explicit/workspace",
	}, tools.Context{
		Context:    context.Background(),
		WorkingDir: "/tmp/workspace",
	})
	_ = firstOutput(t, events)
	if !fm.startCalled {
		t.Fatal("expected start to be called")
	}
	if fm.startWorkspaceDir != "/explicit/workspace" {
		t.Fatalf("expected explicit workspace dir, got %q", fm.startWorkspaceDir)
	}
}

func callTool(t *testing.T, input map[string]any) []tools.ToolEvent {
	t.Helper()
	return callToolWithContext(t, input, tools.NewContext(context.Background()))
}

func callToolWithContext(t *testing.T, input map[string]any, callCtx tools.Context) []tools.ToolEvent {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	ch, err := NewLSPTool().Call(callCtx, raw)
	if err != nil {
		t.Fatalf("call lsp tool: %v", err)
	}
	events := make([]tools.ToolEvent, 0, 4)
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func firstOutput(t *testing.T, events []tools.ToolEvent) string {
	t.Helper()
	for _, ev := range events {
		if ev.Kind == tools.ToolEventOutput {
			return ev.Output
		}
	}
	t.Fatalf("no output event in: %+v", events)
	return ""
}

func firstError(t *testing.T, events []tools.ToolEvent) string {
	t.Helper()
	for _, ev := range events {
		if ev.Kind == tools.ToolEventError && ev.Err != nil {
			return ev.Err.Error()
		}
	}
	t.Fatalf("no error event in: %+v", events)
	return ""
}
