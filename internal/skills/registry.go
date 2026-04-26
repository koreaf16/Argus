package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/koreaf16/argus/internal/constants"
)

type Executor func(args []string) (string, error)

type Metadata struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Template    string `json:"template,omitempty"`
}

type skillEntry struct {
	meta Metadata
	exec Executor
}

type Registry struct {
	entries map[string]skillEntry
}

func NewRegistry() *Registry {
	r := &Registry{entries: make(map[string]skillEntry)}
	r.registerBundled()
	_ = r.LoadUserSkills(filepath.Join(constants.ConfigDir(), "skills"))
	return r
}

func (r *Registry) registerBundled() {
	r.Register(Metadata{Name: "remember", Description: "Store concise memory notes"}, func(args []string) (string, error) {
		return "remember: " + strings.Join(args, " "), nil
	})
	r.Register(Metadata{Name: "loop", Description: "Repeat a routine action"}, func(args []string) (string, error) {
		return "loop: " + strings.Join(args, " "), nil
	})
	r.Register(Metadata{Name: "batch", Description: "Run grouped operations"}, func(args []string) (string, error) {
		return "batch: " + strings.Join(args, " "), nil
	})
	r.Register(Metadata{Name: "update-config", Description: "Update config values"}, func(args []string) (string, error) {
		return "update-config: " + strings.Join(args, " "), nil
	})
	r.Register(Metadata{Name: "debug", Description: "Debug helper skill"}, func(args []string) (string, error) {
		return "debug: " + strings.Join(args, " "), nil
	})
}

func (r *Registry) Register(meta Metadata, exec Executor) {
	name := strings.TrimSpace(meta.Name)
	if name == "" || exec == nil {
		return
	}
	meta.Name = name
	r.entries[name] = skillEntry{meta: meta, exec: exec}
}

func (r *Registry) List() []string {
	out := make([]string, 0, len(r.entries))
	for name := range r.entries {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) Describe(name string) (Metadata, bool) {
	entry, ok := r.entries[name]
	if !ok {
		return Metadata{}, false
	}
	return entry.meta, true
}

func (r *Registry) Run(name string, args []string) (string, error) {
	entry, ok := r.entries[name]
	if !ok {
		return "", fmt.Errorf("unknown skill: %s", name)
	}
	return entry.exec(args)
}

func (r *Registry) LoadUserSkills(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		full := filepath.Join(dir, name)
		switch {
		case strings.HasSuffix(strings.ToLower(name), ".skill.json"):
			if err := r.loadJSONSkill(full); err != nil {
				return err
			}
		case strings.HasSuffix(strings.ToLower(name), ".md"):
			if err := r.loadMarkdownSkill(full); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Registry) loadJSONSkill(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("parse skill json %s: %w", path, err)
	}
	if strings.TrimSpace(meta.Name) == "" {
		return nil
	}
	template := meta.Template
	r.Register(meta, func(args []string) (string, error) {
		payload := strings.Join(args, " ")
		if template != "" {
			return strings.ReplaceAll(template, "{{args}}", payload), nil
		}
		return fmt.Sprintf("%s: %s", meta.Name, payload), nil
	})
	return nil
}

func (r *Registry) loadMarkdownSkill(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return nil
	}
	first := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(first, "#") {
		return nil
	}
	name := strings.TrimSpace(strings.TrimPrefix(first, "#"))
	name = strings.TrimPrefix(strings.ToLower(name), "skill:")
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	desc := ""
	if len(lines) > 1 {
		desc = strings.TrimSpace(lines[1])
	}
	template := strings.TrimSpace(strings.Join(lines[2:], "\n"))
	meta := Metadata{Name: name, Description: desc, Template: template}
	r.Register(meta, func(args []string) (string, error) {
		payload := strings.Join(args, " ")
		if template != "" {
			return strings.ReplaceAll(template, "{{args}}", payload), nil
		}
		return fmt.Sprintf("%s: %s", name, payload), nil
	})
	return nil
}
