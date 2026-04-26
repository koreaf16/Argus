// Package utils — 세션 환경변수 저장소.
//
// 파일 역할: /env KEY=VALUE 슬래시 명령으로 설정한 세션 전용 환경변수를 보관한다.
//
//	전역 싱글턴 GlobalSessionEnv 와 패키지 수준 래퍼 함수를 제공한다.
//
// 포함 모듈:
//   - SessionEnvStore: 뮤텍스로 보호되는 key-value 환경변수 맵
//   - GlobalSessionEnv: 프로세스 생명주기와 동일한 전역 인스턴스
//   - SetSessionEnvVar / GetSessionEnvVars / DeleteSessionEnvVar: 편의 래퍼
//
// 호출/사용 방식:
//   - internal/repl 패키지의 /env 슬래시 핸들러가 Set/Delete 를 호출
//   - internal/tools/bash/bash_tool.go 가 GetSessionEnvVars 로 환경변수를 읽어 프로세스에 주입
//
// 연결:
//   - 원본: src/utils/sessionEnvironment.ts (일부), src/utils/ShellConfig 개념
package utils

import "sync"

// SessionEnvStore 는 세션 전용 환경변수를 스레드 안전하게 보관한다.
type SessionEnvStore struct {
	mu   sync.RWMutex
	vars map[string]string
}

// GlobalSessionEnv 는 프로세스 전체에서 공유하는 세션 환경변수 저장소다.
var GlobalSessionEnv = &SessionEnvStore{vars: make(map[string]string)}

// Set 은 key 에 value 를 저장한다.
func (s *SessionEnvStore) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vars[key] = value
}

// Get 은 현재 세션 환경변수 맵의 복사본을 반환한다.
func (s *SessionEnvStore) Get() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.vars))
	for k, v := range s.vars {
		out[k] = v
	}
	return out
}

// Delete 는 key 를 삭제한다.
func (s *SessionEnvStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.vars, key)
}

// SetSessionEnvVar 는 GlobalSessionEnv.Set 의 패키지 수준 래퍼다.
func SetSessionEnvVar(key, value string) {
	GlobalSessionEnv.Set(key, value)
}

// GetSessionEnvVars 는 GlobalSessionEnv.Get 의 패키지 수준 래퍼다.
func GetSessionEnvVars() map[string]string {
	return GlobalSessionEnv.Get()
}

// DeleteSessionEnvVar 는 GlobalSessionEnv.Delete 의 패키지 수준 래퍼다.
func DeleteSessionEnvVar(key string) {
	GlobalSessionEnv.Delete(key)
}
