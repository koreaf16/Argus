// Package main — CLI help text output.
//
// 파일 역할: `--help` / `-h` 호출 시 Argus의 사용법과 주요 경로를 출력한다.
// 포함 모듈:
//   - printHelp: 전체 도움말 문자열 출력.
//
// 호출/사용 방식:
//   - cmd/argus/main.go 의 main()이 help 플래그에서 호출한다.
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): constants.
//   - 이 파일을 import 하는 주요 패키지: 없음.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/constants"
)

var (
	helpTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#4492e6")).Bold(true)
	helpAccentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("70")).Bold(true)
	helpDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpBoxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2)
)

func printHelp(w io.Writer) {
	header := fmt.Sprintf("%s (%s) - Claude CLI style Go agent",
		helpTitleStyle.Render(constants.DisplayName),
		helpDimStyle.Render(constants.Version))

	usage := fmt.Sprintf("%s\n  %s [flags]\n  Run without flags to enter the scrollback TUI.",
		helpTitleStyle.Render("Usage"),
		constants.Name)

	flags := []string{
		helpTitleStyle.Render("Flags"),
		fmt.Sprintf("  %s, %s              print help", helpAccentStyle.Render("-h"), helpAccentStyle.Render("--help")),
		fmt.Sprintf("  %s, %s           print version/build info", helpAccentStyle.Render("-v"), helpAccentStyle.Render("--version")),
		fmt.Sprintf("      %s              interactive setup and config bootstrap", helpAccentStyle.Render("--init")),
		fmt.Sprintf("      %s %s     set active model alias for this session", helpAccentStyle.Render("--model"), helpDimStyle.Render("<alias>")),
		fmt.Sprintf("  %s, %s %s       resume a saved session by ID", helpAccentStyle.Render("-r"), helpAccentStyle.Render("--resume"), helpDimStyle.Render("<id>")),
		fmt.Sprintf("      %s %s    single prompt mode (planned)", helpAccentStyle.Render("--print"), helpDimStyle.Render("<prompt>")),
		fmt.Sprintf("      %s           emit NDJSON analysis trace (stdout + session file)", helpAccentStyle.Render("--aidebug")),
		fmt.Sprintf("      %s      auto-approve tool permission prompts", helpAccentStyle.Render("--auto-approve")),
	}

	env := []string{
		helpTitleStyle.Render("Environment"),
		fmt.Sprintf("  %s       Anthropic API key", helpAccentStyle.Render("ANTHROPIC_API_KEY")),
		fmt.Sprintf("  %s          Gemini API key", helpAccentStyle.Render("GEMINI_API_KEY")),
		fmt.Sprintf("  %s           OpenAI or compatible API key", helpAccentStyle.Render("OPENAI_API_KEY")),
	}

	paths := []string{
		helpTitleStyle.Render("Paths"),
		fmt.Sprintf("  config dir:   %s", helpDimStyle.Render(constants.ConfigDir())),
		fmt.Sprintf("  models:       %s", helpDimStyle.Render(constants.ModelsPath())),
		fmt.Sprintf("  settings:     %s", helpDimStyle.Render(constants.SettingsPath())),
		fmt.Sprintf("  history:      %s", helpDimStyle.Render(constants.HistoryPath())),
	}

	content := strings.Join([]string{
		header,
		"",
		usage,
		"",
		strings.Join(flags, "\n"),
		"",
		strings.Join(env, "\n"),
		"",
		strings.Join(paths, "\n"),
	}, "\n")

	fmt.Fprintln(w, helpBoxStyle.Render(content))
}
