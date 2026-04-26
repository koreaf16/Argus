package session_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	ctxpkg "github.com/koreaf16/argus/internal/context"
	"github.com/koreaf16/argus/internal/memdir"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/session"
)

func TestSnapshotV1RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := memdir.NewStore(dir)
	if err := store.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	// v1 snapshot 저장
	snap := session.Snapshot{
		SavedAt: time.Now().UTC(),
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleUser, "hello"),
			llm.TextMessage(llm.RoleAssistant, "hi"),
		},
	}
	if err := store.SaveSession("test-v1", snap); err != nil {
		t.Fatal(err)
	}

	// 로드 후 검증
	var loaded session.Snapshot
	if err := store.LoadSession("test-v1", &loaded); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.IsV2() {
		t.Error("v1 snapshot should not report IsV2()")
	}
}

func TestSnapshotV2RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := memdir.NewStore(dir)
	if err := store.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	// v2 snapshot 저장
	snap := session.Snapshot{
		Version: session.SnapshotVersion,
		SavedAt: time.Now().UTC(),
		Messages: []llm.Message{
			llm.TextMessage(llm.RoleUser, "hello"),
		},
		Graph: []*ctxpkg.Node{
			{ID: "1-user", Kind: ctxpkg.NodeKindUser, Seq: 1, Text: "hello"},
			{ID: "2-assistant", Kind: ctxpkg.NodeKindAssistant, Seq: 2, Text: "hi"},
		},
		Artifacts: []*ctxpkg.ArtifactRef{
			{ID: "1-bash-abc123", Path: "/tmp/artifact.txt", Chars: 5000, ToolName: "bash"},
		},
	}
	if err := store.SaveSession("test-v2", snap); err != nil {
		t.Fatal(err)
	}

	// 로드 후 검증
	var loaded session.Snapshot
	if err := store.LoadSession("test-v2", &loaded); err != nil {
		t.Fatal(err)
	}
	if !loaded.IsV2() {
		t.Error("v2 snapshot should report IsV2()")
	}
	if len(loaded.Graph) != 2 {
		t.Errorf("expected 2 graph nodes, got %d", len(loaded.Graph))
	}
	if len(loaded.Artifacts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(loaded.Artifacts))
	}
}

func TestSessionNewID(t *testing.T) {
	id1, err := session.NewID()
	if err != nil {
		t.Fatal(err)
	}
	id2, err := session.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Error("session IDs should be unique")
	}
	if len(id1) != 36 {
		t.Errorf("expected UUID length 36, got %d", len(id1))
	}
}

func TestArtifactStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	store := ctxpkg.NewArtifactStore(dir, "sess-test")
	if err := store.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	content := "line1\nline2\nline3"
	ref, err := store.Save(1, "bash", "call-xyz", content)
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID == "" {
		t.Error("artifact ID should not be empty")
	}

	// 파일 존재 확인
	if _, err := os.Stat(ref.Path); os.IsNotExist(err) {
		t.Error("artifact file should exist")
	}

	// 경로 검증: session-artifacts/sess-test/ 아래여야 함
	expectedDir := filepath.Join(dir, "session-artifacts", "sess-test")
	if filepath.Dir(ref.Path) != expectedDir {
		t.Errorf("artifact path dir mismatch: got %s, want %s", filepath.Dir(ref.Path), expectedDir)
	}
}
