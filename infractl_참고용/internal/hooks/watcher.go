// Package hooks
// File: watcher.go
// Description: fsnotify 기반 hooks.yaml 파일 감시 및 핫리로드 트리거
// Responsibility: ctx 취소까지 파일 변경 이벤트를 감시하고 Reload 를 호출

package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watcherDebounce = 500 * time.Millisecond

// Watch 는 path 파일을 감시하다가 변경 감지 시 Reload 를 호출한다.
// ctx 취소 시 정리 후 반환한다. 에디터의 atomic save (rename) 도 감지한다.
// 이 함수는 goroutine 에서 실행해야 한다.
func Watch(ctx context.Context, path string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(path); err != nil {
		// 파일이 없어도 감시 실패는 경고로만 남기고 진행
		slog.Warn("hooks.yaml watcher: cannot watch file", "path", path, "err", err)
	}

	var debounceTimer *time.Timer
	triggerReload := func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(watcherDebounce, func() {
			// rename/create 이후 파일 재등록 (editor atomic save 대응)
			_ = watcher.Add(path)
			if err := Reload(path); err != nil {
				slog.Warn("hooks.yaml hot reload failed", "err", err)
			}
		})
	}

	slog.Info("hooks.yaml watcher started", "path", path)
	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			slog.Info("hooks.yaml watcher stopped")
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				slog.Debug("hooks.yaml change detected", "op", event.Op.String(), "path", event.Name)
				triggerReload()
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Warn("hooks.yaml watcher error", "err", err)
		}
	}
}
