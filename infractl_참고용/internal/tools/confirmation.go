package tools

import "context"

// ConfirmationHandler handles tool execution confirmations.
type ConfirmationHandler interface {
	Confirm(ctx context.Context, title, message string) (bool, error)
}
