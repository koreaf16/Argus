// Package llm — registry CRUD and runtime selection.
//
// 파일 역할: Registry의 모델/서버 CRUD, 활성 모델 전환,
//
//	런타임 LLM 인스턴스 빌드와 validation 로직을 담당한다.
//
// 포함 모듈:
//   - Add/AddServer/Remove/RemoveServer: catalog 변경 연산.
//   - ResolveSwitchToken/ResolveServerToken: index 또는 alias 해석.
//   - Build: 활성 ModelEntry를 실제 provider 구현으로 변환.
//
// 호출/사용 방식:
//   - cmd/argus/init*.go, internal/repl/repl.go, cmd/argus/main.go에서 호출한다.
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): 없음.
//   - 이 파일을 import 하는 주요 패키지: cmd/argus, internal/repl.
package llm

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

func (r *Registry) Add(entry ModelEntry) error {
	normalized, err := normalizeModelEntry(entry)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.presets[normalized.Alias]; ok {
		return fmt.Errorf("alias already exists in presets: %s", normalized.Alias)
	}
	if _, ok := r.users[normalized.Alias]; ok {
		return fmt.Errorf("alias already exists: %s", normalized.Alias)
	}
	r.users[normalized.Alias] = normalized
	if r.active == "" {
		r.active = normalized.Alias
	}
	return nil
}

func (r *Registry) AddServer(entry ServerEntry) error {
	normalized, err := normalizeServerEntry(entry)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.servers[normalized.Alias]; ok {
		return fmt.Errorf("server alias already exists: %s", normalized.Alias)
	}
	r.servers[normalized.Alias] = normalized
	return nil
}

func (r *Registry) Remove(alias string) error {
	alias = strings.TrimSpace(alias)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.presets[alias]; ok {
		return fmt.Errorf("cannot remove preset alias: %s", alias)
	}
	if _, ok := r.users[alias]; !ok {
		return fmt.Errorf("unknown alias: %s", alias)
	}
	delete(r.users, alias)
	if r.active == alias {
		r.active = r.firstAliasUnlocked()
	}
	return nil
}

func (r *Registry) RemoveServer(alias string) error {
	alias = strings.TrimSpace(alias)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.servers[alias]; !ok {
		return fmt.Errorf("unknown server alias: %s", alias)
	}
	delete(r.servers, alias)
	activeRemoved := false
	for modelAlias, entry := range r.users {
		if entry.ServerAlias == alias {
			if r.active == modelAlias {
				activeRemoved = true
			}
			delete(r.users, modelAlias)
		}
	}
	if activeRemoved {
		r.active = r.firstAliasUnlocked()
	}
	return nil
}

func (r *Registry) List() []ModelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelEntry, 0, len(r.presets)+len(r.users))
	for _, entry := range r.presets {
		out = append(out, entry)
	}
	for _, entry := range r.users {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

func (r *Registry) ListUserModels() []ModelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelEntry, 0, len(r.users))
	for _, entry := range r.users {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

func (r *Registry) ListServers() []ServerEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ServerEntry, 0, len(r.servers))
	for _, entry := range r.servers {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

func (r *Registry) ModelsForServer(alias string) []ModelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelEntry, 0)
	for _, entry := range r.users {
		if entry.ServerAlias == alias {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

func (r *Registry) ActiveAlias() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

func (r *Registry) ActiveEntry() (ModelEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookupUnlocked(r.active)
}

func (r *Registry) Get(alias string) (ModelEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookupUnlocked(alias)
}

func (r *Registry) GetServer(alias string) (ServerEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.servers[strings.TrimSpace(alias)]
	return entry, ok
}

func (r *Registry) SetActive(alias string) error {
	alias = strings.TrimSpace(alias)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.lookupUnlocked(alias); !ok {
		return fmt.Errorf("unknown alias: %s", alias)
	}
	r.active = alias
	return nil
}

func (r *Registry) ResolveSwitchToken(token string) (ModelEntry, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return ModelEntry{}, errors.New("missing model alias or index")
	}
	if idx, err := strconv.Atoi(token); err == nil {
		list := r.List()
		if idx < 1 || idx > len(list) {
			return ModelEntry{}, fmt.Errorf("index out of range: %d", idx)
		}
		return list[idx-1], nil
	}
	entry, ok := r.Get(token)
	if !ok {
		return ModelEntry{}, fmt.Errorf("unknown alias: %s", token)
	}
	return entry, nil
}

func (r *Registry) ResolveServerToken(token string) (ServerEntry, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return ServerEntry{}, errors.New("missing server alias or index")
	}
	if idx, err := strconv.Atoi(token); err == nil {
		list := r.ListServers()
		if idx < 1 || idx > len(list) {
			return ServerEntry{}, fmt.Errorf("index out of range: %d", idx)
		}
		return list[idx-1], nil
	}
	entry, ok := r.GetServer(token)
	if !ok {
		return ServerEntry{}, fmt.Errorf("unknown server alias: %s", token)
	}
	return entry, nil
}

