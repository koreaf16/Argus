package hooks

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/koreaf16/argus/internal/types"
)

// HookDispatcher 는 이벤트를 수신해 매칭되는 훅을 실행하고 결과를 집계한다.
type HookDispatcher struct {
	mu           sync.RWMutex
	config       HooksConfig // settings.json 기반 훅
	sessionHooks HooksConfig // 런타임 등록 훅
	sessionID    string
	workDir      string
	executor     *Executor
	onceSeen     sync.Map // key: hooksOnceKey(cmd) → struct{}{}
}

// NewDispatcher 는 HookDispatcher 를 생성한다.
func NewDispatcher(cfg HooksConfig, sessionID, workDir string) *HookDispatcher {
	if cfg == nil {
		cfg = HooksConfig{}
	}
	return &HookDispatcher{
		config:       cfg,
		sessionHooks: HooksConfig{},
		sessionID:    sessionID,
		workDir:      workDir,
		executor:     &Executor{},
	}
}

// SetSubQueryFn 은 prompt 타입 훅에 사용할 LLM 쿼리 함수를 주입한다.
func (d *HookDispatcher) SetSubQueryFn(fn SubQueryFn) {
	d.mu.Lock()
	d.executor.subQueryFn = fn
	d.mu.Unlock()
}

// AddSessionHook 은 런타임에 훅 매처를 등록한다.
func (d *HookDispatcher) AddSessionHook(event types.HookEvent, matcher HookMatcher) {
	d.mu.Lock()
	d.sessionHooks[event] = append(d.sessionHooks[event], matcher)
	d.mu.Unlock()
}

// RemoveSessionHook 은 타입+명령+URL 기준으로 특정 훅 명령을 제거한다.
func (d *HookDispatcher) RemoveSessionHook(event types.HookEvent, cmd HookCommand) {
	d.mu.Lock()
	defer d.mu.Unlock()
	matchers := d.sessionHooks[event]
	for mi, m := range matchers {
		filtered := m.Hooks[:0]
		for _, h := range m.Hooks {
			if h.Type != cmd.Type || h.Command != cmd.Command || h.URL != cmd.URL {
				filtered = append(filtered, h)
			}
		}
		matchers[mi].Hooks = filtered
	}
	d.sessionHooks[event] = matchers
}

// ClearSessionHooks 는 런타임 등록 훅을 전부 제거한다.
func (d *HookDispatcher) ClearSessionHooks() {
	d.mu.Lock()
	d.sessionHooks = HooksConfig{}
	d.mu.Unlock()
}

// NotifyCwdChanged 는 작업 디렉터리 변경 시 CwdChanged 훅을 실행한다.
func (d *HookDispatcher) NotifyCwdChanged(ctx context.Context, newCwd string) {
	d.mu.Lock()
	d.workDir = newCwd
	d.mu.Unlock()
	go d.Dispatch(ctx, types.HookEventCwdChanged, HookInput{WorkingDir: newCwd})
}

// Dispatch 는 이벤트에 매칭되는 모든 훅을 순서대로 실행하고 집계 결과를 반환한다.
// 첫 번째 Continue=false 훅 발견 시 나머지 훅 실행을 중단한다.
func (d *HookDispatcher) Dispatch(
	ctx context.Context,
	event types.HookEvent,
	input HookInput,
) AggregatedResult {
	d.mu.RLock()
	cfg := d.config
	sessionHooks := d.sessionHooks
	sessionID := d.sessionID
	workDir := d.workDir
	d.mu.RUnlock()

	// workDir/sessionID를 input에 보완
	if input.WorkingDir == "" {
		input.WorkingDir = workDir
	}
	if input.SessionID == "" {
		input.SessionID = sessionID
	}

	// config 훅 먼저, 세션 훅 뒤에 추가
	cmds := resolveCommands(cfg, event, input.ToolName)
	cmds = append(cmds, resolveCommands(sessionHooks, event, input.ToolName)...)
	if len(cmds) == 0 {
		return AggregatedResult{Continue: true}
	}

	agg := AggregatedResult{Continue: true}

	for _, cmd := range cmds {
		// command 타입: 명령 문자열 필수 / http 타입: URL 필수 / prompt 타입: Prompt 필수
		switch cmd.Type {
		case HookCommandTypeCommand:
			if strings.TrimSpace(cmd.Command) == "" {
				continue
			}
		case HookCommandTypeHTTP:
			if strings.TrimSpace(cmd.URL) == "" {
				continue
			}
		case HookCommandTypePrompt:
			if strings.TrimSpace(cmd.Prompt) == "" {
				continue
			}
		default:
			continue
		}

		// if 조건 필터
		if !matchesIfCondition(cmd.If, input) {
			continue
		}

		// once: 세션당 1회만 실행
		if cmd.Once {
			key := hooksOnceKey(cmd)
			if _, loaded := d.onceSeen.LoadOrStore(key, struct{}{}); loaded {
				continue
			}
		}

		// async: 비동기 실행 (AggregatedResult 에 영향 없음)
		if cmd.Async {
			go d.executor.Run(ctx, cmd, input)
			continue
		}

		res := d.executor.Run(ctx, cmd, input)

		if res.ExitCode == 0 && strings.TrimSpace(res.Stdout) != "" {
			// exit 0: stdout을 LLM 컨텍스트로 누적
			if agg.Stdout != "" {
				agg.Stdout += "\n"
			}
			agg.Stdout += strings.TrimSpace(res.Stdout)
		}

		if res.ExitCode != 0 && res.ExitCode != 2 {
			// exit 0/2 외: 경고 누적
			if strings.TrimSpace(res.Stderr) != "" {
				agg.Warnings = append(agg.Warnings, strings.TrimSpace(res.Stderr))
			}
		}

		if res.UpdatedInput != nil {
			agg.UpdatedInput = res.UpdatedInput
		}

		if !res.Continue {
			agg.Continue = false
			agg.StopReason = res.StopReason
			if res.ExitCode == 2 {
				agg.BlockReason = strings.TrimSpace(res.Stderr)
				if agg.BlockReason == "" {
					agg.BlockReason = res.StopReason
				}
			} else {
				agg.BlockReason = res.Reason
				if agg.BlockReason == "" {
					agg.BlockReason = res.StopReason
				}
			}
			// 차단 발생 시 나머지 훅 실행 중단
			return agg
		}
	}

	return agg
}

// UpdateConfig 는 런타임 중 훅 설정을 교체한다.
func (d *HookDispatcher) UpdateConfig(cfg HooksConfig) {
	d.mu.Lock()
	d.config = cfg
	d.mu.Unlock()
}

// IsEmpty 는 이 dispatcher 에 등록된 훅이 없는지 반환한다.
func (d *HookDispatcher) IsEmpty() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.config) == 0 && len(d.sessionHooks) == 0
}

// hooksOnceKey 는 once 추적을 위한 훅 고유 키를 반환한다.
func hooksOnceKey(cmd HookCommand) string {
	return fmt.Sprintf("%s:%s:%s", cmd.Type, cmd.Command, cmd.URL)
}
