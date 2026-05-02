package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerLoadListAndRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	content := `{
  "servers": [
    {
      "name": "alpha",
      "command": "node",
      "tools": [{"name": "search", "inputSchema": {"type": "object", "properties": {"query": {"type": "string"}}}}],
      "resources": [{"uri": "mem://a", "content": "hello"}]
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	m := NewManager(path)
	if err := m.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	names := m.ServerNames()
	if len(names) != 1 || names[0] != "alpha" {
		t.Fatalf("unexpected names: %v", names)
	}
	tools := m.ListTools("alpha")
	if len(tools) != 1 || tools[0] != "mcp__alpha__search" {
		t.Fatalf("unexpected tools: %v", tools)
	}
	configs := m.ToolConfigs("alpha")
	if len(configs) != 1 || configs[0].InputSchema["type"] != "object" {
		t.Fatalf("unexpected tool configs: %+v", configs)
	}
	resources := m.ListResources("alpha")
	if len(resources) != 1 || resources[0] != "mem://a" {
		t.Fatalf("unexpected resources: %v", resources)
	}
	got, err := m.ReadResource("alpha", "mem://a")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != "hello" {
		t.Fatalf("unexpected resource content: %q", got)
	}
}
