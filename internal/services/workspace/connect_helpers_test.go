package workspace

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectAndActivateLocal(t *testing.T) {
	mgr := NewManager(NewRegistry(""), nil)

	alias, err := mgr.ConnectAndActivate(LocalAlias)
	if err != nil {
		t.Fatalf("ConnectAndActivate(local) returned error: %v", err)
	}
	if alias != LocalAlias {
		t.Fatalf("alias = %q, want %q", alias, LocalAlias)
	}
	if mgr.ActiveAlias() != LocalAlias {
		t.Fatalf("active alias = %q, want %q", mgr.ActiveAlias(), LocalAlias)
	}
}

func TestFormatPasswordPromptIncludesAccountTarget(t *testing.T) {
	reg := NewRegistry(filepath.Join(t.TempDir(), "servers.json"))
	if err := reg.Add(ServerEntry{
		Alias: "db",
		Kind:  ServerKindSSH,
		Host:  "10.0.0.10",
		Port:  22,
		User:  "oracle",
	}); err != nil {
		t.Fatalf("add server entry: %v", err)
	}

	got := FormatPasswordPrompt(reg, "db", "ssh", "")
	if !strings.Contains(got, "[db]") {
		t.Fatalf("prompt missing alias: %q", got)
	}
	if !strings.Contains(got, "oracle@10.0.0.10:22") {
		t.Fatalf("prompt missing account target: %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "password") {
		t.Fatalf("prompt missing password label: %q", got)
	}
}

func TestFormatPasswordPromptWithoutRegistry(t *testing.T) {
	got := FormatPasswordPrompt(nil, "devbox", "ssh", "SSH password:")
	want := "[devbox] SSH password:"
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestFormatInspectSummary(t *testing.T) {
	snap := InspectSnapshot{
		OS:        "Linux kernel\nNAME=Ubuntu",
		User:      "oracle",
		Shell:     "/bin/bash\n",
		Uptime:    " 1 day ",
		Services:  "svc-a\nsvc-b\n",
		Listeners: "80\n443\n22\n",
	}

	got := FormatInspectSummary(snap)
	if !strings.Contains(got, "OS: Linux kernel") {
		t.Fatalf("missing OS summary: %q", got)
	}
	if !strings.Contains(got, "user: oracle") {
		t.Fatalf("missing user summary: %q", got)
	}
	if !strings.Contains(got, "shell: /bin/bash") {
		t.Fatalf("missing shell summary: %q", got)
	}
	if !strings.Contains(got, "services: 2 running") {
		t.Fatalf("missing services summary: %q", got)
	}
	if !strings.Contains(got, "listeners: 3 ports") {
		t.Fatalf("missing listeners summary: %q", got)
	}
}
