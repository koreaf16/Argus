package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type rpcClient interface {
	Call(method string, params any, out any) error
	Notify(method string, params any) error
	Close()
	DiagnosticsMap() map[string][]Diagnostic
}

type serverState struct {
	cmd    *exec.Cmd
	client rpcClient
	status string
}

type Manager struct {
	mu      sync.RWMutex
	servers map[string]*serverState
}

func NewManager() *Manager {
	return &Manager{servers: make(map[string]*serverState)}
}

func (m *Manager) Start(language, command string, args []string, workspaceDir string) error {
	lang := strings.TrimSpace(language)
	if lang == "" {
		return fmt.Errorf("language is required")
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("command is required")
	}

	m.mu.Lock()
	if st, ok := m.servers[lang]; ok && st.cmd != nil && st.cmd.Process != nil {
		m.mu.Unlock()
		return fmt.Errorf("lsp server already running for %s", lang)
	}
	m.mu.Unlock()

	cmd := exec.Command(command, args...)
	workspacePath := strings.TrimSpace(workspaceDir)
	if workspacePath != "" {
		absPath, err := filepath.Abs(workspacePath)
		if err != nil {
			return fmt.Errorf("resolve workspace path: %w", err)
		}
		workspacePath = absPath
		cmd.Dir = workspacePath
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}

	// killAndReap closes pipes, kills the process, and reaps the zombie.
	killAndReap := func(c rpcClient) {
		if c != nil {
			c.Close()
		}
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}

	rootURI := pathToFileURI(workspacePath)
	workspaceFolders := []map[string]any{}
	if rootURI != "" {
		name := filepath.Base(workspacePath)
		if name == "." || name == string(filepath.Separator) {
			name = "workspace"
		}
		workspaceFolders = append(workspaceFolders, map[string]any{
			"uri":  rootURI,
			"name": name,
		})
	}

	initParams := map[string]any{
		"processId": nil,
		"rootUri":   nil,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"publishDiagnostics": map[string]any{},
			},
		},
	}
	if rootURI != "" {
		initParams["rootUri"] = rootURI
		initParams["workspaceFolders"] = workspaceFolders
	}

	client := NewClient(stdin, stdout)
	if err := client.Call("initialize", initParams, nil); err != nil {
		killAndReap(client)
		return fmt.Errorf("initialize failed: %w", err)
	}
	if err := client.Notify("initialized", map[string]any{}); err != nil {
		killAndReap(client)
		return fmt.Errorf("initialized notify failed: %w", err)
	}

	m.mu.Lock()
	m.servers[lang] = &serverState{
		cmd:    cmd,
		client: client,
		status: "running",
	}
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		client.Close()
		m.mu.Lock()
		defer m.mu.Unlock()
		st, ok := m.servers[lang]
		if !ok || st.cmd != cmd {
			return
		}
		if err != nil {
			st.status = "exited_with_error"
			return
		}
		st.status = "exited"
	}()

	return nil
}

func (m *Manager) Stop(language string) error {
	lang := strings.TrimSpace(language)
	if lang == "" {
		return fmt.Errorf("language is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.servers[lang]
	if !ok || st.cmd == nil || st.cmd.Process == nil {
		return fmt.Errorf("lsp server is not running for %s", lang)
	}
	if st.client != nil {
		_ = st.client.Call("shutdown", map[string]any{}, nil)
		_ = st.client.Notify("exit", map[string]any{})
		st.client.Close()
	}
	if err := st.cmd.Process.Kill(); err != nil {
		return err
	}
	st.status = "stopped"
	return nil
}

func (m *Manager) Hover(language, uri string, line, character int) (string, error) {
	client, err := m.clientForLanguage(language)
	if err != nil {
		return "", err
	}

	var out map[string]any
	err = client.Call("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position": map[string]any{
			"line":      line,
			"character": character,
		},
	}, &out)
	if err != nil {
		return "", err
	}
	if out == nil {
		return "", nil
	}
	content := out["contents"]
	switch typed := content.(type) {
	case string:
		return typed, nil
	case map[string]any:
		if v, ok := typed["value"].(string); ok {
			return v, nil
		}
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
				continue
			}
			if m, ok := item.(map[string]any); ok {
				if v, ok := m["value"].(string); ok {
					parts = append(parts, v)
				}
			}
		}
		return strings.Join(parts, "\n"), nil
	}
	return "", nil
}

func (m *Manager) Definition(language, uri string, line, character int) ([]Location, error) {
	client, err := m.clientForLanguage(language)
	if err != nil {
		return nil, err
	}
	var out any
	if err := client.Call("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position": map[string]any{
			"line":      line,
			"character": character,
		},
	}, &out); err != nil {
		return nil, err
	}
	return parseLocations(out), nil
}

func (m *Manager) References(language, uri string, line, character int, includeDeclaration bool) ([]Location, error) {
	client, err := m.clientForLanguage(language)
	if err != nil {
		return nil, err
	}
	var out any
	if err := client.Call("textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position": map[string]any{
			"line":      line,
			"character": character,
		},
		"context": map[string]any{
			"includeDeclaration": includeDeclaration,
		},
	}, &out); err != nil {
		return nil, err
	}
	return parseLocations(out), nil
}

func (m *Manager) DocumentSymbols(language, uri string) ([]DocumentSymbol, error) {
	client, err := m.clientForLanguage(language)
	if err != nil {
		return nil, err
	}
	var out any
	if err := client.Call("textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	}, &out); err != nil {
		return nil, err
	}
	return parseDocumentSymbols(out), nil
}

