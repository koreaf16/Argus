// Package background
// File: storage_test.go
// Description: storage.go 파일 I/O 및 회전 정책 단위 테스트
// Responsibility: openJobFiles, rotateFile, writeStatusFile, cleanOldFiles 검증

package background

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOpenJobFilesCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	so, se, err := openJobFiles(dir, 1, 10*1024*1024)
	if err != nil {
		t.Fatalf("openJobFiles: %v", err)
	}
	so.Close()
	se.Close()

	stdoutPath := filepath.Join(dir, "1.stdout")
	stderrPath := filepath.Join(dir, "1.stderr")
	if _, err := os.Stat(stdoutPath); err != nil {
		t.Errorf("stdout file missing: %v", err)
	}
	if _, err := os.Stat(stderrPath); err != nil {
		t.Errorf("stderr file missing: %v", err)
	}
}

func TestOpenJobFilesPermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-style file permissions not enforced on Windows")
	}
	dir := t.TempDir()
	so, se, err := openJobFiles(dir, 2, 10*1024*1024)
	if err != nil {
		t.Fatalf("openJobFiles: %v", err)
	}
	so.Close()
	se.Close()

	fi, err := os.Stat(filepath.Join(dir, "2.stdout"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("want perm 0600, got %o", fi.Mode().Perm())
	}
}

func TestRotateFileShiftsSequentially(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.stdout")

	// 원본 파일 생성
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := rotateFile(path); err != nil {
		t.Fatalf("rotateFile: %v", err)
	}

	// 원본이 .1로 이동됐어야 함
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original file should not exist after rotation")
	}
	data, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read .1: %v", err)
	}
	if string(data) != "original" {
		t.Errorf("want 'original', got %q", data)
	}
}

func TestWriteStatusFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeStatusFile(dir, 3, StatusCompleted, ""); err != nil {
		t.Fatalf("writeStatusFile: %v", err)
	}

	_, _, statusPath := jobFilePaths(dir, 3)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(data), "status:completed") {
		t.Errorf("status file content: %q", data)
	}
}

func TestJobIsFinished(t *testing.T) {
	dir := t.TempDir()

	// 파일 없음 → false
	if jobIsFinished(dir, 99) {
		t.Error("expected false when no status file")
	}

	// completed → true
	if err := writeStatusFile(dir, 10, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	if !jobIsFinished(dir, 10) {
		t.Error("expected true for completed")
	}
}

func TestCleanOldFilesRespectsMaxResults(t *testing.T) {
	dir := t.TempDir()

	// 5개 작업 파일 생성 (각기 다른 수정 시간)
	for i := 1; i <= 5; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%d.stdout", i))
		if err := os.WriteFile(p, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		// 수정 시간 조정
		mtime := time.Now().Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	// maxResults=3, keepDays=365 → 오래된 2개 삭제
	if err := cleanOldFiles(dir, 365, 3); err != nil {
		t.Fatalf("cleanOldFiles: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("want 3 files remaining, got %d", len(entries))
	}
}
