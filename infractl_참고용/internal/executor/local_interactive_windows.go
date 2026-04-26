//go:build windows

package executor

import (
	"context"
	"fmt"
)

// ExecuteInteractive is not supported on Windows local execution.
func (e *LocalExecutor) ExecuteInteractive(context.Context, InteractiveSpec, func(string)) (ExecSession, error) {
	return nil, fmt.Errorf("interactive PTY execution is not supported on Windows")
}
