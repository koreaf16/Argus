// Package taskctx
// File: guardrail_test.go
// Description: Guardrails 컴파일 및 Evaluate 판정 단위 테스트
// Responsibility: Forbidden literal/regex, 서버/계정/디렉토리 불일치, root 예외 검증

package taskctx

import (
	"testing"
)

func makeTask(server, account, dir string, forbidden []string) *TaskContext {
	return &TaskContext{
		TargetServer:  server,
		TargetAccount: account,
		WorkingDir:    dir,
		Forbidden:     forbidden,
		Status:        TaskActive,
	}
}

func TestGuardrail_ForbiddenLiteralBlocked(t *testing.T) {
	gr, err := CompileForbidden([]string{"rm -rf /"})
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	task := makeTask("db01", "oracle", "/u01", []string{"rm -rf /"})
	v := gr.Evaluate(task, "oracle", "shell_exec", map[string]any{"command": "rm -rf /"}, "db01")

	if !v.Blocked {
		t.Error("want Blocked=true for forbidden literal")
	}
}

func TestGuardrail_ForbiddenLiteral_PrefixBlocked(t *testing.T) {
	gr, err := CompileForbidden([]string{"rm -rf"})
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	task := makeTask("db01", "oracle", "/u01", []string{"rm -rf"})
	// prefix 매칭: "rm -rf /data" → blocked
	v := gr.Evaluate(task, "oracle", "shell_exec", map[string]any{"command": "rm -rf /data"}, "db01")

	if !v.Blocked {
		t.Error("want Blocked=true for forbidden literal prefix")
	}
}

func TestGuardrail_ForbiddenRegexBlocked(t *testing.T) {
	gr, err := CompileForbidden([]string{"^/shutdown.*"})
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	task := makeTask("db01", "oracle", "/u01", []string{"^/shutdown.*"})
	v := gr.Evaluate(task, "oracle", "shell_exec", map[string]any{"command": "/shutdown now"}, "db01")

	if !v.Blocked {
		t.Error("want Blocked=true for forbidden regex")
	}
}

func TestGuardrail_ForbiddenRegex_NoMatch_Allowed(t *testing.T) {
	gr, err := CompileForbidden([]string{"^/shutdown.*"})
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	task := makeTask("db01", "oracle", "/u01", []string{"^/shutdown.*"})
	v := gr.Evaluate(task, "oracle", "shell_exec", map[string]any{"command": "ls /u01"}, "db01")

	if v.Blocked || v.Warned {
		t.Errorf("want clean verdict, got Blocked=%v Warned=%v Reason=%q", v.Blocked, v.Warned, v.Reason)
	}
}

func TestGuardrail_TargetServer_Mismatch_Warned(t *testing.T) {
	gr, err := CompileForbidden(nil)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	task := makeTask("db01", "oracle", "/u01", nil)
	v := gr.Evaluate(task, "oracle", "shell_exec", map[string]any{"command": "ls"}, "db02")

	if !v.Warned {
		t.Error("want Warned=true for target server mismatch")
	}
	if v.Blocked {
		t.Error("want Blocked=false for server mismatch (only warn)")
	}
}

func TestGuardrail_TargetAccount_Mismatch_Warned(t *testing.T) {
	gr, err := CompileForbidden(nil)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	task := makeTask("db01", "mysql", "/var/lib/mysql", nil)
	// elevationUser=oracle, target=mysql → 불일치 → warned
	v := gr.Evaluate(task, "oracle", "shell_exec", map[string]any{"command": "ls"}, "db01")

	if !v.Warned {
		t.Error("want Warned=true for account mismatch")
	}
}

func TestGuardrail_Root_AccountMismatch_NotWarned(t *testing.T) {
	gr, err := CompileForbidden(nil)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	task := makeTask("db01", "mysql", "/var/lib/mysql", nil)
	// elevationUser=root → root는 모든 계정 커버 → warned 아님
	v := gr.Evaluate(task, "root", "shell_exec", map[string]any{"command": "ls"}, "db01")

	if v.Warned || v.Blocked {
		t.Errorf("root should cover any account, but got Warned=%v Blocked=%v Reason=%q",
			v.Warned, v.Blocked, v.Reason)
	}
}

func TestGuardrail_NilTask_AllPass(t *testing.T) {
	gr, err := CompileForbidden([]string{"rm -rf /"})
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	v := gr.Evaluate(nil, "root", "shell_exec", map[string]any{"command": "rm -rf /"}, "any")

	if v.Blocked || v.Warned {
		t.Errorf("nil task should pass all, got Blocked=%v Warned=%v", v.Blocked, v.Warned)
	}
}

func TestGuardrail_InvalidRegex_CompileError(t *testing.T) {
	_, err := CompileForbidden([]string{"^/[invalid"})
	if err == nil {
		t.Error("expected compile error for invalid regex, got nil")
	}
}

func TestGuardrail_CmdKey_Alias(t *testing.T) {
	gr, err := CompileForbidden([]string{"dangerous"})
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	task := makeTask("db01", "oracle", "/u01", []string{"dangerous"})
	// "cmd" 키로 명령어 전달
	v := gr.Evaluate(task, "oracle", "shell_exec", map[string]any{"cmd": "dangerous command"}, "db01")

	if !v.Blocked {
		t.Error("want Blocked=true for cmd alias forbidden match")
	}
}
