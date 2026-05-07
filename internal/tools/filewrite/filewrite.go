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
	sb.WriteString(path)
	if server != "" {
		sb.WriteString(" [" + server + "]")
	}
	sb.WriteString(fmt.Sprintf(" (%d bytes)", len(content)))
	return toolui.FormatToolCall("Write", sb.String(), 160, theme)
}

func (r *FileWriteRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	msg := resultText
	if durationMs > 0 {
		msg += fmt.Sprintf(" in %dms", durationMs)
	}
	return toolui.FormatResultLines([]string{msg}, true, false, theme)
}

func NewFileWriteTool() *FileWriteTool {
	return &FileWriteTool{}
}

func (t *FileWriteTool) Name() string {
	return "filewrite"
}

func (t *FileWriteTool) Description(ctx tool.Context) string {
	return "파일에 내용을 작성합니다. 기존 파일이 있으면 덮어씁니다."
}

func (t *FileWriteTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "작성할 파일의 경로"},
			"content": map[string]any{"type": "string", "description": "작성할 내용"},
			"server":  map[string]any{"type": "string", "description": "선택적 워크스페이스 별칭. 기본값은 활성 워크스페이스입니다."},
			"role":    map[string]any{"type": "string", "description": "선택적 워크플로우 역할."},
			"channel": map[string]any{"type": "string", "description": "선택적 워크플로우 채널."},
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
			events <- tool.NewErrorEvent(fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다"))
			return
		}
		if strings.TrimSpace(targetAlias) != "" && ctx.Workspace != nil {
			if _, ok := ctx.Workspace.Registry().Get(targetAlias); !ok {
				events <- tool.NewErrorEvent(fmt.Errorf("알 수 없는 서버 별칭: %s", targetAlias))
				return
			}
		}

		if err := writeContent(ctx, targetAlias, req.Path, []byte(req.Content)); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		events <- tool.NewOutputEvent("파일을 성공적으로 작성했습니다")
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
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: "워크스페이스 관리자를 사용할 수 없습니다"}, nil
		}
		if _, ok := ctx.Workspace.Registry().Get(targetAlias); !ok {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: fmt.Sprintf("알 수 없는 서버 별칭: %s", targetAlias)}, nil
		}
	}
	if tool.IsRemoteWorkspace(ctx, targetAlias) {
		if isSensitiveWritePath(tool.ExtractStringInput(input, "path")) {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: "민감한 경로에 대한 쓰기가 거부되었습니다"}, nil
		}
		if tool.IsWriteModeAllowed(ctx) {
			return tool.DefaultAllowPermission(), nil
		}
		return tool.PermissionResult{
			Behavior: types.BehaviorAsk,
			Message:  "현재 모드에서 파일을 작성하려면 승인이 필요합니다",
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
		Message:  "현재 모드에서 파일을 작성하려면 승인이 필요합니다",
	}, nil
}

func writeContent(ctx tool.Context, alias, path string, content []byte) error {
	if tool.IsRemoteWorkspace(ctx, alias) {
		if ctx.Workspace == nil {
			return fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다")
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
		return fmt.Errorf("경로가 필요합니다")
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
