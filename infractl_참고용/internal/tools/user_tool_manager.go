// Package tools
// File: user_tool_manager.go
// Description: 사용자 정의 도구 생명주기 관리
// Responsibility: 스크립트 파일 저장, DB 영속화, 레지스트리 동적 등록/해제

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"

	"github.com/yourorg/infractl/internal/store"
)

// validToolName은 안전한 도구 이름 패턴이다 (소문자, 숫자, 언더스코어).
var validToolName = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)

// UserToolManager는 사용자 정의 도구의 생성, 로드, 삭제를 관리한다.
type UserToolManager struct {
	store      store.UserToolStore
	registry   *Registry
	scriptsDir string
}

// NewUserToolManager는 UserToolManager를 생성한다.
func NewUserToolManager(s store.UserToolStore, reg *Registry, scriptsDir string) *UserToolManager {
	return &UserToolManager{store: s, registry: reg, scriptsDir: scriptsDir}
}

// LoadAll은 DB에 저장된 모든 사용자 도구를 레지스트리에 등록한다.
func (m *UserToolManager) LoadAll(ctx context.Context) error {
	entries, err := m.store.ListUserTools(ctx)
	if err != nil {
		return fmt.Errorf("list user tools: %w", err)
	}
	for _, entry := range entries {
		tool := NewUserDefinedTool(entry)
		if err := m.registry.Register(tool); err != nil {
			slog.Warn("register user tool", "name", entry.Name, "err", err)
		} else {
			slog.Info("user tool loaded", "name", entry.Name)
		}
	}
	return nil
}

// CreateTool은 스크립트 파일을 저장하고 DB에 등록한 후 레지스트리에 동적으로 추가한다.
func (m *UserToolManager) CreateTool(ctx context.Context, entry store.UserToolEntry, scriptContent string) (int64, error) {
	if !validToolName.MatchString(entry.Name) {
		return 0, fmt.Errorf("invalid tool name %q: must match [a-z][a-z0-9_]{1,49}", entry.Name)
	}

	if err := os.MkdirAll(m.scriptsDir, 0o700); err != nil {
		return 0, fmt.Errorf("create scripts dir: %w", err)
	}

	scriptPath := filepath.Join(m.scriptsDir, entry.Name+".sh")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o700); err != nil {
		return 0, fmt.Errorf("write script file: %w", err)
	}
	entry.ScriptPath = scriptPath

	id, err := m.store.SaveUserTool(ctx, entry)
	if err != nil {
		_ = os.Remove(scriptPath)
		return 0, fmt.Errorf("save user tool: %w", err)
	}

	// 이미 등록된 동명 도구가 있으면 교체
	m.registry.Unregister(entry.Name)
	if regErr := m.registry.Register(NewUserDefinedTool(entry)); regErr != nil {
		return 0, fmt.Errorf("register tool: %w", regErr)
	}

	slog.Info("user tool created", "name", entry.Name, "path", scriptPath)
	return id, nil
}

// RemoveTool은 도구를 레지스트리, DB, 스크립트 파일에서 모두 삭제한다.
func (m *UserToolManager) RemoveTool(ctx context.Context, name string) error {
	entry, err := m.store.GetUserTool(ctx, name)
	if err != nil {
		return fmt.Errorf("get user tool: %w", err)
	}

	m.registry.Unregister(name)

	if err := m.store.DeleteUserTool(ctx, name); err != nil {
		return fmt.Errorf("delete user tool: %w", err)
	}

	if entry.ScriptPath != "" {
		if removeErr := os.Remove(entry.ScriptPath); removeErr != nil && !os.IsNotExist(removeErr) {
			slog.Warn("remove script file", "path", entry.ScriptPath, "err", removeErr)
		}
	}

	slog.Info("user tool removed", "name", name)
	return nil
}
