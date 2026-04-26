// Package executor
// File: manager.go
// Description: Executor 라우팅 매니저 — 로컬/원격 executor 풀 관리
// Responsibility: target 이름으로 올바른 Executor를 조회하고 생명주기를 관리

package executor

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Manager는 로컬 executor와 복수의 SSH executor를 보관하고 target 이름으로 라우팅한다.
// 모든 public 메서드는 goroutine-safe하다.
type Manager struct {
	local   Executor
	remotes map[string]Executor
	mu      sync.RWMutex
}

// NewManager는 localExec을 기본 로컬 executor로 사용하는 Manager를 생성한다.
func NewManager(localExec Executor) *Manager {
	return &Manager{
		local:   localExec,
		remotes: make(map[string]Executor),
	}
}

// Get은 target에 해당하는 Executor를 반환한다.
// target이 빈 문자열, "localhost", "local"이면 로컬 executor를 반환한다.
// target이 원격 서버 이름(Alias)이나 IP 주소와 일치하는지 확인한다.
// 등록되지 않은 target이면 사용 가능한 서버 목록을 포함한 error를 반환한다.
func (m *Manager) Get(target string) (Executor, error) {
	if IsLocalTarget(target) {
		return m.local, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. 이름(Alias)으로 검색
	if exec, ok := m.remotes[target]; ok {
		return exec, nil
	}

	// 2. IP 주소(Host)로 검색 (Fallback)
	for _, exec := range m.remotes {
		if exec.Host() == target {
			return exec, nil
		}
	}

	available := m.listNamesLocked()
	return nil, fmt.Errorf("workspace %q not found (available: %s)", target, strings.Join(available, ", "))
}

// Has reports whether a remote executor is currently registered.
func (m *Manager) Has(name string) bool {
	if IsLocalTarget(name) {
		return true
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.remotes[name]
	return ok
}

// Register는 name으로 원격 executor를 등록한다.
// 이미 동일한 이름이 있으면 기존 executor를 Close하고 교체한다.
func (m *Manager) Register(name string, exec Executor) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if old, ok := m.remotes[name]; ok {
		if closer, ok := old.(io.Closer); ok {
			closer.Close()
		}
	}
	m.remotes[name] = exec
}

// Remove는 이름으로 원격 executor를 제거하고 연결을 닫는다.
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	exec, ok := m.remotes[name]
	if !ok {
		return fmt.Errorf("workspace %q not registered", name)
	}

	if closer, ok := exec.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("close executor %s: %w", name, err)
		}
	}
	delete(m.remotes, name)
	return nil
}

// ServerInfo는 등록된 원격 서버의 요약 정보이다.
type ServerInfo struct {
	Name   string
	Target string // executor.Target() 반환값
}

// ListRemotes는 등록된 원격 서버 목록을 반환한다.
func (m *Manager) ListRemotes() []ServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]ServerInfo, 0, len(m.remotes))
	for name, exec := range m.remotes {
		list = append(list, ServerInfo{Name: name, Target: exec.Target()})
	}
	return list
}

// ListPersistentSessions returns all active persistent shell sessions grouped by server name.
// Only executors that implement PersistentSessionExecutor are queried.
func (m *Manager) ListPersistentSessions() map[string][]SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]SessionInfo)
	for name, exec := range m.remotes {
		if pse, ok := exec.(PersistentSessionExecutor); ok {
			if sessions, err := pse.SessionList(context.Background()); err == nil && len(sessions) > 0 {
				result[name] = sessions
			}
		}
	}
	return result
}

// Close는 모든 원격 executor를 종료한다.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []string
	for name, exec := range m.remotes {
		if closer, ok := exec.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %s", name, err))
			}
		}
	}
	m.remotes = make(map[string]Executor)

	if len(errs) > 0 {
		return fmt.Errorf("close executors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// listNamesLocked는 등록된 서버 이름 목록을 반환한다. 호출 전 RLock이 잡혀 있어야 한다.
func (m *Manager) listNamesLocked() []string {
	names := make([]string, 0, len(m.remotes)+1)
	names = append(names, "localhost")
	for name := range m.remotes {
		names = append(names, name)
	}
	return names
}
