// Package shell
// File: registry.go
// Description: ShellProvider 등록 및 조회 레지스트리
// Responsibility: 이름 → 프로바이더 매핑 관리

package shell

import (
	"fmt"
	"sync"
)

var (
	mu        sync.RWMutex
	providers = map[string]ShellProvider{}
)

// Register 는 이름으로 ShellProvider 를 등록한다.
// 중복 등록 시 덮어쓴다.
func Register(p ShellProvider) {
	mu.Lock()
	defer mu.Unlock()
	providers[p.Name()] = p
}

// Lookup 은 이름으로 등록된 ShellProvider 를 반환한다.
func Lookup(name string) (ShellProvider, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("shell provider not found: %q", name)
	}
	return p, nil
}
