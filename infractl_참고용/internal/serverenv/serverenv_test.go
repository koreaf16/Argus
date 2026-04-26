package serverenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateDirUsesCurrentWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if got != filepath.Join(dir, StateDirName) {
		t.Fatalf("StateDir=%q, want %q", got, filepath.Join(dir, StateDirName))
	}
}

func TestPOSIXShellPathExpandsHome(t *testing.T) {
	if got := POSIXShellPath(""); got != "$HOME/'.infractl/workspace'" {
		t.Fatalf("default POSIXShellPath=%q", got)
	}
	if got := POSIXShellPath("~/work dir"); got != "$HOME/'work dir'" {
		t.Fatalf("home POSIXShellPath=%q", got)
	}
	if got := POSIXShellPath("/opt/work"); got != "'/opt/work'" {
		t.Fatalf("absolute POSIXShellPath=%q", got)
	}
}
