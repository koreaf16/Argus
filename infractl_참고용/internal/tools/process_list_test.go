// Package tools
// File: process_list_test.go
// Description: process_list 도구 단위 테스트
// Responsibility: 필터 미일치/실제 오류/정상 결과 케이스 검증

package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
)

type processListTestExecutor struct {
	result executor.ExecResult
	err    error
}

func (e processListTestExecutor) Execute(_ context.Context, _ string) (executor.ExecResult, error) {
	return e.result, e.err
}

func (e processListTestExecutor) Target() string           { return "test-server" }
func (e processListTestExecutor) Host() string             { return "test-server" }
func (e processListTestExecutor) Platform() executor.Platform { return executor.PlatformLinux }
func (e processListTestExecutor) ShellName() string         { return "bash" }

// TestProcessListFilterNoMatchShowsFriendlyMessage verifies that a grep exit 1
// with no stderr (= no matching processes) produces a friendly "not found" message,
// not an error string.
func TestProcessListFilterNoMatchShowsFriendlyMessage(t *testing.T) {
	tool := &ProcessListTool{}
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"filter": "oracle",
	}, processListTestExecutor{
		result: executor.ExecResult{ExitCode: 1, Stdout: "", Stderr: ""},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !out.Success {
		t.Fatalf("expected success, got failure: %s", out.Content)
	}
	if strings.Contains(out.Content, "Error listing processes") {
		t.Fatalf("expected friendly no-match message, got error string: %s", out.Content)
	}
	if !strings.Contains(out.Content, "oracle") {
		t.Fatalf("expected filter name in output, got: %s", out.Content)
	}
}

// TestProcessListRealErrorReportsError verifies that a non-zero exit with stderr
// is reported as an error (actual failure, not a filter miss).
func TestProcessListRealErrorReportsError(t *testing.T) {
	tool := &ProcessListTool{}
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"filter": "oracle",
	}, processListTestExecutor{
		result: executor.ExecResult{ExitCode: 1, Stdout: "", Stderr: "ps: command not found"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.Content, "Error listing processes") {
		t.Fatalf("expected error message, got: %s", out.Content)
	}
	if !strings.Contains(out.Content, "ps: command not found") {
		t.Fatalf("expected stderr in output, got: %s", out.Content)
	}
}

// TestProcessListSuccessReturnsStdout verifies that exit 0 with stdout passes through unchanged.
func TestProcessListSuccessReturnsStdout(t *testing.T) {
	tool := &ProcessListTool{}
	stdout := "oracle   1234  0.0  1.2  /u01/app/oracle/product/19c/dbhome_1/bin/oracle"
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"filter": "oracle",
	}, processListTestExecutor{
		result: executor.ExecResult{ExitCode: 0, Stdout: stdout},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Content != stdout {
		t.Fatalf("expected stdout passthrough, got: %s", out.Content)
	}
}

// TestProcessListNoFilterEmptyOutputShowsPlaceholder verifies that an empty result
// without a filter returns a readable placeholder.
func TestProcessListNoFilterEmptyOutputShowsPlaceholder(t *testing.T) {
	tool := &ProcessListTool{}
	out, err := tool.Execute(context.Background(), map[string]interface{}{}, processListTestExecutor{
		result: executor.ExecResult{ExitCode: 0, Stdout: ""},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out.Content == "" {
		t.Fatalf("expected placeholder, got empty string")
	}
}
