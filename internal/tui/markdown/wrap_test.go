package markdown

import (
	"strings"
	"testing"
)

func TestWrapStyled_ASCII(t *testing.T) {
	lines := WrapStyled("hello world foo bar", 10)
	for _, l := range lines {
		if VisibleWidth(l) > 10 {
			t.Errorf("line %q exceeds width 10 (visible %d)", l, VisibleWidth(l))
		}
	}
}

func TestWrapStyled_Korean(t *testing.T) {
	// 한국어: 각 글자는 2칸. "안녕하세요" = 10칸
	text := "안녕하세요 반갑습니다 오늘도 좋은 하루 되세요"
	lines := WrapStyled(text, 15)
	for _, l := range lines {
		if VisibleWidth(l) > 15 {
			t.Errorf("line %q exceeds width 15 (visible %d)", l, VisibleWidth(l))
		}
	}
	if len(lines) < 2 {
		t.Errorf("expected wrapping but got single line: %v", lines)
	}
}

func TestWrapStyled_MixedKoreanEnglish(t *testing.T) {
	// 사용자 시나리오: 한국어 + 영문 옵션 혼합
	text := "해당 파드를 생성하는 컨트롤러의 설정에서 --enable-reasoning 인자를 제거해야 합니다"
	lines := WrapStyled(text, 40)
	for _, l := range lines {
		w := VisibleWidth(l)
		if w > 40 {
			t.Errorf("line %q exceeds width 40 (visible %d)", l, w)
		}
	}
	// 단어가 중간에서 잘리지 않아야 함 ("--enable-reasoni\nng" 같은 케이스 방지)
	for _, l := range lines {
		stripped := StripANSI(l)
		if strings.HasPrefix(stripped, "ng") {
			t.Errorf("word split across lines: line starts with 'ng': %q", l)
		}
	}
}

func TestWrapStyled_ANSIPreserved(t *testing.T) {
	// ANSI 시퀀스 포함 문자열에서 폭 계산 정확성 확인
	styled := "\x1b[32mhello\x1b[0m world \x1b[31mfoo\x1b[0m bar baz"
	lines := WrapStyled(styled, 12)
	for _, l := range lines {
		if VisibleWidth(l) > 12 {
			t.Errorf("line %q exceeds width 12 (visible %d)", l, VisibleWidth(l))
		}
		// ANSI reset must still be present somewhere in output
		if !strings.Contains(strings.Join(lines, "\n"), "\x1b[0m") {
			t.Errorf("ANSI reset sequence lost in output")
		}
	}
}

func TestRenderListItem_KoreanIndent(t *testing.T) {
	// DisableANSI=false: 실제 렌더링 경로 사용, VisibleWidth로 폭 검증
	p := Palette{Body: "#ffffff"}
	termWidth := 40

	result := renderListItem("1.", "해당 파드를 생성하는 컨트롤러의 설정에서 --enable-reasoning 인자를 제거해야 합니다", "", termWidth, p)

	lines := strings.Split(result, "\n")
	prefixW := 3 // "1. " = 3칸

	for i, l := range lines {
		if VisibleWidth(l) > termWidth {
			t.Errorf("line %d %q exceeds termWidth %d (visible %d)", i, l, termWidth, VisibleWidth(l))
		}
		if i > 0 && !strings.HasPrefix(StripANSI(l), strings.Repeat(" ", prefixW)) {
			t.Errorf("continuation line %d %q missing %d-space indent", i, StripANSI(l), prefixW)
		}
	}
}

func TestIsBoxDrawingLine(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"┌─────┬───┐", true},
		{"│ 프로세스명 │ PID │", true},
		{"└──┘", true},
		{"  ┌──┐  ", true},
		{"───────", true},
		{"╔══╗", true},
		{"║ a ║ b ║", true},
		{"", false},
		{"hello world", false},
		{"| ascii pipe |", false}, // ASCII pipe는 markdown 표
		{"---", false},
		{"┌── header", false}, // 보더에 일반 텍스트가 섞이면 거부
	}
	for _, tc := range cases {
		if got := IsBoxDrawingLine(tc.in); got != tc.want {
			t.Errorf("IsBoxDrawingLine(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestRenderMarkdown_BoxDrawingTablePreserved(t *testing.T) {
	// LLM이 직접 그린 박스 드로잉 표는 셀 폭이 한 글자도 어긋나지 않고
	// 그대로 보존되어야 한다. paragraph wrap을 거치면 │가 단어로 쪼개져 깨진다.
	p := Palette{Body: "#ffffff"}
	src := "" +
		"┌─────────────────────────────┬───────┐\n" +
		"│ 프로세스명 (Process Name)   │ PID   │\n" +
		"├─────────────────────────────┼───────┤\n" +
		"│ WindowsTerminal             │ 20720 │\n" +
		"└─────────────────────────────┴───────┘"

	result := Render(src, 100, p)

	inLines := strings.Split(src, "\n")
	outLines := strings.Split(result, "\n")
	if len(inLines) != len(outLines) {
		t.Fatalf("line count changed: in=%d out=%d", len(inLines), len(outLines))
	}
	for i, inL := range inLines {
		gotW := VisibleWidth(outLines[i])
		wantW := VisibleWidth(inL)
		if gotW != wantW {
			t.Errorf("line %d visible width changed: %q (%d) -> %q (%d)",
				i, inL, wantW, StripANSI(outLines[i]), gotW)
		}
		if StripANSI(outLines[i]) != inL {
			t.Errorf("line %d content changed:\n  in:  %q\n  out: %q", i, inL, StripANSI(outLines[i]))
		}
	}
}

func TestRenderMarkdown_ListWrap(t *testing.T) {
	// DisableANSI=false: 실제 렌더링 경로, VisibleWidth로 폭 검증
	p := Palette{Body: "#ffffff"}
	termWidth := 40

	md := "1. 해당 파드를 생성하는 컨트롤러의 설정에서 --enable-reasoning 인자를 제거해야 합니다\n2. 수정 명령을 실행하세요"
	result := Render(md, termWidth, p)

	for _, l := range strings.Split(result, "\n") {
		if VisibleWidth(l) > termWidth {
			t.Errorf("line %q exceeds termWidth %d (visible %d)", StripANSI(l), termWidth, VisibleWidth(l))
		}
	}
}
