package query

import "testing"

func TestNoteAssistantTurnText_ResetsOnNewText(t *testing.T) {
	var st persistenceState
	st.NoteAssistantTurnText("first reply")
	if st.RepeatedAssistantCount != 1 {
		t.Fatalf("count=%d, want 1", st.RepeatedAssistantCount)
	}
	st.NoteAssistantTurnText("different reply")
	if st.RepeatedAssistantCount != 1 {
		t.Fatalf("count=%d, want 1 after change", st.RepeatedAssistantCount)
	}
	if st.LastAssistantSnapshot != "different reply" {
		t.Fatalf("snapshot=%q", st.LastAssistantSnapshot)
	}
}

func TestNoteAssistantTurnText_IncrementsOnRepeat(t *testing.T) {
	var st persistenceState
	st.NoteAssistantTurnText("same")
	st.NoteAssistantTurnText("same")
	if st.RepeatedAssistantCount != 2 {
		t.Fatalf("count=%d, want 2", st.RepeatedAssistantCount)
	}
	st.NoteAssistantTurnText("same")
	if st.RepeatedAssistantCount != 3 {
		t.Fatalf("count=%d, want 3", st.RepeatedAssistantCount)
	}
}

func TestNoteAssistantTurnText_EmptyDoesNotResetCounter(t *testing.T) {
	var st persistenceState
	st.NoteAssistantTurnText("retry")
	st.NoteAssistantTurnText("retry")
	st.NoteAssistantTurnText("")
	if st.RepeatedAssistantCount != 2 {
		t.Fatalf("count=%d, want 2 (empty turn must not affect counter)", st.RepeatedAssistantCount)
	}
}

func TestShouldForcePersistenceContinuation_BlocksOnRepeatedAssistant(t *testing.T) {
	policy := persistencePolicy{
		Enabled:                true,
		MaxForcedContinuations: 5,
		MaxSameFailureRetries:  3,
	}
	st := persistenceState{
		LastRecoverableError:   true,
		SameFailureRetries:     1,
		RepeatedAssistantCount: maxRepeatedAssistantTurns,
	}
	if shouldForcePersistenceContinuation(policy, st) {
		t.Fatal("expected force continuation to be blocked when repetition cap reached")
	}
}

func TestShouldForcePersistenceContinuation_AllowsBelowRepetitionCap(t *testing.T) {
	policy := persistencePolicy{
		Enabled:                true,
		MaxForcedContinuations: 5,
		MaxSameFailureRetries:  3,
	}
	st := persistenceState{
		LastRecoverableError:   true,
		SameFailureRetries:     1,
		RepeatedAssistantCount: 1,
	}
	if !shouldForcePersistenceContinuation(policy, st) {
		t.Fatal("expected force continuation to be allowed below repetition cap")
	}
}
