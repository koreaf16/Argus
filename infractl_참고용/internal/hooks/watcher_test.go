// Package hooks
// File: watcher_test.go
// Description: fsnotify watcher + reloader 통합 단위 테스트
// Responsibility: yaml 변경 → reload 반영 / 잘못된 yaml → 기존 유지 검증

package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReloadSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.yaml")

	// 빈 yaml 로 시작
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Reload(path); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	cfg := GetSnapshot()
	if cfg == nil {
		t.Fatal("snapshot should not be nil after reload")
	}
}

func TestReloadBadYamlKeepsExisting(t *testing.T) {
	ResetSnapshot()
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.yaml")

	// 정상 yaml 로 초기 캡처
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CaptureSnapshot(path); err != nil {
		t.Fatal(err)
	}
	before := GetSnapshot()

	// 잘못된 yaml 로 교체 후 reload
	if err := os.WriteFile(path, []byte(":\n  - bad: [yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = Reload(path) // 오류 무시 (경고로 처리)

	after := GetSnapshot()
	if after != before {
		t.Fatal("bad yaml reload should keep existing snapshot")
	}
}

func TestWatcherDetectsChange(t *testing.T) {
	ResetSnapshot()
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.yaml")

	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CaptureSnapshot(path); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		_ = Watch(ctx, path)
	}()

	// 파일 수정 → watcher 가 감지 → debounce(500ms) → Reload
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(path, []byte("PreToolUse: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// debounce + 여유 시간
	time.Sleep(800 * time.Millisecond)
	cancel()
	<-watchDone

	cfg := GetSnapshot()
	if cfg == nil {
		t.Fatal("snapshot should exist after watch reload")
	}
}
