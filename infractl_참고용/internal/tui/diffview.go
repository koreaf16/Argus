// Package tui
// File: diffview.go
// Description: 라인번호 gutter 포함 structured diff 렌더링
// Responsibility: unified diff를 라인번호 + 색상으로 변환하여 터미널에 표시

package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var hunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// RenderDiff는 unified diff를 라인번호 gutter + 색상으로 렌더링한다.
//
//	10 │ -old line content
//	11 │ +new line content
//	   │  unchanged context
//	   │  ...
//	28 │ @@ -28,4 +30,6 @@
func RenderDiff(content string, width int) string {
	if content == "" {
		return content
	}

	lines := strings.Split(content, "\n")

	// 최대 라인번호를 계산하여 gutter 폭 결정
	maxLineNo := findMaxLineNo(lines)
	gutterW := len(strconv.Itoa(maxLineNo)) + 1
	if gutterW < 3 {
		gutterW = 3
	}

	pipe := StyleDiffGutter.Render(" │ ")
	var oldLine, newLine int
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			result = append(result, "")
			continue
		}

		switch {
		case strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ "):
			gutter := StyleDiffGutter.Render(strings.Repeat(" ", gutterW))
			result = append(result, gutter+pipe+StyleDiffHeader.Render(line))

		case strings.HasPrefix(line, "@@ "):
			m := hunkHeaderRegex.FindStringSubmatch(line)
			if len(m) >= 3 {
				oldLine, _ = strconv.Atoi(m[1])
				newLine, _ = strconv.Atoi(m[2])
			}
			// hunk 구분자
			gutter := StyleDiffGutter.Render(strings.Repeat(" ", gutterW))
			sep := StyleDiffGutter.Render(strings.Repeat("·", max(width-gutterW-3, 5)))
			result = append(result, gutter+pipe+sep)
			result = append(result, gutter+pipe+StyleDiffHunk.Render(line))

		case strings.HasPrefix(line, "+"):
			gutter := formatGutter(newLine, gutterW)
			result = append(result, gutter+pipe+StyleDiffAdd.Render(line))
			newLine++

		case strings.HasPrefix(line, "-"):
			gutter := formatGutter(oldLine, gutterW)
			result = append(result, gutter+pipe+StyleDiffDel.Render(line))
			oldLine++

		default:
			// context line
			gutter := formatGutter(newLine, gutterW)
			result = append(result, gutter+pipe+line)
			oldLine++
			newLine++
		}
	}

	return strings.Join(result, "\n")
}

// formatGutter는 라인번호를 우측 정렬된 gutter 문자열로 반환한다.
func formatGutter(lineNo int, width int) string {
	if lineNo <= 0 {
		return StyleDiffGutter.Render(strings.Repeat(" ", width))
	}
	num := strconv.Itoa(lineNo)
	pad := width - len(num)
	if pad < 0 {
		pad = 0
	}
	return StyleDiffGutter.Render(strings.Repeat(" ", pad) + num)
}

// findMaxLineNo는 diff에서 예상되는 최대 라인번호를 추정한다.
func findMaxLineNo(lines []string) int {
	maxLine := 0
	for _, line := range lines {
		m := hunkHeaderRegex.FindStringSubmatch(line)
		if len(m) >= 3 {
			old, _ := strconv.Atoi(m[1])
			new_, _ := strconv.Atoi(m[2])
			lineCount := 0
			for _, l := range lines {
				if !strings.HasPrefix(l, "---") && !strings.HasPrefix(l, "+++") && !strings.HasPrefix(l, "@@") {
					lineCount++
				}
			}
			est := max(old, new_) + lineCount
			if est > maxLine {
				maxLine = est
			}
		}
	}
	if maxLine == 0 {
		return 100 // 기본값
	}
	return maxLine
}

// hunkSeparator는 hunk 간 구분선을 생성한다.
func hunkSeparator(gutterW int) string {
	gutter := StyleDiffGutter.Render(strings.Repeat(" ", gutterW))
	pipe := StyleDiffGutter.Render(" │ ")
	dots := StyleDiffGutter.Render(fmt.Sprintf("%-5s", "..."))
	return gutter + pipe + dots
}
