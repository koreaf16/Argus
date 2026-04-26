package shellsignal

import "strings"

const (
	// BackgroundRequest is a control signal sent from TUI interactive shell views
	// to request detaching a running shell command into a background job.
	BackgroundRequest = "__ARGUS_BACKGROUND_REQUEST__"
)

func IsBackgroundRequest(input string) bool {
	return strings.TrimSpace(input) == BackgroundRequest
}
