// Package background
// File: poller.go
// Description: 백그라운드 작업 stdout 파일 tail 읽기
// Responsibility: 주어진 byte offset부터 최대 maxBytes를 읽어 반환, 작업 완료 여부 보고

package background

import (
	"fmt"
	"io"
	"os"
)

// PollResult는 단일 폴 호출의 결과이다.
type PollResult struct {
	Data      []byte // 읽은 바이트 (len <= MaxBytes)
	NewOffset int64  // 다음 호출에 사용할 offset
	Done      bool   // true이면 작업이 완료되어 추가 폴링 불필요
	Truncated bool   // true이면 maxBytes를 초과해 잘렸음
}

// PollStdout은 jobID의 stdout 파일에서 offset부터 최대 maxBytes를 읽어 반환한다.
// storageDir와 jobID로 파일 경로를 결정한다.
// 파일이 아직 없으면 빈 결과를 반환한다 (작업이 아직 출력을 쓰지 않은 경우).
func PollStdout(storageDir string, id int, offset int64, maxBytes int) (PollResult, error) {
	path := jobStdoutPath(storageDir, id)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 파일 아직 없음 — 작업이 곧 생성할 것
			done := jobIsFinished(storageDir, id)
			return PollResult{NewOffset: offset, Done: done}, nil
		}
		return PollResult{}, fmt.Errorf("open stdout file: %w", err)
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return PollResult{}, fmt.Errorf("seek stdout file: %w", err)
	}

	buf := make([]byte, maxBytes+1) // +1 로 truncation 감지
	n, readErr := f.Read(buf)
	if readErr != nil && readErr != io.EOF {
		return PollResult{}, fmt.Errorf("read stdout file: %w", readErr)
	}

	truncated := n > maxBytes
	if truncated {
		n = maxBytes
	}

	done := jobIsFinished(storageDir, id)
	return PollResult{
		Data:      buf[:n],
		NewOffset: offset + int64(n),
		Done:      done,
		Truncated: truncated,
	}, nil
}
