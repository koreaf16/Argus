// Package agent
// File: intel_gate_test.go
// Description: shouldSkipIntel 단위 테스트
// Responsibility: intel 서브에이전트 skip 게이트 로직의 정확성 검증

package agent

import "testing"

func TestShouldSkipIntel(t *testing.T) {
	cases := []struct {
		name            string
		isReasoning     bool
		taskType        string
		pendingActive   bool
		isShortFollowUp bool
		wantSkip        bool
		wantReason      string
	}{
		{
			name:        "InstallReasoningKeepsIntel",
			isReasoning: true,
			taskType:    "install",
			wantSkip:    false,
			wantReason:  "",
		},
		{
			name:        "GeneralTierSkips",
			isReasoning: false,
			taskType:    "install",
			wantSkip:    true,
			wantReason:  "general_tier",
		},
		{
			name:        "EmptyTaskTypeSkips",
			isReasoning: true,
			taskType:    "",
			wantSkip:    true,
			wantReason:  "task_type_unclassified",
		},
		{
			name:          "PendingActiveSkips",
			isReasoning:   true,
			taskType:      "install",
			pendingActive: true,
			wantSkip:      true,
			wantReason:    "pending_action_active",
		},
		{
			name:            "ShortFollowUpSkips",
			isReasoning:     true,
			taskType:        "install",
			isShortFollowUp: true,
			wantSkip:        true,
			wantReason:      "short_follow_up",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			skip, reason := shouldSkipIntel(tc.isReasoning, tc.taskType, tc.pendingActive, tc.isShortFollowUp)
			if skip != tc.wantSkip {
				t.Errorf("skip = %v, want %v", skip, tc.wantSkip)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}
