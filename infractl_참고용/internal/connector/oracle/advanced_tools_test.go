package oracle

import "testing"

func TestTuningProposalLifecycle(t *testing.T) {
	c := New()
	proposal := c.saveProposal("CREATE INDEX IX_A ON HR.T1(C1);", "idx rec", "full scan evidence")

	if proposal.ID == "" {
		t.Fatal("expected proposal id")
	}
	if proposal.SQLHash == "" {
		t.Fatal("expected proposal SQL hash")
	}

	got, ok := c.getProposal(proposal.ID)
	if !ok {
		t.Fatal("expected proposal to exist")
	}
	if normalizeSQL(got.SQL) != normalizeSQL("CREATE INDEX IX_A ON HR.T1(C1);") {
		t.Fatalf("unexpected proposal SQL: %q", got.SQL)
	}

	c.deleteProposal(proposal.ID)
	if _, ok := c.getProposal(proposal.ID); ok {
		t.Fatal("proposal should be deleted")
	}
}

func TestSplitOwnerTable(t *testing.T) {
	owner, table := splitOwnerTable("HR|EMP")
	if owner != "HR" || table != "EMP" {
		t.Fatalf("unexpected owner/table for pipe format: %s/%s", owner, table)
	}
	owner, table = splitOwnerTable("SCOTT.DEPT")
	if owner != "SCOTT" || table != "DEPT" {
		t.Fatalf("unexpected owner/table for dot format: %s/%s", owner, table)
	}
}
