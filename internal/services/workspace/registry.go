package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/koreaf16/argus/internal/constants"
)

const LocalAlias = "local"

type Registry struct {
	mu      sync.RWMutex
	path    string
	active  string
	servers map[string]ServerEntry
}

func NewRegistry(path string) *Registry {
	if strings.TrimSpace(path) == "" {
		path = filepath.Join(constants.ConfigDir(), "ssh_servers.json")
	}
	return &Registry{
		path:    path,
		active:  LocalAlias,
		servers: make(map[string]ServerEntry),
	}
}

func (r *Registry) Path() string {
	return r.path
}

func (r *Registry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			r.ensureLocalLocked()
			return nil
		}
		return err
	}

	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return fmt.Errorf("parse %s: %w", r.path, err)
	}

	r.servers = make(map[string]ServerEntry)
	for _, raw := range cat.Servers {
		entry, err := normalizeEntry(raw)
		if err != nil {
			return err
		}
		r.servers[entry.Alias] = entry
	}

	active := strings.TrimSpace(cat.Active)
	if active == "" {
		active = LocalAlias
	}
	if active != LocalAlias {
		if _, ok := r.servers[active]; !ok {
			active = LocalAlias
		}
	}
	r.active = active
	r.ensureLocalLocked()
	return nil
}

func (r *Registry) Save() error {
	r.mu.RLock()
	cat := Catalog{
		Active:  r.active,
		Servers: make([]ServerEntry, 0, len(r.servers)),
	}
	for _, entry := range r.servers {
		cat.Servers = append(cat.Servers, entry)
	}
	r.mu.RUnlock()

	sort.Slice(cat.Servers, func(i, j int) bool {
		return cat.Servers[i].Alias < cat.Servers[j].Alias
	})

	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, payload, 0o600)
}

func (r *Registry) Active() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if strings.TrimSpace(r.active) == "" {
		return LocalAlias
	}
	return r.active
}

func (r *Registry) SetActive(alias string) error {
	alias = normalizeAlias(alias)
	if alias == "" {
		alias = LocalAlias
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if alias != LocalAlias {
		if _, ok := r.servers[alias]; !ok {
			return fmt.Errorf("unknown server alias: %s", alias)
		}
	}
	r.active = alias
	return nil
}

func (r *Registry) Add(entry ServerEntry) error {
	normalized, err := normalizeEntry(entry)
	if err != nil {
		return err
	}
	if normalized.Alias == LocalAlias {
		return fmt.Errorf("alias %q is reserved", LocalAlias)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.servers[normalized.Alias]; exists {
		return fmt.Errorf("server alias already exists: %s", normalized.Alias)
	}
	r.servers[normalized.Alias] = normalized
	return nil
}

func (r *Registry) Remove(alias string) error {
	alias = normalizeAlias(alias)
	if alias == "" {
		return fmt.Errorf("alias is required")
	}
	if alias == LocalAlias {
		return fmt.Errorf("cannot remove reserved alias: %s", LocalAlias)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.servers[alias]; !ok {
		return fmt.Errorf("unknown server alias: %s", alias)
	}
	delete(r.servers, alias)
	if r.active == alias {
		r.active = LocalAlias
	}
	return nil
}

func (r *Registry) Get(alias string) (ServerEntry, bool) {
	alias = normalizeAlias(alias)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if alias == LocalAlias {
		return ServerEntry{Alias: LocalAlias, Kind: ServerKindLocal}, true
	}
	entry, ok := r.servers[alias]
	return entry, ok
}

func (r *Registry) List() []ServerEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ServerEntry, 0, len(r.servers)+1)
	out = append(out, ServerEntry{Alias: LocalAlias, Kind: ServerKindLocal})
	for _, entry := range r.servers {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

func (r *Registry) ensureLocalLocked() {
	if strings.TrimSpace(r.active) == "" {
		r.active = LocalAlias
	}
}

func normalizeEntry(entry ServerEntry) (ServerEntry, error) {
	entry.Alias = normalizeAlias(entry.Alias)
	if entry.Alias == "" {
		return ServerEntry{}, fmt.Errorf("server alias is required")
	}
	if entry.Kind == "" {
		entry.Kind = ServerKindSSH
	}
	switch entry.Kind {
	case ServerKindLocal:
		if entry.Alias != LocalAlias {
			return ServerEntry{}, fmt.Errorf("local kind is only allowed for alias %q", LocalAlias)
		}
		return ServerEntry{Alias: LocalAlias, Kind: ServerKindLocal}, nil
	case ServerKindSSH:
		if strings.TrimSpace(entry.Host) == "" {
			return ServerEntry{}, fmt.Errorf("host is required for %s", entry.Alias)
		}
		if entry.Port <= 0 {
			entry.Port = 22
		}
		if strings.TrimSpace(entry.User) == "" {
			return ServerEntry{}, fmt.Errorf("user is required for %s", entry.Alias)
		}
		return entry, nil
	default:
		return ServerEntry{}, fmt.Errorf("unsupported server kind: %s", entry.Kind)
	}
}

func normalizeAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}
