// Package todo
// File: todo_test.go
// Description: signals / prompt_injector / enforcer 단위 테스트
// Responsibility: 신호어 감지, 프롬프트 주입, enforcer allow/deny 검증

package todo

import "testing"

// --- signals ---

func TestDetectMultiStep(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{"MySQL 5.7 → 8.0 마이그레이션 진행해줘", true},
		{"nginx를 설치해줘", true},
		{"배포 스크립트 실행", true},
		{"deploy the app to production", true},
		{"install nodejs", true},
		{"파일 내용 보여줘", false},
		{"서버 목록 조회", false},
		{"ls -la", false},
	}
	for _, c := range cases {
		got := DetectMultiStep(c.input)
		if got != c.expected {
			t.Errorf("DetectMultiStep(%q) = %v, want %v", c.input, got, c.expected)
		}
	}
}

// --- prompt injector ---

func TestInjectIfNeeded(t *testing.T) {
	base := "system base"

	// 신호어 없으면 그대로
	out := InjectIfNeeded("파일 보여줘", base)
	if out != base {
		t.Errorf("expected base unchanged, got %q", out)
	}

	// 신호어 있으면 hint 주입
	out = InjectIfNeeded("nginx 설치해줘", base)
	if out == base {
		t.Error("expected hint injected")
	}
	if len(out) <= len(base) {
		t.Error("injected prompt should be longer")
	}
}

// --- enforcer ---

func TestEnforcerReadOnlyAlwaysAllowed(t *testing.T) {
	e := NewEnforcer(NewStore())
	ok, _ := e.Enforce("file_read", true)
	if !ok {
		t.Fatal("read-only tool should always be allowed")
	}
}

func TestEnforcerDenyWhenTodoEmpty(t *testing.T) {
	store := NewStore()
	e := NewEnforcer(store)

	ok, reason := e.Enforce("shell_exec", false)
	if ok {
		t.Fatal("mutation with empty todo should be denied")
	}
	if reason == "" {
		t.Fatal("reason should not be empty")
	}
}

func TestEnforcerAllowWhenTodoPresent(t *testing.T) {
	store := NewStore()
	_ = store.Add(TodoItem{ID: "1", Content: "1단계", Status: StatusPending})
	e := NewEnforcer(store)

	ok, _ := e.Enforce("shell_exec", false)
	if !ok {
		t.Fatal("mutation with non-empty todo should be allowed")
	}
}
