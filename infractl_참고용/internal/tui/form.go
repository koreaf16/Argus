// Package tui
// File: form.go
// Description: 다중 필드 폼 입력 상태, 렌더링, TUI 브릿지 핸들러
// Responsibility: formState 라이프사이클 관리, 상태 박스 렌더링, 채널 기반 goroutine 브릿지

package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yourorg/infractl/internal/tools"
)

// formState는 다중 필드 폼 입력 모드의 상태를 관리한다.
// 실제 텍스트 입력은 기존 inputBar에 위임하고, 이 구조체는 값 수집과 상태 박스 렌더링만 담당한다.
type formState struct {
	active        bool
	title         string
	headerLabel   string
	fields        []tools.FormFieldDef
	values        []string // 필드별 입력값
	cursor        int      // 현재 활성 필드 인덱스
	selectCursors []int    // SELECT 필드별 현재 하이라이트 인덱스
	replyCh       chan FormResult
}

// Activate는 폼 입력 모드를 활성화한다.
func (f *formState) Activate(title string, fields []tools.FormFieldDef, replyCh chan FormResult, header string) {
	f.active = true
	f.title = title
	f.headerLabel = header
	f.fields = fields
	f.values = make([]string, len(fields))
	f.selectCursors = make([]int, len(fields))
	for i, field := range fields {
		if field.FieldType == "select" && len(field.Options) > 0 {
			// SELECT 필드: 첫 번째 옵션을 기본값으로
			f.values[i] = field.Options[0]
			// DefaultValue가 Options 중 하나이면 그 인덱스로 초기화
			for j, opt := range field.Options {
				if opt == field.DefaultValue {
					f.selectCursors[i] = j
					f.values[i] = opt
					break
				}
			}
		} else {
			f.values[i] = field.DefaultValue
		}
	}
	f.cursor = 0
	f.replyCh = replyCh
}

// Deactivate는 폼 입력 모드를 비활성화하고 상태를 초기화한다.
func (f *formState) Deactivate() {
	f.active = false
	f.title = ""
	f.headerLabel = ""
	f.fields = nil
	f.values = nil
	f.selectCursors = nil
	f.cursor = 0
	f.replyCh = nil
}

// CurrentFieldLabel은 현재 활성 필드의 라벨을 반환한다.
func (f *formState) CurrentFieldLabel() string {
	if !f.active || f.cursor >= len(f.fields) {
		return ""
	}
	return f.fields[f.cursor].Label
}

// CurrentPlaceholder는 현재 필드의 placeholder를 반환한다.
func (f *formState) CurrentPlaceholder() string {
	if !f.active || f.cursor >= len(f.fields) {
		return ""
	}
	if f.fields[f.cursor].FieldType == "select" {
		return "" // SELECT 필드는 텍스트 입력 없음
	}
	return f.fields[f.cursor].Placeholder
}

// IsSelectField는 현재 활성 필드가 SELECT 타입인지 반환한다.
func (f *formState) IsSelectField() bool {
	if !f.active || f.cursor >= len(f.fields) {
		return false
	}
	return f.fields[f.cursor].FieldType == "select" && len(f.fields[f.cursor].Options) > 0
}

// SelectUp은 현재 SELECT 필드의 하이라이트를 위로 이동하고 값을 즉시 반영한다.
func (f *formState) SelectUp() {
	i := f.cursor
	if i >= len(f.fields) || f.fields[i].FieldType != "select" {
		return
	}
	if f.selectCursors[i] > 0 {
		f.selectCursors[i]--
		f.values[i] = f.fields[i].Options[f.selectCursors[i]]
	}
}

// SelectDown은 현재 SELECT 필드의 하이라이트를 아래로 이동하고 값을 즉시 반영한다.
func (f *formState) SelectDown() {
	i := f.cursor
	if i >= len(f.fields) || f.fields[i].FieldType != "select" {
		return
	}
	opts := f.fields[i].Options
	if f.selectCursors[i] < len(opts)-1 {
		f.selectCursors[i]++
		f.values[i] = opts[f.selectCursors[i]]
	}
}

// AcceptValue는 현재 필드에 값을 저장한다 (텍스트 필드용).
func (f *formState) AcceptValue(value string) {
	if f.cursor < len(f.values) {
		f.values[f.cursor] = value
	}
}

// AdvanceToNext는 커서를 다음 필드로 이동한다.
// 모든 필드가 입력되면 즉시 true를 반환한다 (Review 단계 없음).
func (f *formState) AdvanceToNext() bool {
	f.cursor++
	if f.cursor >= len(f.fields) {
		f.cursor = len(f.fields) - 1
		return true
	}
	return false
}

// NextField는 다음 필드로 이동한다 (Tab 네비게이션).
func (f *formState) NextField() {
	if f.cursor < len(f.fields)-1 {
		f.cursor++
	}
}

// PrevField는 이전 필드로 이동한다 (Shift+Tab 네비게이션).
func (f *formState) PrevField() {
	if f.cursor > 0 {
		f.cursor--
	}
}

// BuildResult는 현재 입력값으로 FormResult를 구성한다.
func (f *formState) BuildResult() FormResult {
	values := make(map[string]string, len(f.fields))
	for i, field := range f.fields {
		val := ""
		if i < len(f.values) {
			val = f.values[i]
		}
		values[field.Name] = val
	}
	return FormResult{Values: values, Cancelled: false}
}

