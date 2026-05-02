package workspace

import "testing"

func TestAccountTargetResolvesToParentHost(t *testing.T) {
	reg := NewRegistry("")
	if err := reg.Add(ServerEntry{
		Alias: "parent",
		Kind:  ServerKindSSH,
		Host:  "10.0.0.10",
		User:  "master",
	}); err != nil {
		t.Fatalf("add parent: %v", err)
	}
	if err := reg.Add(ServerEntry{
		Alias:        "parent-app",
		Kind:         ServerKindAccount,
		ParentAlias:  "parent",
		User:         "app",
		SwitchMethod: "su",
	}); err != nil {
		t.Fatalf("add account: %v", err)
	}

	mgr := NewManager(reg, nil)
	target, err := mgr.ResolveExecutionTarget("parent-app")
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if !target.IsAccount {
		t.Fatal("target should be account")
	}
	if target.HostAlias != "parent" {
		t.Fatalf("host alias = %q, want parent", target.HostAlias)
	}
	if target.HostEntry.User != "master" || target.TargetUser != "app" {
		t.Fatalf("unexpected users: host=%q target=%q", target.HostEntry.User, target.TargetUser)
	}
	if !mgr.IsRemoteAlias("parent-app") {
		t.Fatal("account target should be remote")
	}

	spec, err := mgr.ResolveEntry("parent-app")
	if err != nil {
		t.Fatalf("resolve channel entry: %v", err)
	}
	if spec.Alias != "parent" || spec.User != "master" || spec.Host != "10.0.0.10" {
		t.Fatalf("channel entry should use parent host, got %+v", spec)
	}
}

func TestRemovingHostRemovesAccountTargets(t *testing.T) {
	reg := NewRegistry("")
	if err := reg.Add(ServerEntry{Alias: "parent", Kind: ServerKindSSH, Host: "10.0.0.10", User: "master"}); err != nil {
		t.Fatalf("add parent: %v", err)
	}
	if err := reg.Add(ServerEntry{Alias: "parent-app", Kind: ServerKindAccount, ParentAlias: "parent", User: "app"}); err != nil {
		t.Fatalf("add account: %v", err)
	}
	if err := reg.SetActive("parent-app"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if err := reg.Remove("parent"); err != nil {
		t.Fatalf("remove parent: %v", err)
	}
	if _, ok := reg.Get("parent-app"); ok {
		t.Fatal("child account target still exists after parent removal")
	}
	if reg.Active() != LocalAlias {
		t.Fatalf("active = %q, want local", reg.Active())
	}
}
