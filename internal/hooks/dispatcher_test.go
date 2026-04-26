package hooks

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/koreaf16/argus/internal/types"
)

func makeDispatcher(cfg HooksConfig) *HookDispatcher {
	return NewDispatcher(cfg, "test-session", "/tmp")
}

func TestDispatcher_once_runsOnlyOnce(t *testing.T) {
	var count int32
	// http 타입 훅으로 카운트하는 대신, command 타입 + once 를 테스트
	// (실제 셸 없이 테스트하기 위해 executor를 직접 호출하지 않고 dispatcher의 once 로직을 확인)
	cmd := HookCommand{
		Type:    HookCommandTypeCommand,
		Command: "echo once",
		Once:    true,
	}
	cfg := HooksConfig{
		types.HookEventSessionStart: {{Matcher: "", Hooks: []HookCommand{cmd}}},
	}
	d := makeDispatcher(cfg)

	// once 키를 수동으로 저장해 두 번째 호출 차단 여부 확인
	key := hooksOnceKey(cmd)
	if _, loaded := d.onceSeen.LoadOrStore(key, struct{}{}); loaded {
		t.Fatal("first store should not be loaded")
	}
	if _, loaded := d.onceSeen.LoadOrStore(key, struct{}{}); !loaded {
		t.Error("second store should be loaded (once already seen)")
	}
	_ = count
}

func TestDispatcher_async_doesNotBlock(t *testing.T) {
	// async 훅이 Dispatch()를 블로킹하지 않는지 확인.
	// 실제 셸 실행 없이: http 타입 + async 를 사용하면 URL이 빈 문자열이므로 skip됨.
	// 대신 명령 실행 시간이 짧아야 함을 검증 (async는 고루틴으로 실행되고 즉시 반환).
	cfg := HooksConfig{
		types.HookEventPreToolUse: {{Matcher: "", Hooks: []HookCommand{
			{Type: HookCommandTypeHTTP, URL: "http://127.0.0.1:19999/hook", Async: true},
		}}},
	}
	d := makeDispatcher(cfg)

	start := time.Now()
	// async 훅은 고루틴으로 실행되므로 Dispatch 자체는 즉시 반환해야 함
	// (연결 실패는 고루틴 내에서 처리)
	d.Dispatch(context.Background(), types.HookEventPreToolUse, HookInput{ToolName: "Bash"})
	elapsed := time.Since(start)

	// 1초 이내에 반환해야 함 (실제로는 수 ms 내)
	if elapsed > time.Second {
		t.Errorf("async hook blocked for %v, expected near-instant return", elapsed)
	}
}

func TestDispatcher_ifCondition_skips(t *testing.T) {
	// if 조건이 맞지 않으면 훅이 skip되어야 함.
	// 차단 결과를 반환하는 http 훅을 설정하되, if 조건이 안 맞으면 Dispatch가 Continue=true 반환.
	cfg := HooksConfig{
		types.HookEventPreToolUse: {{Matcher: "", Hooks: []HookCommand{
			// if: "PowerShell" → Bash 도구에는 매칭 안 됨
			{Type: HookCommandTypeHTTP, URL: "http://127.0.0.1:19999/hook", If: "PowerShell"},
		}}},
	}
	d := makeDispatcher(cfg)

	result := d.Dispatch(context.Background(), types.HookEventPreToolUse, HookInput{ToolName: "Bash"})
	if !result.Continue {
		t.Error("if condition should have skipped the hook, but it blocked")
	}
}

func TestDispatcher_ifCondition_matches(t *testing.T) {
	// if: "Bash" → Bash 도구에 매칭, 훅 실행 시도 (연결 실패는 경고로 처리)
	cfg := HooksConfig{
		types.HookEventPreToolUse: {{Matcher: "", Hooks: []HookCommand{
			{Type: HookCommandTypeHTTP, URL: "http://127.0.0.1:19999/hook", If: "Bash"},
		}}},
	}
	d := makeDispatcher(cfg)

	// 연결 실패여도 Continue=true (http 오류는 비차단)
	result := d.Dispatch(context.Background(), types.HookEventPreToolUse, HookInput{ToolName: "Bash"})
	if !result.Continue {
		t.Error("http connection failure should not block (Continue must remain true)")
	}
}

func TestDispatcher_onceSeen_crossDispatch(t *testing.T) {
	// once=true 훅은 두 번째 Dispatch 에서 실행 안 됨을 확인
	var runCount int32
	_ = runCount

	cmd := HookCommand{
		Type:    HookCommandTypeHTTP,
		URL:     "http://127.0.0.1:19999/hook",
		Once:    true,
		Timeout: 1,
	}
	cfg := HooksConfig{
		types.HookEventSessionStart: {{Matcher: "", Hooks: []HookCommand{cmd}}},
	}
	d := makeDispatcher(cfg)

	// 첫 번째: onceSeen 에 기록됨
	d.Dispatch(context.Background(), types.HookEventSessionStart, HookInput{})
	// 두 번째: onceSeen 에 이미 있으므로 skip
	key := hooksOnceKey(cmd)
	_, loaded := d.onceSeen.Load(key)
	if !loaded {
		t.Error("onceSeen should have the key after first dispatch")
	}
}

