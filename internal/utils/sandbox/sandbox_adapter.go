// Package sandbox — Sandbox execution adapter and management.
//
// 파일 역할: Provides an interface for sandboxed command execution.
//
//	For Phase 1, this is a stub that returns errors or passes through to normal execution.
//
// 포함 모듈:
//   - SandboxManager: Interface for sandbox operations
//   - CreateSandbox: Creates a new sandbox environment (stub)
//   - RunCommandInSandbox: Executes a command in a sandbox (stub)
//   - IsAvailable: Checks if sandboxing is available
//
// 호출/사용 방식:
//   - tools/bash calls SandboxManager to conditionally sandbox commands
//   - internal/utils calls this for protected execution
//
// 연결:
//   - internal/utils: shell execution
//   - 원본: src/utils/sandbox/ (Phase 2 full implementation)
package sandbox

// SandboxManager is an interface for managing sandboxed execution.
type SandboxManager interface {
	// CreateSandbox creates a new sandbox environment.
	CreateSandbox(name string) (Sandbox, error)

	// IsAvailable returns true if sandboxing is available on this platform.
	IsAvailable() bool

	// RunCommand executes a command in a sandbox.
	RunCommand(sandboxID string, command string, args []string) (stdout, stderr string, code int, err error)

	// DestroyS andbox cleans up a sandbox.
	DestroySandbox(sandboxID string) error
}

// Sandbox represents an active sandbox environment.
type Sandbox interface {
	// ID returns the unique identifier of this sandbox.
	ID() string

	// Run executes a command in this sandbox.
	Run(command string, args []string) (stdout, stderr string, code int, err error)

	// WriteFile writes a file into the sandbox.
	WriteFile(path string, content []byte) error

	// ReadFile reads a file from the sandbox.
	ReadFile(path string) ([]byte, error)

	// Cleanup destroys this sandbox.
	Cleanup() error
}

// DefaultSandboxManager is a stub sandbox manager for Phase 1.
type DefaultSandboxManager struct {
	// Phase 1: No state needed, all methods are stubs
}

// CreateSandbox returns an error for Phase 1 (sandboxing not yet implemented).
func (m *DefaultSandboxManager) CreateSandbox(name string) (Sandbox, error) {
	// Phase 1: Return a stub that does nothing
	return &StubSandbox{name: name}, nil
}

// IsAvailable returns false for Phase 1.
func (m *DefaultSandboxManager) IsAvailable() bool {
	return false
}

// RunCommand returns an unimplemented error for Phase 1.
func (m *DefaultSandboxManager) RunCommand(sandboxID, command string, args []string) (string, string, int, error) {
	// Phase 1: Just return an error
	return "", "", 1, ErrSandboxingNotImplemented
}

// DestroySandbox is a no-op for Phase 1.
func (m *DefaultSandboxManager) DestroySandbox(sandboxID string) error {
	return nil
}

// StubSandbox is a no-op sandbox for Phase 1.
type StubSandbox struct {
	name string
}

// ID returns the sandbox ID.
func (s *StubSandbox) ID() string {
	return "stub-" + s.name
}

// Run returns an error for Phase 1.
func (s *StubSandbox) Run(command string, args []string) (string, string, int, error) {
	return "", "", 1, ErrSandboxingNotImplemented
}

// WriteFile is a no-op for Phase 1.
func (s *StubSandbox) WriteFile(path string, content []byte) error {
	return nil
}

// ReadFile returns empty for Phase 1.
func (s *StubSandbox) ReadFile(path string) ([]byte, error) {
	return nil, ErrSandboxingNotImplemented
}

// Cleanup is a no-op for Phase 1.
func (s *StubSandbox) Cleanup() error {
	return nil
}

// NewDefaultSandboxManager creates a default (stub) sandbox manager.
func NewDefaultSandboxManager() SandboxManager {
	return &DefaultSandboxManager{}
}

// ShouldSandbox determines if a command should be sandboxed based on policy.
// Phase 1: Always returns false (no sandboxing available).
func ShouldSandbox(command string, policy SandboxPolicy) bool {
	if policy == nil {
		return false
	}

	return false // Phase 1: no sandboxing
}

// SandboxPolicy defines when and how to sandbox commands.
type SandboxPolicy interface {
	// ShouldApply returns true if sandboxing should apply to this command.
	ShouldApply(command string) bool

	// GetTimeout returns the maximum execution time for sandboxed commands.
	GetTimeout() int // milliseconds
}

// DefaultSandboxPolicy is a basic policy that never sandboxes (Phase 1).
type DefaultSandboxPolicy struct{}

// ShouldApply always returns false for Phase 1.
func (p *DefaultSandboxPolicy) ShouldApply(command string) bool {
	return false
}

// GetTimeout returns 30000ms.
func (p *DefaultSandboxPolicy) GetTimeout() int {
	return 30000
}
