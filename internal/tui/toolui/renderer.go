package toolui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tool "github.com/koreaf16/argus/internal/tools"
)

// ThemeContext는 TUI가 제공하는 테마 색상 및 스타일 정보에 접근하기 위한 인터페이스입니다.
// 도구 UI가 TUI 패키지에 직접 의존하지 않도록 분리합니다.
type ThemeContext interface {
	Style(color string) lipgloss.Style
	BaseBodyStyle() lipgloss.Style
	Width() int
	BodyColor() string
	MutedColor() string
	BorderColor() string
	ToolUseColor() string
	ToolResultColor() string
	StatusSuccessColor() string
	StatusWarningColor() string
	StatusErrorColor() string
	ClaudeAccentColor() string
	AnimFrame() int
}

// ToolRenderer는 각 도구가 스스로의 실행(Use) 및 결과(Result) 화면을 그리도록 하는 인터페이스입니다.
type ToolRenderer interface {
	// 정적 렌더링 (스크롤백 기록용)
	RenderToolUse(args map[string]any, streamBody string, theme ThemeContext) string
	RenderToolResult(resultText string, durationMs int64, theme ThemeContext) string

	// 동적 상호작용 (현재 실행 중이거나 포커스된 상태)
	// nil을 반환하면 정적 렌더링만 사용합니다.
	CreateInteractiveModel(args map[string]any, theme ThemeContext) InteractiveModel
}

// InteractiveModel은 BubbleTea의 Model을 확장하여 도구 전용 기능을 추가한 인터페이스입니다.
type InteractiveModel interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (InteractiveModel, tea.Cmd)
	View() string

	// 포커스 관련
	SetFocus(focus bool)
	IsFocused() bool

	// 확장/축소 관련 (Ctrl+O)
	SetExpanded(expanded bool)
	IsExpanded() bool

	// 스트리밍 데이터 업데이트 (stdout 등)
	OnStreamDelta(delta string)

	// 입력 채널 설정 (PTY stdin 등)
	SetInputResponse(input chan string)

	// 완료 상태 설정
	SetFinished(finished bool)

	// 도구 결과 전달 (tool_result body — 스트리밍 없이 최종 결과를 받는 도구용)
	SetResult(text string)
}

var renderers = make(map[string]ToolRenderer)

// Register는 특정 도구 이름에 대해 렌더러를 등록합니다.
func Register(toolName string, renderer ToolRenderer) {
	renderers[tool.CanonicalName(toolName)] = renderer
}

// GetRenderer는 등록된 도구 렌더러를 반환합니다.
func GetRenderer(toolName string) ToolRenderer {
	return renderers[tool.CanonicalName(toolName)]
}
