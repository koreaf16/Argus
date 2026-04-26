// Package pipeline
// File: truncate.go
// Description: 명령 출력 스마트 트렁케이션
// Responsibility: 출력이 길 때 head + tail을 보존하고 중간을 생략하는 함수 제공

package pipeline

import (
	"fmt"
	"strings"
)

const (
	// DefaultHeadLines는 SmartTruncate 기본 head 보존 라인 수이다.
	DefaultHeadLines = 50
	// DefaultTailLines는 SmartTruncate 기본 tail 보존 라인 수이다.
	DefaultTailLines = 50

	// omitMsgReserve는 생략 메시지에 예약하는 바이트 수이다 (한글 포함 여유분).
	omitMsgReserve = 80
)

// SmartTruncate는 라인 목록을 maxBytes 이하로 잘라낸다.
// 전체가 maxBytes 이내면 그대로 반환한다.
// 초과하면 앞 headLines + 뒤 tailLines를 보존하고 중간에 생략 메시지를 삽입한다.
// tail이 우선순위: head를 줄여서라도 tail(에러 메시지가 자주 위치)을 보존한다.
// tailLines가 0이면 head-only 동작과 동일하다.
func SmartTruncate(lines []string, maxBytes, headLines, tailLines int) string {
	if headLines <= 0 {
		headLines = DefaultHeadLines
	}
	if tailLines < 0 {
		tailLines = 0
	}

	joined := strings.Join(lines, "\n")
	if len(joined) <= maxBytes {
		return joined
	}

	// tail 섹션 결정 (head와 겹치지 않도록)
	tailStart := len(lines) - tailLines
	tailStart = max(tailStart, 0)
	var tail []string
	if tailLines > 0 && tailStart < len(lines) {
		tail = lines[tailStart:]
	}
	tailSection := strings.Join(tail, "\n")

	// tail이 maxBytes를 단독으로 초과하면 tail 끝 부분만 바이트 컷 (끝 보존)
	if len(tailSection) >= maxBytes {
		cutFrom := max(len(tailSection)-maxBytes, 0)
		return "...\n" + tailSection[cutFrom:]
	}

	// head에 할당 가능한 바이트: maxBytes - tail - 생략 메시지 예약
	headBudget := max(maxBytes-len(tailSection)-omitMsgReserve, 0)

	// head 라인을 budget 안에 맞춤
	var head []string
	if headLines > 0 && len(lines) > tailLines {
		headMax := min(len(lines)-tailLines, headLines)
		for i := range headMax {
			next := strings.Join(append(head, lines[i]), "\n")
			if len(next) > headBudget {
				break
			}
			head = append(head, lines[i])
		}
	}

	// headStart=len(head), tailStart 사이의 생략된 라인 수
	omitted := len(lines) - len(head) - len(tail)
	if tailStart < len(head) {
		omitted = 0
	}

	var sb strings.Builder
	if len(head) > 0 {
		sb.WriteString(strings.Join(head, "\n"))
	}
	if omitted > 0 {
		fmt.Fprintf(&sb, "\n... [%d 라인 생략] ...\n", omitted)
	} else if len(head) > 0 && len(tail) > 0 {
		sb.WriteByte('\n')
	}
	if len(tail) > 0 {
		sb.WriteString(tailSection)
	}

	return sb.String()
}

// SmartTruncateString은 단일 문자열을 라인 분리 후 SmartTruncate를 적용한다.
func SmartTruncateString(s string, maxBytes, headLines, tailLines int) string {
	if len(s) <= maxBytes {
		return s
	}
	lines := strings.Split(s, "\n")
	return SmartTruncate(lines, maxBytes, headLines, tailLines)
}
