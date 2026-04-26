// Package taskctx
// File: proposal_test.go
// Description: PendingProposal 의도 분류 및 만료 단위 테스트
// Responsibility: IsConfirmation/IsCancel/IsRevision/IsStale/Summary/ToDeclareRequest 검증

package taskctx

import (
	"testing"
	"time"
)

func newTestProposal() *PendingProposal {
	return &PendingProposal{
		ID: "test-id",
		Draft: TaskContextDraft{
			Title:         "Oracle 19c 설치",
			Kind:          KindInstall,
			TargetServer:  "db01",
			TargetAccount: "oracle",
		},
		RiskAnalysis: "설치 중 약 30분 다운타임",
		RollbackPlan: []string{"바이너리 제거", "환경변수 롤백"},
		Reasoning:    "운영 정책상 binary 설치 방식 선택",
		CreatedAt:    time.Now(),
		TurnIndex:    5,
	}
}

func TestIsConfirmation_TrueValues(t *testing.T) {
	p := newTestProposal()
	cases := []string{"yes", "YES", "ok", "OK", "Okay", "진행", "좋아", "좋아요", "확인"}
	for _, input := range cases {
		if !p.IsConfirmation(input) {
			t.Errorf("IsConfirmation(%q) want true, got false", input)
		}
	}
}

func TestIsConfirmation_FalseForRevision(t *testing.T) {
	p := newTestProposal()
	// 수정 지시는 확인이 아님
	if p.IsConfirmation("계정을 바꿔줘") {
		t.Error("IsConfirmation('계정을 바꿔줘') want false, got true")
	}
}

func TestIsCancel_TrueValues(t *testing.T) {
	p := newTestProposal()
	cases := []string{"취소", "cancel", "CANCEL", "no", "No", "아니", "아니요"}
	for _, input := range cases {
		if !p.IsCancel(input) {
			t.Errorf("IsCancel(%q) want true, got false", input)
		}
	}
}

func TestIsCancel_FalseForConfirmation(t *testing.T) {
	p := newTestProposal()
	if p.IsCancel("yes") {
		t.Error("IsCancel('yes') want false, got true")
	}
}

func TestIsRevision_ReturnsHint(t *testing.T) {
	p := newTestProposal()
	hint, ok := p.IsRevision("계정을 root로 변경해줘")
	if !ok {
		t.Error("IsRevision('계정을 root로') want true, got false")
	}
	if hint == "" {
		t.Error("IsRevision hint should not be empty")
	}
}

func TestIsRevision_FalseForConfirmation(t *testing.T) {
	p := newTestProposal()
	_, ok := p.IsRevision("yes")
	if ok {
		t.Error("IsRevision('yes') want false (confirmation)")
	}
}

func TestIsRevision_FalseForCancel(t *testing.T) {
	p := newTestProposal()
	_, ok := p.IsRevision("취소")
	if ok {
		t.Error("IsRevision('취소') want false (cancel)")
	}
}

func TestIsStale_TurnDiff3(t *testing.T) {
	p := newTestProposal()
	now := time.Now()
	// turn 차이 3 → stale
	if !p.IsStale(p.TurnIndex+3, now) {
		t.Error("IsStale with turnDiff=3 want true")
	}
}

func TestIsStale_Time5m1s(t *testing.T) {
	p := newTestProposal()
	p.CreatedAt = time.Now().Add(-5*time.Minute - 1*time.Second)
	if !p.IsStale(p.TurnIndex, time.Now()) {
		t.Error("IsStale with 5m1s elapsed want true")
	}
}

func TestIsStale_NotStale(t *testing.T) {
	p := newTestProposal()
	p.CreatedAt = time.Now().Add(-1 * time.Minute)
	if p.IsStale(p.TurnIndex+1, time.Now()) {
		t.Error("IsStale with turnDiff=1 and 1min elapsed want false")
	}
}

func TestSummary_Format(t *testing.T) {
	p := newTestProposal()
	s := p.Summary()
	if s != "[제안 대기] Oracle 19c 설치" {
		t.Errorf("Summary mismatch: %q", s)
	}
}

func TestToDeclareRequest_Conversion(t *testing.T) {
	draft := TaskContextDraft{
		Title:         "서비스 재시작",
		Kind:          KindService,
		TargetServer:  "web01",
		TargetAccount: "root",
		WorkingDir:    "/etc/systemd",
		Forbidden:     []string{"rm -rf"},
		PlanSteps:     []string{"상태 확인", "재시작"},
		KindFields: map[string]string{
			"service_name":      "nginx",
			"action":            "restart",
			"expected_downtime": "약 5초",
			"dependents":        "WAS",
		},
	}

	req := draft.ToDeclareRequest()

	if req.Title != draft.Title {
		t.Errorf("Title mismatch: %q vs %q", req.Title, draft.Title)
	}
	if req.Kind != draft.Kind {
		t.Errorf("Kind mismatch: %q vs %q", req.Kind, draft.Kind)
	}
	if len(req.Forbidden) != len(draft.Forbidden) {
		t.Errorf("Forbidden len mismatch: %d vs %d", len(req.Forbidden), len(draft.Forbidden))
	}
	if len(req.PlanSteps) != len(draft.PlanSteps) {
		t.Errorf("PlanSteps len mismatch: %d vs %d", len(req.PlanSteps), len(draft.PlanSteps))
	}
	if req.KindFields["service_name"] != "nginx" {
		t.Errorf("KindFields[service_name] mismatch: %q", req.KindFields["service_name"])
	}
}
