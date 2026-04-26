// Package compact
// File: collapse_test.go
// Description: staged context collapse 단위 테스트
// Responsibility: 우선순위 큐 순서, N개 적재 후 N turn drain 소진 검증

package compact

import (
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/llm"
)

func TestCollapse_StagesAndDrainsOnce(t *testing.T) {
	large := strings.Repeat("x", collapseMinChars+100)
	s := &State{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: large},
		},
		ContextWindow: 200_000,
	}

	c := NewCollapse()
	c.ApplyIfNeeded(s, TokenWarning)

	if !c.HasStaged() {
		t.Fatal("expected staged items after ApplyIfNeeded")
	}

	drained := c.Drain(s)
	if !drained {
		t.Error("Drain should return true on first call")
	}
	if strings.Contains(s.Messages[0].Content, strings.Repeat("x", collapseMinChars)) {
		t.Error("message should be collapsed after drain")
	}
	if !strings.Contains(s.Messages[0].Content, "[collapsed:") {
		t.Error("collapsed marker missing after drain")
	}
}

func TestCollapse_NoDrainBelowWarning(t *testing.T) {
	large := strings.Repeat("x", collapseMinChars+100)
	s := &State{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: large},
		},
	}

	c := NewCollapse()
	c.ApplyIfNeeded(s, TokenOK)

	if c.HasStaged() {
		t.Error("should not stage when TokenOK")
	}
}

func TestCollapse_LargestFirst(t *testing.T) {
	small := strings.Repeat("a", collapseMinChars+100)
	large := strings.Repeat("b", collapseMinChars+10_000)
	s := &State{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: small},
			{Role: llm.RoleUser, Content: large},
		},
	}

	c := NewCollapse()
	c.ApplyIfNeeded(s, TokenWarning)
	c.Drain(s)

	// The large message (index 1) should be drained first
	if strings.Contains(s.Messages[1].Content, "[collapsed:") {
		return // correct — large was collapsed first
	}
	if strings.Contains(s.Messages[0].Content, "[collapsed:") {
		// small was drained first — wrong priority
		t.Error("expected largest message to be drained first")
	}
}

func TestCollapse_EmptyDrainReturnsFalse(t *testing.T) {
	s := &State{}
	c := NewCollapse()
	if c.Drain(s) {
		t.Error("Drain on empty queue should return false")
	}
}
