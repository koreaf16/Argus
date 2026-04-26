// Package agent
// File: equivalence_test.go
// Description: Regression tests for hook-based classification equivalence cases.
// Responsibility: Verify system_risk script output and ComputeMetadata read-only classification.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/agent/query"
	"gopkg.in/yaml.v3"
)

type equivCaseFile struct {
	Cases []equivCase `yaml:"cases"`
}

type equivCase struct {
	ID             string `yaml:"id"`
	Tool           string `yaml:"tool"`
	Command        string `yaml:"command"`
	ExpectDeny     bool   `yaml:"expect_deny"`
	ExpectKeyword  string `yaml:"expect_keyword"`
	ExpectReadonly bool   `yaml:"expect_readonly"`
}

func scriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	name := "system_risk.sh"
	if runtime.GOOS == "windows" {
		name = "system_risk.ps1"
	}
	script := filepath.Join(root, "internal", "hooks", "builtins", name)
	if _, err := os.Stat(script); err != nil {
		t.Skipf("%s not found (%v) - skipping system-risk tests", name, err)
	}
	return script
}

func runScript(t *testing.T, script, tool, command string) (decision string, reason string) {
	t.Helper()

	input := map[string]any{
		"event": "PreToolUse",
		"tool":  tool,
	}
	if command != "" {
		input["input"] = map[string]any{"command": command}
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(input); err != nil {
		t.Fatalf("encode hook input: %v", err)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(context.Background(), "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	} else {
		cmd = exec.CommandContext(context.Background(), "sh", script)
	}
	cmd.Stdin = bytes.NewReader(buf.Bytes())

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("script %s failed: %v", script, err)
	}

	var result struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &result); err != nil {
		t.Fatalf("parse script output %q: %v", string(out), err)
	}
	return result.Decision, result.Reason
}

func loadCases(t *testing.T, name string) []equivCase {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	path := filepath.Join(root, "testdata", "equivalence", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cf equivCaseFile
	if err := yaml.Unmarshal(b, &cf); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return cf.Cases
}

func TestEquivalence_SystemRisk_Deny(t *testing.T) {
	script := scriptPath(t)
	cases := loadCases(t, "system_risk_deny.yaml")

	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("%s/%s", tc.ID, tc.Tool), func(t *testing.T) {
			decision, reason := runScript(t, script, tc.Tool, tc.Command)
			if tc.ExpectDeny && decision != "deny" {
				t.Errorf("[%s] want deny, got %q (cmd=%q)", tc.ID, decision, tc.Command)
			}
			if !tc.ExpectDeny && decision == "deny" {
				t.Errorf("[%s] want allow, got deny (reason=%q, cmd=%q)", tc.ID, reason, tc.Command)
			}
			if tc.ExpectKeyword != "" && !strings.Contains(reason, tc.ExpectKeyword) {
				t.Logf("[%s] WARN: reason %q does not contain expected keyword %q", tc.ID, reason, tc.ExpectKeyword)
			}
		})
	}
}

func TestEquivalence_SystemRisk_Allow(t *testing.T) {
	script := scriptPath(t)
	cases := loadCases(t, "system_risk_allow.yaml")

	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("%s/%s", tc.ID, tc.Tool), func(t *testing.T) {
			decision, reason := runScript(t, script, tc.Tool, tc.Command)
			if tc.ExpectDeny && decision != "deny" {
				t.Errorf("[%s] want deny, got %q (cmd=%q)", tc.ID, decision, tc.Command)
			}
			if !tc.ExpectDeny && decision == "deny" {
				t.Errorf("[%s] want allow, got deny (reason=%q, cmd=%q)", tc.ID, reason, tc.Command)
			}
		})
	}
}

func TestEquivalence_ShellReadonly(t *testing.T) {
	cases := loadCases(t, "shell_readonly.yaml")

	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("%s/%s", tc.ID, tc.Tool), func(t *testing.T) {
			args := map[string]any{}
			if tc.Command != "" {
				args["command"] = tc.Command
			}
			md := query.ComputeMetadata(tc.Tool, args)
			if tc.ExpectReadonly && !md.ReadOnly {
				t.Errorf("[%s] want read_only=true, got %+v (cmd=%q)", tc.ID, md, tc.Command)
			}
			if !tc.ExpectReadonly && md.ReadOnly {
				t.Errorf("[%s] want read_only=false, got %+v (cmd=%q)", tc.ID, md, tc.Command)
			}
		})
	}
}

func TestEquivalence_PassThrough(t *testing.T) {
	script := scriptPath(t)
	cases := loadCases(t, "pass_through.yaml")

	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("%s/%s", tc.ID, tc.Tool), func(t *testing.T) {
			decision, reason := runScript(t, script, tc.Tool, tc.Command)
			if tc.ExpectDeny && decision != "deny" {
				t.Errorf("[%s] want deny, got %q (tool=%s cmd=%q)", tc.ID, decision, tc.Tool, tc.Command)
			}
			if !tc.ExpectDeny && decision == "deny" {
				t.Errorf("[%s] want allow, got deny (reason=%q, tool=%s cmd=%q)", tc.ID, reason, tc.Tool, tc.Command)
			}
		})
	}
}
