package workspace

import "testing"

func TestParseEndpointPathAlias(t *testing.T) {
	alias, path, err := ParseEndpointPath("dev:/tmp/a.zip", LocalAlias)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias != "dev" {
		t.Fatalf("alias=%q, want dev", alias)
	}
	if path != "/tmp/a.zip" {
		t.Fatalf("path=%q, want /tmp/a.zip", path)
	}
}

func TestParseEndpointPathWindowsAbsolute(t *testing.T) {
	alias, path, err := ParseEndpointPath(`C:\Users\jhkwa\Downloads\a.zip`, "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias != LocalAlias {
		t.Fatalf("alias=%q, want %q", alias, LocalAlias)
	}
	if path != `C:\Users\jhkwa\Downloads\a.zip` {
		t.Fatalf("path=%q, want drive path", path)
	}
}

func TestParseEndpointPathFallbackActive(t *testing.T) {
	alias, path, err := ParseEndpointPath("/var/log/app.log", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias != LocalAlias {
		t.Fatalf("alias=%q, want %q", alias, LocalAlias)
	}
	if path != "/var/log/app.log" {
		t.Fatalf("path=%q", path)
	}
}

func TestParseEndpointPathEmpty(t *testing.T) {
	_, _, err := ParseEndpointPath("   ", LocalAlias)
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
}
