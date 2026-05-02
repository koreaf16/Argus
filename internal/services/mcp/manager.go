package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/koreaf16/argus/internal/constants"
)

type ServerConfig struct {
	Name      string            `json:"name"`
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Tools     []ToolConfig      `json:"tools,omitempty"`
	Resources []ResourceConfig  `json:"resources,omitempty"`
}

type ToolConfig struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type ResourceConfig struct {
	URI     string `json:"uri"`
	Name    string `json:"name,omitempty"`
	Mime    string `json:"mime_type,omitempty"`
	Content string `json:"content,omitempty"`
}

type Config struct {
	Servers []ServerConfig `json:"servers"`
}

type Manager struct {
	path    string
	servers map[string]ServerConfig
}

func NewManager(configPath string) *Manager {
	if strings.TrimSpace(configPath) == "" {
		configPath = filepath.Join(constants.ConfigDir(), "mcp.json")
	}
	return &Manager{
		path:    configPath,
		servers: make(map[string]ServerConfig),
	}
}

func (m *Manager) Load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", m.path, err)
	}
	m.servers = make(map[string]ServerConfig)
	for _, srv := range cfg.Servers {
		if strings.TrimSpace(srv.Name) == "" {
			continue
		}
		m.servers[srv.Name] = srv
	}
	return nil
}

func (m *Manager) Path() string {
	return m.path
}

func (m *Manager) ServerNames() []string {
	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *Manager) ListTools(server string) []string {
	srv, ok := m.servers[server]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(srv.Tools))
	for _, t := range srv.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		out = append(out, "mcp__"+server+"__"+name)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) ListResources(server string) []string {
	srv, ok := m.servers[server]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(srv.Resources))
	for _, r := range srv.Resources {
		uri := strings.TrimSpace(r.URI)
		if uri == "" {
			continue
		}
		out = append(out, uri)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) ReadResource(server, uri string) (string, error) {
	srv, ok := m.servers[server]
	if !ok {
		return "", fmt.Errorf("unknown mcp server: %s", server)
	}
	for _, r := range srv.Resources {
		if r.URI == uri {
			return r.Content, nil
		}
	}
	return "", fmt.Errorf("resource not found: %s", uri)
}

func (m *Manager) HasServer(server string) bool {
	_, ok := m.servers[server]
	return ok
}

func (m *Manager) ToolConfigs(server string) []ToolConfig {
	srv, ok := m.servers[server]
	if !ok {
		return nil
	}
	out := make([]ToolConfig, len(srv.Tools))
	copy(out, srv.Tools)
	return out
}

func (m *Manager) Execute(ctx context.Context, server, toolName string, args map[string]any) (string, bool, error) {
	cfg, ok := m.servers[server]
	if !ok {
		return "", true, fmt.Errorf("unknown mcp server: %s", server)
	}

	client, err := newMCPClient(cfg)
	if err != nil {
		return "", true, fmt.Errorf("connect mcp server %s: %w", server, err)
	}
	defer client.Close()

	if err := client.Initialize(ctx); err != nil {
		return "", true, err
	}

	return client.CallTool(ctx, toolName, args)
}

func (m *Manager) AddServer(cfg ServerConfig) error {
	m.servers[cfg.Name] = cfg
	return m.Save()
}

func (m *Manager) RemoveServer(name string) error {
	if _, ok := m.servers[name]; !ok {
		return fmt.Errorf("server %q not found", name)
	}
	delete(m.servers, name)
	return m.Save()
}

func (m *Manager) Save() error {
	if m.path == "" {
		return fmt.Errorf("manager path is empty")
	}
	var cfg Config
	names := m.ServerNames()
	cfg.Servers = make([]ServerConfig, 0, len(names))
	for _, name := range names {
		cfg.Servers = append(cfg.Servers, m.servers[name])
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, b, 0644)
}

func (m *Manager) GetServer(name string) (ServerConfig, bool) {
	srv, ok := m.servers[name]
	return srv, ok
}

func (m *Manager) DiscoverTools(ctx context.Context, cfg ServerConfig) ([]ToolConfig, error) {
	client, err := newMCPClient(cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if err := client.Initialize(ctx); err != nil {
		return nil, err
	}

	raw, err := client.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}

	var tools []ToolConfig
	for _, t := range result.Tools {
		tools = append(tools, ToolConfig{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return tools, nil
}