func (r *Registry) Build() (LLM, error) {
	entry, ok := r.ActiveEntry()
	if !ok {
		return nil, errors.New("no active model")
	}
	key := strings.TrimSpace(os.Getenv(entry.APIKeyEnv))
	switch entry.Provider {
	case ProviderAnthropic, ProviderGemini:
		if strings.TrimSpace(entry.APIKeyEnv) == "" || key == "" {
			return nil, fmt.Errorf("missing api key env: %s", entry.APIKeyEnv)
		}
	case ProviderOpenAICompat:
		// OpenAI-compatible local servers may not require auth.
	default:
		return nil, fmt.Errorf("unsupported provider: %s", entry.Provider)
	}

	client := r.currentHTTPClient()
	switch entry.Provider {
	case ProviderAnthropic:
		return NewAnthropic(entry, key, client), nil
	case ProviderGemini:
		return NewGemini(entry, key, client), nil
	case ProviderOpenAICompat:
		return NewOpenAICompat(entry, key, client), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", entry.Provider)
	}
}

func (r *Registry) currentHTTPClient() *http.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.httpClient != nil {
		return r.httpClient
	}
	return &http.Client{}
}

func (r *Registry) firstAliasUnlocked() string {
	if _, ok := r.presets["claude-sonnet-4-6"]; ok {
		return "claude-sonnet-4-6"
	}
	aliases := make([]string, 0, len(r.presets)+len(r.users))
	for alias := range r.presets {
		aliases = append(aliases, alias)
	}
	for alias := range r.users {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	if len(aliases) == 0 {
		return ""
	}
	return aliases[0]
}

func (r *Registry) lookupUnlocked(alias string) (ModelEntry, bool) {
	if entry, ok := r.users[alias]; ok {
		return entry, true
	}
	entry, ok := r.presets[alias]
	return entry, ok
}

func normalizeModelEntry(entry ModelEntry) (ModelEntry, error) {
	entry.Alias = strings.TrimSpace(entry.Alias)
	entry.ModelID = strings.TrimSpace(entry.ModelID)
	entry.APIKeyEnv = strings.TrimSpace(entry.APIKeyEnv)
	entry.BaseURL = strings.TrimRight(strings.TrimSpace(entry.BaseURL), "/")
	entry.Display = strings.TrimSpace(entry.Display)
	entry.ServerAlias = strings.TrimSpace(entry.ServerAlias)
	if entry.Alias == "" || entry.ModelID == "" {
		return ModelEntry{}, errors.New("alias and model_id are required")
	}
	if err := validateProvider(entry.Provider); err != nil {
		return ModelEntry{}, err
	}
	if entry.Provider != ProviderOpenAICompat && entry.APIKeyEnv == "" {
		return ModelEntry{}, errors.New("api_key_env is required for this provider")
	}
	if entry.Display == "" {
		entry.Display = entry.ModelID
	}
	if entry.Caps == (Caps{}) {
		entry.Caps = DefaultCapsForProvider(entry.Provider)
	}
	entry.Preset = false
	return entry, nil
}

func normalizeServerEntry(entry ServerEntry) (ServerEntry, error) {
	entry.Alias = strings.TrimSpace(entry.Alias)
	entry.Display = strings.TrimSpace(entry.Display)
	entry.APIKeyEnv = strings.TrimSpace(entry.APIKeyEnv)
	entry.BaseURL = strings.TrimRight(strings.TrimSpace(entry.BaseURL), "/")
	entry.Scheme = strings.TrimSpace(entry.Scheme)
	entry.Host = strings.TrimSpace(entry.Host)
	entry.PathPrefix = strings.TrimSpace(entry.PathPrefix)
	if entry.Alias == "" {
		return ServerEntry{}, errors.New("server alias is required")
	}
	if err := validateProvider(entry.Provider); err != nil {
		return ServerEntry{}, err
	}
	if entry.Display == "" {
		entry.Display = entry.Alias
	}
	switch entry.Provider {
	case ProviderAnthropic, ProviderGemini:
		if entry.APIKeyEnv == "" {
			return ServerEntry{}, errors.New("api_key_env is required for this provider")
		}
		if entry.BaseURL == "" {
			entry.BaseURL = DefaultServerBaseURL(entry.Provider)
		}
	case ProviderOpenAICompat:
		if entry.BaseURL == "" && entry.Host == "" {
			return ServerEntry{}, errors.New("host or base_url is required for openai-compatible servers")
		}
		if entry.BaseURL == "" && entry.Port <= 0 {
			return ServerEntry{}, errors.New("port is required for openai-compatible servers")
		}
		if entry.BaseURL != "" {
			entry.Scheme = ""
			entry.Host = ""
			entry.Port = 0
			entry.PathPrefix = ""
		} else {
			if entry.Scheme == "" {
				entry.Scheme = "http"
			}
			if entry.PathPrefix == "" {
				entry.PathPrefix = "/v1"
			}
		}
	default:
		return ServerEntry{}, fmt.Errorf("unsupported provider: %s", entry.Provider)
	}
	return entry, nil
}

func validateProvider(provider Provider) error {
	switch provider {
	case ProviderAnthropic, ProviderGemini, ProviderOpenAICompat:
		return nil
	default:
		return fmt.Errorf("unsupported provider: %s", provider)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
