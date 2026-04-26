// Package hooks
// File: reloader.go
// Description: hooks.yaml 변경 시 정책 atomic 교체
// Responsibility: LoadConfig → 검증 → snapshotMu.Lock atomic swap

package hooks

import (
	"fmt"
	"log/slog"
)

// Reload 는 path 의 hooks.yaml 을 재파싱하여 현재 스냅샷을 atomic 으로 교체한다.
// 파싱/검증 실패 시 기존 정책을 유지하고 slog WARN 을 남긴다.
// 성공 시 nil 을 반환한다.
func Reload(path string) error {
	cfg, err := LoadConfig(path)
	if err != nil {
		slog.Warn("hooks.yaml reload failed — keeping existing policy",
			"path", path,
			"err", err,
		)
		return fmt.Errorf("reload hooks: %w", err)
	}

	if err := validateConfig(cfg); err != nil {
		slog.Warn("hooks.yaml reload validation failed — keeping existing policy",
			"path", path,
			"err", err,
		)
		return fmt.Errorf("reload hooks validation: %w", err)
	}

	snapshotMu.Lock()
	snapConfig = cfg
	snapshotMu.Unlock()

	slog.Info("hooks.yaml reloaded", "path", path, "events", len(cfg.Events))
	return nil
}

// validateConfig 는 Config 의 matcher 등 필드를 검증한다.
// 현재는 비어있지 않은지만 확인한다 (matcher 상세 검증은 runner 에서).
func validateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("hooks config is nil")
	}
	return nil
}
