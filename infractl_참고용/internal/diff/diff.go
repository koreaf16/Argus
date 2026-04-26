// Package diff
// File: diff.go
// Description: unified diff 생성 및 감지 유틸리티
// Responsibility: 두 텍스트를 비교하여 unified diff를 생성하고, 문자열이 diff 형식인지 감지

package diff

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// GenerateUnifiedDiff는 old와 new 두 텍스트를 비교하여 unified diff 포맷 문자열을 반환한다.
// 변경 사항이 없으면 빈 문자열을 반환한다.
func GenerateUnifiedDiff(old, new_, filename string) string {
	if old == new_ {
		return ""
	}

	dmp := diffmatchpatch.New()
	a, b, lines := dmp.DiffLinesToChars(old, new_)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lines)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n", filename))
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", filename))

	for _, d := range diffs {
		text := strings.TrimSuffix(d.Text, "\n")
		if text == "" {
			continue
		}
		lineSlice := strings.Split(text, "\n")
		for _, line := range lineSlice {
			switch d.Type {
			case diffmatchpatch.DiffDelete:
				sb.WriteString("-" + line + "\n")
			case diffmatchpatch.DiffInsert:
				sb.WriteString("+" + line + "\n")
			case diffmatchpatch.DiffEqual:
				sb.WriteString(" " + line + "\n")
			}
		}
	}

	return sb.String()
}

// IsDiff는 content가 unified diff 형식인지 감지한다.
// "---", "+++"로 시작하는 헤더 줄이 모두 있으면 true를 반환한다.
func IsDiff(content string) bool {
	if content == "" {
		return false
	}
	hasOld := false
	hasNew := false
	for _, line := range strings.SplitN(content, "\n", 10) {
		if strings.HasPrefix(line, "--- ") {
			hasOld = true
		}
		if strings.HasPrefix(line, "+++ ") {
			hasNew = true
		}
		if hasOld && hasNew {
			return true
		}
	}
	return false
}
