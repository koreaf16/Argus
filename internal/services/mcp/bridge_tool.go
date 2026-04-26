package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	tool "github.com/koreaf16/argus/internal/tools"
)

type BridgeTool struct {
	server  string
	name    string
	desc    string
	manager *Manager
}

func NewBridgeTool(server, name, description string, manager *Manager) *BridgeTool {
	return &BridgeTool{
		server:  server,
		name:    name,
		desc:    description,
		manager: manager,
	}
}

func (t *BridgeTool) Name() string {
	return "mcp__" + t.server + "__" + t.name
}

func (t *BridgeTool) Description(ctx tool.Context) string {
	if t.desc != "" {
		return t.desc
	}
	return "MCP bridged tool from server " + t.server
}

func (t *BridgeTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
	}
}

func (t *BridgeTool) IsReadOnly() bool { return false }

func (t *BridgeTool) MaxResultSizeChars() int {
	return 100000
}

func (t *BridgeTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAskPermission(), nil
}

func (t *BridgeTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	ch := make(chan tool.ToolEvent, 4)

	go func() {
		defer close(ch)

		if t.manager == nil {
			ch <- tool.NewErrorEvent(fmt.Errorf("mcp manager not initialized for server %q", t.server))
			return
		}

		var args map[string]any
		if len(input) > 0 {
			_ = json.Unmarshal(input, &args)
		}

		callCtx := context.Background()
		if ctx.Context != nil {
			callCtx = ctx.Context
		}

		output, isError, err := t.manager.Execute(callCtx, t.server, t.name, args)
		if err != nil {
			ch <- tool.NewErrorEvent(err)
			return
		}
		if isError {
			ch <- tool.NewErrorEvent(fmt.Errorf("%s", output))
			return
		}
		ch <- tool.NewOutputEvent(output)
		ch <- tool.NewDoneEvent()
	}()

	return ch, nil
}
