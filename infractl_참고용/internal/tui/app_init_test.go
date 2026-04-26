// Package tui
// File: app_init_test.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInitialWindowSizeCmdFallsBackToDefault(t *testing.T) {
	msg := initialWindowSizeCmd(nil)()
	sizeMsg, ok := msg.(tea.WindowSizeMsg)
	if !ok {
		t.Fatalf("expected tea.WindowSizeMsg, got %T", msg)
	}
	if sizeMsg.Width != defaultAppWidth || sizeMsg.Height != defaultAppHeight {
		t.Fatalf("expected default size %dx%d, got %dx%d",
			defaultAppWidth, defaultAppHeight, sizeMsg.Width, sizeMsg.Height)
	}
}

