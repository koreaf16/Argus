package precheck

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
)

type dirWritableTestExecutor struct{}

func (dirWritableTestExecutor) Execute(_ context.Context, command string) (executor.ExecResult, error) {
	switch {
	case strings.Contains(command, "while [ ! -d"):
		return executor.ExecResult{Stdout: "/home/oracle\n", ExitCode: 0}, nil
	case strings.Contains(command, "test -w '/home/oracle'"):
		return executor.ExecResult{ExitCode: 1}, nil
	case strings.Contains(command, "stat -c"):
		return executor.ExecResult{Stdout: "oracle\n", ExitCode: 0}, nil
	default:
		return executor.ExecResult{ExitCode: 0}, nil
	}
}

func (dirWritableTestExecutor) Target() string { return "db-server" }
func (dirWritableTestExecutor) Host() string   { return "db-server" }

func TestDirWritableCheckerChecksNearestExistingAncestor(t *testing.T) {
	checker := NewDirWritableChecker("/home/oracle/database/19c", SeverityBlock)
	result := checker.Run(context.Background(), dirWritableTestExecutor{})
	if result.OK {
		t.Fatalf("expected writable check to fail")
	}
	if result.Severity != SeverityBlock {
		t.Fatalf("severity = %v, want block", result.Severity)
	}
	if !strings.Contains(result.Message, "checked ancestor \"/home/oracle\"") {
		t.Fatalf("expected ancestor in message, got %q", result.Message)
	}
}
