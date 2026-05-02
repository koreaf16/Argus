package lane

import (
	"context"
	"testing"
)

func TestAccountStackOps(t *testing.T) {
	s := AccountStack{}
	if s.Top() != "" {
		t.Fatal("empty top must be ''")
	}

	s = s.Push("sandbox")
	s = s.Push("postgres")
	if s.Top() != "postgres" {
		t.Errorf("top = %q, want postgres", s.Top())
	}
	if s.String() != "sandbox>postgres" {
		t.Errorf("string = %q", s.String())
	}

	s, popped := s.Pop()
	if popped != "postgres" {
		t.Errorf("pop = %q, want postgres", popped)
	}
	if s.Top() != "sandbox" {
		t.Errorf("after pop top = %q", s.Top())
	}
}

func TestLaneIDStability(t *testing.T) {
	a := LaneIDFor("sandbox", AccountStack{"sandbox", "postgres"})
	b := LaneIDFor("sandbox", AccountStack{"sandbox", "postgres"})
	if a != b {
		t.Errorf("LaneIDFor not deterministic: %s vs %s", a, b)
	}
	c := LaneIDFor("sandbox", AccountStack{"sandbox", "oracle"})
	if a == c {
		t.Errorf("different stacks produced same id: %s", a)
	}
	bare := LaneIDFor("sandbox", nil)
	if bare != "sandbox" {
		t.Errorf("empty-stack id = %q, want sandbox", bare)
	}
}

func TestNoopManagerRejects(t *testing.T) {
	m := NewNoopManager()
	if _, err := m.Acquire(context.Background(), Target{Alias: "sandbox"}, AcquireOpts{}); err == nil {
		t.Fatal("Acquire on noop manager must error")
	}
	if snaps := m.Snapshots(); len(snaps) != 0 {
		t.Errorf("snapshots = %d, want 0", len(snaps))
	}
	if err := m.CloseAll(); err != nil {
		t.Errorf("CloseAll = %v", err)
	}
}
