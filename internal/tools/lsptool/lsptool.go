package lsptool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/koreaf16/argus/internal/services/lsp"
	tool "github.com/koreaf16/argus/internal/tools"
)

var manager managerAPI = lsp.NewManager()

type managerAPI interface {
	Start(language, command string, args []string, workspaceDir string) error
	Stop(language string) error
	Hover(language, uri string, line, character int) (string, error)
	Diagnostics(language string) map[string][]lsp.Diagnostic
	Status() map[string]string
	Definition(language, uri string, line, character int) ([]lsp.Location, error)
	References(language, uri string, line, character int, includeDeclaration bool) ([]lsp.Location, error)
	DocumentSymbols(language, uri string) ([]lsp.DocumentSymbol, error)
}

func SetManager(m managerAPI) {
	if m != nil {
		manager = m
	}
}

type LSPTool struct{}

func NewLSPTool() *LSPTool { return &LSPTool{} }

func (t *LSPTool) Name() string { return "lsp" }

func (t *LSPTool) Description(ctx tool.Context) string {
	return "Manage local LSP servers (status/start/stop)"
}

func (t *LSPTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"action":   map[string]any{"type": "string"},
			"language": map[string]any{"type": "string"},
			"command":  map[string]any{"type": "string"},
			"args":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"workdir":  map[string]any{"type": "string"},
			"uri":      map[string]any{"type": "string"},
			"line":     map[string]any{"type": "integer"},
			"character": map[string]any{
				"type": "integer",
			},
			"include_declaration": map[string]any{"type": "boolean"},
		},
		"required": []string{"action"},
	}
}

func (t *LSPTool) IsReadOnly() bool { return false }

func (t *LSPTool) MaxResultSizeChars() int {
	return 100000
}

func (t *LSPTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAskPermission(), nil
}

func (t *LSPTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	var req struct {
		Action             string   `json:"action"`
		Language           string   `json:"language"`
		Command            string   `json:"command"`
		Args               []string `json:"args"`
		WorkDir            string   `json:"workdir"`
		URI                string   `json:"uri"`
		Line               int      `json:"line"`
		Char               int      `json:"character"`
		IncludeDeclaration bool     `json:"include_declaration"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	ch := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(ch)
		action := strings.ToLower(strings.TrimSpace(req.Action))
		switch action {
		case "status":
			status := manager.Status()
			keys := make([]string, 0, len(status))
			for k := range status {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			lines := make([]string, 0, len(keys))
			for _, k := range keys {
				lines = append(lines, fmt.Sprintf("%s=%s", k, status[k]))
			}
			if len(lines) == 0 {
				ch <- tool.NewOutputEvent("lsp servers: none")
			} else {
				ch <- tool.NewOutputEvent("lsp servers: " + strings.Join(lines, ", "))
			}
		case "start":
			workspaceDir := strings.TrimSpace(req.WorkDir)
			if workspaceDir == "" {
				workspaceDir = strings.TrimSpace(ctx.WorkingDir)
			}
			if err := manager.Start(req.Language, req.Command, req.Args, workspaceDir); err != nil {
				ch <- tool.NewErrorEvent(err)
				return
			}
			ch <- tool.NewOutputEvent(fmt.Sprintf("lsp server started for %s", req.Language))
		case "stop":
			if err := manager.Stop(req.Language); err != nil {
				ch <- tool.NewErrorEvent(err)
				return
			}
			ch <- tool.NewOutputEvent(fmt.Sprintf("lsp server stopped for %s", req.Language))
		case "hover":
			text, err := manager.Hover(req.Language, req.URI, req.Line, req.Char)
			if err != nil {
				ch <- tool.NewErrorEvent(err)
				return
			}
			ch <- tool.NewOutputEvent(text)
		case "definition":
			locs, err := manager.Definition(req.Language, req.URI, req.Line, req.Char)
			if err != nil {
				ch <- tool.NewErrorEvent(err)
				return
			}
			ch <- tool.NewOutputEvent(formatLocations(locs))
		case "references":
			locs, err := manager.References(req.Language, req.URI, req.Line, req.Char, req.IncludeDeclaration)
			if err != nil {
				ch <- tool.NewErrorEvent(err)
				return
			}
			ch <- tool.NewOutputEvent(formatLocations(locs))
		case "symbols":
			symbols, err := manager.DocumentSymbols(req.Language, req.URI)
			if err != nil {
				ch <- tool.NewErrorEvent(err)
				return
			}
			ch <- tool.NewOutputEvent(formatSymbols(symbols))
		case "diagnostics":
			diag := manager.Diagnostics(req.Language)
			lines := make([]string, 0, len(diag))
			for uri, items := range diag {
				lines = append(lines, fmt.Sprintf("%s: %d", uri, len(items)))
			}
			sort.Strings(lines)
			if len(lines) == 0 {
				ch <- tool.NewOutputEvent("no diagnostics")
			} else {
				ch <- tool.NewOutputEvent(strings.Join(lines, ", "))
			}
		default:
			ch <- tool.NewErrorEvent(fmt.Errorf("unsupported lsp action: %s", req.Action))
			return
		}
		ch <- tool.NewDoneEvent()
	}()
	return ch, nil
}

func formatLocations(locs []lsp.Location) string {
	if len(locs) == 0 {
		return "no locations"
	}
	lines := make([]string, 0, len(locs))
	for _, loc := range locs {
		lines = append(lines, fmt.Sprintf("%s:%d:%d", loc.URI, loc.Range.Start.Line+1, loc.Range.Start.Character+1))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func formatSymbols(symbols []lsp.DocumentSymbol) string {
	if len(symbols) == 0 {
		return "no symbols"
	}
	lines := make([]string, 0, len(symbols))
	lines = flattenSymbols(lines, symbols, "")
	return strings.Join(lines, "\n")
}

func flattenSymbols(dst []string, symbols []lsp.DocumentSymbol, indent string) []string {
	for _, sym := range symbols {
		dst = append(dst, fmt.Sprintf(
			"%s%s (%d) %d:%d",
			indent,
			sym.Name,
			sym.Kind,
			sym.SelectionRange.Start.Line+1,
			sym.SelectionRange.Start.Character+1,
		))
		if len(sym.Children) > 0 {
			dst = flattenSymbols(dst, sym.Children, indent+"  ")
		}
	}
	return dst
}
