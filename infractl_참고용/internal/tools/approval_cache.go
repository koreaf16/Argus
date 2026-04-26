// Package tools
// File: approval_cache.go
// Description: 위험 작업에 대한 일시적 사용자 승인 상태 관리
// Responsibility: 특정 시간 동안 동일 타겟/작업에 대한 반복 승인 요청을 방지

package tools

import (
	"sync"
	"time"
)

// ApprovalCache는 타겟별 위험 작업 승인 상태를 저장한다.
type ApprovalCache struct {
	mu        sync.RWMutex
	approvals map[string]time.Time
}

func NewApprovalCache() *ApprovalCache {
	return &ApprovalCache{
		approvals: make(map[string]time.Time),
	}
}

// IsApproved는 특정 타겟의 작업이 아직 유효한 승인 상태인지 확인한다.
func (c *ApprovalCache) IsApproved(target, riskType string) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := target + ":" + riskType
	expiry, ok := c.approvals[key]
	if !ok {
		return false
	}
	return time.Now().Before(expiry)
}

// Grant는 특정 타겟의 작업에 대해 일정 시간 동안 승인을 부여한다.
func (c *ApprovalCache) Grant(target, riskType string, duration time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	key := target + ":" + riskType
	c.approvals[key] = time.Now().Add(duration)
}

// Revoke는 승인 상태를 즉시 취소한다.
func (c *ApprovalCache) Revoke(target, riskType string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	key := target + ":" + riskType
	delete(c.approvals, key)
}
