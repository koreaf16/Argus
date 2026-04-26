// Package main — 버전 정보 출력.
//
// 파일 역할: --version / -v 호출 시 제품명·버전·커밋·Go 런타임 정보를 한 줄 요약으로 출력.
// 포함 모듈:
//   - printVersion(w): 버전 문자열을 w 로 출력.
//
// 호출/사용 방식:
//   - cmd/argus/main.go 의 main() 이 flags.version == true 일 때 호출.
//
// 연결:
//   - internal/constants: Name, Version, Commit 상수.
//   - runtime: Go 버전·OS/ARCH 정보.
package main

import (
	"fmt"
	"io"
	"runtime"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/components/logo"
	"github.com/koreaf16/argus/internal/constants"
)

// printVersion 은 아이콘과 함께 세련된 형식으로 버전 정보를 w 로 출력한다.
func printVersion(w io.Writer) {
	icon := logo.RenderIcon(false)

	titleS := lipgloss.NewStyle().Foreground(lipgloss.Color("#4492e6")).Bold(true)
	verS := lipgloss.NewStyle().Foreground(lipgloss.Color("70")).Bold(true)
	dimS := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	info := fmt.Sprintf("%s %s\n%s\n%s",
		titleS.Render(constants.DisplayName),
		verS.Render("v"+constants.Version),
		dimS.Render("commit "+constants.Commit),
		dimS.Render(fmt.Sprintf("%s/%s, %s", runtime.GOOS, runtime.GOARCH, runtime.Version())),
	)

	fmt.Fprintln(w, lipgloss.JoinHorizontal(lipgloss.Top, icon, "  ", info))
}
