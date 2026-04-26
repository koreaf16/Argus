//go:build tools
// +build tools

package main

import (
	"fmt"
	"github.com/mattn/go-runewidth"
)

func main() {
	var s = "째C"
	fmt.Printf("runewidth: %d\n", runewidth.StringWidth(s))
}