// View는 폼 상태 박스를 렌더링한다.
// activeValue: 현재 활성 텍스트 필드에서 타이핑 중인 값 (textarea에서 읽어옴)
// cursorOffset: 활성 필드 내 커서 위치 (rune 단위)
func (f *formState) View(width int, activeValue string, cursorOffset int) string {
	if !f.active {
		return ""
	}
	return f.viewEdit(width, activeValue, cursorOffset)
}

func (f *formState) viewEdit(width int, activeValue string, cursorOffset int) string {
	innerW := width - 6
	if innerW < 30 {
		innerW = 30
	}

	var body strings.Builder
	body.WriteString("\n")

	hdr := strings.TrimSpace(f.headerLabel)
	if hdr == "" {
		hdr = strings.TrimSpace(f.title)
	}
	if hdr == "" {
		hdr = "Answer Questions"
	}
	body.WriteString(StyleGeminiHeader.Render("  "+hdr) + "\n")
	body.WriteString("\n")

	// fake cursor 블록: 역상(reverse video) 공백으로 실제 커서처럼 보임
	cursorBlock := lipgloss.NewStyle().Reverse(true).Render(" ")

	for i, field := range f.fields {
		val := ""
		if i < len(f.values) {
			val = f.values[i]
		}

		label := field.Label + ":"
		labelPadded := fmt.Sprintf("%-16s", label)

		isSelect := field.FieldType == "select" && len(field.Options) > 0

		switch {
		case i < f.cursor:
			// 입력 완료 필드
			checkMark := StyleSuccess.Render("✓")
			labelStr := StyleGeminiSubDesc.Render(labelPadded)
			valStr := StyleGeminiSelected.Render(val)
			body.WriteString(fmt.Sprintf("  %s %s %s\n", checkMark, labelStr, valStr))

		case i == f.cursor && isSelect:
			// 현재 활성 SELECT 필드: 옵션 목록 인라인 표시
			arrow := StyleGeminiBullet.Render("→")
			labelStr := StyleGeminiSubDesc.Render(labelPadded)
			body.WriteString(fmt.Sprintf("  %s %s\n", arrow, labelStr))
			selCursor := 0
			if i < len(f.selectCursors) {
				selCursor = f.selectCursors[i]
			}
			for j, opt := range field.Options {
				if j == selCursor {
					bullet := StyleGeminiBullet.Render(">")
					optStr := StyleGeminiSelected.Render(opt)
					body.WriteString(fmt.Sprintf("      %s %s\n", bullet, optStr))
				} else {
					optStr := StyleGeminiOption.Render("  " + opt)
					body.WriteString(fmt.Sprintf("      %s\n", optStr))
				}
			}

		case i == f.cursor:
			// 현재 활성 텍스트 필드: fake cursor 삽입
			arrow := StyleGeminiBullet.Render("→")
			labelStr := StyleGeminiSubDesc.Render(labelPadded)
			var displayVal string
			if activeValue == "" && field.Placeholder != "" {
				displayVal = cursorBlock + StyleGeminiSubDesc.Render(field.Placeholder)
			} else {
				runes := []rune(activeValue)
				off := cursorOffset
				if off > len(runes) {
					off = len(runes)
				}
				before := StyleGeminiOption.Render(string(runes[:off]))
				after := StyleGeminiOption.Render(string(runes[off:]))
				displayVal = before + cursorBlock + after
			}
			body.WriteString(fmt.Sprintf("  %s %s %s\n", arrow, labelStr, displayVal))

		default:
			// 미입력 필드
			circle := StyleGeminiOption.Render("○")
			labelStr := StyleGeminiOption.Render(labelPadded)
			displayVal := ""
			if val != "" {
				displayVal = StyleGeminiOption.Render(val)
			}
			body.WriteString(fmt.Sprintf("  %s %s %s\n", circle, labelStr, displayVal))
		}
	}

	body.WriteString("\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorGeminiBox).
		PaddingLeft(1).
		PaddingRight(1).
		Width(innerW + 2)

	return boxStyle.Render(body.String())
}

// Height는 현재 폼 박스의 예상 줄 수를 반환한다.
func (f *formState) Height() int {
	if !f.active {
		return 0
	}
	extra := 0
	for i, field := range f.fields {
		if i == f.cursor && field.FieldType == "select" {
			extra += len(field.Options)
		}
	}
	return len(f.fields) + 5 + extra
}

// ─── TUIFormHandler ────────────────────────────────────────────────────────

// TUIFormHandler는 agent goroutine과 TUI 간의 채널 기반 폼 입력 브릿지다.
type TUIFormHandler struct {
	program *tea.Program
}

// NewTUIFormHandler는 tea.Program을 참조하는 핸들러를 생성한다.
func NewTUIFormHandler(p *tea.Program) *TUIFormHandler {
	return &TUIFormHandler{program: p}
}

// SetProgram은 tea.Program 참조를 갱신한다.
func (h *TUIFormHandler) SetProgram(p *tea.Program) { h.program = p }

// RequestForm은 TUI에 폼 입력 UI를 요청하고 사용자 입력을 기다린다.
func (h *TUIFormHandler) RequestForm(ctx context.Context, req tools.FormRequest) (tools.FormResponse, error) {
	if h.program == nil {
		return tools.FormResponse{Cancelled: true}, nil
	}
	replyCh := make(chan FormResult, 1)
	h.program.Send(FormRequestMsg{
		Title:   req.Title,
		Header:  req.Header,
		Fields:  req.Fields,
		ReplyCh: replyCh,
	})
	select {
	case <-ctx.Done():
		return tools.FormResponse{Cancelled: true}, nil
	case result := <-replyCh:
		return tools.FormResponse{
			Values:    result.Values,
			Cancelled: result.Cancelled,
		}, nil
	}
}
