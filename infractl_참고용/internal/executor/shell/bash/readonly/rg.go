// Package readonly
// File: rg.go
// Description: ripgrep 읽기 전용 명령어 화이트리스트
// Responsibility: RipgrepReadOnlyCommands 매핑 제공

// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts:RIPGREP_READ_ONLY_COMMANDS

package readonly

// RipgrepReadOnlyCommands 는 안전한 rg(ripgrep) 플래그 목록이다.
var RipgrepReadOnlyCommands = map[string]CommandConfig{
	"rg": {SafeFlags: map[string]FlagArgType{
		"-e": FlagArgString, "--regexp": FlagArgString,
		"-f": FlagArgString,
		"-i": FlagArgNone, "--ignore-case": FlagArgNone,
		"-S": FlagArgNone, "--smart-case": FlagArgNone,
		"-F": FlagArgNone, "--fixed-strings": FlagArgNone,
		"-w": FlagArgNone, "--word-regexp": FlagArgNone,
		"-v": FlagArgNone, "--invert-match": FlagArgNone,
		"-c": FlagArgNone, "--count": FlagArgNone,
		"-l": FlagArgNone, "--files-with-matches": FlagArgNone,
		"--files-without-match": FlagArgNone,
		"-n": FlagArgNone, "--line-number": FlagArgNone,
		"-o": FlagArgNone, "--only-matching": FlagArgNone,
		"-A": FlagArgNumber, "--after-context": FlagArgNumber,
		"-B": FlagArgNumber, "--before-context": FlagArgNumber,
		"-C": FlagArgNumber, "--context": FlagArgNumber,
		"-H": FlagArgNone, "-h": FlagArgNone,
		"--heading": FlagArgNone, "--no-heading": FlagArgNone,
		"-q": FlagArgNone, "--quiet": FlagArgNone,
		"--column": FlagArgNone,
		"-g": FlagArgString, "--glob": FlagArgString,
		"-t": FlagArgString, "--type": FlagArgString,
		"-T": FlagArgString, "--type-not": FlagArgString,
		"--type-list": FlagArgNone,
		"--hidden": FlagArgNone, "--no-ignore": FlagArgNone,
		"-u": FlagArgNone,
		"-m": FlagArgNumber, "--max-count": FlagArgNumber,
		"-d": FlagArgNumber, "--max-depth": FlagArgNumber,
		"-a": FlagArgNone, "--text": FlagArgNone,
		"-z": FlagArgNone, "-L": FlagArgNone, "--follow": FlagArgNone,
		"--color": FlagArgString, "--json": FlagArgNone, "--stats": FlagArgNone,
		"--help": FlagArgNone, "--version": FlagArgNone, "--debug": FlagArgNone,
		"--": FlagArgNone,
	}},
}
