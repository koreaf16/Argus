// Package config
// File: multitask.go
// Description: Phase F 멀티태스크 설정 타입 및 기본값
// Responsibility: Background / Subagent / Schedule / Monitor 설정 구조체 제공

package config

import (
	"log/slog"
	"path/filepath"
	"time"
)

// RetentionPolicy는 결과 보관 정책이다.
// KeepDays와 MaxResults를 동시에 적용 — 일수 초과 또는 개수 초과 시 정리.
type RetentionPolicy struct {
	KeepDays   int `yaml:"keep_days,omitempty"`
	MaxResults int `yaml:"max_results,omitempty"`
}

// BackgroundConfig는 백그라운드 작업 실행과 저장 정책이다.
// PromotionThreshold는 "30s", "1m" 등 time.Duration 문자열 포맷을 사용한다.
type BackgroundConfig struct {
	PromotionThreshold string          `yaml:"promotion_threshold,omitempty"`
	StorageDir         string          `yaml:"storage_dir,omitempty"`
	MaxFileSize        int64           `yaml:"max_file_size,omitempty"`
	Retention          RetentionPolicy `yaml:"retention,omitempty"`
}

// PromotionDuration은 PromotionThreshold를 time.Duration으로 파싱한다.
// 파싱 실패 시 기본값 30s를 반환한다 (로그 경고).
func (b BackgroundConfig) PromotionDuration() time.Duration {
	if b.PromotionThreshold == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(b.PromotionThreshold)
	if err != nil {
		slog.Warn("invalid background.promotion_threshold, falling back to 30s",
			"value", b.PromotionThreshold, "err", err)
		return 30 * time.Second
	}
	return d
}

// MonitorConfig는 Monitor LLM 도구 출력 제한이다.
type MonitorConfig struct {
	MaxPerCall    int `yaml:"max_per_call,omitempty"`
	MaxCumulative int `yaml:"max_cumulative,omitempty"`
}

// SubagentRuntimeConfig는 서브에이전트 병렬 실행 정책이다.
type SubagentRuntimeConfig struct {
	MaxParallel int `yaml:"max_parallel,omitempty"`
}

// ScheduleLogConfig는 스케줄 실행 로그 파일 정책이다.
type ScheduleLogConfig struct {
	Path    string `yaml:"path,omitempty"`
	MaxSize int64  `yaml:"max_size,omitempty"`
}

// ScheduleRuntimeConfig는 스케줄 retention 및 로그 정책이다.
type ScheduleRuntimeConfig struct {
	Retention RetentionPolicy   `yaml:"retention,omitempty"`
	Log       ScheduleLogConfig `yaml:"log,omitempty"`
}

// applyMultitaskDefaults는 멀티태스크 관련 누락 설정에 기본값을 적용한다.
// Phase F 사용자 선택: 30s threshold / 7일 / 100개 / 10 parallel / 8KB-64KB monitor.
func applyMultitaskDefaults(cfg *Config) {
	stateDir, _ := DefaultConfigDir()

	// background
	if cfg.Background.PromotionThreshold == "" {
		cfg.Background.PromotionThreshold = "30s"
	}
	if cfg.Background.StorageDir == "" && stateDir != "" {
		cfg.Background.StorageDir = filepath.Join(stateDir, "bg")
	}
	if cfg.Background.MaxFileSize == 0 {
		cfg.Background.MaxFileSize = 10 * 1024 * 1024 // 10MB
	}
	applyRetentionDefaults(&cfg.Background.Retention)

	// subagent
	if cfg.Subagent.MaxParallel == 0 {
		cfg.Subagent.MaxParallel = 10
	}

	// monitor
	if cfg.Monitor.MaxPerCall == 0 {
		cfg.Monitor.MaxPerCall = 8 * 1024 // 8KB
	}
	if cfg.Monitor.MaxCumulative == 0 {
		cfg.Monitor.MaxCumulative = 64 * 1024 // 64KB
	}

	// schedule
	applyRetentionDefaults(&cfg.Schedule.Retention)
	if cfg.Schedule.Log.Path == "" && stateDir != "" {
		cfg.Schedule.Log.Path = filepath.Join(stateDir, "schedule.log")
	}
	if cfg.Schedule.Log.MaxSize == 0 {
		cfg.Schedule.Log.MaxSize = 100 * 1024 * 1024 // 100MB
	}
}

func applyRetentionDefaults(r *RetentionPolicy) {
	if r.KeepDays == 0 {
		r.KeepDays = 7
	}
	if r.MaxResults == 0 {
		r.MaxResults = 100
	}
}
