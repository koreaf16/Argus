package task

import (
	"strings"

	"github.com/koreaf16/argus/internal/tui/toolui"
)

func init() {
	toolui.Register("TaskCreate", &TaskCreateRenderer{})
	toolui.Register("TaskUpdate", &TaskUpdateRenderer{})
}

type TaskCreateRenderer struct{}

func (r *TaskCreateRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	subject := pickArg(args, "subject", "task_name")
	body := subject
	if desc := pickArg(args, "description"); desc != "" {
		body += " · " + desc
	}
	return toolui.FormatToolCall("TaskCreate", body, 160, theme)
}

func (r *TaskCreateRenderer) RenderToolResult(result string, _ int64, theme toolui.ThemeContext) string {
	if strings.TrimSpace(result) == "" {
		return ""
	}
	return toolui.FormatResultLines([]string{strings.TrimSpace(result)}, true, false, theme)
}

func (r *TaskCreateRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

type TaskUpdateRenderer struct{}

func (r *TaskUpdateRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	id := pickArg(args, "task_id")
	status := pickArg(args, "status")
	body := id
	if status != "" {
		body += " · " + status
	}
	return toolui.FormatToolCall("TaskUpdate", body, 160, theme)
}

func (r *TaskUpdateRenderer) RenderToolResult(result string, _ int64, theme toolui.ThemeContext) string {
	if strings.TrimSpace(result) == "" {
		return ""
	}
	return toolui.FormatResultLines([]string{strings.TrimSpace(result)}, true, false, theme)
}

func (r *TaskUpdateRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func pickArg(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok {
				if t := strings.TrimSpace(s); t != "" {
					return t
				}
			}
		}
	}
	return ""
}
