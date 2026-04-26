// Package builtins
// File: runtime.go
// Description: Runtime builtin hook registry.
// Responsibility: Lookup and execute in-process builtin hooks by name.

package builtins

import "strings"

// RuntimeInput is the in-process builtin hook input payload.
type RuntimeInput struct {
	Event   string
	Tool    string
	Input   map[string]any
	Output  map[string]any
	Session map[string]any
}

// RuntimeOutput is the in-process builtin hook output payload.
type RuntimeOutput struct {
	Decision      string
	Reason        string
	SystemMessage string
	NewInput      map[string]any
}

// RuntimeHook is an in-process builtin hook implementation.
type RuntimeHook interface {
	Name() string
	Run(input RuntimeInput) (RuntimeOutput, bool)
}

var runtimeRegistry = map[string]RuntimeHook{
	"permission_retry": PermissionRetryHook{},
}

// LookupRuntime returns a runtime builtin hook by name.
func LookupRuntime(name string) (RuntimeHook, bool) {
	hook, ok := runtimeRegistry[strings.ToLower(strings.TrimSpace(name))]
	return hook, ok
}
