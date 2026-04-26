// Package hooks
// File: backend_builtin.go
// Description: In-process builtin hook backend.
// Responsibility: Execute builtins.LookupRuntime hooks and map outputs.

package hooks

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/hooks/builtins"
)

type builtinBackend struct{}

func (b *builtinBackend) Run(_ context.Context, def HookDef, input HookInput) (HookOutput, error) {
	name := strings.TrimSpace(def.Builtin)
	if name == "" {
		return HookOutput{}, fmt.Errorf("builtin hook: empty builtin name")
	}
	hook, ok := builtins.LookupRuntime(name)
	if !ok {
		return HookOutput{}, fmt.Errorf("builtin hook: unknown builtin %q", name)
	}

	out, matched := hook.Run(builtins.RuntimeInput{
		Event:  string(input.Event),
		Tool:   input.Tool,
		Input:  input.Input,
		Output: input.Output,
		Session: map[string]any{
			"id":   input.Session.ID,
			"user": input.Session.User,
			"cwd":  input.Session.CWD,
		},
	})
	if !matched {
		return HookOutput{Decision: DecisionAllow}, nil
	}

	decision := Decision(strings.TrimSpace(out.Decision))
	if decision == "" {
		decision = DecisionAllow
	}
	return HookOutput{
		Decision:      decision,
		Reason:        out.Reason,
		SystemMessage: out.SystemMessage,
		NewInput:      out.NewInput,
	}, nil
}
