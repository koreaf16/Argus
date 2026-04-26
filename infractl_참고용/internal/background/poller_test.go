// Package background
// File: poller_test.go
// Description: poller.go PollStdout 단위 테스트
// Responsibility: offset 기반 증분 읽기, truncation, done 플래그 검증

package background

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPollStdoutBasicRead(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "1.stdout")

	content := "hello world"
	if err := os.WriteFile(stdoutPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	res, err := PollStdout(dir, 1, 0, 1024)
	if err != nil {
		t.Fatalf("PollStdout: %v", err)
	}
	if string(res.Data) != content {
		t.Errorf("want %q, got %q", content, res.Data)
	}
	if res.NewOffset != int64(len(content)) {
		t.Errorf("want offset %d, got %d", len(content), res.NewOffset)
	}
	if res.Truncated {
		t.Error("should not be truncated")
	}
}

func TestPollStdoutIncremental(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "2.stdout")

	if err := os.WriteFile(stdoutPath, []byte("abcdefgh"), 0600); err != nil {
		t.Fatal(err)
	}

	// 첫 4바이트
	res, err := PollStdout(dir, 2, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Data) != "abcd" {
		t.Errorf("first chunk: got %q", res.Data)
	}

	// 다음 4바이트
	res2, err := PollStdout(dir, 2, res.NewOffset, 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(res2.Data) != "efgh" {
		t.Errorf("second chunk: got %q", res2.Data)
	}
}

func TestPollStdoutTruncation(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "3.stdout")

	if err := os.WriteFile(stdoutPath, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}

	res, err := PollStdout(dir, 3, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Error("expected truncated=true")
	}
	if len(res.Data) != 5 {
		t.Errorf("want 5 bytes, got %d", len(res.Data))
	}
}

func TestPollStdoutFileNotExist(t *testing.T) {
	dir := t.TempDir()
	// 파일 없음 → 빈 결과, 에러 없음
	res, err := PollStdout(dir, 99, 0, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Data) != 0 {
		t.Errorf("want empty data, got %d bytes", len(res.Data))
	}
}

func TestPollStdoutDoneFlag(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "4.stdout")
	if err := os.WriteFile(stdoutPath, []byte("done"), 0600); err != nil {
		t.Fatal(err)
	}

	// status 파일 없음 → done=false
	res, err := PollStdout(dir, 4, 0, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if res.Done {
		t.Error("want done=false without status file")
	}

	// completed status 기록 → done=true
	if err := writeStatusFile(dir, 4, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	res2, err := PollStdout(dir, 4, 0, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Done {
		t.Error("want done=true with completed status")
	}
}