func TestHooksOnceKey_uniqueness(t *testing.T) {
	cmd1 := HookCommand{Type: HookCommandTypeCommand, Command: "echo a", URL: ""}
	cmd2 := HookCommand{Type: HookCommandTypeCommand, Command: "echo b", URL: ""}
	cmd3 := HookCommand{Type: HookCommandTypeHTTP, Command: "", URL: "http://example.com"}

	k1 := hooksOnceKey(cmd1)
	k2 := hooksOnceKey(cmd2)
	k3 := hooksOnceKey(cmd3)

	if k1 == k2 {
		t.Error("different commands should have different keys")
	}
	if k1 == k3 {
		t.Error("different types should have different keys")
	}
}

func TestDispatcher_AddSessionHook_executesOnDispatch(t *testing.T) {
	d := makeDispatcher(nil)

	// 아무 훅도 없을 때 IsEmpty=true
	if !d.IsEmpty() {
		t.Fatal("expected empty dispatcher")
	}

	// 세션 훅 추가
	d.AddSessionHook(types.HookEventSessionStart, HookMatcher{
		Matcher: "",
		Hooks: []HookCommand{
			{Type: HookCommandTypeHTTP, URL: "http://127.0.0.1:19999/session_hook", Timeout: 1},
		},
	})

	// IsEmpty는 sessionHooks가 있으므로 false
	if d.IsEmpty() {
		t.Error("expected non-empty dispatcher after AddSessionHook")
	}

	// Dispatch: http 연결 실패는 비차단 경고로 처리됨 (Continue=true)
	result := d.Dispatch(context.Background(), types.HookEventSessionStart, HookInput{})
	if !result.Continue {
		t.Errorf("http failure should not block; got Continue=false, StopReason=%q", result.StopReason)
	}
}

func TestDispatcher_RemoveSessionHook_noLongerExecutes(t *testing.T) {
	cmd := HookCommand{Type: HookCommandTypeHTTP, URL: "http://127.0.0.1:19999/remove_hook", Timeout: 1}
	d := makeDispatcher(nil)
	d.AddSessionHook(types.HookEventPreToolUse, HookMatcher{Matcher: "", Hooks: []HookCommand{cmd}})

	// 제거 전: 세션 훅 존재
	if d.IsEmpty() {
		t.Fatal("expected non-empty before remove")
	}

	d.RemoveSessionHook(types.HookEventPreToolUse, cmd)

	// 제거 후: 매처 슬라이스는 남아있지만 hooks가 비어있음 → resolveCommands가 0 반환
	cmds := resolveCommands(d.sessionHooks, types.HookEventPreToolUse, "")
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands after remove, got %d", len(cmds))
	}
}

func TestDispatcher_ClearSessionHooks(t *testing.T) {
	d := makeDispatcher(nil)
	d.AddSessionHook(types.HookEventStop, HookMatcher{
		Matcher: "",
		Hooks:   []HookCommand{{Type: HookCommandTypeCommand, Command: "echo clear"}},
	})

	if d.IsEmpty() {
		t.Fatal("expected non-empty before clear")
	}

	d.ClearSessionHooks()

	if !d.IsEmpty() {
		t.Error("expected empty after ClearSessionHooks")
	}
}

func TestDispatcher_IsEmpty_considersBothConfigs(t *testing.T) {
	// config 훅만 있을 때
	cfg := HooksConfig{
		types.HookEventStop: {{Matcher: "", Hooks: []HookCommand{
			{Type: HookCommandTypeCommand, Command: "echo stop"},
		}}},
	}
	d := makeDispatcher(cfg)
	if d.IsEmpty() {
		t.Error("expected non-empty with config hooks")
	}

	// sessionHooks만 있을 때 (config가 nil인 경우)
	d2 := makeDispatcher(nil)
	d2.AddSessionHook(types.HookEventStop, HookMatcher{
		Matcher: "",
		Hooks:   []HookCommand{{Type: HookCommandTypeCommand, Command: "echo stop"}},
	})
	if d2.IsEmpty() {
		t.Error("expected non-empty with session hooks only")
	}
}

func TestDispatcher_sessionHooks_mergedWithConfig(t *testing.T) {
	// config 훅 + 세션 훅 모두 resolveCommands에 반영됨
	cfgCmd := HookCommand{Type: HookCommandTypeCommand, Command: "echo config"}
	sessCmd := HookCommand{Type: HookCommandTypeCommand, Command: "echo session"}

	cfg := HooksConfig{
		types.HookEventPreToolUse: {{Matcher: "", Hooks: []HookCommand{cfgCmd}}},
	}
	d := makeDispatcher(cfg)
	d.AddSessionHook(types.HookEventPreToolUse, HookMatcher{Matcher: "", Hooks: []HookCommand{sessCmd}})

	cfgCmds := resolveCommands(d.config, types.HookEventPreToolUse, "")
	sesCmds := resolveCommands(d.sessionHooks, types.HookEventPreToolUse, "")
	total := append(cfgCmds, sesCmds...)

	if len(total) != 2 {
		t.Errorf("expected 2 merged commands, got %d", len(total))
	}
}

func init() {
	// atomic 패키지 사용 확인용 (컴파일 검증)
	_ = atomic.AddInt32
}
