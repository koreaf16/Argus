//go:build windows

package utils

func SupportsPTY() bool {
	return false
}
