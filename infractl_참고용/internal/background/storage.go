// Package background
// File: storage.go
// Description: 백그라운드 작업 출력 파일 I/O — 생성/회전/정리
// Responsibility: ~/<storageDir>/<id>.{stdout,stderr,status} 파일 생성·쓰기·크기 회전·보관 정책 실행

package background

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	extStdout = ".stdout"
	extStderr = ".stderr"
	extStatus = ".status"
)

func jobFilePaths(storageDir string, id int) (stdout, stderr, status string) {
	base := filepath.Join(storageDir, fmt.Sprintf("%d", id))
	return base + extStdout, base + extStderr, base + extStatus
}

// openJobFiles는 storageDir를 생성하고 stdout/stderr 파일을 0600 권한으로 연다.
// 파일이 maxFileSize 이상이면 .1, .2, ... 순차 회전 후 새 파일을 생성한다.
func openJobFiles(storageDir string, id int, maxFileSize int64) (*os.File, *os.File, error) {
	if err := os.MkdirAll(storageDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("create storage dir: %w", err)
	}

	stdoutPath, stderrPath, _ := jobFilePaths(storageDir, id)

	stdout, err := openWithRotation(stdoutPath, maxFileSize)
	if err != nil {
		return nil, nil, fmt.Errorf("open stdout file: %w", err)
	}

	stderr, err := openWithRotation(stderrPath, maxFileSize)
	if err != nil {
		stdout.Close()
		return nil, nil, fmt.Errorf("open stderr file: %w", err)
	}

	return stdout, stderr, nil
}

func openWithRotation(path string, maxSize int64) (*os.File, error) {
	if fi, err := os.Stat(path); err == nil && fi.Size() >= maxSize {
		if rotErr := rotateFile(path); rotErr != nil {
			slog.Warn("file rotation failed, overwriting", "path", path, "err", rotErr)
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
}

// rotateFile은 path → path.1 으로 이동한다. 기존 .N 파일은 .N+1 로 밀어낸다 (최대 9단).
func rotateFile(path string) error {
	const maxRotations = 9
	for i := maxRotations; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("rotate %s -> %s: %w", src, dst, err)
		}
	}
	return os.Rename(path, path+".1")
}

// writeStatusFile은 <id>.status 파일에 완료 상태와 에러 메시지를 기록한다.
func writeStatusFile(storageDir string, id int, status JobStatus, errMsg string) error {
	_, _, statusPath := jobFilePaths(storageDir, id)
	content := fmt.Sprintf("status:%s\nerror:%s\n", status, errMsg)
	return os.WriteFile(statusPath, []byte(content), 0600)
}

// cleanOldFiles는 보관 정책(keepDays, maxResults)에 따라 오래된 작업 파일을 삭제한다.
func cleanOldFiles(storageDir string, keepDays, maxResults int) error {
	entries, err := collectStorageEntries(storageDir)
	if err != nil {
		return err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime.After(entries[j].modTime)
	})

	cutoff := time.Now().AddDate(0, 0, -keepDays)
	for i, e := range entries {
		if i >= maxResults || e.modTime.Before(cutoff) {
			for _, p := range e.paths {
				if removeErr := os.Remove(p); removeErr != nil && !os.IsNotExist(removeErr) {
					slog.Warn("failed to remove old bg file", "path", p, "err", removeErr)
				}
			}
		}
	}
	return nil
}

type storageEntry struct {
	id      int
	modTime time.Time
	paths   []string
}

func collectStorageEntries(storageDir string) ([]storageEntry, error) {
	dirEntries, err := os.ReadDir(storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read storage dir: %w", err)
	}

	byID := map[int]*storageEntry{}
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if !strings.HasSuffix(name, extStdout) {
			continue
		}
		var id int
		if _, scanErr := fmt.Sscanf(strings.TrimSuffix(name, extStdout), "%d", &id); scanErr != nil {
			continue
		}
		fi, infoErr := de.Info()
		if infoErr != nil {
			continue
		}
		entry := &storageEntry{id: id, modTime: fi.ModTime()}
		sp, ep, stp := jobFilePaths(storageDir, id)
		for _, p := range []string{sp, ep, stp} {
			if _, statErr := os.Stat(p); statErr == nil {
				entry.paths = append(entry.paths, p)
			}
		}
		byID[id] = entry
	}

	result := make([]storageEntry, 0, len(byID))
	for _, e := range byID {
		result = append(result, *e)
	}
	return result, nil
}

// jobStdoutPath는 주어진 storageDir와 jobID의 stdout 파일 경로를 반환한다.
func jobStdoutPath(storageDir string, id int) string {
	p, _, _ := jobFilePaths(storageDir, id)
	return p
}

// jobIsFinished는 <id>.status 파일에서 작업 완료 여부를 판단한다.
func jobIsFinished(storageDir string, id int) bool {
	_, _, statusPath := jobFilePaths(storageDir, id)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, "status:completed") ||
		strings.Contains(s, "status:failed") ||
		strings.Contains(s, "status:cancelled")
}
