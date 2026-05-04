package promptkit_test

import (
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/tools/promptkit"
)

func TestLineBuilder(t *testing.T) {
	b := promptkit.NewLine()
	b.Add("bash 명령 실행.")
	b.Add("")       // ignored
	b.Add("  \t  ") // whitespace-only ignored
	b.Add("`server` 필수.")
	got := b.String()
	want := "bash 명령 실행. `server` 필수."
	if got != want {
		t.Errorf("LineBuilder.String() = %q, want %q", got, want)
	}
}

func TestLineBuilderEmpty(t *testing.T) {
	b := promptkit.NewLine()
	if got := b.String(); got != "" {
		t.Errorf("empty LineBuilder.String() = %q, want \"\"", got)
	}
}

func TestSectionsAllSections(t *testing.T) {
	s := promptkit.New()
	s.WhenToUse("3단계 이상", "복잡한 작업")
	s.WhenNotToUse("단순 작업")
	s.UsageNotes("완료 후 갱신 필수")
	s.Examples("Oracle 설치 전체 추적")
	s.Tips("명령형으로 작성")
	s.Parameters("`subject`: 필수")

	guide := s.Build()

	headers := []string{
		"## 언제 이 도구를 쓰는가",
		"## 언제 쓰지 않는가",
		"## 사용 노트",
		"## 예시",
		"## 팁",
		"## 매개변수",
	}
	for _, h := range headers {
		if !strings.Contains(guide, h) {
			t.Errorf("Build() missing header %q", h)
		}
	}
	if !strings.Contains(guide, "- 3단계 이상") {
		t.Errorf("Build() missing bullet item")
	}
	if !strings.Contains(guide, "1. Oracle 설치 전체 추적") {
		t.Errorf("Build() missing numbered example")
	}
}

func TestSectionsAppendNote(t *testing.T) {
	s := promptkit.New()
	s.WhenToUse("항목")
	s.AppendNote("계획 모드에서는 plan 종료 후 사용")

	guide := s.Build()
	if !strings.Contains(guide, "> **참고**:") {
		t.Errorf("AppendNote() not rendered in output")
	}
}

func TestSectionsEmptyItemsIgnored(t *testing.T) {
	s := promptkit.New()
	s.WhenToUse("", "  ", "유효한 항목", "")
	guide := s.Build()
	if !strings.Contains(guide, "유효한 항목") {
		t.Errorf("non-empty item not present")
	}
	if strings.Contains(guide, "- \n") {
		t.Errorf("empty bullet found in output")
	}
}

func TestSectionsEmptyBuild(t *testing.T) {
	s := promptkit.New()
	if got := s.Build(); got != "" {
		t.Errorf("empty Sections.Build() = %q, want \"\"", got)
	}
}

func TestSectionsSkipsAllEmptySection(t *testing.T) {
	s := promptkit.New()
	s.WhenToUse() // no items — should be skipped
	s.UsageNotes("노트")
	guide := s.Build()
	if strings.Contains(guide, "언제 이 도구를 쓰는가") {
		t.Errorf("empty section should not appear in output")
	}
	if !strings.Contains(guide, "## 사용 노트") {
		t.Errorf("non-empty section should appear")
	}
}
