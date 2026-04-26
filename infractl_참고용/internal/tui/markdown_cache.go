// Package tui
// File: markdown_cache.go
// Description: 스트리밍 마크다운의 stablePrefix 캐시 관리
// Responsibility: 완성된 블록 경계를 감지하고 캐시/tail 분리

package tui

import "strings"

// stableCache는 스트리밍 마크다운의 안정적인 부분과 변동 부분을 분리 캐시한다.
type stableCache struct {
	stableLines []string // 마지막 완성 블록까지의 렌더 결과
	stableEnd   int      // 안정 영역의 바이트 오프셋
	tailLines   []string // stableEnd 이후 tail 렌더 결과
}

// Lines는 전체 렌더 결과를 반환한다 (stable + tail).
func (c *stableCache) Lines() []string {
	result := make([]string, 0, len(c.stableLines)+len(c.tailLines))
	result = append(result, c.stableLines...)
	result = append(result, c.tailLines...)
	return result
}

// Reset은 캐시를 초기화한다.
func (c *stableCache) Reset() {
	c.stableLines = nil
	c.stableEnd = 0
	c.tailLines = nil
}

// findStableEnd는 마크다운 텍스트에서 마지막 완성된 블록 경계의 바이트 오프셋을 반환한다.
// 완성된 블록: 빈 줄(\n\n)로 구분되며, 열린 코드블록이 없는 지점.
func findStableEnd(md string) int {
	if len(md) == 0 {
		return 0
	}

	// 마지막 빈 줄 경계를 역순으로 찾되, 코드블록이 열려있지 않은 지점까지
	lastDoubleNewline := -1
	for i := len(md) - 1; i > 0; i-- {
		if md[i] == '\n' && i > 0 && md[i-1] == '\n' {
			candidate := i + 1
			prefix := md[:candidate]
			if !hasOpenCodeFence(prefix) {
				lastDoubleNewline = candidate
				break
			}
		}
	}

	if lastDoubleNewline <= 0 {
		return 0
	}
	return lastDoubleNewline
}

// hasOpenCodeFence는 텍스트에 닫히지 않은 코드 펜스(```)가 있는지 확인한다.
func hasOpenCodeFence(text string) bool {
	count := strings.Count(text, "```")
	return count%2 != 0
}

// closeDanglingFences는 미완성 마크다운의 열린 코드 펜스를 닫는다.
func closeDanglingFences(content string) string {
	if hasOpenCodeFence(content) {
		content += "\n```"
	}
	return content
}
