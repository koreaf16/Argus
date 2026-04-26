package executor

import (
	"context"
	"time"
)

// ExecResult captures the outcome of a command execution.
type ExecResult struct {
	Stdout        string
	Stderr        string
	ExitCode      int
	Duration      time.Duration
	ErrorCategory string
	ErrorHint     string
}

// Executor runs a command on a target.
type Executor interface {
	Execute(ctx context.Context, command string) (ExecResult, error)
	Target() string
	Host() string
}

// WorkspaceProvider exposes the default working directory for an executor.
type WorkspaceProvider interface {
	WorkspaceDir() string
}

// ExecSession represents an active command execution that supports stdin injection.
type ExecSession interface {
	InjectStdin(line string) error
	SendEOF() error
	// Wait blocks until the command finishes and returns the final result.
	Wait() (ExecResult, error)
}

// StdinInjector is an executor that supports injecting stdin into a running command.
type StdinInjector interface {
	InjectStdin(line string) error
}

// StdinEOFSender is an executor that supports sending EOF/EOT to a running command.
type StdinEOFSender interface {
	SendEOF() error
}

// StreamExecutor streams stdout line-by-line while the command is running.
type StreamExecutor interface {
	Executor
	// ExecuteStream runs the command, streaming stdout via onLine.
	// Returns an ExecSession whose Wait() blocks until the command finishes.
	ExecuteStream(ctx context.Context, command string, onLine func(string)) (ExecSession, error)
}

// InteractiveSpec describes an interactive command execution request.
type InteractiveSpec struct {
	Command    string
	RequirePTY bool
}

// InteractiveExecutor runs interactive commands while streaming raw terminal chunks.
type InteractiveExecutor interface {
	Executor
	// ExecuteInteractive runs the command with PTY streaming.
	// Returns an ExecSession whose Wait() blocks until the command finishes.
	ExecuteInteractive(ctx context.Context, spec InteractiveSpec, onChunk func(string)) (ExecSession, error)
}

// FileTransferExecutor uploads and downloads files using the underlying transport (e.g. SFTP over SSH).
// Avoids password prompts by reusing the already-authenticated connection.
type FileTransferExecutor interface {
	Executor
	Upload(ctx context.Context, localPath, remotePath string, onProgress func(transferred, total int64)) error
	Download(ctx context.Context, remotePath, localPath string, onProgress func(transferred, total int64)) error
}
