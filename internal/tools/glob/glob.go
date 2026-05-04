package glob

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tui/toolui"
	"github.com/koreaf16/argus/internal/types"
)

type GlobTool struct{}

func init() {
	toolui.Register("glob", &GlobRenderer{})
}

type GlobRenderer struct{}

func (r *GlobRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *GlobRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	pattern, _ := args["pattern"].(string)
	server := renderTargetAlias(args)

	var sb strings.Builder
	sb.WriteString(theme.Style(theme.ToolUseColor()).Bold(true).Render("  Glob: "))
	sb.WriteString(theme.Style(theme.BodyColor()).Render(pattern))
	if server != "" {
		sb.WriteString(theme.Style(theme.MutedColor()).Render(" [" + server + "]"))
	}
	return sb.String()
}

func (r *GlobRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	var files []string
	count := 0
	if err := json.Unmarshal([]byte(resultText), &files); err == nil {
		count = len(files)
	}
	msg := fmt.Sprintf("Found %d files", count)
	if durationMs > 0 {
		msg += fmt.Sprintf(" in %dms", durationMs)
	}
	return theme.Style(theme.StatusSuccessColor()).Render("  [done] " + msg)
}

func NewGlobTool() *GlobTool {
	return &GlobTool{}
}

func (t *GlobTool) Name() string {
	return "glob"
}

func (t *GlobTool) Description(ctx tool.Context) string {
	return "패턴과 일치하는 파일을 찾습니다."
}

func (t *GlobTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob 패턴"},
			"server":  map[string]any{"type": "string", "description": "선택적 워크스페이스 별칭. 기본값은 활성 워크스페이스입니다."},
			"role":    map[string]any{"type": "string", "description": "선택적 워크플로우 역할."},
			"channel": map[string]any{"type": "string", "description": "선택적 워크플로우 채널."},
		},
		"required": []string{"pattern"},
	}
}

func (t *GlobTool) IsReadOnly() bool {
	return true
}

func (t *GlobTool) MaxResultSizeChars() int {
	return 100000
}

func (t *GlobTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	var req struct {
		Pattern string `json:"pattern"`
		Server  string `json:"server"`
		Role    string `json:"role"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		targetAlias, _, err := tool.ResolveExecutionRoleServer(ctx, req.Server, req.Role, req.Channel, "glob")
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

		if tool.IsRemoteWorkspace(ctx, targetAlias) {
			out, err := runRemoteGlob(ctx, targetAlias, req.Pattern)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			result, _ := json.Marshal(out)
			events <- tool.NewOutputEvent(string(result))
			events <- tool.NewDoneEvent()
			return
		}

		absPattern, err := tool.ResolveGlobPattern(ctx, req.Pattern)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		files, err := filepath.Glob(absPattern)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		out := make([]string, 0, len(files))
		for _, f := range files {
			clean := filepath.Clean(f)
			if _, err := tool.ResolvePathForRead(ctx, clean); err != nil {
				continue
			}
			out = append(out, normalizeOutputPath(ctx.WorkingDir, clean))
		}

		result, _ := json.Marshal(out)
		events <- tool.NewOutputEvent(string(result))
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *GlobTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	rawServer := tool.ExtractStringInput(input, "server")
	rawRole := tool.ExtractStringInput(input, "role")
	rawChannel := tool.ExtractStringInput(input, "channel")
	targetAlias, _, err := tool.ResolveExecutionRoleServer(ctx, rawServer, rawRole, rawChannel, "glob")
	if err != nil {
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
		return tool.DefaultAllowPermission(), nil
	}
	pattern := tool.ExtractStringInput(input, "pattern")
	if _, err := tool.ResolveGlobPattern(ctx, pattern); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	return tool.DefaultAllowPermission(), nil
}

func normalizeOutputPath(workingDir, path string) string {
	if strings.TrimSpace(workingDir) == "" {
		return path
	}
	rel, err := filepath.Rel(workingDir, path)
	if err != nil {
		return path
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return path
	}
	return rel
}

func runRemoteGlob(ctx tool.Context, alias, pattern string) ([]string, error) {
	if ctx.Workspace == nil {
		return nil, fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다")
	}
	cmd := strings.Join([]string{
		fmt.Sprintf("__argus_pattern=%s", tool.POSIXShellQuote(pattern)),
		"if command -v compgen >/dev/null 2>&1; then",
		"  shopt -s globstar nullglob 2>/dev/null || true",
		"  compgen -G \"$__argus_pattern\" || true",
		"else",
		"  find . -path \"$__argus_pattern\" -print 2>/dev/null || true",
		"fi",
	}, "\n")
	res, err := ctx.Workspace.Exec(ctx.Context, alias, cmd, nil)
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("원격 glob 실행 실패: %s", strings.TrimSpace(res.Stdout+res.Stderr))
	}
	lines := strings.Split(strings.ReplaceAll(res.Stdout, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out, nil
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
