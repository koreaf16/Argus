// Package executor
// File: truncate_test.go
// Description: TruncateOutput / LimitedWriter 단위 테스트
// Responsibility: 출력 크기 제한 로직 검증

package executor

import (
	"bytes"
	"strings"
	"testing"
)

func TestTruncateOutput_NoTruncation(t *testing.T) {
	s := strings.Repeat("a", MaxOutputBytes-1)
	got := TruncateOutput(s, MaxOutputBytes)
	if got != s {
		t.Fatal("should not truncate when within limit")
	}
}

func TestTruncateOutput_Truncates(t *testing.T) {
	s := strings.Repeat("b", MaxOutputBytes+100)
	got := TruncateOutput(s, MaxOutputBytes)
	if len(got) <= MaxOutputBytes {
		t.Fatalf("expected truncation notice to extend beyond %d, got %d", MaxOutputBytes, len(got))
	}
	if !strings.HasPrefix(got[:MaxOutputBytes], strings.Repeat("b", MaxOutputBytes)) {
		t.Fatal("first MaxOutputBytes should be original content")
	}
	if !strings.Contains(got, "[출력 잘림:") {
		t.Fatal("truncation notice not found in output")
	}
}

func TestLimitedWriter_LimitsWrites(t *testing.T) {
	var buf bytes.Buffer
	lw := &LimitedWriter{Buf: &buf, Limit: 10}

	n, err := lw.Write([]byte("hello")) // 5 bytes, fits
	if err != nil || n != 5 {
		t.Fatalf("unexpected write result: n=%d err=%v", n, err)
	}

	n, err = lw.Write([]byte("world!")) // 6 bytes, only 5 fit
	if err != nil || n != 6 { // n must equal len(p)
		t.Fatalf("unexpected write result: n=%d err=%v", n, err)
	}

	if buf.String() != "helloworld" {
		t.Fatalf("expected 'helloworld', got %q", buf.String())
	}
}

func TestLimitedWriter_DropsBeyondLimit(t *testing.T) {
	var buf bytes.Buffer
	lw := &LimitedWriter{Buf: &buf, Limit: 5}

	lw.Write([]byte("hello")) // fills limit
	lw.Write([]byte("MORE")) // should be silently dropped

	if buf.String() != "hello" {
		t.Fatalf("expected 'hello', got %q", buf.String())
	}
}
