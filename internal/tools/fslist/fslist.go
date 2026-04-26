package fslist

import (
	"encoding/json"
	"fmt"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tui/toolui"
	"github.com/koreaf16/argus/internal/types"
)

type FSListTool struct{}

func init() {
	toolui.Register("fs_list", &FSListRenderer{})
}

type FSListRenderer struct{}

func (r *FSListRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *FSListRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	path, _ := args["path"].(string)
	server, _ := args["server"].(string)
	recursive, _ := args["recursive"].(bool)

	var sb strings.Builder
	sb.WriteString(theme.Style(theme.ToolUseColor()).Bold(true).Render("  List: "))
	sb.WriteString(theme.Style(theme.BodyColor()).Render(path))
	if server != "" {
		sb.WriteString(theme.Style(theme.MutedColor()).Render(" on " + server))
	}
	if recursive {
		sb.WriteString(theme.Style(theme.MutedColor()).Render(" (recursive)"))
	}
	return sb.String()
}

func (r *FSListRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	var items []any
	_ = json.Unmarshal([]byte(resultText), &items)

	msg := fmt.Sprintf("Found %d items", len(items))
	if durationMs > 0 {
		msg += fmt.Sprintf(" in %dms", durationMs)
	}
	return theme.Style(theme.StatusSuccessColor()).Render("  [done] " + msg)
}

func NewFSListTool() *FSListTool {
	return &FSListTool{}
}

func (t *FSListTool) Name() string {
	return "fs_list"
}

func (t *FSListTool) Description(ctx tool.Context) string {
	return "List directory entries from local or SSH workspaces."
}

func (t *FSListTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"path":      map[string]any{"type": "string", "description": "Directory path to list"},
			"server":    map[string]any{"type": "string", "description": "Optional workspace alias. Defaults to active workspace."},
			"recursive": map[string]any{"type": "boolean", "description": "Whether to recursively list descendants"},
			"depth":     map[string]any{"type": "integer", "description": "Maximum recursive depth. 0 means unlimited when recursive=true"},
		},
		"required": []string{"path"},
	}
}

func (t *FSListTool) IsReadOnly() bool {
	return true
}

func (t *FSListTool) MaxResultSizeChars() int {
	return 100000
}

func (t *FSListTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	if ctx.Workspace == nil {
		return nil, fmt.Errorf("workspace manager is unavailable")
	}

	var req struct {
		Path      string `json:"path"`
		Server    string `json:"server"`
		Recursive bool   `json:"recursive"`
		Depth     int    `json:"depth"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		alias := tool.ResolveWorkspaceAlias(ctx, req.Server)
		if strings.TrimSpace(req.Server) != "" {
			if _, ok := ctx.Workspace.Registry().Get(alias); !ok {
				events <- tool.NewErrorEvent(fmt.Errorf("unknown server alias: %s", alias))
				return
			}
		}

		targetPath := strings.TrimSpace(req.Path)
		if targetPath == "" {
			events <- tool.NewErrorEvent(fmt.Errorf("path is required"))
			return
		}
		if !tool.IsRemoteWorkspace(ctx, alias) {
			resolved, err := tool.ResolvePathForRead(ctx, targetPath)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			targetPath = resolved
		}

		items, err := ctx.Workspace.ListDir(ctx.Context, alias, targetPath, req.Recursive, req.Depth)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		raw, _ := json.Marshal(items)
		events <- tool.NewOutputEvent(string(raw))
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *FSListTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	if ctx.Workspace == nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: "workspace manager is unavailable"}, nil
	}

	rawServer := tool.ExtractStringInput(input, "server")
	alias := tool.ResolveWorkspaceAlias(ctx, rawServer)
	if strings.TrimSpace(rawServer) != "" {
		if _, ok := ctx.Workspace.Registry().Get(alias); !ok {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: fmt.Sprintf("unknown server alias: %s", alias)}, nil
		}
	}
	path := strings.TrimSpace(tool.ExtractStringInput(input, "path"))
	if path == "" {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: "path is required"}, nil
	}
	if tool.IsRemoteWorkspace(ctx, alias) {
		return tool.DefaultAllowPermission(), nil
	}
	if _, err := tool.ResolvePathForRead(ctx, path); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	return tool.DefaultAllowPermission(), nil
}
