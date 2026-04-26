// Package hooks
// File: snapshot.go
// Description: hooks.yaml 설정 in-memory 스냅샷 관리
// Responsibility: 세션 시작 시 hooks.yaml 을 1회 로드해 메모리에 캐시 (Phase G 에서 fsnotify 핫리로드로 교체)

// Ported from: claude_cli/src/utils/hooks/hooksConfigSnapshot.ts:119-124
// 핵심 패턴: captureHooksConfigSnapshot (1회 로드) + getHooksConfigFromSnapshot (캐시 반환).

package hooks

import (
	"sync"
)

// snapshot 은 세션 기간 동안 단일 Config 를 공유한다.
var (
	snapshotMu sync.RWMutex
	snapConfig *Config
)

// CaptureSnapshot 은 path 에서 hooks.yaml 을 로드해 스냅샷에 저장한다.
// 이미 캡처된 경우 no-op (Phase G 핫리로드 전까지 세션 당 1회).
func CaptureSnapshot(path string) error {
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	if snapConfig != nil {
		return nil
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	snapConfig = cfg
	return nil
}

// GetSnapshot 은 현재 스냅샷을 반환한다.
// CaptureSnapshot 이 호출되지 않은 경우 빈 Config 를 반환한다.
func GetSnapshot() *Config {
	snapshotMu.RLock()
	defer snapshotMu.RUnlock()
	if snapConfig == nil {
		return &Config{Events: make(map[string][]MatcherDef)}
	}
	return snapConfig
}

// ResetSnapshot 은 스냅샷을 초기화한다 (테스트용).
func ResetSnapshot() {
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	snapConfig = nil
}
