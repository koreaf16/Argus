package grep

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tui/toolui"
	"github.com/koreaf16/argus/internal/types"
)

type GrepTool struct{}

func init() {
	toolui.Register("grep", &GrepRenderer{})
}

type GrepRenderer struct{}

func (r *GrepRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *GrepRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	pattern, _ := args["pattern"].(string)
	path, _ := args["path"].(string)
	server := renderTargetAlias(args)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%q in %s", pattern, path))
	if server != "" {
		sb.WriteString(" [" + server + "]")
	}
	return toolui.FormatToolCall("Grep", sb.String(), 160, theme)
}

func (r *GrepRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	lines := len(strings.Split(strings.TrimSpace(resultText), "\n"))
	if strings.TrimSpace(resultText) == "" {
		lines = 0
	}
	msg := fmt.Sprintf("Found %d matches", lines)
	if durationMs > 0 {
		msg += fmt.Sprintf(" in %dms", durationMs)
	}
	return toolui.FormatResultLines([]string{msg}, true, false, theme)
}

func NewGrepTool() *GrepTool {
	return &GrepTool{}
}

func (t *GrepTool) Name() string {
	return "grep"
}

func (t *GrepTool) Description(ctx tool.Context) string {
	return "파일에서 패턴을 검색합니다."
}

func (t *GrepTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "검색 패턴"},
			"path":    map[string]any{"type": "string", "description": "검색할 파일 또는 디렉토리"},
			"server":  map[string]any{"type": "string", "description": "선택적 워크스페이스 별칭. 기본값은 활성 워크스페이스입니다."},
			"role":    map[string]any{"type": "string", "description": "선택적 워크플로우 역할."},
			"channel": map[string]any{"type": "string", "description": "선택적 워크플로우 채널."},
		},
		"required": []string{"pattern", "path"},
	}
}

func (t *GrepTool) IsReadOnly() bool {
	return true
}

func (t *GrepTool) MaxResultSizeChars() int {
	return 100000
}

func (t *GrepTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)

	var req struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Server  string `json:"server"`
		Role    string `json:"role"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		targetAlias, _, err := tool.ResolveExecutionRoleServer(ctx, req.Server, req.Role, req.Channel, "grep")
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
			out, err := runRemoteSearch(ctx, targetAlias, req.Pattern, req.Path)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
			events <- tool.NewOutputEvent(out)
			events <- tool.NewDoneEvent()
			return
		}

		searchPath, err := tool.ResolvePathForRead(ctx, req.Path)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}
		out, err := runRipgrep(ctx, req.Pattern, searchPath)
		if err != nil {
			out, err = runGoSearch(req.Pattern, searchPath)
			if err != nil {
				events <- tool.NewErrorEvent(err)
				return
			}
		}

		events <- tool.NewOutputEvent(out)
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *GrepTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	rawServer := tool.ExtractStringInput(input, "server")
	rawRole := tool.ExtractStringInput(input, "role")
	rawChannel := tool.ExtractStringInput(input, "channel")
	targetAlias, _, err := tool.ResolveExecutionRoleServer(ctx, rawServer, rawRole, rawChannel, "grep")
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
	path := tool.ExtractStringInput(input, "path")
	if _, err := tool.ResolvePathForRead(ctx, path); err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	return tool.DefaultAllowPermission(), nil
}

func runRipgrep(ctx tool.Context, pattern, path string) (string, error) {
	if _, err := exec.LookPath("rg"); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx.Context, "rg", "--line-number", "--color", "never", "--no-heading", pattern, path)
	cmd.Dir = ctx.WorkingDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		if out.Len() == 0 {
			return "", err
		}
		// rg exits 1 when no matches; return empty output instead of error.
		if strings.TrimSpace(out.String()) == "" {
			return "", nil
		}
	}
	return out.String(), nil
}

func runGoSearch(pattern, path string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			return appendMatches(&out, re, p)
		})
		if err != nil {
			return "", err
		}
		return out.String(), nil
	}
	if err := appendMatches(&out, re, path); err != nil {
		return "", err
	}
	return out.String(), nil
}

func appendMatches(out *strings.Builder, re *regexp.Regexp, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			fmt.Fprintf(out, "%s:%d:%s\n", path, lineNum, line)
		}
	}
	return scanner.Err()
}

func runRemoteSearch(ctx tool.Context, alias, pattern, path string) (string, error) {
	if ctx.Workspace == nil {
		return "", fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다")
	}
	patternQ := tool.POSIXShellQuote(pattern)
	pathQ := tool.POSIXShellQuote(path)
	cmd := strings.Join([]string{
		fmt.Sprintf("if command -v rg >/dev/null 2>&1; then rg --line-number --color never --no-heading -- %s %s; __argus_code=$?; else grep -R -n -- %s %s; __argus_code=$?; fi", patternQ, pathQ, patternQ, pathQ),
		"if [ $__argus_code -gt 1 ]; then exit $__argus_code; fi",
		"exit 0",
	}, "\n")
	res, err := ctx.Workspace.Exec(ctx.Context, alias, cmd, nil)
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("원격 grep 실행 실패: %s", strings.TrimSpace(res.Stdout+res.Stderr))
	}
	return res.Stdout, nil
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
