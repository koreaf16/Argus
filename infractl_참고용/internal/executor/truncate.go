// Package executor
// File: truncate.go
// Description: 명령 출력 크기 제한 공유 헬퍼
// Responsibility: stdout/stderr 버퍼의 최대 크기 제한 및 truncation 메시지 부착

package executor

import (
	"bytes"
	"fmt"
)

// MaxOutputBytes는 모든 executor가 공유하는 출력 캡처 최대 크기이다 (64KB).
const MaxOutputBytes = 64 * 1024

// LimitedWriter는 bytes.Buffer에 쓸 때 limit 바이트 이후를 조용히 버린다.
// session.Stdout/session.Stderr에 직접 할당하여 메모리 폭발을 방지한다.
type LimitedWriter struct {
	Buf   *bytes.Buffer
	Limit int
}

// Write는 io.Writer를 구현한다. Limit 초과분은 조용히 버린다.
// 호출자가 에러로 인식하지 않도록 항상 len(p)를 반환한다.
func (w *LimitedWriter) Write(p []byte) (int, error) {
	origLen := len(p)
	remaining := w.Limit - w.Buf.Len()
	if remaining <= 0 {
		// 이미 한계에 도달 — TruncateOutput이 종료 후 메시지를 붙인다
		return origLen, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	if _, err := w.Buf.Write(p); err != nil {
		return 0, fmt.Errorf("limited writer: %w", err)
	}
	return origLen, nil // 원본 길이를 반환해 Write 호출자가 에러로 인식하지 않도록 함
}

// TruncateOutput은 문자열이 maxBytes를 초과하면 잘라내고 안내 메시지를 붙인다.
func TruncateOutput(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + fmt.Sprintf("\n... [출력 잘림: 전체 %d bytes]", len(s))
}
