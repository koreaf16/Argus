// Package types — 세션·에이전트 ID 브랜드 타입.
//
// 파일 역할: SessionId 와 AgentId 를 일반 string 과 구별하기 위한
//
//	타입 별칭과 생성·검증 헬퍼를 제공한다.
//	컴파일 타임에 두 ID 를 혼동하는 실수를 방지한다.
//
// 포함 모듈:
//   - SessionID: 세션 고유 식별자 타입.
//   - AgentID: 서브에이전트 고유 식별자 타입.
//   - AsSessionID() / AsAgentID(): 원시 문자열 → 브랜드 타입 변환.
//   - ToAgentID(): 패턴 검증 후 AgentID 반환.
//
// 호출/사용 방식:
//   - internal/query/engine.go, internal/repl/repl.go 등에서 ID 관리 시 참조.
//
// 연결:
//   - 원본: src/types/ids.ts
package types

import "regexp"

// SessionID 는 세션을 고유하게 식별하는 브랜드 타입이다.
type SessionID string

// AgentID 는 세션 내 서브에이전트를 고유하게 식별하는 브랜드 타입이다.
type AgentID string

// agentIDPattern 은 createAgentId() 가 생성하는 패턴: `a` + 선택적 `<label>-` + 16 hex chars.
var agentIDPattern = regexp.MustCompile(`^a(?:.+-)?[0-9a-f]{16}$`)

// AsSessionID 는 원시 문자열을 SessionID 로 변환한다. 검증 없이 사용 가능.
func AsSessionID(id string) SessionID {
	return SessionID(id)
}

// AsAgentID 는 원시 문자열을 AgentID 로 변환한다. 검증 없이 사용 가능.
func AsAgentID(id string) AgentID {
	return AgentID(id)
}

// ToAgentID 는 문자열이 AgentID 패턴과 일치하면 AgentID 를, 그렇지 않으면 빈 값을 반환한다.
func ToAgentID(s string) (AgentID, bool) {
	if agentIDPattern.MatchString(s) {
		return AgentID(s), true
	}
	return "", false
}
