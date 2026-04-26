// Package tui
// File: shell_render.go
// Description: 쉘 출력 렌더링 헬퍼 (JSON 포맷, URL 링크, 실행중 표시)
// Responsibility: cmdbox의 쉘 결과를 Claude CLI 스타일로 향상된 포맷으로 렌더링

package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	jsonMaxBytes  = 10240 // JSON 포맷팅 최대 크기 (10KB)
	tailLineCount = 10    // Running 상태에서 보여줄 최근 줄 수
)

var urlRegex = regexp.MustCompile(`https?://[^\s"'<>]+`)

// tryFormatJSON은 문자열이 JSON이면 정렬된 형태로 반환한다.
// JSON이 아니거나 10KB 초과이면 원본을 반환한다.
func tryFormatJSON(s string) string {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) > jsonMaxBytes {
		return s
	}
	if len(trimmed) == 0 {
		return s
	}
	// JSON 객체 또는 배열만 시도
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return s
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return s
	}

	formatted, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return s
	}
	return string(formatted)
}

// linkifyURLs는 텍스트에서 URL을 OSC 8 하이퍼링크로 변환한다.
func linkifyURLs(text string) string {
	return urlRegex.ReplaceAllStringFunc(text, func(url string) string {
		return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, url)
	})
}

// renderRunningShell은 실행 중인 쉘 명령의 상태를 렌더링한다.
// 형식: Running... (3s) + 최근 N줄 + "+M lines" 힌트
func renderRunningShell(cmd string, elapsed time.Duration, lastLines []string, total int) string {
	var b strings.Builder

	// 헤더: Running... (elapsed)
	header := StyleShellRunning.Render("Running...") +
		" " + StyleCmdBoxDim.Render("("+formatElapsedShort(elapsed)+")")
	b.WriteString(header)

	// 명령 미리보기
	if cmd != "" {
		preview := truncateStr(cmd, 60)
		b.WriteString("\n  " + StyleCmdBoxDim.Render(preview))
	}

	// 최근 출력 줄
	start := 0
	if len(lastLines) > tailLineCount {
		start = len(lastLines) - tailLineCount
	}
	for _, line := range lastLines[start:] {
		b.WriteString("\n  " + line)
	}

	// 추가 줄 수 힌트
	if total > tailLineCount {
		hint := fmt.Sprintf("  +%d lines", total-tailLineCount)
		b.WriteString("\n" + StyleCmdBoxDim.Render(hint))
	}

	return b.String()
}

// renderExitCode는 exit code를 색상으로 렌더링한다.
// cmd 인자를 분석하여 특정 명령어(grep, id 등)의 1번 종료 코드를 "결과 없음"으로 완만하게 표현한다.
func renderExitCode(cmd string, code int) string {
	codeStr := strconv.Itoa(code)
	if code == 0 {
		return StyleSuccess.Render("exit " + codeStr)
	}

	// 조사형 명령어 판별
	isProbe := false
	baseCmd := strings.Fields(strings.TrimSpace(cmd))
	if len(baseCmd) > 0 {
		c := strings.ToLower(baseCmd[0])
		// 쉘 내장 명령어 및 일반 조사 도구
		switch c {
		case "grep", "egrep", "fgrep", "id", "which", "ls", "test", "[", "find", "stat", "pgrep":
			isProbe = true
		}
	}

	// 조사형 명령어의 exit 1은 "찾지 못함"이지 에러가 아님
	if isProbe && code == 1 {
		return StyleWarning.Render("— exit " + codeStr + " (no match)")
	}

	return StyleError.Render("✗ exit " + codeStr)
}

// formatElapsedShort는 Duration을 "0.3s", "1m 30s" 형식으로 변환한다.
func formatElapsedShort(d time.Duration) string {
	if d < time.Second {
		ms := d.Milliseconds()
		return fmt.Sprintf("0.%ds", ms/100)
	}
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s = s % 60
	return fmt.Sprintf("%dm %ds", m, s)
}

// truncateStr은 문자열을 maxLen 길이로 자르고 "..."을 추가한다.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// formatShellOutput은 쉘 출력에 JSON 포맷팅과 URL 링크를 적용한다.
func formatShellOutput(output string, verbose bool) string {
	formatted := tryFormatJSON(output)
	if verbose {
		formatted = linkifyURLs(formatted)
	}
	return formatted
}

// renderShellResult는 shell_exec 결과에서 exit code를 파싱하여 컬러링한다.
func renderShellResult(cmd, result string) string {
	// PowerShell 모듈 준비 중 메시지(CLIXML)가 섞여있으면 무시하거나 정제
	if strings.Contains(result, "Preparing modules for first use") && strings.Contains(result, "#< CLIXML") {
		// 해당 블록을 제거하거나 텍스트만 남김 (여기서는 간단히 렌더링 시점에만 처리)
	}

	lines := strings.Split(result, "\n")
	if len(lines) == 0 {
		return result
	}

	first := lines[0]
	restStart := 1
	firstStyled := first

	if strings.HasPrefix(first, "Execution Context: ") {
		firstStyled = StyleCmdBoxDim.Render(first)
		if len(lines) > 1 {
			exitLine := lines[1]
			if strings.HasPrefix(exitLine, "[Exit Code: 0]") {
				firstStyled += "\n" + StyleSuccess.Render(exitLine)
			} else if strings.HasPrefix(exitLine, "[Exit Code: 1]") {
				// 조사형 명령어 특수 처리
				firstStyled += "\n" + renderExitCode(cmd, 1)
			} else if strings.HasPrefix(exitLine, "[Exit Code:") {
				firstStyled += "\n" + StyleError.Render(exitLine)
			} else {
				firstStyled += "\n" + exitLine
			}
			restStart = 2
		}
	} else if strings.HasPrefix(first, "[Exit Code: 0]") {
		firstStyled = StyleSuccess.Render(first)
	} else if strings.HasPrefix(first, "[Exit Code: 1]") {
		firstStyled = renderExitCode(cmd, 1)
	} else if strings.HasPrefix(first, "[Exit Code:") {
		firstStyled = StyleError.Render(first)
	}

	var filteredLines []string
	for i := restStart; i < len(lines); i++ {
		line := lines[i]
		// CLIXML 메시지 필터링
		if strings.Contains(line, "#< CLIXML") || strings.Contains(line, "<Objs Version=") || strings.Contains(line, "Preparing modules") {
			continue
		}
		filteredLines = append(filteredLines, line)
	}

	if len(filteredLines) == 0 {
		return firstStyled
	}

	return firstStyled + "\n" + strings.Join(filteredLines, "\n")
}

// renderFileReadResult는 file_read 결과에 줄 번호를 추가한다.
func renderFileReadResult(result string) string {
	lines := strings.Split(result, "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		num := StyleDiffLineNum.Render(fmt.Sprintf("%4d │ ", i+1))
		out = append(out, num+line)
	}
	return strings.Join(out, "\n")
}
