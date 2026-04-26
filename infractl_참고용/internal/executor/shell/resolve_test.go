// Package shell
// File: resolve_test.go
// Description: Resolve() OS/SHELL 기반 프로바이더 선택 테스트
// Responsibility: 등록된 프로바이더가 반환되는지 검증

package shell

import (
	"context"
	"testing"
)

type mockProvider struct{ name string }

func (m *mockProvider) Name() string                                        { return m.name }
func (m *mockProvider) Wrap(_ context.Context, cmd string) (string, error) { return cmd, nil }
func (m *mockProvider) Prepare(_ context.Context, cmd string) (PreparedCmd, error) {
	return PreparedCmd{Argv: []string{cmd}}, nil
}
func (m *mockProvider) Analyze(_ context.Context, cmd string) (Analysis, error) {
	return Analysis{RawCommand: cmd}, nil
}

func TestLookup_NotFound(t *testing.T) {
	_, err := Lookup("__nonexistent__")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRegisterAndLookup(t *testing.T) {
	p := &mockProvider{name: "testshell"}
	Register(p)
	got, err := Lookup("testshell")
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if got.Name() != "testshell" {
		t.Errorf("expected testshell, got %s", got.Name())
	}
}
