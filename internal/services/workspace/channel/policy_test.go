package channel_test

import (
	"strings"
	"testing"

	"github.com/koreaf16/argus/internal/services/workspace/channel"
)

// staticCreds implements CredentialResolver with in-memory maps.
type staticCreds struct {
	sudo  map[string]string // "alias|user" -> password
	login map[string]string // alias -> password
}

func TestEvaluateElevation_RejectionMessagesUseServerEdit(t *testing.T) {
	cases := []channel.ElevationVerdict{
		channel.EvaluateElevation("srv", channel.ElevationPolicy{Allowed: false}, channel.AccountTransitionHint{IsEscalation: true, TargetUser: "root"}, nil),
		channel.EvaluateElevation("srv", channel.ElevationPolicy{Allowed: true, Mode: "password", TargetUsers: []string{"root"}}, channel.AccountTransitionHint{IsEscalation: true, TargetUser: "postgres"}, &staticCreds{sudo: map[string]string{"srv|root": "pw"}}),
		channel.EvaluateElevation("srv", channel.ElevationPolicy{Allowed: true, Mode: "password"}, channel.AccountTransitionHint{IsEscalation: true, TargetUser: "root"}, &staticCreds{sudo: map[string]string{}}),
		channel.EvaluateElevation("srv", channel.ElevationPolicy{Allowed: true, Mode: "reuse_login"}, channel.AccountTransitionHint{IsEscalation: true, TargetUser: "root"}, &staticCreds{login: map[string]string{}}),
	}
	for _, verdict := range cases {
		if verdict.Allow {
			t.Fatalf("test setup produced an allowed verdict: %+v", verdict)
		}
		if strings.Contains(verdict.Reason, "/elevate") {
			t.Fatalf("rejection reason still mentions /elevate: %s", verdict.Reason)
		}
		if !strings.Contains(verdict.Reason, "/server edit srv") {
			t.Fatalf("rejection reason does not mention /server edit srv: %s", verdict.Reason)
		}
	}
}

func (s *staticCreds) GetSudoPassword(alias, targetUser string) string {
	return s.sudo[alias+"|"+targetUser]
}
func (s *staticCreds) GetLoginPassword(alias string) string {
	return s.login[alias]
}

func TestEvaluateElevation_NonEscalation(t *testing.T) {
	p := channel.ElevationPolicy{Allowed: false}
	hint := channel.AccountTransitionHint{IsEscalation: false}
	v := channel.EvaluateElevation("srv", p, hint, nil)
	if !v.Allow {
		t.Errorf("non-escalation transitions must always be allowed")
	}
}

func TestEvaluateElevation_DisabledPolicy(t *testing.T) {
	p := channel.ElevationPolicy{Allowed: false}
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "root"}
	v := channel.EvaluateElevation("srv", p, hint, nil)
	if v.Allow {
		t.Errorf("expected rejection when Allowed=false")
	}
	if v.Code != channel.RejectCodeElevationDisabled {
		t.Errorf("expected code %q, got %q", channel.RejectCodeElevationDisabled, v.Code)
	}
}

func TestEvaluateElevation_ModeNone(t *testing.T) {
	p := channel.ElevationPolicy{Allowed: true, Mode: "none"}
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "root"}
	v := channel.EvaluateElevation("srv", p, hint, nil)
	if v.Allow {
		t.Errorf("mode=none must reject even with Allowed=true")
	}
	if v.Code != channel.RejectCodeElevationDisabled {
		t.Errorf("expected elevation_disabled, got %q", v.Code)
	}
}

func TestEvaluateElevation_TargetUserBlocked_Obsolete(t *testing.T) {
	// This test used to expect rejection, but now with root omnipotence,
	// if "root" is in the list, "postgres" is allowed.
	p := channel.ElevationPolicy{
		Allowed:     true,
		Mode:        "password",
		TargetUsers: []string{"web"}, // Changed from "root" to "web" to still test rejection
	}
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "postgres"}
	creds := &staticCreds{sudo: map[string]string{"srv|web": "webpw"}}
	v := channel.EvaluateElevation("srv", p, hint, creds)
	if v.Allow {
		t.Errorf("expected rejection when target user not in allowlist (and root not in list)")
	}
	if v.Code != channel.RejectCodeTargetUserBlocked {
		t.Errorf("expected %q, got %q", channel.RejectCodeTargetUserBlocked, v.Code)
	}
}

