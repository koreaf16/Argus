//go:build windows

package workspace

import (
	"context"
	"fmt"
)

func (m *Manager) openLocalAccountShell(ctx context.Context, opts ExecOptions) (*accountShellSession, error) {
	_ = ctx
	_ = opts
	return nil, fmt.Errorf("local account shell sessions are only supported on Unix-like systems")
}
