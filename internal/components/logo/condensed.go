// Package logo — Argus 브랜드 스플래시 렌더러.
//
// 파일 역할: 세션 시작 시 스크롤백에 1회 출력되는 Constellation 스타일 스플래시를
// 렌더한다. 디자인 컨셉은 "100개의 눈을 가진 파수꾼" 으로, 별자리(starfield) 사이에
// ARGUS 로고 블록을 배치하고 하단에 제작자 리본 서명을 둔다.
//
// 포함 모듈:
//   - Data: 로고에 필요한 데이터 구조체(호출부 호환 목적으로 기존 필드 보존).
//   - Render(d Data, aiDebug bool) string: 완성된 스플래시 문자열 반환.
//
// 호출/사용 방식:
//   - internal/tui/tui.go 에서 세션 시작 시 p.Println() 으로 1회 출력.
//   - internal/tui/render.go 의 renderLogoBlock() 에서 호출.
//   - cmd/argus/init_menu.go 의 init 오버뷰 출력에서도 호출.
//
// 연결:
//   - internal/components/logo/argus_icon.go: --version 출력용 소형 아이콘.
//   - github.com/charmbracelet/lipgloss: 컬러/스타일링.
package logo

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Data 는 스플래시 렌더에 필요한 데이터다.
// 현재 스플래시는 의도적으로 시스템 정보를 노출하지 않으므로 대부분 필드는
// 호출부 호환 목적으로 보존한다(실제 렌더에는 사용되지 않음).
type Data struct {
	Version      string
	ModelDisplay string
	EffortLevel  string
	ProviderName string
	Cwd          string
	SourcePath   string
	AgentName    string
	Columns      int
}

// starfield 그라데이션 색상 팔레트(보라→분홍→주황→황금→초록→청록→파랑).
var gradPalette = []lipgloss.Color{
	lipgloss.Color("#9b72cb"),
	lipgloss.Color("#d96570"),
	lipgloss.Color("#f49b4f"),
	lipgloss.Color("#f5c451"),
	lipgloss.Color("#61cf8e"),
	lipgloss.Color("#5cc4d6"),
	lipgloss.Color("#4492e6"),
}

var (
	starFaintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c6c6c"))
	taglineStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7a7a7a")).Italic(true)
	ribbonStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d4a14a"))
	ribbonBrand    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d4a14a")).Bold(true)
	ribbonItalic   = lipgloss.NewStyle().Foreground(lipgloss.Color("#d4a14a")).Italic(true)
	ribbonSubtle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7a7a7a"))
)

// ARGUS 로고 블록(반블럭 문자, 3행).
var argusLogoRows = []string{
	"█▀▀█ █▀▀█ █▀▀▀ █  █ █▀▀",
	"█▄▄█ █▄▄▀ █ ▀█ █  █ ▀▀█",
	"█  █ █  █ ▀▀▀▀  ▀▀  ▀▀▀",
}

// 상단/하단 별자리 행(각 3행). 패턴은 고정이되 컬러는 렌더 시점 seed 로 순환.
var topStarRows = []string{
	"    ·      ∙        ·       ·         ∙        ·      ∙      ·       ∙",
	"  ∙    ●        ·        ●       ·       ∙       ●       ·        ●",
	"       ·       ∙      ·       ∙       ·        ∙      ·      ·       ∙",
}

var bottomStarRows = []string{
	"   ·       ∙       ·         ·        ∙        ·       ∙       ·      ·",
	"     ●         ∙       ●         ·       ●        ·        ●        ∙",
	"  ∙        ·        ·        ∙       ·       ·        ∙       ·      ∙",
}

const (
	creatorKorean  = "곽 지 훈"
	creatorRoman   = "Jihoon Kwak"
	creatorTagline = "· one hundred eyes · never sleeps ·"
)

