package tui

import "strings"

func IsSlashCommand(input string) bool {
	if !strings.HasPrefix(input, "/") {
		return false
	}
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return false
	}
	word := fields[0]
	if strings.Count(word, "/") != 1 {
		return false
	}
	_, ok := resolveSlashCommand(word)
	return ok
}

func IsSlashCommandPrefix(lineStr string) bool {
	if !strings.HasPrefix(lineStr, "/") {
		return false
	}
	word := lineStr
	if idx := strings.IndexAny(lineStr, " \t"); idx >= 0 {
		word = lineStr[:idx]
	}
	return len(slashCommandMatchesPrefix(word)) > 0
}
