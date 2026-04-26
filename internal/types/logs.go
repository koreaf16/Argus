// Package types — 세션 로그·트랜스크립트 타입.
//
// 파일 역할: 세션 메시지를 파일로 직렬화할 때 사용하는 타입을 정의한다.
//
//	Phase 1 에서는 기본 LogOption 과 SerializedMessage 만 포함한다.
//	이후 히스토리 재개 기능에서 확장된다.
//
// 포함 모듈:
//   - SerializedMessage: 파일로 저장할 때 메시지에 붙는 세션 메타데이터.
//   - LogOption: 로그 파일 하나를 나타내는 타입.
//   - SortLogs(): 수정 시각 내림차순 정렬 헬퍼.
//
// 호출/사용 방식:
//   - internal/state/ 에서 세션 히스토리 저장·로드 시 참조.
//
// 연결:
//   - 원본: src/types/logs.ts (Phase 1 핵심만 포함)
package types

import (
	"sort"
	"time"
)

// SerializedMessage 는 디스크에 저장되는 메시지 + 세션 메타데이터다.
type SerializedMessage struct {
	Message
	CWD       string    `json:"cwd"`
	UserType  string    `json:"userType"`
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	GitBranch string    `json:"gitBranch,omitempty"`
}

// LogOption 은 로그 파일 하나를 나타낸다.
type LogOption struct {
	Date         string              `json:"date"`
	Messages     []SerializedMessage `json:"messages,omitempty"`
	FullPath     string              `json:"fullPath,omitempty"`
	Created      time.Time           `json:"created"`
	Modified     time.Time           `json:"modified"`
	FirstPrompt  string              `json:"firstPrompt"`
	MessageCount int                 `json:"messageCount"`
	FileSize     int64               `json:"fileSize,omitempty"`
	IsSidechain  bool                `json:"isSidechain"`
	SessionID    string              `json:"sessionId,omitempty"`
}

// SortLogs 는 로그 목록을 수정 시각 내림차순으로 정렬한다.
func SortLogs(logs []LogOption) {
	sort.Slice(logs, func(i, j int) bool {
		diff := logs[j].Modified.Sub(logs[i].Modified)
		if diff != 0 {
			return diff < 0
		}
		return logs[j].Created.Sub(logs[i].Created) < 0
	})
}
