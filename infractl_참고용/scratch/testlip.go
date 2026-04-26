//go:build tools
// +build tools

package main

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
)

func main() {
	var s = "째C"
	fmt.Printf("lipgloss.Width: %d\n", lipgloss.Width(s))
}

