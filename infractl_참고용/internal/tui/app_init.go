// Package tui
// File: app_init.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

const (
	defaultAppWidth  = 80
	defaultAppHeight = 24
)

type fdProvider interface {
	Fd() uintptr
}

// initialWindowSizeCmd sends an initial size message so inline TUI mode can
// boot even when Bubble Tea fails to detect the wrapped output as a TTY.
func initialWindowSizeCmd(fd fdProvider) tea.Cmd {
	return func() tea.Msg {
		width, height := defaultAppWidth, defaultAppHeight
		if fd != nil {
			if w, h, err := term.GetSize(fd.Fd()); err == nil && w > 0 && h > 0 {
				width, height = w, h
			}
		}
		return tea.WindowSizeMsg{Width: width, Height: height}
	}
}

