// Package channel implements the MobaXterm-style multi-channel SSH abstraction.
// A single master *ssh.Client per host is shared by virtual channels keyed by
// (alias, privilege, purpose). Privilege channels isolate `sudo -i`, `su -`,
// and `sudo -i -u <other>` shells so commands always execute in the lane that
// matches the requested privilege. Purpose-pinned channels (sftp, metrics,
// tunnel) reuse the default privilege regardless of caller stack state.
package channel

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/services/workspace/lane"
)

// PrivilegeKey identifies a privilege channel within a host. The empty value
// (PrivilegeDefault) maps to the SSH login user; non-empty values serialize
// the AccountStack rooted at the login user, joined with ">". For example
// "alice>root" represents `alice` who entered `sudo -i`, and "alice>postgres"
// represents `sudo -i -u postgres` from the same login.
type PrivilegeKey string

const PrivilegeDefault PrivilegeKey = ""

// PrivilegeKeyFromStack serializes an AccountStack into a stable PrivilegeKey.
// A stack containing only the login user (or empty) collapses to PrivilegeDefault.
// Whitespace and CR characters that PTY shells sometimes append to `id -un`
// output are stripped so two equivalent stacks always produce the same key.
func PrivilegeKeyFromStack(loginUser string, stack lane.AccountStack) PrivilegeKey {
	loginUser = sanitizeUser(loginUser)
	cleaned := make(lane.AccountStack, 0, len(stack))
	for _, u := range stack {
		s := sanitizeUser(u)
		if s == "" {
			continue
		}
		cleaned = append(cleaned, s)
	}
	if len(cleaned) == 0 {
		return PrivilegeDefault
	}
	if len(cleaned) == 1 && cleaned[0] == loginUser {
		return PrivilegeDefault
	}
	return PrivilegeKey(cleaned.String())
}

func sanitizeUser(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "\r\n\t ")
}

// StackForPrivilege is the inverse of PrivilegeKeyFromStack. Returns the
// AccountStack that the given key represents on top of loginUser. The default
// key returns an empty stack so the caller treats it as "login only".
func StackForPrivilege(loginUser string, key PrivilegeKey) lane.AccountStack {
	if key == PrivilegeDefault {
		return lane.AccountStack{}
	}
	parts := strings.Split(string(key), ">")
	out := make(lane.AccountStack, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// ChannelPurpose names the workload a channel serves. Interactive and Exec
// share a PTY shell and respect PrivilegeKey. SFTP, Metrics, and Tunnel run on
// independent SSH channel types and pin to PrivilegeDefault.
type ChannelPurpose string

const (
	PurposeInteractive ChannelPurpose = "interactive"
	PurposeExec        ChannelPurpose = "exec"
	PurposeSFTP        ChannelPurpose = "sftp"
	PurposeMetrics     ChannelPurpose = "metrics"
	PurposeTunnel      ChannelPurpose = "tunnel"
	PurposeInventory   ChannelPurpose = "inventory"
)

// ChannelKey is the cache key used by ChannelManager.Acquire.
type ChannelKey struct {
	Alias     string
	Privilege PrivilegeKey
	Purpose   ChannelPurpose
}

// String produces a stable, hashable representation: "alias|privilege|purpose".
func (k ChannelKey) String() string {
	return string(k.Alias) + "|" + string(k.Privilege) + "|" + string(k.Purpose)
}

// Hash returns a short, collision-resistant fingerprint of the key, useful for
// log lines and TUI snapshots that need a compact identifier.
func (k ChannelKey) Hash() string {
	h := sha1.Sum([]byte(k.String()))
	return hex.EncodeToString(h[:6])
}

// ChannelState describes the runtime status of a channel.
type ChannelState string

const (
	StateReady   ChannelState = "ready"
	StateBusy    ChannelState = "busy"
	StateError   ChannelState = "error"
	StateClosed  ChannelState = "closed"
	StateOpening ChannelState = "opening"
)

// ChannelSnapshot is a read-only view of a channel for status bars and logs.
type ChannelSnapshot struct {
	Key          ChannelKey
	LoginUser    string
	AccountStack lane.AccountStack
	CWD          string
	State        ChannelState
	LastUsed     time.Time
	LastError    string
}

// ExecRequest is what router.RouteFor produces and ExecCapable.Exec consumes.
type ExecRequest struct {
	Command      string
	WorkingDir   string
	SudoPassword string
	OnChunk      func(string)
	Timeout      time.Duration
	Stdin        io.Reader
}

// ExecOutcome merges the lane.ExecResult with channel telemetry.
type ExecOutcome struct {
	Stdout string
	Stderr string
	Code   int
	CWD    string
	User   string
}

// StreamHandle exposes streaming exec semantics, mirroring workspace.ExecHandle.
type StreamHandle struct {
	Stream <-chan string
	Result <-chan ExecOutcome
	Write  func(string) error
	Kill   func()
}

// AcquireOpts carries credentials and policy hints when opening a channel.
type AcquireOpts struct {
	Password     string // SSH login password (forwarded to ConnectionPool if needed)
	RootPassword string // sudo/su password to inject during EnterAccount
	Role         string
	Channel      string
}

// Channel is the polymorphic abstraction every per-host workload implements.
type Channel interface {
	Key() ChannelKey
	Snapshot() ChannelSnapshot
	Close() error
}

// ExecCapable is implemented by exec/interactive channels.
type ExecCapable interface {
	Channel
	Exec(ctx context.Context, req ExecRequest) (ExecOutcome, error)
	StartStreaming(ctx context.Context, req ExecRequest) (*StreamHandle, error)
	EnterAccount(ctx context.Context, user, method, password string) error
	ExitAccount(ctx context.Context) error
}

// MetricsCapable runs an on-demand /proc snapshot collection.
type MetricsCapable interface {
	Channel
	Collect(ctx context.Context) (RawMetrics, error)
}

// InventoryCapable runs an arbitrary batched bash script in a fresh SSH session.
// Unlike ExecCapable it does NOT go through the PTY lane, so concurrent user
// commands are not serialized behind the inventory scan.
type InventoryCapable interface {
	Channel
	Run(ctx context.Context, script string) (string, error)
}

// RawMetrics is the union of /proc fields the metrics channel collects in one
// batched script. It is intentionally raw text — Manager parses it into the
// public workspace.MetricsSnapshot.
type RawMetrics struct {
	LoadAvg   string
	MemInfo   string
	DiskInfo  string
	UptimeRaw string
	Processes string
	GPU       string
	Errors    map[string]string
}

// SFTPCapable wraps a multiplexed sftp.Client pool over the master connection.
type SFTPCapable interface {
	Channel
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, content []byte, overwrite bool) error
	OpenReader(path string) (io.ReadCloser, error)
	OpenWriter(path string, overwrite bool) (io.WriteCloser, error)
	ListDir(root string, recursive bool, depth int) ([]FileEntry, error)
	Stat(path string) (FileEntry, error)
	Remove(path string) error
	Mkdir(path string) error
}

// FileEntry mirrors workspace.FileEntry but lives here to keep the channel
// package importable without a cycle. Manager re-exports it for callers.
type FileEntry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	Mode    string
	ModTime time.Time
}

// TunnelCapable wraps the SSH port-forward registry under one channel.
type TunnelCapable interface {
	Channel
	Open(local, remote string) (TunnelInfo, error)
	CloseTunnel(id string) error
	List() []TunnelInfo
}

// TunnelInfo mirrors workspace.TunnelInfo for the same reason as FileEntry.
type TunnelInfo struct {
	ID         string
	Alias      string
	LocalAddr  string
	RemoteAddr string
	Active     bool
}
