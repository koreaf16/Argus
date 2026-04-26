// Package schedule
// File: log.go
// Description: 스케줄 실행 로그 — 파일 append + 100MB 회전 + 크리덴셜 마스킹
// Responsibility: Logger 타입 제공 — 각 스케줄 실행마다 append, 임계 초과 시 파일 회전

package schedule

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultLogMaxSize는 기본 로그 회전 임계 (100 MB).
const DefaultLogMaxSize int64 = 100 * 1024 * 1024

// Logger는 스케줄 실행 로그를 파일에 기록한다.
// 모든 메서드는 동시 호출에 안전하다.
type Logger struct {
	path    string
	maxSize int64
	mu      sync.Mutex
}

// NewLogger는 Logger를 생성한다. maxSize가 0이면 DefaultLogMaxSize를 사용한다.
func NewLogger(path string, maxSize int64) *Logger {
	if maxSize <= 0 {
		maxSize = DefaultLogMaxSize
	}
	return &Logger{path: path, maxSize: maxSize}
}

// Write는 스케줄 실행 결과를 로그 파일에 append한다.
// prompt와 result는 크리덴셜 마스킹 후 기록된다.
func (l *Logger) Write(name, prompt, result string, execErr error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.buildEntry(name, prompt, result, execErr)

	if err := l.ensureDir(); err != nil {
		slog.Warn("schedule log mkdir", "err", err)
		return
	}
	if err := l.rotateIfNeeded(); err != nil {
		slog.Warn("schedule log rotate", "err", err)
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		slog.Warn("open schedule log", "path", l.path, "err", err)
		return
	}
	defer f.Close()

	if _, err := fmt.Fprint(f, entry); err != nil {
		slog.Warn("write schedule log", "err", err)
	}
}

// buildEntry는 로그 항목 문자열을 구성한다.
func (l *Logger) buildEntry(name, prompt, result string, execErr error) string {
	status := "ok"
	errLine := ""
	if execErr != nil {
		status = "error"
		errLine = fmt.Sprintf("error: %s\n", MaskCredentials(execErr.Error()))
	}
	return fmt.Sprintf("[%s] schedule=%s status=%s\nprompt: %s\nresult: %s\n%s---\n",
		time.Now().Format(time.RFC3339),
		name, status,
		MaskCredentials(prompt),
		MaskCredentials(result),
		errLine,
	)
}

// ensureDir는 로그 파일 디렉토리가 존재하도록 생성한다.
func (l *Logger) ensureDir() error {
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return nil
}

// rotateIfNeeded는 로그 파일이 maxSize를 초과하면 회전한다.
// 호출 전 mu 잠금이 보유되어 있어야 한다.
func (l *Logger) rotateIfNeeded() error {
	info, err := os.Stat(l.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat log: %w", err)
	}
	if info.Size() < l.maxSize {
		return nil
	}
	return rotateLogFile(l.path)
}

// rotateLogFile은 path 를 .1, .1→.2, ... 순으로 이동한다 (최대 9단).
func rotateLogFile(path string) error {
	const maxRotations = 9
	for i := maxRotations - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("rotate %s → %s: %w", src, dst, err)
		}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return fmt.Errorf("rotate %s → %s.1: %w", path, path, err)
	}
	return nil
}
