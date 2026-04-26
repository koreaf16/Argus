// Package logo — Argus 브랜드 아이콘 및 condensed 로고 렌더러.
//
// 파일 역할: 터미널에 출력되는 Argus 고유 ASCII 아이콘(3행, 9-col)을 렌더링한다.
// 포함 모듈:
//   - RenderIcon(aiDebug bool) string: Argus 아이콘 문자열 반환.
//
// 호출/사용 방식:
//   - condensed.go 의 Render() 에서 좌측 컬럼으로 사용.
//
// 연결:
//   - github.com/charmbracelet/lipgloss: 컬러 스타일링.
package logo

import "github.com/charmbracelet/lipgloss"

// bodyColor 는 아이콘 본체 색상.
var bodyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))

// RenderIcon 은 Argus 아이콘 4행 ASCII 문자열을 반환한다.
// aiDebug=true 이면 컬러 없이 plaintext 반환.
func RenderIcon(aiDebug bool) string {
	// gemini-cli 스타일의 세련된 아이콘 구조
	rows := []string{
		"▝▜▄  ",
		"  ▝▜▄",
		" ▗▟▀ ",
		"▝▀   ",
	}

	if aiDebug {
		return rows[0] + "\n" + rows[1] + "\n" + rows[2] + "\n" + rows[3]
	}

	// 그라데이션 색상 (theme.go의 GlimmerTrailColors 참고)
	colors := []string{"#9b72cb", "#d96570", "#f49b4f", "#61cf8e"}

	var renderedRows []string
	for i, row := range rows {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(colors[i%len(colors)]))
		renderedRows = append(renderedRows, style.Render(row))
	}

	return renderedRows[0] + "\n" +
		renderedRows[1] + "\n" +
		renderedRows[2] + "\n" +
		renderedRows[3]
}
