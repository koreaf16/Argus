package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/yourorg/infractl/internal/executor"
)

type accountPreflightTestExec struct {
	currentUser   string
	currentGroups string
	userGroups    map[string]string
	statOut       string
}

func (e accountPreflightTestExec) Execute(_ context.Context, command string) (executor.ExecResult, error) {
	switch {
	case command == "id -un 2>/dev/null || whoami", command == "whoami":
		return executor.ExecResult{ExitCode: 0, Stdout: e.currentUser + "\n"}, nil
	case command == "id -Gn":
		return executor.ExecResult{ExitCode: 0, Stdout: e.currentGroups + "\n"}, nil
	case strings.HasPrefix(command, "id -Gn "):
		user := strings.Trim(strings.TrimPrefix(command, "id -Gn "), "'")
		if groups, ok := e.userGroups[user]; ok {
			return executor.ExecResult{ExitCode: 0, Stdout: groups + "\n"}, nil
		}
		return executor.ExecResult{ExitCode: 1, Stderr: "no such user"}, nil
	case strings.Contains(command, "stat -c"):
		return executor.ExecResult{ExitCode: 0, Stdout: e.statOut}, nil
	default:
		return executor.ExecResult{ExitCode: 0}, nil
	}
}

func (e accountPreflightTestExec) Target() string { return "db01" }
func (e accountPreflightTestExec) Host() string   { return "db01" }
func (e accountPreflightTestExec) Platform() executor.Platform {
	return executor.PlatformLinux
}
func (e accountPreflightTestExec) ShellName() string { return "bash" }

func TestValidateMutationPathAccessBlocksOwnerMismatch(t *testing.T) {
	exec := accountPreflightTestExec{
		currentUser:   "sandbox",
		currentGroups: "sandbox",
		statOut:       "/home/oracle\toracle\toinstall\t750\n",
	}

	err := ValidateMutationPathAccess(context.Background(), exec, MutationPathPreflight{
		Operation: "archive extraction",
		Paths:     []string{"/home/oracle/dbhome"},
	})
	if err == nil {
		t.Fatal("expected owner mismatch to be blocked")
	}
	for _, want := range []string{"permission pre-check failed", "effective user \"sandbox\"", "become_user=\"oracle\""} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in error:\n%s", want, err)
		}
	}
}

func TestValidateMutationPathAccessAllowsGroupWritablePath(t *testing.T) {
	exec := accountPreflightTestExec{
		currentUser:   "appuser",
		currentGroups: "appuser oinstall",
		statOut:       "/u01/app/oracle\toracle\toinstall\t775\n",
	}

	err := ValidateMutationPathAccess(context.Background(), exec, MutationPathPreflight{
		Operation: "file copy",
		Paths:     []string{"/u01/app/oracle/product"},
	})
	if err != nil {
		t.Fatalf("expected group-writable path to pass, got %v", err)
	}
}

func TestValidateMutationPathAccessAllowsBecomeOwner(t *testing.T) {
	exec := accountPreflightTestExec{
		currentUser: "sandbox",
		userGroups:  map[string]string{"oracle": "oinstall dba"},
		statOut:     "/home/oracle\toracle\toinstall\t700\n",
	}

	err := ValidateMutationPathAccess(context.Background(), exec, MutationPathPreflight{
		Operation:    "file upload",
		Paths:        []string{"/home/oracle/db.zip"},
		BecomeMethod: "sudo",
		BecomeUser:   "oracle",
	})
	if err != nil {
		t.Fatalf("expected become owner to pass, got %v", err)
	}
}

func TestValidateMutationPathAccessAllowsRoot(t *testing.T) {
	exec := accountPreflightTestExec{
		currentUser: "sandbox",
		statOut:     "/root\troot\troot\t700\n",
	}

	err := ValidateMutationPathAccess(context.Background(), exec, MutationPathPreflight{
		Operation:    "file write",
		Paths:        []string{"/root/secret"},
		BecomeMethod: "sudo",
	})
	if err != nil {
		t.Fatalf("expected root become to pass, got %v", err)
	}
}
