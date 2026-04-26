// Package readonly
// File: posix.go
// Description: POSIX 기본 읽기 전용 명령어 화이트리스트
// Responsibility: PosixReadOnlyCommands — ls/cat/grep 등 표준 읽기 도구

package readonly

// PosixReadOnlyCommands 는 POSIX 표준 읽기 전용 명령어 목록이다.
// 쓰기 부작용이 없는 명령어만 포함한다.
// claude_cli 의 EXTERNAL_READONLY_COMMANDS + BashTool READONLY_COMMANDS 기반
var PosixReadOnlyCommands = map[string]CommandConfig{
	"ls":       {SafeFlags: map[string]FlagArgType{"-l": FlagArgNone, "-a": FlagArgNone, "-h": FlagArgNone, "-t": FlagArgNone, "-r": FlagArgNone, "-R": FlagArgNone, "-1": FlagArgNone, "-s": FlagArgNone, "--color": FlagArgNone, "-F": FlagArgNone, "-d": FlagArgNone}},
	"cat":      {SafeFlags: map[string]FlagArgType{"-n": FlagArgNone, "-A": FlagArgNone, "-v": FlagArgNone, "-e": FlagArgNone, "-t": FlagArgNone}},
	"head":     {SafeFlags: map[string]FlagArgType{"-n": FlagArgNumber, "-c": FlagArgNumber, "-q": FlagArgNone, "-v": FlagArgNone}},
	"tail":     {SafeFlags: map[string]FlagArgType{"-n": FlagArgNumber, "-c": FlagArgNumber, "-f": FlagArgNone, "-q": FlagArgNone, "-v": FlagArgNone, "--follow": FlagArgNone}},
	"wc":       {SafeFlags: map[string]FlagArgType{"-l": FlagArgNone, "-w": FlagArgNone, "-c": FlagArgNone, "-m": FlagArgNone, "-L": FlagArgNone}},
	"grep":     {SafeFlags: map[string]FlagArgType{"-i": FlagArgNone, "-v": FlagArgNone, "-n": FlagArgNone, "-r": FlagArgNone, "-R": FlagArgNone, "-l": FlagArgNone, "-c": FlagArgNone, "-e": FlagArgString, "-E": FlagArgNone, "-F": FlagArgNone, "-P": FlagArgNone, "-o": FlagArgNone, "-q": FlagArgNone, "--color": FlagArgNone, "-A": FlagArgNumber, "-B": FlagArgNumber, "-C": FlagArgNumber, "-H": FlagArgNone, "-h": FlagArgNone, "-w": FlagArgNone, "--include": FlagArgString, "--exclude": FlagArgString}},
	"egrep":    {SafeFlags: map[string]FlagArgType{"-i": FlagArgNone, "-v": FlagArgNone, "-n": FlagArgNone, "-r": FlagArgNone, "-l": FlagArgNone, "-c": FlagArgNone, "-o": FlagArgNone, "-q": FlagArgNone, "-A": FlagArgNumber, "-B": FlagArgNumber, "-C": FlagArgNumber}},
	"find":     {SafeFlags: map[string]FlagArgType{"-name": FlagArgString, "-type": FlagArgString, "-maxdepth": FlagArgNumber, "-mindepth": FlagArgNumber, "-size": FlagArgString, "-mtime": FlagArgNumber, "-newer": FlagArgString, "-not": FlagArgNone, "-o": FlagArgNone, "-and": FlagArgNone, "-or": FlagArgNone, "-ls": FlagArgNone, "-print": FlagArgNone, "-print0": FlagArgNone}, IsDangerous: func(_ string, args []string) bool {
		for _, a := range args {
			if a == "-delete" || a == "-exec" || a == "-execdir" || a == "-ok" || a == "-okdir" {
				return true
			}
		}
		return false
	}},
	"stat":     {SafeFlags: map[string]FlagArgType{"-c": FlagArgString, "--format": FlagArgString, "-L": FlagArgNone, "--dereference": FlagArgNone, "-f": FlagArgNone, "--file-system": FlagArgNone}},
	"file":     {SafeFlags: map[string]FlagArgType{"-b": FlagArgNone, "-i": FlagArgNone, "-L": FlagArgNone, "-z": FlagArgNone}},
	"tree":     {SafeFlags: map[string]FlagArgType{"-a": FlagArgNone, "-d": FlagArgNone, "-L": FlagArgNumber, "-f": FlagArgNone, "-h": FlagArgNone, "--du": FlagArgNone, "-I": FlagArgString, "-P": FlagArgString}},
	"du":       {SafeFlags: map[string]FlagArgType{"-h": FlagArgNone, "-s": FlagArgNone, "-a": FlagArgNone, "--max-depth": FlagArgNumber, "-d": FlagArgNumber, "-c": FlagArgNone, "--apparent-size": FlagArgNone}},
	"df":       {SafeFlags: map[string]FlagArgType{"-h": FlagArgNone, "-T": FlagArgNone, "-i": FlagArgNone, "-a": FlagArgNone, "--total": FlagArgNone}},
	"which":    {SafeFlags: map[string]FlagArgType{"-a": FlagArgNone}},
	"echo":     {SafeFlags: map[string]FlagArgType{"-n": FlagArgNone, "-e": FlagArgNone, "-E": FlagArgNone}},
	"printf":   {SafeFlags: map[string]FlagArgType{}},
	"pwd":      {SafeFlags: map[string]FlagArgType{"-L": FlagArgNone, "-P": FlagArgNone}},
	"date":     {SafeFlags: map[string]FlagArgType{"-u": FlagArgNone, "-R": FlagArgNone, "-d": FlagArgString, "--date": FlagArgString}},
	"whoami":   {SafeFlags: map[string]FlagArgType{}},
	"id":       {SafeFlags: map[string]FlagArgType{"-u": FlagArgNone, "-g": FlagArgNone, "-G": FlagArgNone, "-n": FlagArgNone, "-r": FlagArgNone}},
	"uname":    {SafeFlags: map[string]FlagArgType{"-a": FlagArgNone, "-s": FlagArgNone, "-n": FlagArgNone, "-r": FlagArgNone, "-m": FlagArgNone, "-p": FlagArgNone, "-i": FlagArgNone, "-o": FlagArgNone}},
	"env":      {SafeFlags: map[string]FlagArgType{"-0": FlagArgNone, "--null": FlagArgNone}},
	"sort":     {SafeFlags: map[string]FlagArgType{"-r": FlagArgNone, "-n": FlagArgNone, "-k": FlagArgString, "-t": FlagArgChar, "-u": FlagArgNone, "-h": FlagArgNone, "-f": FlagArgNone, "-b": FlagArgNone, "-V": FlagArgNone}},
	"uniq":     {SafeFlags: map[string]FlagArgType{"-c": FlagArgNone, "-d": FlagArgNone, "-u": FlagArgNone, "-i": FlagArgNone, "-f": FlagArgNumber, "-s": FlagArgNumber}},
	"awk":      {SafeFlags: map[string]FlagArgType{"-F": FlagArgString, "-v": FlagArgString, "-f": FlagArgString}},
	"sed":      {SafeFlags: map[string]FlagArgType{"-n": FlagArgNone, "-e": FlagArgString, "-f": FlagArgString, "-r": FlagArgNone, "-E": FlagArgNone}, IsDangerous: func(_ string, args []string) bool {
		for _, a := range args {
			if a == "-i" || a == "--in-place" {
				return true
			}
		}
		return false
	}},
	"column":   {SafeFlags: map[string]FlagArgType{"-t": FlagArgNone, "-s": FlagArgString, "-o": FlagArgString, "-n": FlagArgNone}},
	"basename": {SafeFlags: map[string]FlagArgType{"-a": FlagArgNone, "-s": FlagArgString}},
	"dirname":  {SafeFlags: map[string]FlagArgType{}},
	"readlink": {SafeFlags: map[string]FlagArgType{"-f": FlagArgNone, "-e": FlagArgNone, "-m": FlagArgNone, "-n": FlagArgNone, "-s": FlagArgNone}},
	"realpath": {SafeFlags: map[string]FlagArgType{"-e": FlagArgNone, "-m": FlagArgNone, "-s": FlagArgNone, "--relative-to": FlagArgString, "--relative-base": FlagArgString}},
	"tr":       {SafeFlags: map[string]FlagArgType{"-d": FlagArgNone, "-s": FlagArgNone, "-c": FlagArgNone, "-C": FlagArgNone}},
	"cut":      {SafeFlags: map[string]FlagArgType{"-f": FlagArgString, "-d": FlagArgChar, "-c": FlagArgString, "-b": FlagArgString, "--complement": FlagArgNone, "-s": FlagArgNone, "--zero-terminated": FlagArgNone}},
	"xargs":    {SafeFlags: map[string]FlagArgType{"-n": FlagArgNumber, "-d": FlagArgChar, "-0": FlagArgNone, "-P": FlagArgNumber, "-I": FlagArgString, "-L": FlagArgNumber, "--null": FlagArgNone}},
	"true":     {SafeFlags: map[string]FlagArgType{}},
	"false":    {SafeFlags: map[string]FlagArgType{}},
	"test":     {SafeFlags: map[string]FlagArgType{}},
	"[":        {SafeFlags: map[string]FlagArgType{}},
	"type":     {SafeFlags: map[string]FlagArgType{"-a": FlagArgNone, "-f": FlagArgNone, "-P": FlagArgNone, "-t": FlagArgNone}},
	"command":  {SafeFlags: map[string]FlagArgType{"-v": FlagArgNone, "-V": FlagArgNone, "-p": FlagArgNone}},
	"ps":       {SafeFlags: map[string]FlagArgType{"-e": FlagArgNone, "-f": FlagArgNone, "-u": FlagArgString, "-a": FlagArgNone, "ax": FlagArgNone, "aux": FlagArgNone, "-o": FlagArgString, "--no-headers": FlagArgNone}},
	"lsof":     {SafeFlags: map[string]FlagArgType{"-p": FlagArgString, "-i": FlagArgString, "-u": FlagArgString, "-c": FlagArgString, "-d": FlagArgString, "-a": FlagArgNone, "-n": FlagArgNone}},
	"netstat":  {SafeFlags: map[string]FlagArgType{"-t": FlagArgNone, "-u": FlagArgNone, "-l": FlagArgNone, "-p": FlagArgNone, "-n": FlagArgNone, "-a": FlagArgNone, "-r": FlagArgNone}},
	"ss":       {SafeFlags: map[string]FlagArgType{"-t": FlagArgNone, "-u": FlagArgNone, "-l": FlagArgNone, "-p": FlagArgNone, "-n": FlagArgNone, "-a": FlagArgNone, "-4": FlagArgNone, "-6": FlagArgNone}},
	"hostname": {SafeFlags: map[string]FlagArgType{"-f": FlagArgNone, "-i": FlagArgNone, "-I": FlagArgNone}},
	"uptime":   {SafeFlags: map[string]FlagArgType{"-p": FlagArgNone, "-s": FlagArgNone}},
	"free":     {SafeFlags: map[string]FlagArgType{"-h": FlagArgNone, "-m": FlagArgNone, "-g": FlagArgNone, "-b": FlagArgNone, "-s": FlagArgNumber, "-c": FlagArgNumber}},
}
