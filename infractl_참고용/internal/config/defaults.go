// Package config
// File: defaults.go
// Description: 설정 기본값 적용 및 환경변수 오버라이드 로직
// Responsibility: applyDefaults와 applyEnvOverrides 함수 제공

package config

import (
	"os"
	"regexp"
)

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func boolPtr(v bool) *bool {
	b := v
	return &b
}

// expandEnvVars는 문자열에서 ${VAR} 패턴을 환경변수 값으로 치환한다.
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return match
	})
}

// applyEnvOverrides는 특정 환경변수로 설정을 오버라이드한다.
func applyEnvOverrides(cfg *Config) {
	// 레거시 단일 LLM 오버라이드 (하위호환)
	if v := os.Getenv("INFRACTL_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("INFRACTL_LLM_ENDPOINT"); v != "" {
		cfg.LLM.Endpoint = v
	}
	if v := os.Getenv("INFRACTL_LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}

	// reasoning 티어 오버라이드
	if v := os.Getenv("INFRACTL_REASONING_ENDPOINT"); v != "" {
		if cfg.Models.Reasoning == nil {
			cfg.Models.Reasoning = &LLMConfig{}
		}
		cfg.Models.Reasoning.Endpoint = v
	}
	if v := os.Getenv("INFRACTL_REASONING_MODEL"); v != "" {
		if cfg.Models.Reasoning == nil {
			cfg.Models.Reasoning = &LLMConfig{}
		}
		cfg.Models.Reasoning.Model = v
	}
	if v := os.Getenv("INFRACTL_REASONING_API_KEY"); v != "" {
		if cfg.Models.Reasoning == nil {
			cfg.Models.Reasoning = &LLMConfig{}
		}
		cfg.Models.Reasoning.APIKey = v
	}

	// fast 티어 오버라이드
	if v := os.Getenv("INFRACTL_FAST_ENDPOINT"); v != "" {
		if cfg.Models.Fast == nil {
			cfg.Models.Fast = &LLMConfig{}
		}
		cfg.Models.Fast.Endpoint = v
	}
	if v := os.Getenv("INFRACTL_FAST_MODEL"); v != "" {
		if cfg.Models.Fast == nil {
			cfg.Models.Fast = &LLMConfig{}
		}
		cfg.Models.Fast.Model = v
	}
	if v := os.Getenv("INFRACTL_FAST_API_KEY"); v != "" {
		if cfg.Models.Fast == nil {
			cfg.Models.Fast = &LLMConfig{}
		}
		cfg.Models.Fast.APIKey = v
	}

	if v := os.Getenv("INFRACTL_ENABLE_RAW_LOG"); v == "true" {
		cfg.EnableRawLog = true
	}
}

// defaultTemperature는 temperature 미설정 시 사용할 기본값이다.
var defaultTemperature = func() *float64 { v := 0.0; return &v }()

// applyDefaults는 누락된 설정에 기본값을 적용한다.
func applyDefaults(cfg *Config) {
	if cfg.LLM.Timeout == 0 {
		cfg.LLM.Timeout = 60
	}
	if cfg.LLM.Mode == "" {
		cfg.LLM.Mode = "full"
	}
	if cfg.LLM.Temperature == nil {
		cfg.LLM.Temperature = defaultTemperature
	}

	// models.general이 없으면 레거시 llm 필드를 general로 승격한다 (하위호환)
	if cfg.Models.General == nil && cfg.LLM.Endpoint != "" {
		general := cfg.LLM
		cfg.Models.General = &general
	}

	// general 티어 기본값
	if cfg.Models.General != nil {
		if cfg.Models.General.Timeout == 0 {
			cfg.Models.General.Timeout = 60
		}
		if cfg.Models.General.Mode == "" {
			cfg.Models.General.Mode = "full"
		}
		if cfg.Models.General.Temperature == nil {
			cfg.Models.General.Temperature = defaultTemperature
		}
	}

	// reasoning 티어 기본값
	if cfg.Models.Reasoning != nil {
		if cfg.Models.Reasoning.Timeout == 0 {
			cfg.Models.Reasoning.Timeout = 120
		}
		if cfg.Models.Reasoning.Temperature == nil {
			cfg.Models.Reasoning.Temperature = defaultTemperature
		}
	}

	// fast 티어 기본값
	if cfg.Models.Fast != nil {
		if cfg.Models.Fast.Timeout == 0 {
			cfg.Models.Fast.Timeout = 30
		}
		if cfg.Models.Fast.Temperature == nil {
			cfg.Models.Fast.Temperature = defaultTemperature
		}
	}

	// reranker 기본값
	if cfg.Reranker.TopK == 0 {
		cfg.Reranker.TopK = 5
	}

	if cfg.Embedding.Provider == "" {
		cfg.Embedding.Provider = "ollama"
	}
	if cfg.Embedding.Model == "" {
		cfg.Embedding.Model = "all-minilm"
	}
	if cfg.Embedding.Endpoint == "" {
		cfg.Embedding.Endpoint = cfg.LLM.Endpoint
	}

	if cfg.Search.TimeoutSeconds == 0 {
		cfg.Search.TimeoutSeconds = 10
	}
	if cfg.Search.CacheTTLSeconds == 0 {
		cfg.Search.CacheTTLSeconds = 120
	}
	if cfg.Search.Adapters.GitHub == nil {
		cfg.Search.Adapters.GitHub = boolPtr(true)
	}
	if cfg.Search.Adapters.HuggingFace == nil {
		cfg.Search.Adapters.HuggingFace = boolPtr(true)
	}
	if cfg.Search.Adapters.DockerHub == nil {
		cfg.Search.Adapters.DockerHub = boolPtr(true)
	}
	if cfg.Search.Adapters.PyPI == nil {
		cfg.Search.Adapters.PyPI = boolPtr(true)
	}
	if cfg.Search.Adapters.NPM == nil {
		cfg.Search.Adapters.NPM = boolPtr(true)
	}
	if cfg.Search.Adapters.OSV == nil {
		cfg.Search.Adapters.OSV = boolPtr(true)
	}

	// Phase F: 멀티태스크 기본값 적용
	applyMultitaskDefaults(cfg)
}
