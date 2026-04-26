// Package tools
// File: registry_test.go
// Description: Registry 도구 등록/조회 단위 테스트
// Responsibility: 대소문자 정규화, 중복 등록, 조회 동작 검증

package tools

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
)

type mockTool struct{ name string }

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return "" }
func (m *mockTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}
func (m *mockTool) IsReadOnly() bool { return true }
func (m *mockTool) IsEnabled() bool  { return true }
func (m *mockTool) Execute(_ context.Context, _ map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	return ToolOutcome{Content: "ok", Success: true}, nil
}

func TestRegistry_CaseInsensitive(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&mockTool{"oracle_AI26.query"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 소문자로 조회
	tool, ok := r.Get("oracle_ai26.query")
	if !ok {
		t.Fatal("Get(lowercase) should succeed")
	}
	if tool.Name() != "oracle_AI26.query" {
		t.Errorf("Name() = %q, want original %q", tool.Name(), "oracle_AI26.query")
	}

	// 대문자로 조회
	_, ok = r.Get("ORACLE_AI26.QUERY")
	if !ok {
		t.Fatal("Get(UPPER) should succeed")
	}

	// Has 대소문자 무관
	if !r.Has("oracle_AI26.query") {
		t.Fatal("Has(original) should be true")
	}
	if !r.Has("ORACLE_AI26.QUERY") {
		t.Fatal("Has(UPPER) should be true")
	}
}

func TestRegistry_DuplicateAcrossCase(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&mockTool{"oracle_AI26.query"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	// 대소문자만 다른 이름으로 재등록 시 duplicate 에러
	err := r.Register(&mockTool{"oracle_ai26.query"})
	if err == nil {
		t.Fatal("second Register with different case should return error")
	}
}

func TestRegistry_UnregisterCaseInsensitive(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&mockTool{"oracle_AI26.query"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	r.Unregister("ORACLE_AI26.QUERY")
	if r.Has("oracle_ai26.query") {
		t.Fatal("Has after Unregister should be false")
	}
}

func TestRegistry_ListPreservesOriginalName(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&mockTool{"Oracle_TEST.query"})

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}
	if list[0].Name() != "Oracle_TEST.query" {
		t.Errorf("List[0].Name() = %q, want original case", list[0].Name())
	}
}
