// Package planmode
// File: readonly_filter_test.go
// Description: ReadOnlyFilter 단위 테스트
// Responsibility: read tool 통과 / write tool 차단 / Plan off 시 통과 검증

package planmode

import (
	"testing"

	"github.com/yourorg/infractl/internal/hooks"
)

func TestReadOnlyFilterByMetadata(t *testing.T) {
	f := NewReadOnlyFilter(nil)

	// read-only metadata → 통과
	ok, reason := f.Allow("shell_exec", hooks.HookMetadata{ReadOnly: true})
	if !ok {
		t.Fatalf("read-only metadata should allow: %s", reason)
	}

	// disk modifying metadata → 차단
	ok, reason = f.Allow("shell_exec", hooks.HookMetadata{DiskModifying: true})
	if ok {
		t.Fatal("disk modifying should be blocked")
	}
	if reason == "" {
		t.Fatal("reason should not be empty on block")
	}
}

func TestReadOnlyFilterFallbackBlock(t *testing.T) {
	f := NewReadOnlyFilter(nil)
	// metadata 힌트 없고 registry nil → FAIL-CLOSED
	ok, _ := f.Allow("unknown_tool", hooks.HookMetadata{})
	if ok {
		t.Fatal("unknown tool without registry should be blocked (fail-closed)")
	}
}
