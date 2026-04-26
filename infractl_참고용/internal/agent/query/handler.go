// Package query
// File: handler.go
// Description: QueryEventSink 인터페이스 — query.Engine 이벤트 소비자 계약
// Responsibility: 이벤트 스트림 소비자(UI, 로그, 테스트)가 구현할 단일 진입점 정의

package query

// QueryEventSink 는 query.Engine.Run 이 반환하는 이벤트 채널을 소비하는 소비자의 인터페이스다.
// 단일 HandleQueryEvent 메서드를 구현하면 UI, 테스트 spy, noop 등 다양한 소비자를 만들 수 있다.
// 구현은 각 이벤트 타입을 type switch 로 처리해야 한다.
//
// 스레드 안전성: HandleQueryEvent 는 단일 goroutine 에서 순서대로 호출된다.
// 구현이 내부적으로 goroutine 을 생성하는 경우 자체적으로 동기화를 보장해야 한다.
type QueryEventSink interface {
	HandleQueryEvent(ev QueryEvent)
}

// NoopEventSink 는 모든 이벤트를 무시하는 기본 구현이다.
// 테스트나 선택적 sink 가 없는 경우에 사용한다.
type NoopEventSink struct{}

func (NoopEventSink) HandleQueryEvent(_ QueryEvent) {}
