// Package background
// File: types_notice.go
// Description: Non-terminal background job notices (e.g., stall hints).
// Responsibility: Define notice payload and notifier interface for watchdogs.

package background

import "time"

// NoticeEvent is a soft signal from background runtime observers.
type NoticeEvent struct {
	JobID int
	Kind  string // currently: "stall"
	Tail  []string
	At    time.Time
}

// WatchdogNotifier consumes soft non-terminal background notices.
type WatchdogNotifier interface {
	Notify(NoticeEvent)
}

// WatchdogNotifierFunc adapts a function to WatchdogNotifier.
type WatchdogNotifierFunc func(NoticeEvent)

// Notify implements WatchdogNotifier.
func (f WatchdogNotifierFunc) Notify(ev NoticeEvent) {
	if f != nil {
		f(ev)
	}
}
