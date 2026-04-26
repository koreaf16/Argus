package memdir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/constants"
)

type Store struct {
	baseDir string
}

type MemoryRecord struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func NewStore(baseDir string) *Store {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = constants.ConfigDir()
	}
	return &Store{baseDir: baseDir}
}

func (s *Store) BaseDir() string {
	return s.baseDir
}

func (s *Store) Bootstrap() error {
	for _, dir := range []string{
		s.sessionsDir(),
		s.memoryDir(),
		s.todosDir(),
		s.plansDir(),
		s.worktreesDir(),
		s.scheduledTasksDir(),
		s.tracesDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveSession(name string, payload any) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("session name is empty")
	}
	path := filepath.Join(s.sessionsDir(), name+".json")
	return writeJSONAtomic(path, payload)
}

func (s *Store) LoadSession(name string, out any) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("session name is empty")
	}
	path := filepath.Join(s.sessionsDir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (s *Store) ListSessions() ([]string, error) {
	entries, err := os.ReadDir(s.sessionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, filepath.Ext(name)))
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) AddMemory(text string, tags []string) (MemoryRecord, error) {
	if strings.TrimSpace(text) == "" {
		return MemoryRecord{}, fmt.Errorf("memory text is empty")
	}
	now := time.Now().UTC()
	rec := MemoryRecord{
		ID:        now.Format("20060102T150405.000000000"),
		Text:      text,
		Tags:      tags,
		CreatedAt: now,
	}
	path := filepath.Join(s.memoryDir(), rec.ID+".json")
	if err := writeJSONAtomic(path, rec); err != nil {
		return MemoryRecord{}, err
	}
	return rec, nil
}

func (s *Store) ListMemories() ([]MemoryRecord, error) {
	entries, err := os.ReadDir(s.memoryDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]MemoryRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(s.memoryDir(), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var rec MemoryRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) FindRelevantMemories(query string, limit int) ([]MemoryRecord, error) {
	all, err := s.ListMemories()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 5
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		if len(all) > limit {
			return all[:limit], nil
		}
		return all, nil
	}
	filtered := make([]MemoryRecord, 0, len(all))
	for _, rec := range all {
		if strings.Contains(strings.ToLower(rec.Text), q) {
			filtered = append(filtered, rec)
			continue
		}
		for _, tag := range rec.Tags {
			if strings.Contains(strings.ToLower(tag), q) {
				filtered = append(filtered, rec)
				break
			}
		}
	}
	if len(filtered) > limit {
		return filtered[:limit], nil
	}
	return filtered, nil
}

func (s *Store) sessionsDir() string {
	return filepath.Join(s.baseDir, "sessions")
}

func (s *Store) memoryDir() string {
	return filepath.Join(s.baseDir, "memory")
}

func (s *Store) todosDir() string {
	return filepath.Join(s.baseDir, "todos")
}

func (s *Store) worktreesDir() string {
	return filepath.Join(s.baseDir, "worktrees")
}

func (s *Store) scheduledTasksDir() string {
	return filepath.Join(s.baseDir, "scheduled-tasks")
}

func (s *Store) plansDir() string {
	return filepath.Join(s.baseDir, "plans")
}

func (s *Store) tracesDir() string {
	return filepath.Join(s.baseDir, "traces")
}

func (s *Store) TracePath(sessionID string) string {
	return filepath.Join(s.tracesDir(), strings.TrimSpace(sessionID)+".jsonl")
}

func (s *Store) SaveTodos(sessionID string, payload any) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session id is empty")
	}
	return writeJSONAtomic(filepath.Join(s.todosDir(), sessionID+".json"), payload)
}

func (s *Store) LoadTodos(sessionID string, out any) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session id is empty")
	}
	data, err := os.ReadFile(filepath.Join(s.todosDir(), sessionID+".json"))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (s *Store) SavePlan(sessionID, text string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session id is empty")
	}
	return writeFileAtomic(filepath.Join(s.plansDir(), sessionID+".md"), []byte(text))
}

func (s *Store) LoadPlan(sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("session id is empty")
	}
	path := filepath.Join(s.plansDir(), sessionID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeJSONAtomic(path string, payload any) error {
	return writeJSONAtomicWithRename(path, payload, os.Rename)
}

func writeJSONAtomicWithRename(path string, payload any, renameFn func(string, string) error) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomicWithRename(path, data, renameFn)
}

// writeFileAtomic writes data to path via a temp file + rename for crash safety.
func writeFileAtomic(path string, data []byte) error {
	return writeFileAtomicWithRename(path, data, os.Rename)
}

func writeFileAtomicWithRename(path string, data []byte, renameFn func(string, string) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := renameFn(tmp, path); err == nil {
		return nil
	}
	// Some Windows environments deny rename but allow direct writes.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	_ = os.Remove(tmp)
	return nil
}
