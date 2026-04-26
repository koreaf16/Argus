// Package bash — Parse bash command semantics.
package bash

import (
	"regexp"
	"strings"
)

// CommandSemantics represents the analyzed properties of a shell command.
type CommandSemantics struct {
	IsReadOnly  bool
	IsDangerous bool
	Description string
}

// writeOpsRe matches shell operators that modify files.
var writeOpsRe = regexp.MustCompile(`(>|>>|2>|&>|\|\||&&)`)

// writeCommandsRe matches common commands that modify the filesystem.
var writeCommandsRe = regexp.MustCompile(`(?i)\b(rm|mv|cp|touch|mkdir|rmdir|chmod|chown|sed\s+-i|git\s+(commit|push|pull|checkout|merge|rebase|reset|add))\b`)

// readOnlyCommandsRe matches commands that are strictly read-only.
var readOnlyCommandsRe = regexp.MustCompile(`(?i)\b(ls|cat|grep|rg|find|diff|git\s+(diff|log|show|status|branch|tag|remote|ls-files|blame|rev-parse))\b`)

// ParseCommandSemantics analyzes a bash command to determine its safety and intent.
func ParseCommandSemantics(cmd string) CommandSemantics {
	semantics := CommandSemantics{
		IsReadOnly:  true,
		IsDangerous: false,
	}

	// 리다이렉션 또는 파이프 연결 확인
	if writeOpsRe.MatchString(cmd) {
		semantics.IsReadOnly = false
	}

	// 쓰기 명령어가 포함되어 있는지 확인
	if writeCommandsRe.MatchString(cmd) {
		semantics.IsReadOnly = false
	}

	// 위험한 명령어 여부 (rm -rf 등)
	if strings.Contains(cmd, "rm -rf") || strings.Contains(cmd, "sudo ") {
		semantics.IsDangerous = true
	}

	// 만약 명시적인 읽기 명령어만 있고 쓰기 오퍼레이션이 없다면 ReadOnly 유지
	if readOnlyCommandsRe.MatchString(cmd) && !writeOpsRe.MatchString(cmd) {
		semantics.IsReadOnly = true
	}

	return semantics
}

// IsCommandReadOnly returns true if the command is considered safe to run in read-only mode.
func IsCommandReadOnly(cmd string) bool {
	return ParseCommandSemantics(cmd).IsReadOnly
}
