// Package cache
// File: prefix_marker_test.go
// Description: Split / HasBoundary 회귀 테스트
// Responsibility: boundary 없음 / 있음 / 빈 문자열 케이스 검증

package cache

import (
	"testing"

	infracontext "github.com/yourorg/infractl/internal/context"
)

func TestSplit_NoBoundary(t *testing.T) {
	prefix, suffix := Split("hello world")
	if prefix != "hello world" || suffix != "" {
		t.Errorf("unexpected: prefix=%q suffix=%q", prefix, suffix)
	}
}

func TestSplit_WithBoundary(t *testing.T) {
	assembled := "static\n" + infracontext.DynamicBoundary + "\ndynamic"
	prefix, suffix := Split(assembled)
	if prefix != "static\n" {
		t.Errorf("prefix=%q", prefix)
	}
	if suffix != "dynamic" {
		t.Errorf("suffix=%q", suffix)
	}
}

func TestSplit_EmptyString(t *testing.T) {
	prefix, suffix := Split("")
	if prefix != "" || suffix != "" {
		t.Errorf("expected empty, got prefix=%q suffix=%q", prefix, suffix)
	}
}

func TestHasBoundary_True(t *testing.T) {
	s := "abc\n" + infracontext.DynamicBoundary + "\ndef"
	if !HasBoundary(s) {
		t.Error("expected HasBoundary=true")
	}
}

func TestHasBoundary_False(t *testing.T) {
	if HasBoundary("no boundary") {
		t.Error("expected HasBoundary=false")
	}
}
