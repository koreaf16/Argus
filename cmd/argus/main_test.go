package main

import (
	"os"
	"testing"
	"time"

	"github.com/koreaf16/argus/internal/memdir"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/session"
	"github.com/koreaf16/argus/internal/state"
)

func TestParseFlagsResumeShort(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"argus", "-r", "abc123"}

	got, err := parseFlags()
	if err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if got.resume != "abc123" {
		t.Fatalf("expected resume=abc123, got %q", got.resume)
	}
}

func TestInitializeSessionResumeLoadsSnapshot(t *testing.T) {
	store := memdir.NewStore(t.TempDir())
	if err := store.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	saved := session.Snapshot{
		SavedAt: time.Now().UTC(),
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleUser, "hello"),
		},
	}
	if err := store.SaveSession("resume-id", saved); err != nil {
		t.Fatalf("save session: %v", err)
	}
	appState := state.NewAppState()
	snap, err := initializeSession(&parsedFlags{resume: "resume-id"}, appState, store)
	if err != nil {
		t.Fatalf("initialize session: %v", err)
	}
	if snap == nil || len(snap.Messages) != 1 {
		t.Fatalf("expected loaded snapshot with 1 message, got %#v", snap)
	}
	if appState.SessionID() != "resume-id" {
		t.Fatalf("expected session id resume-id, got %q", appState.SessionID())
	}
}

func TestParseFlagsAutoApprove(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"argus", "--auto-approve", "--aidebug", "-p", "hello"}

	got, err := parseFlags()
	if err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if !got.autoOK {
		t.Fatalf("expected auto-approve to be true")
	}
	if !got.aidebug {
		t.Fatalf("expected aidebug to be true")
	}
	if got.print != "hello" {
		t.Fatalf("expected print prompt to be hello, got %q", got.print)
	}
}

