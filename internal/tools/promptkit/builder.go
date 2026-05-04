// Package promptkit provides helpers for building tool system-prompt guides.
// Two main types:
//   - Sections: builds a multi-section markdown guide (dynamic tools).
//   - LineBuilder: concatenates description fragments (tool Description()).
package promptkit

import (
	"fmt"
	"strings"
)

// LineBuilder builds a single-line tool Description by joining parts with a space.
// Useful when the Description string is assembled conditionally at runtime.
//
//	b := promptkit.NewLine()
//	b.Add("bash 셸 명령을 실행합니다.")
//	if multiWS { b.Add("`server` 필수.") }
//	return b.String()
type LineBuilder struct {
	parts []string
}

// NewLine returns an empty LineBuilder.
func NewLine() *LineBuilder { return &LineBuilder{} }

// Add appends a trimmed fragment. Empty strings are silently ignored.
func (b *LineBuilder) Add(s string) *LineBuilder {
	if t := strings.TrimSpace(s); t != "" {
		b.parts = append(b.parts, t)
	}
	return b
}

// String joins all fragments with a single space.
func (b *LineBuilder) String() string {
	return strings.Join(b.parts, " ")
}

// Sections builds a structured system-prompt guide following the standard
// 6-section layout: When to Use → When NOT to Use → Usage Notes → Examples →
// Tips → Parameters.
//
// Usage:
//
//	s := promptkit.New()
//	s.WhenToUse("3단계 이상의 작업", "복잡한 설치 및 설정")
//	s.WhenNotToUse("단순 한 줄 명령")
//	s.UsageNotes("완료 후 반드시 TaskUpdate로 상태 갱신")
//	s.Examples("Oracle DB 설치 전체 파이프라인 추적")
//	s.Tips("subject는 명령형으로 작성 (예: 'Oracle 19c 설치')")
//	s.Parameters("`subject`: 명령형 짧은 제목 (필수)")
//	return s.Build()
type Sections struct {
	parts []string
}

// New returns an empty Sections builder.
func New() *Sections { return &Sections{} }

// WhenToUse adds the "언제 이 도구를 쓰는가" section.
func (s *Sections) WhenToUse(items ...string) *Sections {
	return s.addBulletSection("언제 이 도구를 쓰는가", items)
}

// WhenNotToUse adds the "언제 쓰지 않는가" section.
func (s *Sections) WhenNotToUse(items ...string) *Sections {
	return s.addBulletSection("언제 쓰지 않는가", items)
}

// UsageNotes adds the "사용 노트" section.
func (s *Sections) UsageNotes(items ...string) *Sections {
	return s.addBulletSection("사용 노트", items)
}

// Examples adds the "예시" numbered section.
func (s *Sections) Examples(items ...string) *Sections {
	return s.addNumberedSection("예시", items)
}

// Tips adds the "팁" section.
func (s *Sections) Tips(items ...string) *Sections {
	return s.addBulletSection("팁", items)
}

// Parameters adds the "매개변수" section.
func (s *Sections) Parameters(items ...string) *Sections {
	return s.addBulletSection("매개변수", items)
}

// AppendNote appends a blockquote-style note ("> **참고**: text").
// Useful for conditional runtime warnings (e.g. plan-mode reminder).
func (s *Sections) AppendNote(text string) *Sections {
	if t := strings.TrimSpace(text); t != "" {
		s.parts = append(s.parts, "> **참고**: "+t)
	}
	return s
}

// AppendRaw appends an arbitrary trimmed markdown string as-is.
func (s *Sections) AppendRaw(text string) *Sections {
	if t := strings.TrimSpace(text); t != "" {
		s.parts = append(s.parts, t)
	}
	return s
}

// Build returns the assembled markdown string. Returns "" if no sections added.
func (s *Sections) Build() string {
	return strings.Join(s.parts, "\n\n")
}

func (s *Sections) addBulletSection(header string, items []string) *Sections {
	filtered := filterEmpty(items)
	if len(filtered) == 0 {
		return s
	}
	var sb strings.Builder
	sb.WriteString("## ")
	sb.WriteString(header)
	for _, item := range filtered {
		sb.WriteString("\n- ")
		sb.WriteString(item)
	}
	s.parts = append(s.parts, sb.String())
	return s
}

func (s *Sections) addNumberedSection(header string, items []string) *Sections {
	filtered := filterEmpty(items)
	if len(filtered) == 0 {
		return s
	}
	var sb strings.Builder
	sb.WriteString("## ")
	sb.WriteString(header)
	for i, item := range filtered {
		fmt.Fprintf(&sb, "\n%d. %s", i+1, item)
	}
	s.parts = append(s.parts, sb.String())
	return s
}

func filterEmpty(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if t := strings.TrimSpace(item); t != "" {
			out = append(out, t)
		}
	}
	return out
}
