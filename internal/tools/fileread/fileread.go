package fileread

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tui/toolui"
)

const maxLocalFileSizeBytes = 10 * 1024 * 1024 // 10 MB

type FileReadTool struct{}

func init() {
	toolui.Register("fileread", &FileReadRenderer{})
}

type FileReadRenderer struct{}

func (r *FileReadRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *FileReadRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	path, _ := args["path"].(string)
	start, _ := args["start_line"].(float64)
	end, _ := args["end_line"].(float64)
	server := renderTargetAlias(args)

	var sb strings.Builder
	sb.WriteString(theme.Style(theme.ToolUseColor()).Bold(true).Render("  Read: "))
	sb.WriteString(theme.Style(theme.BodyColor()).Render(path))
	if server != "" {
		sb.WriteString(theme.Style(theme.MutedColor()).Render(" [" + server + "]"))
	}
	if start > 0 || end > 0 {
		sb.WriteString(theme.Style(theme.MutedColor()).Render(fmt.Sprintf(" (lines %v-%v)", start, end)))
	}
	return sb.String()
}

func (r *FileReadRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	lines := len(strings.Split(resultText, "\n"))
	if resultText == "" {
		lines = 0
	}
	msg := fmt.Sprintf("Read %d lines", lines)
	if durationMs > 0 {
		msg += fmt.Sprintf(" in %dms", durationMs)
	}
	return theme.Style(theme.StatusSuccessColor()).Render("  [done] " + msg)
}

func NewFileReadTool() *FileReadTool {
	return &FileReadTool{}
}

func (t *FileReadTool) Name() string {
	return "fileread"
}

func (t *FileReadTool) Description(ctx tool.Context) string {
	return "Read files from the filesystem"
}

func (t *FileReadTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"path":       map[string]any{"type": "string", "description": "The path to the file to read"},
			"start_line": map[string]any{"type": "integer", "description": "Start line (1-indexed)"},
			"end_line":   map[string]any{"type": "integer", "description": "End line (inclusive)"},
			"server":     map[string]any{"type": "string", "description": "Optional workspace alias. Defaults to active workspace."},
			"role":       map[string]any{"type": "string", "description": "Optional workflow role."},
			"channel":    map[string]any{"type": "string", "description": "Optional workflow channel."},
		},
		"required": []string{"path"},
	}
}

func (t *FileReadTool) IsReadOnly() bool {
	return true
}

func (t *FileReadTool) MaxResultSizeChars() int {
	return 100000
}

func (t *FileReadTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)

	var req struct {
		Path      string `json:"path"`
		StartLine *int   `json:"start_line"`
		EndLine   *int   `json:"end_line"`
		Server    string `json:"server"`
		Role      string `json:"role"`
		Channel   string `json:"channel"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		targetAlias, _, err := tool.ResolveExecutionRoleServer(ctx, req.Server, req.Role, req.Channel, "fileread")
		if err != nil {
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

		var data []byte
		if tool.IsRemoteWorkspace(ctx, targetAlias) {
			if ctx.Workspace == nil {
				events <- tool.NewErrorEvent(fmt.Errorf("workspace manager is unavailable"))
				return
			}
			var readErr error
			data, readErr = ctx.Workspace.ReadFile(ctx.Context, targetAlias, req.Path)
			if readErr != nil {
				events <- tool.NewErrorEvent(readErr)
				return
			}
		} else {
			path, pathErr := tool.ResolvePathForRead(ctx, req.Path)
			if pathErr != nil {
				events <- tool.NewErrorEvent(pathErr)
				return
			}
			info, statErr := os.Stat(path)
			if statErr != nil {
				events <- tool.NewErrorEvent(statErr)
				return
			}
			if info.Size() > maxLocalFileSizeBytes {
				events <- tool.NewErrorEvent(fmt.Errorf("file too large (%d bytes); max %d bytes — use start_line/end_line to read a section", info.Size(), maxLocalFileSizeBytes))
				return
			}
			data, err = os.ReadFile(path)
		}
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		content, err := applyLineRange(string(data), req.StartLine, req.EndLine)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		events <- tool.NewOutputEvent(content)
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *FileReadTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	rawServer := tool.ExtractStringInput(input, "server")
	rawRole := tool.ExtractStringInput(input, "role")
	rawChannel := tool.ExtractStringInput(input, "channel")
	server, _, err := tool.ResolveExecutionRoleServer(ctx, rawServer, rawRole, rawChannel, "fileread")
	if err != nil {
		return tool.PermissionResult{Behavior: "deny", Message: err.Error()}, nil
	}
	if strings.TrimSpace(server) != "" && server != "local" {
		if ctx.Workspace == nil {
			return tool.PermissionResult{Behavior: "deny", Message: "workspace manager is unavailable"}, nil
		}
		if _, ok := ctx.Workspace.Registry().Get(server); !ok {
			return tool.PermissionResult{Behavior: "deny", Message: fmt.Sprintf("unknown server alias: %s", server)}, nil
		}
	}
	if tool.IsRemoteWorkspace(ctx, server) {
		return tool.DefaultAllowPermission(), nil
	}
	path := tool.ExtractStringInput(input, "path")
	if _, err := tool.ResolvePathForRead(ctx, path); err != nil {
		return tool.PermissionResult{Behavior: "deny", Message: err.Error()}, nil
	}
	return tool.DefaultAllowPermission(), nil
}

func applyLineRange(content string, startLine, endLine *int) (string, error) {
	if startLine == nil && endLine == nil {
		return content, nil
	}

	lines := strings.Split(content, "\n")
	start := 1
	if startLine != nil {
		start = *startLine
	}
	end := len(lines)
	if endLine != nil {
		end = *endLine
	}

	if start < 1 {
		return "", fmt.Errorf("start_line must be >= 1")
	}
	if end < start {
		return "", fmt.Errorf("end_line must be >= start_line")
	}
	if start > len(lines) {
		return "", nil
	}
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[start-1:end], "\n"), nil
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
