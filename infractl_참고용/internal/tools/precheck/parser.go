// Package precheck
// File: parser.go
// Description: shell 명령어 문자열을 분석해 필요한 pre-check 목록을 추출하는 순수 함수
// Responsibility: I/O 없이 정규식으로 명령어에서 유저·바이너리·디렉토리를 파싱해 []Checker를 반환한다.

package precheck

import (
	"path"
	"regexp"
	"strings"
)

// nonStandardBinaries lists binaries that require a PATH existence check before execution.
// key: binary name, value: severity (Block = cannot proceed; Warn = advisory only).
var nonStandardBinaries = map[string]Severity{
	// Oracle
	"sqlplus": SeverityBlock, "rman": SeverityBlock, "asmcmd": SeverityBlock,
	"lsnrctl": SeverityWarn, "dgmgrl": SeverityWarn, "adrci": SeverityWarn,
	"dbca": SeverityWarn, "netca": SeverityWarn,
	// MySQL / MariaDB
	"mysqladmin": SeverityBlock, "mysqldump": SeverityWarn, "mysqlcheck": SeverityWarn,
	"mysqlimport": SeverityWarn, "mysql": SeverityBlock,
	// PostgreSQL
	"psql": SeverityBlock, "pg_dump": SeverityWarn, "pg_restore": SeverityWarn,
	"pg_basebackup": SeverityWarn, "pg_ctl": SeverityWarn,
	// MongoDB
	"mongosh": SeverityBlock, "mongodump": SeverityWarn, "mongorestore": SeverityWarn,
	// Redis
	"redis-cli": SeverityWarn, "redis-server": SeverityWarn,
	// Python
	"python3": SeverityWarn, "python": SeverityWarn, "pip3": SeverityWarn,
	// Java
	"java": SeverityWarn, "javac": SeverityWarn,
	// Ansible
	"ansible": SeverityWarn, "ansible-playbook": SeverityWarn,
}

