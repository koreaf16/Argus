package todostore

import (
	"testing"

	"github.com/koreaf16/argus/internal/types"
)

func TestSyncForStepsPreservesActiveFormWhenSameSteps(t *testing.T) {
	steps := []Step{
		{Tool: "bash", Prompt: "echo 1"},
		{Tool: "powershell", Prompt: "Get-Date"},
	}
	existing := []types.TodoItem{
		{Content: "bash: echo 1", ActiveForm: "doing a", Status: types.TodoStatusCompleted},
		{Content: "powershell: Get-Date", ActiveForm: "doing b", Status: types.TodoStatusInProgress},
	}
	got := SyncForSteps(existing, steps)
	if len(got) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(got))
	}
	if got[0].Content != "bash: echo 1" || got[0].ActiveForm != "doing a" || got[0].Status != types.TodoStatusPending {
		t.Fatalf("unexpected todo[0]: %+v", got[0])
	}
	if got[1].Content != "powershell: Get-Date" || got[1].ActiveForm != "doing b" || got[1].Status != types.TodoStatusPending {
		t.Fatalf("unexpected todo[1]: %+v", got[1])
	}
}

func TestSyncForStepsRebuildsStaleContentWhenSizesMatch(t *testing.T) {
	steps := []Step{
		{Tool: "bash", Prompt: "echo 2"},
	}
	existing := []types.TodoItem{
		{Content: "bash: echo 1", ActiveForm: "doing old", Status: types.TodoStatusCompleted},
	}
	got := SyncForSteps(existing, steps)
	if len(got) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(got))
	}
	if got[0].Content != "bash: echo 2" || got[0].ActiveForm != "running step 1" {
		t.Fatalf("unexpected rebuilt todo: %+v", got[0])
	}
}

func TestSyncForStepsRebuildsWhenSizesDiffer(t *testing.T) {
	steps := []Step{
		{Tool: "bash", Prompt: "echo 1"},
	}
	existing := []types.TodoItem{
		{Content: "a", ActiveForm: "doing a", Status: types.TodoStatusPending},
		{Content: "b", ActiveForm: "doing b", Status: types.TodoStatusPending},
	}
	got := SyncForSteps(existing, steps)
	if len(got) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(got))
	}
	if got[0].Content != "bash: echo 1" || got[0].ActiveForm != "running step 1" {
		t.Fatalf("unexpected rebuilt todo: %+v", got[0])
	}
}

func TestNormalizeForStorageClearsAllCompleted(t *testing.T) {
	src := []types.TodoItem{
		{Content: "a", ActiveForm: "a", Status: types.TodoStatusCompleted},
		{Content: "b", ActiveForm: "b", Status: types.TodoStatusCompleted},
	}
	got := NormalizeForStorage(src)
	if got != nil {
		t.Fatalf("expected nil when all completed, got %+v", got)
	}
}
