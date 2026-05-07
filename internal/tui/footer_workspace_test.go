package tui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/koreaf16/argus/internal/services/inventory"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/state"
)

func TestBuildFooterMsgKeepsLocalWorkDirWhenInventoryHasCWD(t *testing.T) {
	workDir := t.TempDir()
	inventoryCWD := filepath.Join(t.TempDir(), "inventory-home")

	mgr := workspace.NewManager(workspace.NewRegistry(""), nil)
	mgr.SetInventorySnapshot(workspace.LocalAlias, inventory.InventorySnapshot{
		Alias:       workspace.LocalAlias,
		Status:      inventory.StatusPartial,
		CollectedAt: time.Now(),
		System: &inventory.SystemInfo{
			CWD: inventoryCWD,
		},
	})

	appState := state.NewAppState()
	a := &app{cfg: Config{
		State:     appState,
		WorkDir:   workDir,
		Workspace: mgr,
	}}

	msg := a.buildFooterMsg()
	if msg.Footer.CWD != workDir {
		t.Fatalf("footer cwd = %q, want local workdir %q", msg.Footer.CWD, workDir)
	}
	if got := appState.ActiveTarget().CWD; got != workDir {
		t.Fatalf("active target cwd = %q, want %q", got, workDir)
	}
}
