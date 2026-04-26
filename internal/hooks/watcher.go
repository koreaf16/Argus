package hooks

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/koreaf16/argus/internal/types"
)

// FileWatcher 는 fsnotify 기반 파일 변경 감시기다.
type FileWatcher struct {
	dispatcher *HookDispatcher
	watcher    *fsnotify.Watcher
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewFileWatcher 는 dispatcher 의 FileChanged 훅 설정을 읽어 감시 대상 파일 목록을 구성한다.
// FileChanged 훅이 없거나 감시 대상이 없으면 nil, nil 을 반환한다.
func NewFileWatcher(ctx context.Context, dispatcher *HookDispatcher) (*FileWatcher, error) {
	paths := collectWatchPaths(dispatcher)
	if len(paths) == 0 {
		return nil, nil
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	for _, p := range paths {
		// 파일이 없어도 오류를 무시 — 나중에 생성될 수 있다
		_ = w.Add(p)
	}

	wctx, cancel := context.WithCancel(ctx)
	return &FileWatcher{
		dispatcher: dispatcher,
		watcher:    w,
		ctx:        wctx,
		cancel:     cancel,
	}, nil
}

// Start 는 파일 변경 감시를 백그라운드 고루틴으로 시작한다.
func (fw *FileWatcher) Start() {
	go fw.loop()
}

// Stop 은 파일 감시를 중단하고 자원을 해제한다.
func (fw *FileWatcher) Stop() {
	fw.cancel()
	fw.watcher.Close()
}

func (fw *FileWatcher) loop() {
	for {
		select {
		case <-fw.ctx.Done():
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				fw.dispatcher.Dispatch(fw.ctx, types.HookEventFileChanged, HookInput{
					ToolName: event.Name, // 변경된 파일 경로
				})
			}
		case _, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			// 감시 오류는 무시 (로깅 레이어 없음)
		}
	}
}

// collectWatchPaths 는 FileChanged 훅 매처에서 감시 파일 경로 목록을 수집한다.
// 매처의 Matcher 필드는 파이프(|)로 구분된 파일명 목록이다.
func collectWatchPaths(dispatcher *HookDispatcher) []string {
	dispatcher.mu.RLock()
	cfgMatchers := dispatcher.config[types.HookEventFileChanged]
	sessMatchers := dispatcher.sessionHooks[types.HookEventFileChanged]
	workDir := dispatcher.workDir
	dispatcher.mu.RUnlock()

	seen := make(map[string]bool)
	var paths []string

	for _, matchers := range [][]HookMatcher{cfgMatchers, sessMatchers} {
		for _, m := range matchers {
			for _, name := range strings.Split(m.Matcher, "|") {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				p := filepath.Join(workDir, name)
				if !seen[p] {
					seen[p] = true
					paths = append(paths, p)
				}
			}
		}
	}
	return paths
}
