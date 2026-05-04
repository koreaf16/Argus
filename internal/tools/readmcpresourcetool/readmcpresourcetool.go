package readmcpresourcetool

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/services/mcp"
	tool "github.com/koreaf16/argus/internal/tools"
)

type ReadMcpResourceTool struct{}

func NewReadMcpResourceTool() *ReadMcpResourceTool { return &ReadMcpResourceTool{} }

func (t *ReadMcpResourceTool) Name() string { return "read_mcp_resource" }

func (t *ReadMcpResourceTool) Description(ctx tool.Context) string {
	return "MCP 서버에서 리소스를 읽습니다."
}

func (t *ReadMcpResourceTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{"type": "string", "description": "MCP 서버 이름"},
			"uri":    map[string]any{"type": "string", "description": "리소스 URI"},
		},
		"required": []string{"server", "uri"},
	}
}

func (t *ReadMcpResourceTool) IsReadOnly() bool { return true }

func (t *ReadMcpResourceTool) MaxResultSizeChars() int {
	return 100000
}

func (t *ReadMcpResourceTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}

func (t *ReadMcpResourceTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	var req struct {
		Server string `json:"server"`
		URI    string `json:"uri"`
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
		content, err := manager.ReadResource(req.Server, req.URI)
		if err != nil {
			ch <- tool.NewErrorEvent(err)
			return
		}
		ch <- tool.NewOutputEvent(fmt.Sprintf("resource %s from %s:\n%s", req.URI, req.Server, content))
		ch <- tool.NewDoneEvent()
	}()
	return ch, nil
}
