package mcptool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/services/mcp"
	tool "github.com/koreaf16/argus/internal/tools"
)

type MCPTool struct{}

func NewMCPTool() *MCPTool { return &MCPTool{} }

func (t *MCPTool) Name() string { return "mcp" }

func (t *MCPTool) Description(ctx tool.Context) string { return "Call MCP bridge commands" }

func (t *MCPTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string"},
			"server": map[string]any{"type": "string"},
		},
	}
}

func (t *MCPTool) IsReadOnly() bool { return true }

func (t *MCPTool) MaxResultSizeChars() int {
	return 100000
}

func (t *MCPTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}

func (t *MCPTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	var req struct {
		Action string `json:"action"`
		Server string `json:"server"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	ch := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(ch)
		manager := mcp.NewManager(filepath.Join(constants.ConfigDir(), "mcp.json"))
		if err := manager.Load(); err != nil {
			ch <- tool.NewErrorEvent(err)
			return
		}
		action := strings.ToLower(strings.TrimSpace(req.Action))
		switch action {
		case "", "list":
			ch <- tool.NewOutputEvent(fmt.Sprintf("mcp servers: %v", manager.ServerNames()))
		case "tools":
			ch <- tool.NewOutputEvent(fmt.Sprintf("mcp tools for %s: %v", req.Server, manager.ListTools(req.Server)))
		case "resources":
			ch <- tool.NewOutputEvent(fmt.Sprintf("mcp resources for %s: %v", req.Server, manager.ListResources(req.Server)))
		default:
			ch <- tool.NewErrorEvent(fmt.Errorf("unsupported mcp action: %s", req.Action))
			return
		}
		ch <- tool.NewDoneEvent()
	}()
	return ch, nil
}
