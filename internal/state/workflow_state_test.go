package state

import "testing"

func TestSetWorkflowCardClonesConstraints(t *testing.T) {
	s := NewAppState()
	c := &WorkflowCard{
		Title:       "Postgres migration",
		Category:    WorkflowCategoryMigration,
		Phase:       WorkflowPhaseDiscover,
		Constraints: []string{"prod db", "extension compat"},
		WorkspaceRoles: map[string]string{
			"source_mysql":    "oracle-server",
			"target_postgres": "sandbox-server",
		},
	}
	s.SetWorkflowCard(c)

	c.Constraints[0] = "MUTATED"
	c.WorkspaceRoles["source_mysql"] = "mutated"
	got := s.WorkflowCard()
	if got == nil {
		t.Fatal("expected card, got nil")
	}
	if got.Constraints[0] != "prod db" {
		t.Fatalf("constraint should be deep-copied: %v", got.Constraints)
	}
	if got.WorkspaceRoles["source_mysql"] != "oracle-server" {
		t.Fatalf("workspace role should be deep-copied: %v", got.WorkspaceRoles)
	}

	got.Constraints[1] = "MUTATED AGAIN"
	got.WorkspaceRoles["target_postgres"] = "mutated"
	again := s.WorkflowCard()
	if again.Constraints[1] != "extension compat" {
		t.Fatalf("returned card should be detached: %v", again.Constraints)
	}
	if again.WorkspaceRoles["target_postgres"] != "sandbox-server" {
		t.Fatalf("returned card should be detached: %v", again.WorkspaceRoles)
	}
}

func TestWorkflowPhaseUpdate(t *testing.T) {
	s := NewAppState()
	if got := s.WorkflowCard(); got != nil {
		t.Fatal("expected no card on fresh state")
	}
	s.SetWorkflowCard(&WorkflowCard{
		Title:    "test",
		Category: WorkflowCategoryInstall,
		Phase:    WorkflowPhaseDiscover,
	})
	s.SetWorkflowPhase(WorkflowPhaseResearch)
	got := s.WorkflowCard()
	if got == nil || got.Phase != WorkflowPhaseResearch {
		t.Fatalf("phase update failed: %+v", got)
	}
}

func TestClearWorkflowCard(t *testing.T) {
	s := NewAppState()
	s.SetWorkflowCard(&WorkflowCard{Title: "x", Category: WorkflowCategoryOther, Phase: WorkflowPhaseDiscover})
	s.ClearWorkflowCard()
	if s.WorkflowCard() != nil {
		t.Fatal("expected card cleared")
	}
}

func TestPendingWorkflowInitFlag(t *testing.T) {
	s := NewAppState()
	if s.PendingWorkflowInit() {
		t.Fatal("default should be false")
	}
	s.SetPendingWorkflowInit(true)
	if !s.PendingWorkflowInit() {
		t.Fatal("expected true after set")
	}
	s.SetPendingWorkflowInit(false)
	if s.PendingWorkflowInit() {
		t.Fatal("expected false after clear")
	}
}