// Patterns for extracting OS usernames from privilege-escalation commands.
var (
	// su - oracle  /  su -l oracle  /  su oracle
	// Pattern: su followed by zero or more flags (each "-flag "), then the username.
	reSU = regexp.MustCompile(
		`(?:^|[;&|]\s*)su\s+(?:-[^\s]*\s+)*([A-Za-z_][A-Za-z0-9_.\-]*)`,
	)
	// sudo -u oracle  /  sudo --user oracle  /  sudo --user=oracle
	reSudo = regexp.MustCompile(
		`(?:^|[;&|]\s*)sudo\s+(?:\S+\s+)*(?:-u\s+|--user[= ])([A-Za-z_][A-Za-z0-9_.\-]*)`,
	)
	// runuser -l oracle  /  runuser -u oracle
	reRunuser = regexp.MustCompile(
		`(?:^|[;&|]\s*)runuser\s+(?:\S+\s+)*-[lu]\s+([A-Za-z_][A-Za-z0-9_.\-]*)`,
	)
	// cd /some/absolute/path
	reCD = regexp.MustCompile(
		`(?:^|[;&|]\s*)cd\s+(/[^\s;&|]*)`,
	)
	// mv/cp/ln/unzip source file: mv /tmp/file.zip /dest/
	reFileArg = regexp.MustCompile(
		`(?:^|[;&|]\s*)(?:mv|cp|ln|unzip)\s+(?:-[^\s]*\s+)*(/[^\s;&|'"]*)`,
	)
	// tar extract: tar -xzf /tmp/archive.tar.gz or tar xzf /tmp/archive.tar.gz
	// Only check when flags contain 'x' (extract mode), not 'c' (create).
	reTarExtract = regexp.MustCompile(
		`(?:^|[;&|]\s*)tar\s+(-[a-z]*x[a-z]*f|[a-z]*x[a-z]*f)\s+(/[^\s;&|'"]*)`,
	)
	// source / . script: source /etc/oracle/env.sh  or  . /etc/oracle/env.sh
	// \.\s requires whitespace after dot to distinguish from ./relative
	reSource = regexp.MustCompile(
		`(?:^|[;&|]\s*)(?:source|\.\s)\s*(/[^\s;&|'"]*)`,
	)
	// unzip -d /dest: destination directory write permission check
	reUnzipDest = regexp.MustCompile(
		`(?:^|[;&|]\s*)unzip\s+.*?-d\s+(/[^\s;&|'"]*)`,
	)
	// tar -C /dest: destination directory write permission check
	reTarDest = regexp.MustCompile(
		`(?:^|[;&|]\s*)tar\s+.*?-C\s+(/[^\s;&|'"]*)`,
	)
	// absolute path binary: /some/dir/bin
	reAbsPath = regexp.MustCompile(
		`(?:^|[;&|]\s*)(/(?:[^\s;&|'"]+/)+)([^\s;&|'"]+)`,
	)

	// cp/mv destination: last absolute path argument (destination).
	// Captures the final absolute-path token in cp/mv commands.
	reCpMvDest = regexp.MustCompile(
		`(?:^|[;&|]\s*)(?:cp|mv)\s+(?:-[^\s]*\s+)*(?:/[^\s;&|'"]*\s+)+(/[^\s;&|'"]*)`,
	)

	// mkdir target path (first absolute-path argument after flags).
	reMkdir = regexp.MustCompile(
		`(?:^|[;&|]\s*)mkdir\s+(?:-[^\s]*\s+)*(/[^\s;&|'"]*)`,
	)

	// chmod/chown target file (second positional argument — first is mode/owner spec).
	reChmodTarget = regexp.MustCompile(
		`(?:^|[;&|]\s*)(?:chmod|chown)\s+\S+\s+(/[^\s;&|'"]*)`,
	)

	// environment variable used as a path prefix: $VAR/ or ${VAR}/
	reEnvVarInPath = regexp.MustCompile(
		`\$\{?([A-Z_][A-Z0-9_]*)\}?/`,
	)

	// lsnrctl start — triggers Oracle listener port check.
	reLsnrctlStart = regexp.MustCompile(
		`(?:^|[;&|]\s*)lsnrctl\s+start\b`,
	)

	// redirect destination: > /path or >> /path
	reRedirectDest = regexp.MustCompile(
		`(?:>>?)\s*(/[^\s;&|'"]*)`,
	)

	// touch target file (first absolute-path argument after flags).
	reTouchTarget = regexp.MustCompile(
		`(?:^|[;&|]\s*)touch\s+(?:-[^\s]*\s+)*(/[^\s;&|'"]*)`,
	)

	// rm -rf / , rm -rf /*, rm -fR ~/, rm -Rf $HOME/, etc.
	// Matches -rf, -fr, -fR, -Rf and similar case-insensitive combinations targeting dangerous paths.
	reRmDangerous = regexp.MustCompile(
		`rm\s+.*-[a-zA-Z]*(?:[rR][a-zA-Z]*[fF]|[fF][a-zA-Z]*[rR])[a-zA-Z]*\s+(/\*?$|/\s|~/|/\s*$|\$HOME[/\s]|\$\{HOME\}[/\s]?)`,
	)

	// tee target path: tee /path/to/file (optional flags before path).
	reTeeTarget = regexp.MustCompile(
		`(?:^|[;&|]\s*)tee\s+(?:-[^\s]*\s+)*(/[^\s;&|'"]*)`,
	)

	// systemctl start|stop|restart|reload|enable|disable <service>.
	reSystemctl = regexp.MustCompile(
		`(?:^|[;&|]\s*)systemctl\s+(start|stop|restart|reload|enable|disable)\s+([a-zA-Z0-9_.\-]+)`,
	)

	// curl/wget with an http/https/ftp URL.
	reCurlWget = regexp.MustCompile(
		`(?:^|[;&|]\s*)(?:curl|wget)\s+(?:-[^\s]*\s+)*(?:--\S+\s+)*(https?://[^\s;&|'"]+|ftp://[^\s;&|'"]+)`,
	)

	// kill <pid>: numeric PID after optional signal flag.
	reKill = regexp.MustCompile(
		`(?:^|[;&|]\s*)kill\s+(?:-[^\s]*\s+)*([0-9]+)`,
	)

	// pkill / killall <process-name>.
	rePkill = regexp.MustCompile(
		`(?:^|[;&|]\s*)(?:pkill|killall)\s+(?:-[^\s]*\s+)*([A-Za-z][A-Za-z0-9_.\-]*)`,
	)
)

