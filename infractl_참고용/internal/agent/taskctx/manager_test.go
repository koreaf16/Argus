// Package taskctx
// File: manager_test.go
// Description: Manager 생명주기 단위 테스트
// Responsibility: Declare/Current/Note/End/History/History 최대 5개 제한 검증

package taskctx

import (
	"fmt"
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestDeclare_Success(t *testing.T) {
	m := NewManager()
	m.clock = fixedClock(time.Now())

	tc, err := m.Declare(DeclareRequest{
		Title:        "Oracle 패치",
		Kind:         KindPatch,
		TargetServer: "orcldb",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc == nil {
		t.Fatal("expected non-nil TaskContext")
	}
	if tc.Status != TaskActive {
		t.Errorf("want status %s, got %s", TaskActive, tc.Status)
	}

	cur := m.Current()
	if cur == nil {
		t.Fatal("Current() should return non-nil after Declare")
	}
	if cur.ID != tc.ID {
		t.Errorf("Current().ID mismatch: %s vs %s", cur.ID, tc.ID)
	}
}

func TestDeclare_ConsecutiveDeclare_PreviousAborted(t *testing.T) {
	m := NewManager()
	m.clock = fixedClock(time.Now())

	first, err := m.Declare(DeclareRequest{Title: "First", Kind: KindOther})
	if err != nil {
		t.Fatalf("first declare error: %v", err)
	}
	firstID := first.ID

	_, err = m.Declare(DeclareRequest{Title: "Second", Kind: KindOther})
	if err != nil {
		t.Fatalf("second declare error: %v", err)
	}

	hist := m.History()
	if len(hist) != 1 {
		t.Fatalf("want 1 history entry, got %d", len(hist))
	}
	if hist[0].ID != firstID {
		t.Errorf("history ID mismatch: want %s, got %s", firstID, hist[0].ID)
	}
	if hist[0].Status != TaskAborted {
		t.Errorf("want status %s, got %s", TaskAborted, hist[0].Status)
	}
}

func TestDeclare_InvalidForbiddenRegex(t *testing.T) {
	m := NewManager()
	m.clock = fixedClock(time.Now())

	// 유효한 첫 번째 태스크 선언
	prev, err := m.Declare(DeclareRequest{Title: "Init", Kind: KindOther})
	if err != nil {
		t.Fatalf("initial declare error: %v", err)
	}
	prevID := prev.ID

	// 잘못된 정규식으로 두 번째 선언 시도
	_, err = m.Declare(DeclareRequest{
		Title:     "Bad Regex",
		Kind:      KindOther,
		Forbidden: []string{"^/[invalid"},
	})
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}

	// Current 는 이전 태스크 그대로여야 함
	cur := m.Current()
	if cur == nil {
		t.Fatal("Current() should still return previous task after failed declare")
	}
	if cur.ID != prevID {
		t.Errorf("Current ID should remain %s, got %s", prevID, cur.ID)
	}
}

func TestNote_AppearsInSnapshot(t *testing.T) {
	m := NewManager()
	m.clock = fixedClock(time.Now())

	_, err := m.Declare(DeclareRequest{Title: "Noted Task", Kind: KindOther})
	if err != nil {
		t.Fatalf("declare error: %v", err)
	}

	m.Note("step_advance", "1단계 완료")

	cur := m.Current()
	if cur == nil {
		t.Fatal("Current() nil")
	}
	if len(cur.Notes) != 1 {
		t.Fatalf("want 1 note, got %d", len(cur.Notes))
	}
	if cur.Notes[0].Message != "1단계 완료" {
		t.Errorf("note message mismatch: %q", cur.Notes[0].Message)
	}
}

func TestEnd_CurrentBecomesNil(t *testing.T) {
	m := NewManager()
	m.clock = fixedClock(time.Now())

	_, err := m.Declare(DeclareRequest{Title: "Ending Task", Kind: KindOther})
	if err != nil {
		t.Fatalf("declare error: %v", err)
	}

	m.End(TaskCompleted)

	if cur := m.Current(); cur != nil {
		t.Errorf("Current() should be nil after End, got %+v", cur)
	}
}

func TestHistory_MaxFiveEntries(t *testing.T) {
	m := NewManager()
	m.clock = fixedClock(time.Now())

	// 6개 선언 → 이전 것들이 history에 쌓임
	for i := range 6 {
		_, err := m.Declare(DeclareRequest{
			Title: fmt.Sprintf("Task %d", i),
			Kind:  KindOther,
		})
		if err != nil {
			t.Fatalf("declare %d error: %v", i, err)
		}
	}

	// 마지막 Declare로 이전 5개가 history에 있어야 함 (maxHistory=5)
	hist := m.History()
	if len(hist) > maxHistory {
		t.Errorf("history exceeds max %d, got %d", maxHistory, len(hist))
	}
}

func TestHistory_StatusAfterEnd(t *testing.T) {
	m := NewManager()
	m.clock = fixedClock(time.Now())

	_, err := m.Declare(DeclareRequest{Title: "Task to complete", Kind: KindOther})
	if err != nil {
		t.Fatalf("declare error: %v", err)
	}
	m.End(TaskCompleted)

	hist := m.History()
	if len(hist) != 1 {
		t.Fatalf("want 1 history, got %d", len(hist))
	}
	if hist[0].Status != TaskCompleted {
		t.Errorf("want %s, got %s", TaskCompleted, hist[0].Status)
	}
}
