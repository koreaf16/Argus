// Package lane defines the persistent PTY-based remote execution model that
// replaces per-command SSH channel allocation. A Lane wraps a single long-lived
// shell whose cwd, environment, and account stack persist across calls.
package lane

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"strings"
	"time"
)

// LaneID uniquely identifies a lane by host alias and the account stack
// currently active in that lane (e.g. "sandbox#sandbox>postgres").
type LaneID string

// AccountStack represents the chain of `su -` / `sudo -i` transitions a lane
// has entered. The bottom of the stack is the SSH login user; the top is the
// effective user for the next command.
type AccountStack []string

// Top returns the effective user (top of the stack), or "" if the stack is empty.
func (s AccountStack) Top() string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}

// Push returns a new stack with user appended.
func (s AccountStack) Push(user string) AccountStack {
	out := make(AccountStack, len(s)+1)
	copy(out, s)
	out[len(s)] = user
	return out
}

// Pop returns the stack with its top removed and the popped user. If empty,
// returns the original stack and "".
func (s AccountStack) Pop() (AccountStack, string) {
	if len(s) == 0 {
		return s, ""
	}
	popped := s[len(s)-1]
	out := make(AccountStack, len(s)-1)
	copy(out, s[:len(s)-1])
	return out, popped
}

// Clone returns a defensive copy.
func (s AccountStack) Clone() AccountStack {
	out := make(AccountStack, len(s))
	copy(out, s)
	return out
}

// String returns a human-readable representation: "user1>user2>user3".
func (s AccountStack) String() string {
	return strings.Join(s, ">")
}

// Target identifies the destination of a command: which host, optionally which
// account stack to enter, and the workflow role tag (used for telemetry only).
type Target struct {
	Alias        string
	AccountStack AccountStack
	Role         string
}

// LaneSnapshot is a read-only view of a lane's current state, suitable for
// surfacing to the LLM prompt and TUI status bar.
type LaneSnapshot struct {
	ID           LaneID
	Alias        string
	AccountStack AccountStack
	CWD          string
	PrivState    string
	LastUsed     time.Time
}

// ExecOptions controls how a single command is run within a lane.
type ExecOptions struct {
	WorkingDir string
	Timeout    time.Duration
	Stdin      io.Reader
	OnChunk    func(string)
	// SudoPassword, when non-empty, is injected if the lane detects a sudo /
	// su password prompt while the command is in flight. One injection per
	// Exec call; further prompts surface as command failure.
	SudoPassword string
}

// ExecResult is the outcome of a single command. Stdout/Stderr may be merged
// when the lane's PTY does not separate streams.
type ExecResult struct {
	Stdout string
	Stderr string
	Code   int
	CWD    string
	User   string
}

// AcquireOpts carries credentials and policy hints used when opening a lane.
type AcquireOpts struct {
	Password     string
	RootPassword string
	Role         string
	IdleTimeout  time.Duration
}

// ExecutionLane is a persistent PTY shell on a remote host. Implementations
// must serialize Exec/EnterAccount/ExitAccount/Close — the underlying PTY is
// a single resource.
type ExecutionLane interface {
	ID() LaneID
	Snapshot() LaneSnapshot
	Exec(ctx context.Context, cmd string, opts ExecOptions) (ExecResult, error)
	EnterAccount(ctx context.Context, user, method, password string) error
	ExitAccount(ctx context.Context) error
	Close() error
}

// LaneManager owns the lifecycle of all lanes in the process. Acquire returns
// an existing lane if one matches the target, or opens a fresh one otherwise.
type LaneManager interface {
	Acquire(ctx context.Context, t Target, opts AcquireOpts) (ExecutionLane, error)
	Release(id LaneID)
	Snapshots() []LaneSnapshot
	CloseAll() error
}

// LaneIDFor computes a stable, collision-resistant ID for an (alias, stack) pair.
func LaneIDFor(alias string, stack AccountStack) LaneID {
	if len(stack) == 0 {
		return LaneID(alias)
	}
	h := sha1.Sum([]byte(alias + "\x00" + stack.String()))
	return LaneID(alias + "#" + hex.EncodeToString(h[:6]))
}
