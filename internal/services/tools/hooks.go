package tools

import (
	"context"
	"sync"

	"github.com/koreaf16/argus/internal/services/llm"
)

// PreToolHook은 도구 실행 전에 호출되는 함수 타입입니다.
type PreToolHook func(ctx context.Context, call llm.ToolUseStart) error

// PostToolHook은 도구 실행 후에 호출되는 함수 타입입니다.
type PostToolHook func(ctx context.Context, call llm.ToolUseStart, result string, isErr bool) error

// HookRegistry는 등록된 훅들을 관리하고 실행하는 레지스트리입니다.
type HookRegistry struct {
	mu        sync.RWMutex
	preHooks  []PreToolHook
	postHooks []PostToolHook
}

// NewHookRegistry는 새로운 HookRegistry 인스턴스를 생성합니다.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{}
}

// RegisterPreHook은 새로운 PreToolHook을 등록합니다.
func (r *HookRegistry) RegisterPreHook(h PreToolHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preHooks = append(r.preHooks, h)
}

// RegisterPostHook은 새로운 PostToolHook을 등록합니다.
func (r *HookRegistry) RegisterPostHook(h PostToolHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.postHooks = append(r.postHooks, h)
}

// RunPreHooks는 등록된 모든 PreToolHook을 실행합니다.
func (r *HookRegistry) RunPreHooks(ctx context.Context, call llm.ToolUseStart) error {
	r.mu.RLock()
	hooks := r.preHooks
	r.mu.RUnlock()

	for _, h := range hooks {
		if err := h(ctx, call); err != nil {
			return err
		}
	}
	return nil
}

// RunPostHooks는 등록된 모든 PostToolHook을 실행합니다.
func (r *HookRegistry) RunPostHooks(ctx context.Context, call llm.ToolUseStart, result string, isErr bool) error {
	r.mu.RLock()
	hooks := r.postHooks
	r.mu.RUnlock()

	for _, h := range hooks {
		if err := h(ctx, call, result, isErr); err != nil {
			return err
		}
	}
	return nil
}