// notRootBinaries lists Oracle and DB binaries that should never be executed as root.
var notRootBinaries = map[string]bool{
	"sqlplus": true, "rman": true, "dbca": true, "netca": true,
	"runInstaller": true, "gridSetup": true, "dbua": true,
	"asmcmd": true, "lsnrctl": true,
}

// ParseChecks analyses a shell command string and returns the Checkers that should
// be run before executing it. This function is pure: no I/O, no side effects.
func ParseChecks(command string) []Checker {
	seen := make(map[string]bool)
	var checks []Checker

	add := func(c Checker) {
		key := string(c.Kind()) + ":" + c.Subject()
		if !seen[key] {
			seen[key] = true
			checks = append(checks, c)
		}
	}

	// 1. OS user existence checks (su / sudo / runuser).
	for _, m := range reSU.FindAllStringSubmatch(command, -1) {
		if u := strings.TrimSpace(m[1]); u != "" && u != "root" {
			add(NewUserExistsChecker(u, SeverityBlock))
		}
	}
	for _, m := range reSudo.FindAllStringSubmatch(command, -1) {
		if u := strings.TrimSpace(m[1]); u != "" && u != "root" {
			add(NewUserExistsChecker(u, SeverityBlock))
			// 8. sudo permission check for the target user.
			add(NewSudoAllowedChecker(u, SeverityWarn))
		}
	}
	for _, m := range reRunuser.FindAllStringSubmatch(command, -1) {
		if u := strings.TrimSpace(m[1]); u != "" && u != "root" {
			add(NewUserExistsChecker(u, SeverityBlock))
		}
	}

	// 2. Non-standard binary existence checks.
	// Split command into segments by shell operators and check the first token of each.
	for _, seg := range splitSegments(command) {
		tok := firstToken(seg)
		if tok == "" || strings.HasPrefix(tok, "$") {
			continue
		}
		if sev, ok := nonStandardBinaries[tok]; ok {
			add(NewBinaryExistsChecker(tok, sev))
		}
	}

	// 3. cd target directory existence check.
	for _, m := range reCD.FindAllStringSubmatch(command, -1) {
		dir := strings.TrimSpace(m[1])
		if dir != "" && !strings.HasPrefix(dir, "$") {
			add(NewDirExistsChecker(dir, SeverityWarn))
		}
	}

	// 4. mv/cp/ln/unzip source file existence check.
	for _, m := range reFileArg.FindAllStringSubmatch(command, -1) {
		src := strings.TrimSpace(m[1])
		if src != "" && !strings.HasPrefix(src, "$") {
			add(NewFileExistsChecker(src, SeverityBlock))
		}
	}

	// 4a. tar extract source archive existence check.
	for _, m := range reTarExtract.FindAllStringSubmatch(command, -1) {
		src := strings.TrimSpace(m[2])
		if src != "" && !strings.HasPrefix(src, "$") {
			add(NewFileExistsChecker(src, SeverityBlock))
		}
	}

	// 4b. source / . script file existence check.
	for _, m := range reSource.FindAllStringSubmatch(command, -1) {
		src := strings.TrimSpace(m[1])
		if src != "" && !strings.HasPrefix(src, "$") {
			add(NewFileExistsChecker(src, SeverityWarn))
		}
	}

	// 4c. unzip -d destination write permission check + disk space + fs type.
	for _, m := range reUnzipDest.FindAllStringSubmatch(command, -1) {
		dest := strings.TrimSpace(m[1])
		if dest != "" && !strings.HasPrefix(dest, "$") {
			add(NewDirWritableChecker(dest, SeverityBlock))
			add(NewDiskSpaceChecker(dest, 5*1024*1024*1024, SeverityWarn))
			add(NewFsTypeChecker(dest, SeverityWarn))
		}
	}

	// 4d. tar -C destination write permission check + disk space + fs type.
	for _, m := range reTarDest.FindAllStringSubmatch(command, -1) {
		dest := strings.TrimSpace(m[1])
		if dest != "" && !strings.HasPrefix(dest, "$") {
			add(NewDirWritableChecker(dest, SeverityBlock))
			add(NewDiskSpaceChecker(dest, 5*1024*1024*1024, SeverityWarn))
			add(NewFsTypeChecker(dest, SeverityWarn))
		}
	}

	// 5. Absolute-path binary: check that the parent directory exists.
	for _, m := range reAbsPath.FindAllStringSubmatch(command, -1) {
		dir := strings.TrimRight(m[1], "/")
		if dir != "" && dir != "/usr/bin" && dir != "/usr/sbin" && dir != "/bin" && dir != "/sbin" {
			add(NewDirExistsChecker(path.Clean(dir), SeverityWarn))
		}
	}

	// 6. cp/mv destination directory write permission check.
	for _, m := range reCpMvDest.FindAllStringSubmatch(command, -1) {
		dest := strings.TrimSpace(m[1])
		if dest == "" || strings.HasPrefix(dest, "$") {
			continue
		}
		var dir string
		if strings.HasSuffix(dest, "/") {
			dir = dest
		} else {
			dir = path.Dir(dest)
		}
		if dir != "" && dir != "." && dir != "/" {
			add(NewDirWritableChecker(dir, SeverityBlock))
		}
	}

	// 7. mkdir parent directory write permission check.
	for _, m := range reMkdir.FindAllStringSubmatch(command, -1) {
		p := strings.TrimSpace(m[1])
		if p == "" || strings.HasPrefix(p, "$") {
			continue
		}
		parent := path.Dir(p)
		if parent != "" && parent != "." && parent != "/" {
			add(NewDirWritableChecker(parent, SeverityWarn))
		}
	}

	// 8. chmod/chown target file ownership check.
	for _, m := range reChmodTarget.FindAllStringSubmatch(command, -1) {
		target := strings.TrimSpace(m[1])
		if target != "" && !strings.HasPrefix(target, "$") {
			add(NewFileOwnedChecker(target, SeverityWarn))
		}
	}

	// 9. Environment variable set check (path-prefix variables only).
	// Standard shell variables that are always present are skipped.
	alwaysSet := map[string]bool{
		"PATH": true, "HOME": true, "PWD": true, "USER": true,
		"SHELL": true, "TERM": true, "LANG": true, "IFS": true,
	}
	for _, m := range reEnvVarInPath.FindAllStringSubmatch(command, -1) {
		varName := m[1]
		if varName == "" || alwaysSet[varName] {
			continue
		}
		add(NewEnvVarSetChecker(varName, SeverityWarn))
	}

	// 10. Oracle listener start — port 1521 must be free.
	if reLsnrctlStart.MatchString(command) {
		add(NewPortFreeChecker(1521, SeverityBlock))
	}

	// 11. Redirect destination parent directory write permission check.
	for _, m := range reRedirectDest.FindAllStringSubmatch(command, -1) {
		dest := strings.TrimSpace(m[1])
		if dest == "" || strings.HasPrefix(dest, "$") {
			continue
		}
		parent := path.Dir(dest)
		if parent != "" && parent != "." && parent != "/" {
			add(NewDirWritableChecker(parent, SeverityWarn))
		}
	}

	// 12. touch target parent directory write permission check.
	for _, m := range reTouchTarget.FindAllStringSubmatch(command, -1) {
		target := strings.TrimSpace(m[1])
		if target == "" || strings.HasPrefix(target, "$") {
			continue
		}
		parent := path.Dir(target)
		if parent != "" && parent != "." && parent != "/" {
			add(NewDirWritableChecker(parent, SeverityWarn))
		}
	}

	// 13. Dangerous rm command safety check.
	for _, m := range reRmDangerous.FindAllStringSubmatch(command, -1) {
		add(NewRmSafetyChecker(m[0], SeverityBlock))
	}

	// 14. tee target parent directory write permission check.
	for _, m := range reTeeTarget.FindAllStringSubmatch(command, -1) {
		target := strings.TrimSpace(m[1])
		if target == "" || strings.HasPrefix(target, "$") {
			continue
		}
		parent := path.Dir(target)
		if parent != "" && parent != "." && parent != "/" {
			add(NewDirWritableChecker(parent, SeverityWarn))
		}
	}

	// 15. systemctl service existence and state check.
	for _, m := range reSystemctl.FindAllStringSubmatch(command, -1) {
		action := strings.TrimSpace(m[1])
		svc := strings.TrimSpace(m[2])
		if svc != "" {
			add(NewServiceExistsChecker(svc, action, SeverityWarn))
		}
	}

	// 16. curl/wget host reachability check.
	for _, m := range reCurlWget.FindAllStringSubmatch(command, -1) {
		rawURL := strings.TrimSpace(m[1])
		if rawURL == "" {
			continue
		}
		host, port, ok := parseURLHostPort(rawURL)
		if ok && host != "" && port != "" {
			add(NewHostReachableChecker(host, port, SeverityWarn))
		}
	}

	// 17. Oracle/DB binaries should not run as root.
	for _, seg := range splitSegments(command) {
		tok := firstToken(seg)
		if tok != "" && notRootBinaries[tok] {
			add(NewNotRootChecker(tok, SeverityWarn))
		}
	}

	// 18. kill <pid> — verify PID exists.
	for _, m := range reKill.FindAllStringSubmatch(command, -1) {
		pid := strings.TrimSpace(m[1])
		if pid != "" {
			add(NewProcessExistsChecker(pid, true, SeverityWarn))
		}
	}

	// 19. pkill/killall <name> — verify process name exists.
	for _, m := range rePkill.FindAllStringSubmatch(command, -1) {
		name := strings.TrimSpace(m[1])
		if name != "" {
			add(NewProcessExistsChecker(name, false, SeverityWarn))
		}
	}

	return checks
}

