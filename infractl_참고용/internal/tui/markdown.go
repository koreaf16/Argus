// Package tui
// File: markdown.go
// Description: goldmark 기반 마크다운 렌더러 진입점
// Responsibility: 마크다운 텍스트를 줄 슬라이스로 변환하는 공개 API 제공

package tui

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// mdRenderer는 goldmark 기반 마크다운 렌더러이다.
// glamour를 대체하며, 반환 타입이 []string이므로 줄 수를 정확히 제어할 수 있다.
type mdRenderer struct {
	width int
	gm    goldmark.Markdown
}

// newMdRenderer는 지정된 터미널 폭의 렌더러를 생성한다.
func newMdRenderer(width int) *mdRenderer {
	if width < 20 {
		width = 80
	}
	ar := newAnsiRenderer(width)
	gm := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(ar, 1),
			),
		),
	)
	return &mdRenderer{width: width, gm: gm}
}

// Render는 완성된 마크다운을 줄 슬라이스로 변환한다.
func (r *mdRenderer) Render(md string) []string {
	if md == "" {
		return nil
	}
	ar := newAnsiRenderer(r.width)
	gm := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(ar, 1),
			),
		),
	)
	var buf bytes.Buffer
	if err := gm.Convert([]byte(md), &buf); err != nil {
		return strings.Split(md, "\n")
	}
	return trimTrailingEmpty(ar.lines)
}

// RenderStreaming은 스트리밍 중인 마크다운을 stablePrefix 캐시를 활용하여 렌더링한다.
// cache의 stableEnd까지는 이전 렌더 결과를 재사용하고, 이후 tail만 재파싱한다.
func (r *mdRenderer) RenderStreaming(md string, cache *stableCache) []string {
	if md == "" {
		return nil
	}

	stableEnd := findStableEnd(md)

	// stable 부분이 이전과 같으면 캐시 재사용
	if stableEnd > 0 && stableEnd == cache.stableEnd && len(cache.stableLines) > 0 {
		tail := md[stableEnd:]
		tail = closeDanglingFences(tail)
		cache.tailLines = r.Render(tail)
		return cache.Lines()
	}

	// stable 부분이 변경되었거나 처음이면 전체 재렌더
	if stableEnd > 0 {
		stable := md[:stableEnd]
		cache.stableLines = r.Render(stable)
		cache.stableEnd = stableEnd

		tail := md[stableEnd:]
		tail = closeDanglingFences(tail)
		cache.tailLines = r.Render(tail)
		return cache.Lines()
	}

	// stable 경계 없음 — 전체를 tail로 렌더
	cache.stableLines = nil
	cache.stableEnd = 0
	safe := closeDanglingFences(md)
	cache.tailLines = r.Render(safe)
	return cache.Lines()
}

// trimTrailingEmpty는 슬라이스 끝의 빈 줄을 제거한다.
func trimTrailingEmpty(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
