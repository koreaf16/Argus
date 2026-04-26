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

	var sb strings.Builder
	sb.WriteString(theme.Style(theme.ToolUseColor()).Bold(true).Render("  Grep: "))
	sb.WriteString(theme.Style(theme.BodyColor()).Render(fmt.Sprintf("\"%s\"", pattern)))
	sb.WriteString(theme.Style(theme.MutedColor()).Render(" in " + path))
	return sb.String()
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
	return theme.Style(theme.StatusSuccessColor()).Render("  [done] " + msg)
}

func NewGrepTool() *GrepTool {
	return &GrepTool{}
}

func (t *GrepTool) Name() string {
	return "grep"
}

func (t *GrepTool) Description(ctx tool.Context) string {
	return "Search for a pattern in files."
}

func (t *GrepTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "The search pattern"},
			"path":    map[string]any{"type": "string", "description": "The file or directory to search"},
			"server":  map[string]any{"type": "string", "description": "Optional workspace alias. Defaults to active workspace."},
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
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		targetAlias := tool.ResolveWorkspaceAlias(ctx, req.Server)
		if strings.TrimSpace(req.Server) != "" && ctx.Workspace == nil {
			events <- tool.NewErrorEvent(fmt.Errorf("workspace manager is unavailable"))
			return
		}
		if strings.TrimSpace(req.Server) != "" && ctx.Workspace != nil {
			if _, ok := ctx.Workspace.Registry().Get(targetAlias); !ok {
				events <- tool.NewErrorEvent(fmt.Errorf("unknown server alias: %s", targetAlias))
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
	if strings.TrimSpace(rawServer) != "" {
		if ctx.Workspace == nil {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: "workspace manager is unavailable"}, nil
		}
		alias := tool.ResolveWorkspaceAlias(ctx, rawServer)
		if _, ok := ctx.Workspace.Registry().Get(alias); !ok {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: fmt.Sprintf("unknown server alias: %s", alias)}, nil
		}
	}
	targetAlias := tool.ResolveWorkspaceAlias(ctx, rawServer)
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
		return "", fmt.Errorf("workspace manager is unavailable")
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
		return "", fmt.Errorf("remote grep failed: %s", strings.TrimSpace(res.Stdout+res.Stderr))
	}
	return res.Stdout, nil
}
