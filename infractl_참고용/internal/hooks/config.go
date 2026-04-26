// Package hooks
// File: config.go
// Description: hooks.yaml 구조체 정의, 파일 로딩, 권한 체크
// Responsibility: ~/.infractl/hooks.yaml 로드 및 권한 검증 — group/world writable 거부

// Ported from: claude_cli/src/schemas/hooks.ts:32-222
// 핵심 패턴: HooksSettings = Partial<Record<HookEvent, HookMatcher[]>>를 Go struct로 매핑.

package hooks

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"gopkg.in/yaml.v3"
)

// BackendType 은 hook backend 종류이다.
type BackendType string

const (
	BackendCommand BackendType = "command"
	BackendPrompt  BackendType = "prompt"
	BackendHTTP    BackendType = "http"
	BackendAgent   BackendType = "agent"
	BackendBuiltin BackendType = "builtin"
)

// HookDef 는 hooks.yaml 의 단일 hook 정의이다.
type HookDef struct {
	Type           BackendType       `yaml:"type"`
	Command        string            `yaml:"command,omitempty"`        // command backend
	Prompt         string            `yaml:"prompt,omitempty"`         // prompt backend
	URL            string            `yaml:"url,omitempty"`            // http backend
	Method         string            `yaml:"method,omitempty"`         // http backend (기본 POST)
	Headers        map[string]string `yaml:"headers,omitempty"`        // http backend
	AllowedEnvVars []string          `yaml:"allowedEnvVars,omitempty"` // http backend
	Agent          string            `yaml:"agent,omitempty"`          // agent backend
	Builtin        string            `yaml:"builtin,omitempty"`        // builtin backend
	Tier           string            `yaml:"tier,omitempty"`           // prompt/agent backend: "fast"|"reasoning"
	Timeout        float64           `yaml:"timeout,omitempty"`        // 초 (0 = 전역 timeout 사용)
	StatusMessage  string            `yaml:"statusMessage,omitempty"`  // 스피너 메시지
	Once           bool              `yaml:"once,omitempty"`           // 1회 실행 후 삭제
	Async          bool              `yaml:"async,omitempty"`          // 백그라운드 실행
}

// MatcherDef 는 hooks.yaml 의 matcher 블록이다.
type MatcherDef struct {
	Matcher string    `yaml:"matcher"` // 예: "Bash(rm *)", "Write(*.yaml)"
	Hooks   []HookDef `yaml:"hooks"`
}

// Config 는 hooks.yaml 전체 구조이다.
// 키는 HookEvent 문자열 (PreToolUse, PostToolUse, ...).
type Config struct {
	Events map[string][]MatcherDef `yaml:",inline"`
}

// LoadConfig 는 파일에서 hooks.yaml 을 로드한다.
// 파일이 없으면 빈 Config 를 반환한다 (no-op — 회귀 0).
// 파일 권한이 group/world writable 이면 보안 위험으로 거부한다.
func LoadConfig(path string) (*Config, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		slog.Debug("hooks.yaml not found, using empty config", "path", path)
		return &Config{Events: make(map[string][]MatcherDef)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat hooks.yaml: %w", err)
	}

	// group/world writable 거부 — 임의 hook 실행 방지.
	// Windows 는 Unix 권한 모델이 없으므로 체크를 건너뛴다.
	if runtime.GOOS != "windows" {
		if mode := info.Mode(); mode&0o022 != 0 {
			slog.Warn("hooks.yaml is group/world writable — refusing to load (security risk)",
				"path", path, "mode", mode)
			return nil, fmt.Errorf("hooks.yaml %s is writable by group/others (mode %o): refusing to load", path, mode)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read hooks.yaml: %w", err)
	}

	var raw map[string][]MatcherDef
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse hooks.yaml: %w", err)
	}

	if raw == nil {
		raw = make(map[string][]MatcherDef)
	}
	cfg := &Config{Events: raw}
	slog.Info("hooks.yaml loaded", "path", path, "events", len(cfg.Events))
	return cfg, nil
}

// MatchersFor 는 주어진 이벤트의 매처 목록을 반환한다.
func (c *Config) MatchersFor(event HookEvent) []MatcherDef {
	return c.Events[string(event)]
}
