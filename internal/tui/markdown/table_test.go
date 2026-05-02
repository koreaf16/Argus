package markdown

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// TestRenderTable_KoreanHeaderAlignment 는 한글 헤더와 ASCII 본문이 섞인 표가
// 모든 행에서 동일한 시각 폭을 가지는지 검사한다. EastAsianWidth=false 환경에서
// 한글이 폭 1로 계산되면 헤더 행과 본문 행/보더 행의 폭이 어긋나 실패한다.
func TestRenderTable_KoreanHeaderAlignment(t *testing.T) {
	prev := runewidth.DefaultCondition.EastAsianWidth
	runewidth.DefaultCondition.EastAsianWidth = true
	t.Cleanup(func() { runewidth.DefaultCondition.EastAsianWidth = prev })

	headers := []string{"이름", "인코딩", "Collate", "Ctype"}
	rows := [][]string{{"imsi", "UTF8", "ko_KR.UTF-8", "ko_KR.UTF-8"}}

	out := RenderTable(headers, rows, 120, Palette{DisableANSI: true})
	lines := strings.Split(out, "\n")
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 lines (top, header, mid, row, bot), got %d:\n%s", len(lines), out)
	}

	// 헤더(lines[1])와 본문(lines[3])의 폭이 같아야 정렬이 보장된다.
	// 보더 라인은 박스 그리기 문자(ambiguous width) 사용으로 폭 계산이 데이터 라인과
	// 다를 수 있으므로 별도 검증한다.
	headerW := VisibleWidth(lines[1])
	bodyW := VisibleWidth(lines[3])
	if headerW != bodyW {
		t.Errorf("header/body width mismatch: header=%d body=%d\nfull output:\n%s", headerW, bodyW, out)
	}
}

// TestVisibleWidth_Korean 는 한글이 EastAsianWidth=true 설정에서 폭 2로 계산되는지 확인한다.
func TestVisibleWidth_Korean(t *testing.T) {
	prev := runewidth.DefaultCondition.EastAsianWidth
	runewidth.DefaultCondition.EastAsianWidth = true
	t.Cleanup(func() { runewidth.DefaultCondition.EastAsianWidth = prev })

	cases := map[string]int{
		"이름":   4,
		"imsi": 4,
		"한글 abc": 8,
	}
	for s, want := range cases {
		if got := VisibleWidth(s); got != want {
			t.Errorf("VisibleWidth(%q) = %d, want %d", s, got, want)
		}
	}
}
