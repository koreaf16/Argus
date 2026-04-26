package executor

import "testing"

func TestFirstWindowsPathDetectsDrivePath(t *testing.T) {
	got, ok := FirstWindowsPath(`Get-ChildItem -LiteralPath 'C:\Users\jhkwa\Downloads'`)
	if !ok {
		t.Fatal("expected Windows drive path to be detected")
	}
	if got != `C:\Users\jhkwa\Downloads` {
		t.Fatalf("path = %q, want %q", got, `C:\Users\jhkwa\Downloads`)
	}
}

func TestFirstWindowsPathDetectsUNCPath(t *testing.T) {
	got, ok := FirstWindowsPath(`copy \\fileserver\share\oracle.zip /tmp/oracle.zip`)
	if !ok {
		t.Fatal("expected UNC path to be detected")
	}
	if got != `\\fileserver\share\oracle.zip` {
		t.Fatalf("path = %q, want %q", got, `\\fileserver\share\oracle.zip`)
	}
}

func TestFirstWindowsPathIgnoresPOSIXPath(t *testing.T) {
	if got, ok := FirstWindowsPath(`/home/oracle/LINUX.X64_193000_db_home.zip`); ok {
		t.Fatalf("unexpected Windows path %q", got)
	}
}
