// Package llm — model and server catalog management.
//
// 파일 역할: Argus가 사용하는 LLM 모델/서버 카탈로그의 메모리 구조와
// CRUD, 활성 모델 선택, 런타임 클라이언트 생성을 담당한다.
// 포함 모듈:
//   - ModelEntry: 실행 가능한 단일 모델 엔트리.
//   - ServerEntry: 모델 discovery에 사용하는 서버 엔트리.
//   - Registry: preset/user 모델과 user 서버를 함께 관리하는 카탈로그.
//
// 호출/사용 방식:
//   - cmd/argus/main.go의 부팅 흐름이 NewRegistry/Load/Build를 호출한다.
//   - cmd/argus/init*.go와 internal/repl/repl.go가 CRUD/조회 메서드를 사용한다.
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): 없음.
//   - 이 파일을 import 하는 주요 패키지: cmd/argus, internal/repl.
package llm

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com/v1"
	defaultGeminiBaseURL    = "https://generativelanguage.googleapis.com/v1beta"
	defaultOpenAIBaseURL    = "https://api.openai.com/v1"
)

type ModelEntry struct {
	Alias       string   `json:"alias"`
	ModelID     string   `json:"model_id"`
	Provider    Provider `json:"provider"`
	BaseURL     string   `json:"base_url,omitempty"`
	APIKeyEnv   string   `json:"api_key_env,omitempty"`
	Display     string   `json:"display"`
	ContextWin  int      `json:"context_win,omitempty"`
	Caps        Caps     `json:"caps"`
	Preset      bool     `json:"preset,omitempty"`
	ServerAlias string   `json:"server_alias,omitempty"`
}

type ServerEntry struct {
	Alias      string   `json:"alias"`
	Provider   Provider `json:"provider"`
	Display    string   `json:"display"`
	Scheme     string   `json:"scheme,omitempty"`
	Host       string   `json:"host,omitempty"`
	Port       int      `json:"port,omitempty"`
	PathPrefix string   `json:"path_prefix,omitempty"`
	BaseURL    string   `json:"base_url,omitempty"`
	APIKeyEnv  string   `json:"api_key_env,omitempty"`
}

func (s ServerEntry) EffectiveBaseURL() string {
	if base := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/"); base != "" {
		return base
	}
	switch s.Provider {
	case ProviderAnthropic:
		return defaultAnthropicBaseURL
	case ProviderGemini:
		return defaultGeminiBaseURL
	case ProviderOpenAICompat:
		scheme := strings.TrimSpace(s.Scheme)
		if scheme == "" {
			scheme = "http"
		}
		host := strings.TrimSpace(s.Host)
		if host == "" {
			return defaultOpenAIBaseURL
		}
		pathPrefix := strings.TrimSpace(s.PathPrefix)
		if pathPrefix == "" {
			pathPrefix = "/v1"
		}
		if !strings.HasPrefix(pathPrefix, "/") {
			pathPrefix = "/" + pathPrefix
		}
		if s.Port > 0 {
			return fmt.Sprintf("%s://%s:%d%s", scheme, host, s.Port, pathPrefix)
		}
		return fmt.Sprintf("%s://%s%s", scheme, host, pathPrefix)
	default:
		return ""
	}
}

type Registry struct {
	mu         sync.RWMutex
	presets    map[string]ModelEntry
	users      map[string]ModelEntry
	servers    map[string]ServerEntry
	active     string
	httpClient *http.Client
}

func NewRegistry() *Registry {
	presets := make(map[string]ModelEntry)
	for _, entry := range DefaultPresetModels() {
		entry.Preset = true
		presets[entry.Alias] = entry
	}
	reg := &Registry{
		presets:    presets,
		users:      make(map[string]ModelEntry),
		servers:    make(map[string]ServerEntry),
		httpClient: &http.Client{},
	}
	reg.active = reg.firstAliasUnlocked()
	return reg
}

func DefaultPresetModels() []ModelEntry {
	return []ModelEntry{}
}

func DefaultCapsForProvider(provider Provider) Caps {
	switch provider {
	case ProviderAnthropic:
		return Caps{Tools: true, Thinking: true, Vision: true, WebSearch: true}
	case ProviderGemini:
		return Caps{Tools: true, Thinking: true, Vision: true, WebSearch: true}
	default:
		return Caps{Tools: true, Thinking: false, Vision: true, WebSearch: true}
	}
}

func DefaultServerBaseURL(provider Provider) string {
	switch provider {
	case ProviderAnthropic:
		return defaultAnthropicBaseURL
	case ProviderGemini:
		return defaultGeminiBaseURL
	case ProviderOpenAICompat:
		return defaultOpenAIBaseURL
	default:
		return ""
	}
}

func EntryFromServer(server ServerEntry, discovered DiscoveredModel, alias, display string, contextWin int) ModelEntry {
	if strings.TrimSpace(display) == "" {
		display = firstNonEmpty(discovered.Display, discovered.ModelID)
	}
	if contextWin <= 0 {
		contextWin = discovered.ContextWin
	}
	caps := discovered.Caps
	if caps == (Caps{}) {
		caps = DefaultCapsForProvider(server.Provider)
	}
	return ModelEntry{
		Alias:       strings.TrimSpace(alias),
		ModelID:     strings.TrimSpace(discovered.ModelID),
		Provider:    server.Provider,
		BaseURL:     server.EffectiveBaseURL(),
		APIKeyEnv:   strings.TrimSpace(server.APIKeyEnv),
		Display:     strings.TrimSpace(display),
		ContextWin:  contextWin,
		Caps:        caps,
		ServerAlias: server.Alias,
	}
}

func (r *Registry) SetHTTPClient(client *http.Client) {
	if client == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.httpClient = client
}
