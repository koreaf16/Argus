package bash

import "testing"

func TestResolveTargetAlias(t *testing.T) {
	if got := resolveTargetAlias(map[string]any{"server": "a100-server"}); got != "a100-server" {
		t.Fatalf("resolveTargetAlias(server) = %q", got)
	}
	if got := resolveTargetAlias(map[string]any{"_active_workspace": "remote"}); got != "remote" {
		t.Fatalf("resolveTargetAlias(active) = %q", got)
	}
	if got := resolveTargetAlias(map[string]any{}); got != "local" {
		t.Fatalf("resolveTargetAlias(default) = %q", got)
	}
}

func TestOnStreamDeltaCapsBuffer(t *testing.T) {
	m := &BashInteractiveModel{}
	large := make([]byte, maxShellOutputBufferChars+1024)
	for i := range large {
		large[i] = 'a'
	}
	m.OnStreamDelta(string(large))
	if len(m.output) > maxShellOutputBufferChars {
		t.Fatalf("output len = %d, want <= %d", len(m.output), maxShellOutputBufferChars)
	}
}
