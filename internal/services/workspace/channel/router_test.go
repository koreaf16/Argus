package channel

import (
	"testing"

	"github.com/koreaf16/argus/internal/services/workspace/lane"
)

func TestPrivilegeKeyFromStack(t *testing.T) {
	cases := []struct {
		name      string
		loginUser string
		stack     lane.AccountStack
		want      PrivilegeKey
	}{
		{"empty stack -> default", "alice", lane.AccountStack{}, PrivilegeDefault},
		{"login only -> default", "alice", lane.AccountStack{"alice"}, PrivilegeDefault},
		{"sudo root", "alice", lane.AccountStack{"alice", "root"}, PrivilegeKey("alice>root")},
		{"sudo postgres", "alice", lane.AccountStack{"alice", "postgres"}, PrivilegeKey("alice>postgres")},
		{"chained su", "alice", lane.AccountStack{"alice", "root", "postgres"}, PrivilegeKey("alice>root>postgres")},
		{"empty login -> bare stack", "", lane.AccountStack{"root"}, PrivilegeKey("root")},
		{"whitespace pruned", "alice", lane.AccountStack{"alice", "  ", "root"}, PrivilegeKey("alice>root")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PrivilegeKeyFromStack(tc.loginUser, tc.stack)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStackForPrivilegeRoundTrip(t *testing.T) {
	loginUser := "alice"
	stacks := []lane.AccountStack{
		{},
		{"alice"},
		{"alice", "root"},
		{"alice", "postgres"},
		{"alice", "root", "postgres"},
	}
	for _, s := range stacks {
		key := PrivilegeKeyFromStack(loginUser, s)
		got := StackForPrivilege(loginUser, key)
		// The default key reconstructs to an empty stack; callers prepend
		// loginUser when they need the full chain.
		if key == PrivilegeDefault {
			if len(got) != 0 {
				t.Fatalf("default key should yield empty stack, got %v", got)
			}
			continue
		}
		if got.String() != s.String() {
			t.Fatalf("round-trip failed: %v -> %s -> %v", s, key, got)
		}
	}
}

// openPolicy returns an ElevationPolicy that allows any escalation, used by
// routing tests that focus on stack transitions rather than policy enforcement.
func openPolicy() ElevationPolicy { return ElevationPolicy{Allowed: true, Mode: "reuse_login"} }

// loginCreds is a minimal CredentialResolver that returns a fixed login password.
type loginCreds struct{ pw string }

func (l *loginCreds) GetSudoPassword(_, _ string) string { return "" }
func (l *loginCreds) GetLoginPassword(_ string) string   { return l.pw }

type aliasCreds struct {
	alias string
	pw    string
}

func (a *aliasCreds) GetSudoPassword(alias, targetUser string) string {
	if alias == a.alias && targetUser == "root" {
		return a.pw
	}
	return ""
}

func (a *aliasCreds) GetLoginPassword(string) string { return "" }

func TestRouteForEnterExit(t *testing.T) {
	loginUser := "alice"
	pol := openPolicy()
	cr := &loginCreds{"pw"}

	t.Run("default to sudo -i pushes root", func(t *testing.T) {
		d := RouteFor("dev", lane.AccountStack{"alice"}, loginUser, "sudo -i", pol, cr)
		if d.Transition.Kind != lane.AccountTransitionEnter {
			t.Fatalf("expected Enter, got %v", d.Transition.Kind)
		}
		if d.Privilege != PrivilegeKey("alice>root") {
			t.Fatalf("expected alice>root, got %q", d.Privilege)
		}
		if d.TargetStack.Top() != "root" {
			t.Fatalf("expected stack top root, got %q", d.TargetStack.Top())
		}
		if d.Rejected != "" {
			t.Fatalf("open policy should not reject: %s", d.Rejected)
		}
	})

	t.Run("sudo -i -u postgres pushes postgres", func(t *testing.T) {
		d := RouteFor("dev", lane.AccountStack{"alice"}, loginUser, "sudo -i -u postgres", pol, cr)
		if d.Privilege != PrivilegeKey("alice>postgres") {
			t.Fatalf("expected alice>postgres, got %q", d.Privilege)
		}
	})

	t.Run("su - root pushes root", func(t *testing.T) {
		d := RouteFor("dev", lane.AccountStack{"alice"}, loginUser, "su -", pol, cr)
		if d.Privilege != PrivilegeKey("alice>root") {
			t.Fatalf("expected alice>root, got %q", d.Privilege)
		}
	})

	t.Run("su - postgres pushes postgres", func(t *testing.T) {
		d := RouteFor("dev", lane.AccountStack{"alice"}, loginUser, "su - postgres", pol, cr)
		if d.Privilege != PrivilegeKey("alice>postgres") {
			t.Fatalf("expected alice>postgres, got %q", d.Privilege)
		}
	})

	t.Run("exit from root pops back to default", func(t *testing.T) {
		d := RouteFor("dev", lane.AccountStack{"alice", "root"}, loginUser, "exit", pol, cr)
		if d.Transition.Kind != lane.AccountTransitionExit {
			t.Fatalf("expected Exit, got %v", d.Transition.Kind)
		}
		// Exit runs on the channel being left (alice>root), but the post-pop
		// stack (TargetStack) should resolve back to default.
		if d.Privilege != PrivilegeKey("alice>root") {
			t.Fatalf("exit should run on the current channel %q, got %q", "alice>root", d.Privilege)
		}
		afterKey := PrivilegeKeyFromStack(loginUser, d.TargetStack)
		if afterKey != PrivilegeDefault {
			t.Fatalf("post-exit stack should be default, got %q", afterKey)
		}
	})
}

func TestRouteForInlineStaysOnCurrentChannel(t *testing.T) {
	loginUser := "alice"
	d := RouteFor("dev", lane.AccountStack{"alice"}, loginUser, "sudo -u postgres psql -c 'SELECT 1'", openPolicy(), &loginCreds{"pw"})
	if d.Transition.Kind != lane.AccountTransitionInline {
		t.Fatalf("expected Inline, got %v", d.Transition.Kind)
	}
	if d.Privilege != PrivilegeDefault {
		t.Fatalf("inline should stay on default, got %q", d.Privilege)
	}
}

func TestRouteForPlainCommandStaysOnCurrentChannel(t *testing.T) {
	loginUser := "alice"
	d := RouteFor("dev", lane.AccountStack{"alice", "root"}, loginUser, "ls /root", openPolicy(), nil)
	if d.Transition.Kind != lane.AccountTransitionNone {
		t.Fatalf("expected None, got %v", d.Transition.Kind)
	}
	if d.Privilege != PrivilegeKey("alice>root") {
		t.Fatalf("plain command should stay on alice>root, got %q", d.Privilege)
	}
}

func TestRouteForElevationDisabledRejects(t *testing.T) {
	pol := ElevationPolicy{Allowed: false}
	d := RouteFor("dev", lane.AccountStack{"alice"}, "alice", "sudo -i", pol, nil)
	if d.Rejected == "" {
		t.Errorf("elevation_disabled policy should set Rejected")
	}
	if d.RejectCode != RejectCodeElevationDisabled {
		t.Errorf("expected %q, got %q", RejectCodeElevationDisabled, d.RejectCode)
	}
}

func TestRouteForElevationInlineRejects(t *testing.T) {
	pol := ElevationPolicy{Allowed: false}
	d := RouteFor("dev", lane.AccountStack{"alice"}, "alice", "sudo -u root whoami", pol, nil)
	if d.Rejected == "" {
		t.Errorf("inline with disabled elevation should be rejected")
	}
}

func TestRouteForUsesServerAliasForCredentials(t *testing.T) {
	pol := ElevationPolicy{Allowed: true, Mode: "password", TargetUsers: []string{"root"}}
	d := RouteFor("parent", lane.AccountStack{"master"}, "master", "sudo -i", pol, &aliasCreds{alias: "parent", pw: "pw"})
	if d.Rejected != "" {
		t.Fatalf("route rejected despite parent credential: %s", d.Rejected)
	}
	if d.SudoPassword != "pw" {
		t.Fatalf("sudo password = %q, want pw", d.SudoPassword)
	}
}

func TestChannelKeyStringAndHash(t *testing.T) {
	k := ChannelKey{Alias: "dev", Privilege: PrivilegeKey("alice>root"), Purpose: PurposeExec}
	if k.String() != "dev|alice>root|exec" {
		t.Fatalf("unexpected string: %s", k.String())
	}
	if len(k.Hash()) != 12 {
		t.Fatalf("expected 12-char hex hash, got %q", k.Hash())
	}
}

func TestIsPrivilegedCommand(t *testing.T) {
	cases := map[string]bool{
		"ls":                              false,
		"whoami":                          false,
		"sudo -i":                         true,
		"sudo -u postgres ls":             true,
		"su - postgres":                   true,
		"exit":                            true,
		"sudo apt update; sudo apt -y up": false, // multi-statement falls through
	}
	for cmd, want := range cases {
		if got := IsPrivilegedCommand(cmd); got != want {
			t.Errorf("%q: got %v, want %v", cmd, got, want)
		}
	}
}

func TestDescribeDecision(t *testing.T) {
	d := RouteFor("dev", lane.AccountStack{"alice"}, "alice", "sudo -i", openPolicy(), &loginCreds{"pw"})
	got := DescribeDecision(d, "dev")
	want := "enter root via sudo -> dev|alice>root|exec"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrivilegeStackEqualsLoginOnly(t *testing.T) {
	if !privilegeStackEqualsLoginOnly("alice", lane.AccountStack{}) {
		t.Errorf("empty should be login-only")
	}
	if !privilegeStackEqualsLoginOnly("alice", lane.AccountStack{"alice"}) {
		t.Errorf("[alice] should be login-only")
	}
	if privilegeStackEqualsLoginOnly("alice", lane.AccountStack{"alice", "root"}) {
		t.Errorf("[alice, root] should NOT be login-only")
	}
}
