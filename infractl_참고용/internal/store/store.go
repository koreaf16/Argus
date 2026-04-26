// Package store
// File: store.go
// Description: 서버 저장소 인터페이스 정의
// Responsibility: 서버 정보 영속화 추상화 인터페이스 제공

package store

import "context"

// ServerStore는 서버 등록 정보의 영속화를 담당하는 인터페이스이다.
// 구현체: SQLiteStore
type ServerStore interface {
	// List는 등록된 모든 서버 목록을 반환한다.
	List(ctx context.Context) ([]Server, error)

	// Get은 이름으로 서버를 조회한다. 없으면 error를 반환한다.
	Get(ctx context.Context, name string) (Server, error)

	// Add는 새 서버를 등록한다. 이름이 중복이면 error를 반환한다.
	Add(ctx context.Context, server Server) error

	// Update는 기존 서버의 정보를 수정한다. 이름 기준으로 조회하여 업데이트한다.
	Update(ctx context.Context, server Server) error

	// Remove는 이름으로 서버를 삭제한다.
	Remove(ctx context.Context, name string) error

	// Close는 저장소 연결을 닫는다.
	Close() error
}
