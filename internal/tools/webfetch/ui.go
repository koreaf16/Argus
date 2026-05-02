package webfetch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/tui/toolui"
)

// WebFetchRenderer provides custom UI for the webfetch tool.
type WebFetchRenderer struct{}

func init() {
	toolui.Register("webfetch", &WebFetchRenderer{})
}

func (r *WebFetchRenderer) CreateInteractiveModel(args map[string]any, theme toolui.ThemeContext) toolui.InteractiveModel {
	return nil
}

func (r *WebFetchRenderer) RenderToolUse(args map[string]any, _ string, theme toolui.ThemeContext) string {
	// claude_cli: renderToolUseMessage는 url 자체만 반환. headline 형태는 render.go가
	// `● WebFetch(url)` 으로 합성한다.
	rawURL, _ := args["url"].(string)
	return toolui.FormatToolCall("WebFetch", rawURL, 160, theme)
}

func (r *WebFetchRenderer) RenderToolResult(resultText string, durationMs int64, theme toolui.ThemeContext) string {
	var out fetchOutput
	if err := json.Unmarshal([]byte(resultText), &out); err != nil {
		return resultText
	}

	// claude_cli WebFetchTool/UI.tsx renderToolResultMessage:
	//   `Received <bold>{formattedSize}</bold> ({code} {codeText})`
	// formatFileSize: 1024^2 ≥ "X.XMB", 1024 ≥ "X.XKB", else "N bytes"
	sizeDisp := formatFileSize(out.Bytes)
	statusPart := ""
	switch {
	case out.Code == 0:
		statusPart = "(OK)"
	case strings.TrimSpace(out.CodeText) != "":
		statusPart = fmt.Sprintf("(%d %s)", out.Code, strings.TrimSpace(out.CodeText))
	default:
		statusPart = fmt.Sprintf("(%d)", out.Code)
	}
	msg := fmt.Sprintf("Received %s %s", sizeDisp, statusPart)
	_ = durationMs
	return toolui.FormatResultLines([]string{msg}, true, false, theme)
}

func formatFileSize(bytes int) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}
