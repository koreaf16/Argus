// Package infrainit
// File: prompts.go
// Description: 위저드용 터미널 I/O 헬퍼 함수 모음
// Responsibility: 표준 입력으로부터 텍스트, Y/N, 번호 선택을 읽는 함수 제공

package infrainit

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourorg/infractl/internal/tui"
)

// promptText는 라벨을 출력하고 한 줄을 읽어 반환한다.
// 빈 입력이면 defaultVal을 반환한다.
func promptText(label, defaultVal string) (string, bool) {
	q := label
	if defaultVal != "" {
		q = fmt.Sprintf("%s [%s]", label, defaultVal)
	}

	result := tui.RunSelect(q, []tui.SelectOption{}, 80)
	if result.IsBack {
		return "", true
	}
	if result.Label == "" {
		return defaultVal, false
	}
	return result.Label, false
}

// promptSecret는 Gemini 박스 스타일로 API Key 등 존감한 값을 입력받는다.
func promptSecret(label string) (string, bool) {
	// Secret 입력도 이제 readFreeText (RawMode)를 통해 뒤로 가기가 가능하게 함
	// 단, 입력값이 별표로 가려지지는 않으므로(현재 구현상) 나중에 개선 필요할 수 있음
	result := tui.RunSelect(label+" (비밀번호/키 입력)", []tui.SelectOption{}, 80)
	if result.IsBack {
		return "", true
	}
	return strings.TrimSpace(result.Label), false
}

// promptYN은 Y/N 질문을 출력하고 bool을 반환한다.
func promptYN(question string, defaultYes bool) (bool, bool) {
	opts := []tui.SelectOption{
		{Label: "Yes", HideOther: true},
		{Label: "No", HideOther: true},
	}
	if !defaultYes {
		opts = []tui.SelectOption{
			{Label: "No", HideOther: true},
			{Label: "Yes", HideOther: true},
		}
	}
	
	result := tui.RunSelect(question, opts, 80)
	if result.IsBack {
		return false, true
	}
	return result.Label == "Yes" || strings.ToLower(result.Label) == "y" || strings.ToLower(result.Label) == "yes", false
}

// promptSelect는 목록을 번호로 출력하고 선택된 0-based 인덱스를 반환한다.
func promptSelect(question string, options []string) (int, bool) {
	opts := make([]tui.SelectOption, len(options))
	for i, o := range options {
		opts[i] = tui.SelectOption{Label: o, HideOther: true}
	}
	result := tui.RunSelect(question, opts, 80)
	if result.IsBack {
		return -1, true
	}
	if result.Index < 0 {
		return 0, false
	}
	return result.Index, false
}

// printSectionHeader는 Gemini 스타일로 단계 헤더를 출력한다.
func printSectionHeader(step, title string) {
	borderColor := lipgloss.NewStyle().Foreground(tui.ColorGeminiBox)
	sep := strings.Repeat("─", 50)
	fmt.Println()
	fmt.Println(borderColor.Render("╭"+sep+"╮"))
	fmt.Println(borderColor.Render("│") + " " +
		tui.StyleGeminiHeader.Render(step) + 
		tui.StyleGeminiSubDesc.Render(" — "+title))
	fmt.Println(borderColor.Render("╰"+sep+"╯"))
}

// printSuccess는 Gemini 스타일로 성공 메시지를 출력한다.
func printSuccess(msg string) {
	fmt.Println()
	fmt.Println(tui.StyleSuccess.Render("✓ ") + tui.StyleGeminiHeader.Render(msg))
}