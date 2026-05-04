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
	return "파일 시스템에서 파일을 읽습니다."
}

func (t *FileReadTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"path":       map[string]any{"type": "string", "description": "읽을 파일의 경로"},
			"start_line": map[string]any{"type": "integer", "description": "시작 줄 번호 (1부터 시작)"},
			"end_line":   map[string]any{"type": "integer", "description": "종료 줄 번호 (포함)"},
			"server":     map[string]any{"type": "string", "description": "선택적 워크스페이스 별칭. 기본값은 활성 워크스페이스입니다."},
			"role":       map[string]any{"type": "string", "description": "선택적 워크플로우 역할."},
			"channel":    map[string]any{"type": "string", "description": "선택적 워크플로우 채널."},
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
			events <- tool.NewErrorEvent(fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다"))
			return
		}
		if strings.TrimSpace(targetAlias) != "" && ctx.Workspace != nil {
			if _, ok := ctx.Workspace.Registry().Get(targetAlias); !ok {
				events <- tool.NewErrorEvent(fmt.Errorf("알 수 없는 서버 별칭: %s", targetAlias))
				return
			}
		}

		var data []byte
		if tool.IsRemoteWorkspace(ctx, targetAlias) {
			if ctx.Workspace == nil {
				events <- tool.NewErrorEvent(fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다"))
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
				events <- tool.NewErrorEvent(fmt.Errorf("파일이 너무 큽니다 (%d 바이트). 최대 %d 바이트까지 허용됩니다. 특정 구간을 읽으려면 start_line/end_line을 사용하세요", info.Size(), maxLocalFileSizeBytes))
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
			return tool.PermissionResult{Behavior: "deny", Message: "워크스페이스 관리자를 사용할 수 없습니다"}, nil
		}
		if _, ok := ctx.Workspace.Registry().Get(server); !ok {
			return tool.PermissionResult{Behavior: "deny", Message: fmt.Sprintf("알 수 없는 서버 별칭: %s", server)}, nil
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
		return "", fmt.Errorf("start_line은 1 이상이어야 합니다")
	}
	if end < start {
		return "", fmt.Errorf("end_line은 start_line보다 크거나 같아야 합니다")
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
