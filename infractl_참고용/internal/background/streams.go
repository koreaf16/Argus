// Package background
// File: streams.go
// Description: 백그라운드 작업 파일 스트림 writer 쌍 정의
// Responsibility: JobStreams 타입 제공 — SubmitStreaming fn 인자로 전달되는 stdout/stderr writer

package background

import "io"

// JobStreams는 백그라운드 작업이 출력을 파일로 기록할 때 사용하는 writer 쌍이다.
// SubmitStreaming에 전달된 fn이 이 writer에 쓰면 storage.go의 파일에 기록된다.
type JobStreams struct {
	Stdout io.Writer
	Stderr io.Writer
}
