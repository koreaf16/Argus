// Package config
// File: config.go
// Description: infractl 설정 구조체, YAML 로드/저장
// Responsibility: workspace state config file (.infractl/config.yaml) management

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yourorg/infractl/internal/serverenv"
	"gopkg.in/yaml.v3"
)

// CostPricing은 모델별 100만 토큰 단가(USD)이다.
type CostPricing struct {
	Input  float64 `yaml:"input"`
	Output float64 `yaml:"output"`
}

// ModelsConfig는 멀티 LLM 티어 설정을 나타낸다.
// Reasoning(고차원 추론), General(일반 작업), Fast(단순/저비용) 세 티어를 지원한다.
// General이 없으면 레거시 LLM 필드로 폴백한다.
type ModelsConfig struct {
	Reasoning *LLMConfig `yaml:"reasoning,omitempty"` // Tier 1: 고차원 추론 (선택)
	General   *LLMConfig `yaml:"general,omitempty"`   // Tier 2: 일반 작업 (필수)
	Fast      *LLMConfig `yaml:"fast,omitempty"`      // Tier 3: 단순/저비용 (선택)
}

// RerankerConfig는 리랭커 서버 설정을 나타낸다.
// Cohere 호환 /rerank API를 사용한다. Endpoint가 비어 있으면 리랭킹을 건너뛴다.
type RerankerConfig struct {
	Endpoint string `yaml:"endpoint"`          // e.g. "http://localhost:8787/v1"
	Model    string `yaml:"model"`             // e.g. "bge-reranker-v2-m3"
	APIKey   string `yaml:"api_key,omitempty"` // 인증이 필요한 경우
	TopK     int    `yaml:"top_k,omitempty"`   // 리랭킹 후 반환할 최대 결과 수 (기본값: 5)
}

// Config는 infractl 전역 설정을 나타낸다.
type Config struct {
	LLM             LLMConfig                  `yaml:"llm,omitempty"`    // 레거시 단일 LLM (하위호환)
	Models          ModelsConfig               `yaml:"models,omitempty"` // 멀티 LLM 티어
	Embedding       EmbeddingConfig            `yaml:"embedding,omitempty"`
	Reranker        RerankerConfig             `yaml:"reranker,omitempty"` // 선택적 리랭커
	MCPServers      map[string]MCPServerConfig `yaml:"mcp_servers,omitempty"`
	SubAgentModel   string                     `yaml:"sub_agent_model,omitempty"`
	SubAgentTimeout int                        `yaml:"sub_agent_timeout,omitempty"`
	CostPerMTokens  map[string]CostPricing     `yaml:"cost_per_1m_tokens,omitempty"`
	EnableRawLog    bool                       `yaml:"enable_raw_log,omitempty"` // true면 llmraw.log에 모든 통신 원본 기록

	// 여러 server가 등록된 경우 시작 시 자동 활성화할 server 이름.
	// server가 1개면 이 값과 무관하게 자동 활성화된다.
	DefaultServer string `yaml:"default_server,omitempty"`

	// Phase F: 멀티태스크 설정
	Background BackgroundConfig      `yaml:"background,omitempty"`
	Subagent   SubagentRuntimeConfig `yaml:"subagent,omitempty"`
	Monitor    MonitorConfig         `yaml:"monitor,omitempty"`
	Schedule   ScheduleRuntimeConfig `yaml:"schedule,omitempty"`
	Search     SearchConfig          `yaml:"search,omitempty"`
}

type SearchConfig struct {
	TimeoutSeconds  int `yaml:"timeout_seconds"`
	CacheTTLSeconds int `yaml:"cache_ttl_seconds"`
	Adapters        struct {
		GitHub      *bool `yaml:"github,omitempty"`
		HuggingFace *bool `yaml:"huggingface,omitempty"`
		DockerHub   *bool `yaml:"dockerhub,omitempty"`
		PyPI        *bool `yaml:"pypi,omitempty"`
		NPM         *bool `yaml:"npm,omitempty"`
		OSV         *bool `yaml:"osv,omitempty"`
	} `yaml:"adapters"`
	Tokens struct {
		GitHub      string `yaml:"github"`
		HuggingFace string `yaml:"huggingface"`
	} `yaml:"tokens"`
}

// GeneralLLM은 general 티어 LLM 설정을 반환한다.
// models.general이 설정되어 있으면 그것을, 없으면 레거시 llm 필드를 반환한다.
func (c *Config) GeneralLLM() LLMConfig {
	if c.Models.General != nil {
		return *c.Models.General
	}
	return c.LLM
}

// EmbeddingConfig는 임베딩 생성 설정을 나타낸다.
// 로컬 knowledge_base 벡터화 및 외부 RAG 소스 검색에 사용한다.
type EmbeddingConfig struct {
	Provider string `yaml:"provider"` // "ollama" | "openai" | "custom"
	Endpoint string `yaml:"endpoint"` // e.g. "http://localhost:11434"
	Model    string `yaml:"model"`    // e.g. "all-minilm"
	APIKey   string `yaml:"api_key"`  // OpenAI/custom 전용
}

// MCPServerConfig는 외부 MCP 서버 하나의 실행 설정이다.
// command와 args로 서브프로세스를 기동하고 stdio로 통신한다.
type MCPServerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env,omitempty"`
}

// LLMConfig는 LLM 연결 설정을 나타낸다.
type LLMConfig struct {
	Endpoint    string   `yaml:"endpoint"`
	Model       string   `yaml:"model"`
	APIKey      string   `yaml:"api_key"`
	Mode        string   `yaml:"mode"`                  // "full", "assisted", "basic"
	Timeout     int      `yaml:"timeout"`               // 초 단위
	Temperature *float64 `yaml:"temperature,omitempty"` // nil이면 서버 기본값(1.0) 사용
}

// DefaultConfigDir returns the state directory for the current workspace.
func DefaultConfigDir() (string, error) {
	return serverenv.StateDir()
}

func configFilePath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Exists는 config.yaml이 존재하는지 확인한다.
func Exists() bool {
	path, err := configFilePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Load는 config.yaml을 로드하고 환경변수를 적용한다.
func Load() (*Config, error) {
	path, err := configFilePath()
	if err != nil {
		return nil, fmt.Errorf("get config path: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// ${ENV_VAR} 치환 후 파싱
	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	applyEnvOverrides(&cfg)
	applyDefaults(&cfg)
	return &cfg, nil
}

// Save는 Config를 config.yaml에 저장한다.
func Save(cfg *Config) error {
	dir, err := DefaultConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}
