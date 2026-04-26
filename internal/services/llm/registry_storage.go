// Package llm — catalog persistence and display helpers.
//
// 파일 역할: Registry의 저장/로드, 표 출력, 상태 표시를 담당한다.
//
//	구버전 models.json 호환을 유지하면서 server/model 카탈로그를 직렬화한다.
//
// 포함 모듈:
//   - persistedCatalog: disk에 저장되는 models.json 구조.
//   - Save/Load: models.json round-trip 및 구버전 호환 처리.
//   - FormatTable/Show: REPL과 init UI에서 사용하는 표시용 문자열 생성.
//
// 호출/사용 방식:
//   - cmd/argus/main.go와 cmd/argus/init*.go가 Load/Save를 호출한다.
//   - internal/repl/repl.go가 FormatTable/Show를 호출한다.
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): 없음.
//   - 이 파일을 import 하는 주요 패키지: cmd/argus, internal/repl.
package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type persistedCatalog struct {
	Active     string        `json:"active,omitempty"`
	Servers    []ServerEntry `json:"servers,omitempty"`
	UserModels []ModelEntry  `json:"userModels"`
}

func (r *Registry) Save(path string) error {
	r.mu.RLock()
	active := r.active
	servers := make([]ServerEntry, 0, len(r.servers))
	for _, entry := range r.servers {
		servers = append(servers, entry)
	}
	users := make([]ModelEntry, 0, len(r.users))
	for _, entry := range r.users {
		entry.Preset = false
		users = append(users, entry)
	}
	r.mu.RUnlock()

	sort.Slice(servers, func(i, j int) bool { return servers[i].Alias < servers[j].Alias })
	sort.Slice(users, func(i, j int) bool { return users[i].Alias < users[j].Alias })

	catalog := persistedCatalog{
		Active:     active,
		Servers:    servers,
		UserModels: users,
	}
	body, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func (r *Registry) Load(path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var catalog persistedCatalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		return err
	}

	servers := make(map[string]ServerEntry, len(catalog.Servers))
	for _, raw := range catalog.Servers {
		entry, err := normalizeServerEntry(raw)
		if err != nil {
			return fmt.Errorf("invalid server %q: %w", raw.Alias, err)
		}
		servers[entry.Alias] = entry
	}

	users := make(map[string]ModelEntry, len(catalog.UserModels))
	for _, raw := range catalog.UserModels {
		entry, err := normalizeModelEntry(raw)
		if err != nil {
			return fmt.Errorf("invalid model %q: %w", raw.Alias, err)
		}
		users[entry.Alias] = entry
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.servers = servers
	r.users = users
	if catalog.Active != "" {
		if _, ok := r.lookupUnlocked(catalog.Active); ok {
			r.active = catalog.Active
		} else {
			r.active = r.firstAliasUnlocked()
		}
	} else {
		r.active = r.firstAliasUnlocked()
	}
	return nil
}

func (r *Registry) FormatTable() string {
	list := r.List()
	active := r.ActiveAlias()

	var builder strings.Builder
	fmt.Fprintf(&builder, "%-4s %-2s %-22s %-13s %-28s %-12s\n", "#", "*", "Alias", "Provider", "ModelID", "Status")
	for i, entry := range list {
		mark := " "
		if entry.Alias == active {
			mark = "*"
		}
		fmt.Fprintf(
			&builder,
			"%-4d %-2s %-22s %-13s %-28s %-12s\n",
			i+1,
			mark,
			entry.Alias,
			entry.Provider,
			entry.ModelID,
			r.EntryStatus(entry),
		)
	}
	return builder.String()
}

func (r *Registry) FormatServerTable() string {
	list := r.ListServers()
	var builder strings.Builder
	fmt.Fprintf(&builder, "%-4s %-18s %-13s %-34s %-12s\n", "#", "Alias", "Provider", "BaseURL", "Auth")
	for i, entry := range list {
		auth := entry.APIKeyEnv
		if auth == "" {
			auth = "optional"
		}
		fmt.Fprintf(
			&builder,
			"%-4d %-18s %-13s %-34s %-12s\n",
			i+1,
			entry.Alias,
			entry.Provider,
			entry.EffectiveBaseURL(),
			auth,
		)
	}
	return builder.String()
}

func (r *Registry) EntryStatus(entry ModelEntry) string {
	if entry.Provider == ProviderOpenAICompat && strings.TrimSpace(entry.APIKeyEnv) == "" {
		return "ready"
	}
	if strings.TrimSpace(entry.APIKeyEnv) == "" {
		return "invalid"
	}
	if strings.TrimSpace(os.Getenv(entry.APIKeyEnv)) == "" {
		return "missing-key"
	}
	return "ready"
}

func (r *Registry) Show(alias string) (string, error) {
	entry, ok := r.Get(alias)
	if !ok {
		return "", fmt.Errorf("unknown alias: %s", alias)
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "Alias      : %s\n", entry.Alias)
	fmt.Fprintf(&builder, "Display    : %s\n", entry.Display)
	fmt.Fprintf(&builder, "Provider   : %s\n", entry.Provider)
	fmt.Fprintf(&builder, "Model ID   : %s\n", entry.ModelID)
	fmt.Fprintf(&builder, "Base URL   : %s\n", entry.BaseURL)
	fmt.Fprintf(&builder, "API key env: %s\n", entry.APIKeyEnv)
	fmt.Fprintf(&builder, "Context    : %d\n", entry.ContextWin)
	if entry.ServerAlias != "" {
		fmt.Fprintf(&builder, "Server     : %s\n", entry.ServerAlias)
	}
	fmt.Fprintf(
		&builder,
		"Capabilities: tools=%t thinking=%t vision=%t web_search=%t\n",
		entry.Caps.Tools,
		entry.Caps.Thinking,
		entry.Caps.Vision,
		entry.Caps.WebSearch,
	)
	if entry.Preset {
		fmt.Fprintln(&builder, "Source     : preset")
	} else {
		fmt.Fprintln(&builder, "Source     : user")
	}
	return builder.String(), nil
}
