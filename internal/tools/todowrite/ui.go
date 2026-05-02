package todowrite

import (
	"encoding/json"

	"github.com/koreaf16/argus/internal/tui/toolui"
	"github.com/koreaf16/argus/internal/types"
)

// TodoWriteRenderer는 TodoWrite 단독 호출(이전 entry가 thinking이 아닌 케이스)에 대해
// `TodoWrite` 헤드라인 + 체크리스트 형식으로 렌더한다. thinking entry에 흡수된 케이스는
// transcript.go의 absorbTodoWriteIntoThinking이 처리하므로 본 렌더러는 호출되지 않는다.
type TodoWriteRenderer struct{}

func init() {
	toolui.Register("TodoWrite", &TodoWriteRenderer{})
}

func (r *TodoWriteRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *TodoWriteRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	headline := toolui.FormatToolCall("TodoWrite", "", 160, theme)

	rawTodos, ok := args["todos"]
	if !ok {
		return headline
	}
	// args가 map[string]any로 풀려있으므로 todos는 []any이다. 한 번 더 직렬화해 unmarshal.
	bytes, err := json.Marshal(rawTodos)
	if err != nil {
		return headline
	}
	var items []types.TodoItem
	if err := json.Unmarshal(bytes, &items); err != nil {
		return headline
	}

	checklist := toolui.RenderTodoChecklist(items, theme)
	if checklist == "" {
		return headline
	}
	return headline + "\n" + checklist
}

// RenderToolResult는 빈 문자열을 반환한다. claude_cli도 결과는 표시하지 않는다.
func (r *TodoWriteRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	return ""
}
