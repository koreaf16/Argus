package state

import "testing"

func TestActiveTargetRoundtrip(t *testing.T) {
	s := NewAppState()
	got := s.ActiveTarget()
	if got.Alias != "" || got.LaneID != "" {
		t.Fatalf("zero target expected, got %+v", got)
	}

	s.SetActiveTarget(ActiveTarget{
		Alias:        "sandbox",
		LaneID:       "sandbox#abc",
		AccountStack: []string{"sandbox", "postgres"},
		CWD:          "/var/lib/pgsql",
	})

	got = s.ActiveTarget()
	if got.Display() != "sandbox:postgres" {
		t.Errorf("display = %q, want sandbox:postgres", got.Display())
	}
	if got.EffectiveUser() != "postgres" {
		t.Errorf("effective user = %q", got.EffectiveUser())
	}

	got.AccountStack[0] = "mutated"
	again := s.ActiveTarget()
	if again.AccountStack[0] != "sandbox" {
		t.Error("ActiveTarget must return defensive copy of AccountStack")
	}
}

func TestActiveTargetHookFiresOnChange(t *testing.T) {
	s := NewAppState()
	var fired []string
	s.SetStateChangeHook(func(field, before, after string) {
		if field == "active_target" {
			fired = append(fired, before+"->"+after)
		}
	})

	s.SetActiveTarget(ActiveTarget{Alias: "sandbox"})
	s.SetActiveTarget(ActiveTarget{Alias: "sandbox"}) // duplicate, no fire
	s.SetActiveTarget(ActiveTarget{Alias: "sandbox", AccountStack: []string{"sandbox", "postgres"}})

	if len(fired) != 2 {
		t.Errorf("hook fired %d times: %v", len(fired), fired)
	}
	if fired[0] != "->sandbox" {
		t.Errorf("first fire = %q", fired[0])
	}
	if fired[1] != "sandbox->sandbox:postgres" {
		t.Errorf("second fire = %q", fired[1])
	}
}