// splitSegments splits a command string by shell operators (;, &&, ||, |).
func splitSegments(cmd string) []string {
	result := make([]string, 0, 4)
	start := 0
	inSingle := false
	inDouble := false

	flush := func(end int) {
		part := strings.TrimSpace(cmd[start:end])
		if part != "" {
			result = append(result, part)
		}
		start = end
	}

	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]
		if inSingle {
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '"' {
				inDouble = false
				continue
			}
			if ch == '\\' && i+1 < len(cmd) {
				i++
			}
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '\\':
			if i+1 < len(cmd) {
				i++
			}
		case ';':
			flush(i)
			start = i + 1
		case '|':
			flush(i)
			if i+1 < len(cmd) && cmd[i+1] == '|' {
				i++
			}
			start = i + 1
		case '&':
			flush(i)
			if i+1 < len(cmd) && cmd[i+1] == '&' {
				i++
			}
			start = i + 1
		}
	}
	flush(len(cmd))
	return result
}

// firstToken returns the first whitespace-delimited token of a command segment,
// skipping common shell prefixes (env, exec, time, nohup) and VAR=val assignments.
func firstToken(seg string) string {
	skip := map[string]bool{"env": true, "exec": true, "time": true, "nohup": true, "nice": true}
	fields := strings.Fields(seg)
	for _, f := range fields {
		if strings.Contains(f, "=") {
			continue // skip VAR=val
		}
		if skip[f] {
			continue
		}
		// Strip leading path components: /usr/bin/sqlplus → sqlplus
		return path.Base(f)
	}
	return ""
}
