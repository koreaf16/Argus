// Package todo
// File: render.go
// Description: TodoItem 목록을 마크다운 체크리스트로 렌더링
// Responsibility: Render() — [ ]/[x]/[-] 마크다운 출력 생성

package todo

import (
	"strings"
)

// Render 는 items 를 마크다운 체크리스트 문자열로 변환한다.
func Render(items []TodoItem) string {
	var sb strings.Builder
	for _, it := range items {
		sb.WriteString(checkMark(it.Status))
		sb.WriteString(" ")
		sb.WriteString(it.Content)
		if len(it.Deps) > 0 {
			sb.WriteString(" _(deps: ")
			sb.WriteString(strings.Join(it.Deps, ", "))
			sb.WriteString(")_")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func checkMark(s Status) string {
	switch s {
	case StatusCompleted:
		return "- [x]"
	case StatusInProgress:
		return "- [-]"
	default:
		return "- [ ]"
	}
}
