package workspace

import (
	"context"
	"os"
	"testing"

	"github.com/koreaf16/argus/internal/services/inventory"
)

func TestNativeLocalInventorySnapshotProvidesSystemInfo(t *testing.T) {
	snap := nativeLocalInventorySnapshot(context.Background(), inventory.StatusPartial)
	if snap.Status != inventory.StatusPartial {
		t.Fatalf("status = %s, want %s", snap.Status, inventory.StatusPartial)
	}
	if snap.System == nil {
		t.Fatal("expected system info")
	}
	if snap.System.OS == "" {
		t.Fatal("expected OS")
	}
	if snap.System.CWD == "" {
		t.Fatal("expected cwd")
	}
	if wd, err := os.Getwd(); err == nil && snap.System.CWD != wd {
		t.Fatalf("cwd = %q, want %q", snap.System.CWD, wd)
	}
}

func TestRescanInventoryStreamingLocalWithoutBashUsesNativeSnapshot(t *testing.T) {
	mgr := NewManager(NewRegistry(""), nil)
	if mgr.makeLocalInventoryExec() != nil {
		t.Skip("local bash is available; native fallback is not used")
	}
	var phases []inventory.InventoryPhase

	mgr.RescanInventoryStreaming(context.Background(), LocalAlias, func(phase inventory.InventoryPhase, snap inventory.InventorySnapshot, err error) {
		if err != nil {
			t.Fatalf("phase %s error: %v", phase, err)
		}
		phases = append(phases, phase)
		if snap.System == nil {
			t.Fatalf("phase %s missing system info", phase)
		}
	})

	if len(phases) == 0 {
		t.Fatal("expected at least one phase")
	}
}
