// Package context
// File: assembler_test.go
// Description: Assemble / SplitAtBoundary 회귀 테스트
// Responsibility: boundary 유무·위치 정확성 검증

package context

import (
	"strings"
	"testing"
)

func TestAssemble_WithBoth(t *testing.T) {
	out := Assemble("static", "dynamic")
	if !strings.Contains(out, DynamicBoundary) {
		t.Errorf("boundary marker missing in output")
	}
	if !strings.HasPrefix(out, "static") {
		t.Errorf("static prefix not at start")
	}
}

func TestAssemble_EmptyStatic(t *testing.T) {
	out := Assemble("", "dynamic")
	if out != "dynamic" {
		t.Errorf("expected dynamic only, got %q", out)
	}
}

func TestAssemble_WhitespaceStatic(t *testing.T) {
	out := Assemble("   ", "dyn")
	if out != "dyn" {
		t.Errorf("expected dynamic only for whitespace static, got %q", out)
	}
}

func TestSplitAtBoundary_NoBoundary(t *testing.T) {
	prefix, suffix := SplitAtBoundary("no boundary here")
	if prefix != "no boundary here" || suffix != "" {
		t.Errorf("unexpected split: prefix=%q suffix=%q", prefix, suffix)
	}
}

func TestSplitAtBoundary_WithBoundary(t *testing.T) {
	assembled := Assemble("A", "B")
	prefix, suffix := SplitAtBoundary(assembled)
	if !strings.Contains(prefix, "A") {
		t.Errorf("prefix should contain A, got %q", prefix)
	}
	if suffix != "B" {
		t.Errorf("suffix should be B, got %q", suffix)
	}
}

func TestSplitAtBoundary_EmptyInput(t *testing.T) {
	prefix, suffix := SplitAtBoundary("")
	if prefix != "" || suffix != "" {
		t.Errorf("expected empty split, got prefix=%q suffix=%q", prefix, suffix)
	}
}
