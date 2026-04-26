// Package infrainit
// File: hooks_test.go
// Description: BootstrapHooks 단위 테스트
// Responsibility: 최초 실행 시 yaml 생성, 기존 파일 보존, builtins 재배포 검증

package infrainit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapHooks_FirstRun(t *testing.T) {
	dir := t.TempDir()
	if err := BootstrapHooks(dir); err != nil {
		t.Fatalf("BootstrapHooks: %v", err)
	}

	yamlPath := filepath.Join(dir, "hooks.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read hooks.yaml: %v", err)
	}
	if !strings.Contains(string(data), "PreToolUse:") {
		t.Error("default yaml should contain PreToolUse section")
	}

	shPath := filepath.Join(dir, "builtins", "system_risk.sh")
	if _, err := os.Stat(shPath); err != nil {
		t.Errorf("system_risk.sh not unboxed: %v", err)
	}

	mdPath := filepath.Join(dir, "builtins", "agent", "shell_validator.md")
	if _, err := os.Stat(mdPath); err != nil {
		t.Errorf("shell_validator.md not unboxed: %v", err)
	}
}

func TestBootstrapHooks_PreservesUserYAML(t *testing.T) {
	dir := t.TempDir()
	custom := []byte("# my custom hooks\nPreToolUse: []\n")
	if err := os.WriteFile(filepath.Join(dir, "hooks.yaml"), custom, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := BootstrapHooks(dir); err != nil {
		t.Fatalf("BootstrapHooks: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "hooks.yaml"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(custom) {
		t.Errorf("user hooks.yaml was overwritten!\nwant: %s\ngot: %s", custom, got)
	}
}

func TestBootstrapHooks_RefreshesBuiltins(t *testing.T) {
	dir := t.TempDir()
	if err := BootstrapHooks(dir); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// 오래된 버전으로 변조
	shPath := filepath.Join(dir, "builtins", "system_risk.sh")
	if err := os.WriteFile(shPath, []byte("#!/bin/sh\necho stale\n"), 0o755); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	if err := BootstrapHooks(dir); err != nil {
		t.Fatalf("second run: %v", err)
	}

	data, err := os.ReadFile(shPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), "stale") {
		t.Error("builtins should be refreshed on subsequent runs")
	}
	if !strings.Contains(string(data), "system_risk.sh") {
		t.Error("refreshed builtin should contain its header")
	}
}

func TestBootstrapHooks_EmptyDir(t *testing.T) {
	if err := BootstrapHooks(""); err == nil {
		t.Error("empty cfgDir should error")
	}
}