func (m *Manager) Diagnostics(language string) map[string][]Diagnostic {
	m.mu.RLock()
	st, ok := m.servers[strings.TrimSpace(language)]
	m.mu.RUnlock()
	if !ok || st.client == nil {
		return nil
	}
	return st.client.DiagnosticsMap()
}

func (m *Manager) SetServerStatus(language, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.servers[language]
	if !ok {
		m.servers[language] = &serverState{status: status}
		return
	}
	st.status = status
}

func (m *Manager) Status() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.servers))
	for k, v := range m.servers {
		out[k] = v.status
	}
	return out
}

func (m *Manager) Languages() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.servers))
	for lang := range m.servers {
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) clientForLanguage(language string) (rpcClient, error) {
	m.mu.RLock()
	st, ok := m.servers[strings.TrimSpace(language)]
	m.mu.RUnlock()
	if !ok || st.client == nil {
		return nil, fmt.Errorf("lsp server not running for %s", language)
	}
	return st.client, nil
}

func parseLocations(v any) []Location {
	switch typed := v.(type) {
	case nil:
		return nil
	case map[string]any:
		loc, ok := parseLocation(typed)
		if !ok {
			return nil
		}
		return []Location{loc}
	case []any:
		out := make([]Location, 0, len(typed))
		for _, item := range typed {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			loc, ok := parseLocation(m)
			if ok {
				out = append(out, loc)
			}
		}
		return out
	default:
		return nil
	}
}

func parseLocation(m map[string]any) (Location, bool) {
	if loc, ok := parseStandardLocation(m); ok {
		return loc, true
	}
	return parseLocationLink(m)
}

func parseStandardLocation(m map[string]any) (Location, bool) {
	uri, _ := m["uri"].(string)
	rangeMap, _ := m["range"].(map[string]any)
	if uri == "" || rangeMap == nil {
		return Location{}, false
	}
	rng, ok := parseRange(rangeMap)
	if !ok {
		return Location{}, false
	}
	return Location{URI: uri, Range: rng}, true
}

func parseLocationLink(m map[string]any) (Location, bool) {
	uri, _ := m["targetUri"].(string)
	if uri == "" {
		return Location{}, false
	}
	var rangeMap map[string]any
	if v, ok := m["targetSelectionRange"].(map[string]any); ok {
		rangeMap = v
	} else if v, ok := m["targetRange"].(map[string]any); ok {
		rangeMap = v
	}
	if rangeMap == nil {
		return Location{}, false
	}
	rng, ok := parseRange(rangeMap)
	if !ok {
		return Location{}, false
	}
	return Location{URI: uri, Range: rng}, true
}

func parseRange(m map[string]any) (Range, bool) {
	startMap, ok := m["start"].(map[string]any)
	if !ok {
		return Range{}, false
	}
	endMap, ok := m["end"].(map[string]any)
	if !ok {
		return Range{}, false
	}
	start, ok := parsePosition(startMap)
	if !ok {
		return Range{}, false
	}
	end, ok := parsePosition(endMap)
	if !ok {
		return Range{}, false
	}
	return Range{Start: start, End: end}, true
}

func parsePosition(m map[string]any) (Position, bool) {
	line, ok := numberToInt(m["line"])
	if !ok {
		return Position{}, false
	}
	character, ok := numberToInt(m["character"])
	if !ok {
		return Position{}, false
	}
	return Position{Line: line, Character: character}, true
}

func parseDocumentSymbols(v any) []DocumentSymbol {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]DocumentSymbol, 0, len(items))
	for _, item := range items {
		symbol, ok := parseDocumentSymbol(item)
		if ok {
			out = append(out, symbol)
		}
	}
	return out
}

func parseDocumentSymbol(v any) (DocumentSymbol, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return DocumentSymbol{}, false
	}
	name, _ := m["name"].(string)
	kind, ok := numberToInt(m["kind"])
	if name == "" || !ok {
		return DocumentSymbol{}, false
	}
	detail, _ := m["detail"].(string)

	// SymbolInformation response shape.
	if locationMap, ok := m["location"].(map[string]any); ok {
		rangeMap, _ := locationMap["range"].(map[string]any)
		if rangeMap == nil {
			return DocumentSymbol{}, false
		}
		rng, ok := parseRange(rangeMap)
		if !ok {
			return DocumentSymbol{}, false
		}
		return DocumentSymbol{
			Name:           name,
			Detail:         detail,
			Kind:           kind,
			Range:          rng,
			SelectionRange: rng,
		}, true
	}

	// DocumentSymbol response shape.
	rangeMap, _ := m["range"].(map[string]any)
	selectionMap, _ := m["selectionRange"].(map[string]any)
	if rangeMap == nil || selectionMap == nil {
		return DocumentSymbol{}, false
	}
	rng, ok := parseRange(rangeMap)
	if !ok {
		return DocumentSymbol{}, false
	}
	selection, ok := parseRange(selectionMap)
	if !ok {
		return DocumentSymbol{}, false
	}
	children := make([]DocumentSymbol, 0)
	if childRaw, ok := m["children"].([]any); ok {
		for _, child := range childRaw {
			if parsed, ok := parseDocumentSymbol(child); ok {
				children = append(children, parsed)
			}
		}
	}
	return DocumentSymbol{
		Name:           name,
		Detail:         detail,
		Kind:           kind,
		Range:          rng,
		SelectionRange: selection,
		Children:       children,
	}, true
}

func numberToInt(v any) (int, bool) {
	switch typed := v.(type) {
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case int32:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint64:
		return int(typed), true
	case uint32:
		return int(typed), true
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

func pathToFileURI(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	slashPath := filepath.ToSlash(trimmed)
	if slashPath == "" {
		return ""
	}
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	return (&url.URL{Scheme: "file", Path: slashPath}).String()
}
