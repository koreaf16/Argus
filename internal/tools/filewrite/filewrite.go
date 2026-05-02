package filewrite

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tui/toolui"
	"github.com/koreaf16/argus/internal/types"
)

type FileWriteTool struct{}

func init() {
	toolui.Register("filewrite", &FileWriteRenderer{})
}

type FileWriteRenderer struct{}

func (r *FileWriteRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *FileWriteRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	server := renderTargetAlias(args)

	var sb strings.Builder
	sb.WriteString(theme.Style(theme.ToolUseColor()).Bold(true).Render("  Write: "))
	sb.WriteString(theme.Style(theme.BodyColor()).Render(path))
	if server != "" {
		sb.WriteString(theme.Style(theme.MutedColor()).Render(" [" + server + "]"))
	}
	sb.WriteString(theme.Style(theme.MutedColor()).Render(fmt.Sprintf(" (%d bytes)", len(content))))
	return sb.String()
}

func (r *FileWriteRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	msg := resultText
	if durationMs > 0 {
		msg += fmt.Sprintf(" in %dms", durationMs)
	}
	return theme.Style(theme.StatusSuccessColor()).Render("  [done] " + msg)
}

func NewFileWriteTool() *FileWriteTool {
	return &FileWriteTool{}
}

func (t *FileWriteTool) Name() string {
	return "filewrite"
}

func (t *FileWriteTool) Description(ctx tool.Context) string {
	return "Write content to a file. Overwrites existing files."
}

func (t *FileWriteTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "The path to the file to write"},
			"content": map[string]any{"type": "string", "description": "The content to write"},
			"server":  map[string]any{"type": "string", "description": "Optional workspace alias. Defaults to active workspace."},
			"role":    map[string]any{"type": "string", "description": "Optional workflow role."},
			"channel": map[string]any{"type": "string", "description": "Optional workflow channel."},
		},
		"required": []string{"path", "content"},
	}
}

func (t *FileWriteTool) IsReadOnly() bool {
	return false
}

func (t *FileWriteTool) MaxResultSizeChars() int {
	return 100000
}

func (t *FileWriteTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Server  string `json:"server"`
		Role    string `json:"role"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		targetAlias, roleCtx, err := tool.ResolveExecutionRoleServer(ctx, req.Server, req.Role, req.Channel, "filewrite")
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if err := tool.ValidateRoleMutation(roleCtx, "filewrite", "", true); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		if strings.TrimSpace(targetAlias) != "" && targetAlias != "local" && ctx.Workspace == nil {
			events <- tool.NewErrorEvent(fmt.Errorf("workspace manager is unavailable"))
			return
		}
		if strings.TrimSpace(targetAlias) != "" && ctx.Workspace != nil {
			if _, ok := ctx.Workspace.Registry().Get(targetAlias); !ok {
				events <- tool.NewErrorEvent(fmt.Errorf("unknown server alias: %s", targetAlias))
				return
			}
		}

		if err := writeContent(ctx, targetAlias, req.Path, []byte(req.Content)); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		events <- tool.NewOutputEvent("File written successfully")
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *FileWriteTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	rawServer := tool.ExtractStringInput(input, "server")
	rawRole := tool.ExtractStringInput(input, "role")
	rawChannel := tool.ExtractStringInput(input, "channel")
	targetAlias, roleCtx, err := tool.ResolveExecutionRoleServer(ctx, rawServer, rawRole, rawChannel, "filewrite")
	if err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	if err := tool.ValidateRoleMutation(roleCtx, "filewrite", "", true); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	if strings.TrimSpace(targetAlias) != "" && targetAlias != "local" {
		if ctx.Workspace == nil {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: "workspace manager is unavailable"}, nil
		}
		if _, ok := ctx.Workspace.Registry().Get(targetAlias); !ok {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: fmt.Sprintf("unknown server alias: %s", targetAlias)}, nil
		}
	}
	if tool.IsRemoteWorkspace(ctx, targetAlias) {
		if isSensitiveWritePath(tool.ExtractStringInput(input, "path")) {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: "write to sensitive path is denied"}, nil
		}
		if tool.IsWriteModeAllowed(ctx) {
			return tool.DefaultAllowPermission(), nil
		}
		return tool.PermissionResult{
			Behavior: types.BehaviorAsk,
			Message:  "writing files requires approval in the current mode",
		}, nil
	}
	path := tool.ExtractStringInput(input, "path")
	if _, err := tool.ResolvePathForWrite(ctx, path); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	if tool.IsWriteModeAllowed(ctx) {
		return tool.DefaultAllowPermission(), nil
	}
	return tool.PermissionResult{
		Behavior: types.BehaviorAsk,
		Message:  "writing files requires approval in the current mode",
	}, nil
}

func writeContent(ctx tool.Context, alias, path string, content []byte) error {
	if tool.IsRemoteWorkspace(ctx, alias) {
		if ctx.Workspace == nil {
			return fmt.Errorf("workspace manager is unavailable")
		}
		return ctx.Workspace.WriteFile(ctx.Context, alias, path, content, true)
	}
	resolved, err := tool.ResolvePathForWrite(ctx, path)
	if err != nil {
		return err
	}
	return writeFileAtomic(resolved, content, 0o644)
}

func writeFileAtomic(path string, content []byte, perm os.FileMode) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, perm); err != nil {
		_ = os.Remove(tmp) // cleanup partial file on write failure
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func isSensitiveWritePath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "/.ssh/") || strings.HasSuffix(lower, "/.ssh") {
		return true
	}
	if strings.Contains(lower, "/id_rsa") || strings.Contains(lower, "/id_ed25519") {
		return true
	}
	if strings.Contains(lower, "/.bashrc") || strings.Contains(lower, "/.zshrc") {
		return true
	}
	if strings.Contains(lower, `\.ssh\`) || strings.HasSuffix(lower, `\.ssh`) {
		return true
	}
	if strings.Contains(lower, `\id_rsa`) || strings.Contains(lower, `\id_ed25519`) {
		return true
	}
	if strings.Contains(lower, `\.bashrc`) || strings.Contains(lower, `\.zshrc`) {
		return true
	}
	return false
}

func renderTargetAlias(args map[string]any) string {
	server := "local"
	if s, ok := args["server"].(string); ok && strings.TrimSpace(s) != "" {
		server = strings.TrimSpace(s)
	} else if active, ok := args["_active_workspace"].(string); ok && strings.TrimSpace(active) != "" {
		server = strings.TrimSpace(active)
	}
	label := server
	if ch, ok := args["channel"].(string); ok && strings.TrimSpace(ch) != "" {
		label += "/" + strings.TrimSpace(ch)
	}
	if role, ok := args["role"].(string); ok && strings.TrimSpace(role) != "" {
		label += " role=" + strings.TrimSpace(role)
	}
	return label
}


