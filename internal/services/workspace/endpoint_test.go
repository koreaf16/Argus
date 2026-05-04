package workspace

import (
	"runtime"
	"testing"
)

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

func TestParseEndpointPathGitBashStyleOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("git-bash MSYS path normalization only applies on Windows")
	}
	alias, path, err := ParseEndpointPath("local:/c/Users/jhkwa/Downloads/file.zip", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias != LocalAlias {
		t.Fatalf("alias=%q, want %q", alias, LocalAlias)
	}
	if path != `C:\Users\jhkwa\Downloads\file.zip` {
		t.Fatalf("path=%q, want C:\\Users\\jhkwa\\Downloads\\file.zip", path)
	}
}

func TestNormalizeLocalPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("normalizeLocalPath only converts on Windows")
	}
	cases := []struct {
		in, want string
	}{
		{"/c/Users/me/file.zip", `C:\Users\me\file.zip`},
		{"/d/data", `D:\data`},
		{"/C/foo/bar", `C:\foo\bar`},
		{`C:\already\windows`, `C:\already\windows`},
		{"/usr/local/bin", "/usr/local/bin"},
		{"/c", "/c"},
		{"/c/", `C:\`},
		{"", ""},
		{"/cabc/path", "/cabc/path"},
	}
	for _, tc := range cases {
		got := normalizeLocalPath(tc.in)
		if got != tc.want {
			t.Errorf("normalizeLocalPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