func TestEvaluateElevation_RootOmnipotence(t *testing.T) {
	p := channel.ElevationPolicy{
		Allowed:     true,
		Mode:        "password",
		TargetUsers: []string{"root"},
	}
	// Even though only "root" is listed, "postgres" should be allowed.
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "postgres"}
	creds := &staticCreds{sudo: map[string]string{"srv|postgres": "pgpw"}}
	v := channel.EvaluateElevation("srv", p, hint, creds)
	if !v.Allow {
		t.Errorf("root is in allowlist, expected postgres to be allowed (root omnipotence)")
	}
	if v.Password != "pgpw" {
		t.Errorf("expected password %q, got %q", "pgpw", v.Password)
	}
}

func TestEvaluateElevation_TargetUserAllowed(t *testing.T) {
	p := channel.ElevationPolicy{
		Allowed:     true,
		Mode:        "password",
		TargetUsers: []string{"root", "postgres"},
	}
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "postgres"}
	creds := &staticCreds{sudo: map[string]string{"srv|postgres": "pgpw"}}
	v := channel.EvaluateElevation("srv", p, hint, creds)
	if !v.Allow {
		t.Errorf("postgres is in allowlist, expected Allow=true")
	}
	if v.Password != "pgpw" {
		t.Errorf("expected password %q, got %q", "pgpw", v.Password)
	}
}

func TestEvaluateElevation_PasswordMissing(t *testing.T) {
	p := channel.ElevationPolicy{Allowed: true, Mode: "password"}
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "root"}
	creds := &staticCreds{sudo: map[string]string{}}
	v := channel.EvaluateElevation("srv", p, hint, creds)
	if v.Allow {
		t.Errorf("expected rejection when sudo password is missing")
	}
	if v.Code != channel.RejectCodePasswordMissing {
		t.Errorf("expected %q, got %q", channel.RejectCodePasswordMissing, v.Code)
	}
}

func TestEvaluateElevation_WildcardSudoFallback(t *testing.T) {
	p := channel.ElevationPolicy{Allowed: true, Mode: "password"}
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "root"}
	// wildcard key (targetUser="")
	creds := &staticCreds{sudo: map[string]string{"srv|": "generalpw"}}
	v := channel.EvaluateElevation("srv", p, hint, creds)
	if !v.Allow {
		t.Errorf("wildcard sudo password should grant access")
	}
	if v.Password != "generalpw" {
		t.Errorf("expected %q, got %q", "generalpw", v.Password)
	}
}

func TestEvaluateElevation_ReuseLogin(t *testing.T) {
	p := channel.ElevationPolicy{Allowed: true, Mode: "reuse_login"}
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "root"}
	creds := &staticCreds{login: map[string]string{"srv": "loginpw"}}
	v := channel.EvaluateElevation("srv", p, hint, creds)
	if !v.Allow {
		t.Errorf("reuse_login with login password should be allowed")
	}
	if v.Password != "loginpw" {
		t.Errorf("expected %q, got %q", "loginpw", v.Password)
	}
}

func TestEvaluateElevation_ReuseLoginMissing(t *testing.T) {
	p := channel.ElevationPolicy{Allowed: true, Mode: "reuse_login"}
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "root"}
	creds := &staticCreds{login: map[string]string{}}
	v := channel.EvaluateElevation("srv", p, hint, creds)
	if v.Allow {
		t.Errorf("reuse_login without cached login password should reject")
	}
	if v.Code != channel.RejectCodeLoginPasswordMissing {
		t.Errorf("expected %q, got %q", channel.RejectCodeLoginPasswordMissing, v.Code)
	}
}

func TestEvaluateElevation_EmptyTargetUsers_AnyAllowed(t *testing.T) {
	p := channel.ElevationPolicy{
		Allowed:     true,
		Mode:        "password",
		TargetUsers: nil, // empty = any allowed
	}
	hint := channel.AccountTransitionHint{IsEscalation: true, TargetUser: "anyuser"}
	creds := &staticCreds{sudo: map[string]string{"srv|anyuser": "pw"}}
	v := channel.EvaluateElevation("srv", p, hint, creds)
	if !v.Allow {
		t.Errorf("empty TargetUsers should allow any target")
	}
}