// Render 는 Argus 스플래시 문자열을 반환한다.
// aiDebug=true 이면 컬러 없이 plaintext 로 반환한다.
func Render(d Data, aiDebug bool) string {
	cols := d.Columns
	if cols <= 0 {
		cols = 80
	}

	if aiDebug {
		return renderPlain(cols)
	}

	var b strings.Builder
	for i, row := range topStarRows {
		b.WriteString(centerLine(colorizeStars(row, i*3), cols))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')

	for i, row := range argusLogoRows {
		b.WriteString(centerLine(gradientLine(row, i), cols))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')

	b.WriteString(centerLine(taglineStyle.Render(creatorTagline), cols))
	b.WriteByte('\n')
	b.WriteByte('\n')

	for i, row := range bottomStarRows {
		b.WriteString(centerLine(colorizeStars(row, i*5+2), cols))
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(centerLine(ribbonLine(cols), cols))
	b.WriteByte('\n')
	b.WriteString(centerLine(ribbonSubtle.Render(creatorRoman), cols))
	b.WriteByte('\n')

	return b.String()
}

// renderPlain 은 --aidebug / 비-TTY 환경용 무채색 버전을 반환한다.
func renderPlain(cols int) string {
	var b strings.Builder
	for _, row := range topStarRows {
		// 특수 문자들을 ASCII로 대체
		row = strings.ReplaceAll(row, "●", "o")
		row = strings.ReplaceAll(row, "·", ".")
		row = strings.ReplaceAll(row, "∙", ".")
		b.WriteString(centerPlain(row, cols))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	for _, row := range argusLogoRows {
		b.WriteString(centerPlain(row, cols))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	// tagline에서도 특수 문자 제거
	plainTagline := strings.ReplaceAll(creatorTagline, "·", "-")
	b.WriteString(centerPlain(plainTagline, cols))
	b.WriteByte('\n')
	b.WriteByte('\n')
	for _, row := range bottomStarRows {
		row = strings.ReplaceAll(row, "●", "o")
		row = strings.ReplaceAll(row, "·", ".")
		row = strings.ReplaceAll(row, "∙", ".")
		b.WriteString(centerPlain(row, cols))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(centerPlain("---  crafted by  "+creatorKorean+"  ---", cols))
	b.WriteByte('\n')
	b.WriteString(centerPlain(creatorRoman, cols))
	b.WriteByte('\n')
	return b.String()
}

// ribbonLine 은 폭에 따라 길이가 달라지는 금색 리본 서명을 반환한다.
//
//	╾─────────✦   crafted by  곽 지 훈   ✦─────────╼
func ribbonLine(cols int) string {
	core := ribbonItalic.Render("crafted by") + "  " + ribbonBrand.Render(creatorKorean)
	coreWidth := lipgloss.Width(core)

	// 리본 끝단 장식 "✦"/"╾"/"╼" 포함, 좌우 대칭.
	decorateW := 2 // "✦ " + " ✦" 각 끝
	gap := 3       // 코어와 별 사이 공백
	target := cols - 8
	if target < coreWidth+decorateW+gap*2+6 {
		// 폭이 너무 좁으면 간소 버전.
		return ribbonStyle.Render("─ ") +
			ribbonItalic.Render("crafted by ") +
			ribbonBrand.Render(creatorKorean) +
			ribbonStyle.Render(" ─")
	}

	dashTotal := target - coreWidth - decorateW*2 - gap*2
	if dashTotal < 6 {
		dashTotal = 6
	}
	leftDash := dashTotal / 2
	rightDash := dashTotal - leftDash

	left := ribbonStyle.Render("╾"+strings.Repeat("─", leftDash)+"✦") + strings.Repeat(" ", gap)
	right := strings.Repeat(" ", gap) + ribbonStyle.Render("✦"+strings.Repeat("─", rightDash)+"╼")

	return left + core + right
}

// gradientLine 은 문자열의 각 글자에 팔레트 색을 순환 적용한다(공백은 유지).
func gradientLine(s string, offset int) string {
	var b strings.Builder
	i := offset
	bold := lipgloss.NewStyle().Bold(true)
	for _, r := range s {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		color := gradPalette[i%len(gradPalette)]
		b.WriteString(bold.Foreground(color).Render(string(r)))
		i++
	}
	return b.String()
}

// colorizeStars 는 별 필드 한 줄을 렌더한다.
//
//	● : 팔레트 색 순환
//	·, ∙ : 은은한 회색
func colorizeStars(row string, seed int) string {
	var b strings.Builder
	idx := seed
	for _, r := range row {
		switch r {
		case '●':
			color := gradPalette[idx%len(gradPalette)]
			b.WriteString(lipgloss.NewStyle().Foreground(color).Render(string(r)))
			idx++
		case '·', '∙':
			b.WriteString(starFaintStyle.Render(string(r)))
		default:
			b.WriteString(string(r))
		}
	}
	return b.String()
}

// centerLine 은 가시 폭(visible width)을 기준으로 좌측에 패딩을 붙여 중앙 정렬한다.
func centerLine(s string, cols int) string {
	w := lipgloss.Width(s)
	if w >= cols {
		return s
	}
	return strings.Repeat(" ", (cols-w)/2) + s
}

// centerPlain 은 ANSI 시퀀스가 없는 plain 문자열의 중앙 정렬 버전이다.
func centerPlain(s string, cols int) string {
	w := len([]rune(s))
	if w >= cols {
		return s
	}
	return strings.Repeat(" ", (cols-w)/2) + s
}
