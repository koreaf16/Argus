package shell

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type bashProvider struct {
	shellPath    string
	snapshotPath string
}

type SnapshotFunc func(shellPath string) (string, error)

func CreateBashShellProvider(ctx context.Context, shellPath string, snapshotFn SnapshotFunc) (ShellProvider, error) {
	var snapshot string
	if snapshotFn != nil {
		s, err := snapshotFn(shellPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create shell snapshot: %v\n", err)
		} else {
			snapshot = s
		}
	}

	return &bashProvider{shellPath: shellPath, snapshotPath: snapshot}, nil
}

func (b *bashProvider) ProviderType() ShellType { return ShellTypeBash }
func (b *bashProvider) ShellPath() string       { return b.shellPath }
func (b *bashProvider) IsDetached() bool        { return true }

func (b *bashProvider) BuildExecCommand(_ context.Context, command string, opts ExecCommandOpts) (ExecCommandResult, error) {
	var commandParts []string

	if b.snapshotPath != "" {
		snapshotPosix := toForwardSlash(b.snapshotPath)
		commandParts = append(commandParts, fmt.Sprintf("source %s 2>/dev/null || true", singleQuote(snapshotPosix)))
	}

	if extCmd := getDisableExtglobCommand(b.shellPath); extCmd != "" {
		commandParts = append(commandParts, extCmd)
	}

	commandParts = append(commandParts, fmt.Sprintf("eval %s", singleQuote(command)))
	commandString := strings.Join(commandParts, " && ")

	return ExecCommandResult{CommandString: commandString}, nil
}

func (b *bashProvider) GetSpawnArgs(commandString string) []string {
	if b.snapshotPath != "" {
		return []string{"-c", commandString}
	}
	return []string{"-c", "-l", commandString}
}

func (b *bashProvider) GetEnvironmentOverrides(_ context.Context, command string) (map[string]string, error) {
	return map[string]string{}, nil
}

func getDisableExtglobCommand(shellPath string) string {
	lower := strings.ToLower(shellPath)
	if strings.Contains(lower, "bash") {
		return "shopt -u extglob 2>/dev/null || true"
	}
	if strings.Contains(lower, "zsh") {
		return "setopt NO_EXTENDED_GLOB 2>/dev/null || true"
	}
	return ""
}

func singleQuote(s string) string {
	escaped := strings.ReplaceAll(s, "'", `'\''`)
	return "'" + escaped + "'"
}

func toForwardSlash(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}
