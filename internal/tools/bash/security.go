// Package bash — Bash security checks.
package bash

import (
	"strings"
)

// CheckBashSecurity validates bash command for dangerous patterns.
func CheckBashSecurity(cmd string) (bool, string) {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return true, ""
	}

	segments := splitCommandPipeline(trimmed)
	for _, seg := range segments {
		fields := strings.Fields(seg)
		if len(fields) == 0 {
			continue
		}

		// Handle env var prefix like EDITOR=vi crontab -e
		startIdx := 0
		for i, f := range fields {
			if strings.Contains(f, "=") && !strings.HasPrefix(f, "-") {
				startIdx = i + 1
				continue
			}
			break
		}
		
		if startIdx >= len(fields) {
			continue
		}
		
		actualFields := fields[startIdx:]
		exe := actualFields[0]

		// Phase 1: Block interactive editors/pagers
		blocked := map[string]bool{
			"vim": true, "vi": true, "nvim": true, "nano": true, "emacs": true,
			"less": true, "more": true, "man": true,
			"top": true, "htop": true, "tmux": true,
			"visudo": true,
		}
		if blocked[exe] {
			return false, "interactive tool blocked: " + exe
		}

		// Phase 2: Block specific interactive patterns
		if exe == "kubectl" || exe == "docker" {
			for _, f := range actualFields {
				if f == "edit" {
					return false, "interactive kubectl edit command blocked"
				}
				if f == "-it" || f == "--interactive" {
					return false, "interactive " + exe + " command blocked"
				}
			}
		}

		if exe == "git" {
			for i, f := range actualFields {
				if f == "rebase" && i+1 < len(actualFields) {
					next := actualFields[i+1]
					if next == "-i" || next == "--interactive" {
						return false, "interactive git rebase blocked"
					}
				}
				if f == "add" && i+1 < len(actualFields) {
					next := actualFields[i+1]
					if next == "-i" || next == "-p" {
						return false, "interactive git add blocked"
					}
				}
				if f == "stash" && i+1 < len(actualFields) && actualFields[i+1] == "-p" {
					return false, "interactive git stash blocked"
				}
			}
		}

		if exe == "crontab" {
			for _, f := range actualFields {
				if f == "-e" {
					return false, "interactive crontab blocked"
				}
			}
		}

		// Phase 3: Block REPLs without arguments
		repls := map[string]bool{
			"python": true, "python3": true, "node": true, "psql": true,
		}
		if repls[exe] {
			if !isReplWithArgs(exe, actualFields) {
				return false, "interactive REPL blocked: " + exe
			}
		}
	}

	return true, ""
}

func splitCommandPipeline(cmd string) []string {
	// Simple split by |, ;, &&
	var segments []string
	
	s := cmd
	s = strings.ReplaceAll(s, "&&", " \x00 ")
	s = strings.ReplaceAll(s, "||", " \x00 ")
	s = strings.ReplaceAll(s, "|", " \x00 ")
	s = strings.ReplaceAll(s, ";", " \x00 ")
	
	parts := strings.Split(s, "\x00")
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			segments = append(segments, trimmed)
		}
	}
	return segments
}

func isReplWithArgs(exe string, fields []string) bool {
	if len(fields) <= 1 {
		return false
	}
	
	// Check if it has flags that indicate non-interactive mode
	for i, f := range fields {
		if i == 0 {
			continue
		}
		// Special case: -i means interactive for python
		if (exe == "python" || exe == "python3") && f == "-i" {
			return false
		}
		
		if (exe == "python" || exe == "python3") && (f == "-c" || !strings.HasPrefix(f, "-")) {
			return true
		}
		if exe == "node" && (f == "-e" || f == "-p" || !strings.HasPrefix(f, "-")) {
			return true
		}
		if exe == "psql" && (f == "-c" || f == "-f") {
			return true
		}
	}
	return false
}
