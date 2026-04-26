// Package agent
// File: wiring.go
// Description: Runtime wiring helpers for install-path/background notifications.
// Responsibility: Expose notify multicasting used by composition roots.

package agent

import "github.com/yourorg/infractl/internal/background"

// ChainBackgroundNotify returns a notify func that fans out to the existing
// callback and the install-path pipeline completion hook.
func (a *Agent) ChainBackgroundNotify(existing background.NotifyFunc) background.NotifyFunc {
	return func(jobID int, description string, success bool) {
		if existing != nil {
			existing(jobID, description, success)
		}
		if a != nil && a.installPath != nil {
			a.installPath.OnJobComplete(jobID, success)
		}
	}
}
