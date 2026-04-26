// Package danger
// File: patterns.go
// Description: 개별 명령어(CallExpr) 위험도 판정 패턴 매칭
// Responsibility: 명령어 이름+인자 조합을 Critical/High/Medium/LevelUserApproval 로 분류

// Ported from: claude_cli/src/utils/bash/ast.ts:checkSemantics + specs/*

package danger

import (
	"strings"

	"github.com/yourorg/infractl/internal/executor/shell"
)

// CheckCallExpr 는 명령어 이름과 인자를 보고 위험도와 이유를 반환한다.
// 읽기 전용 화이트리스트 검사는 호출자(semantics.go)에서 먼저 수행한다.
// Ported from: claude_cli/src/utils/bash/ast.ts:checkSemantics
func CheckCallExpr(name string, args []string) (shell.Level, string) {
	if strings.HasPrefix(name, "mkfs") {
		return shell.LevelCritical, "파일시스템 포맷 명령 (데이터 손실): " + name
	}

	switch name {
	case "unzip":
		return checkUnzip(args)
	case "tar":
		return checkTar(args)
	case "rm":
		return checkRm(args)
	case "dd":
		return checkDd(args)
	case "sudo", "su", "doas":
		return checkSudo(name, args)
	case "chmod":
		return checkChmod(args)
	case "chown":
		return checkChown(args)
	case "iptables", "ip6tables", "nft":
		return checkFirewall(name, args)
	case "ufw":
		return checkUfw(args)
	case "curl":
		return checkCurl(args)
	case "wget":
		return checkWget(args)
	case "eval":
		return shell.LevelHigh, "eval 은 임의 코드를 실행합니다"
	case "exec":
		return shell.LevelHigh, "exec 은 현재 프로세스를 교체합니다"
	case "nc", "netcat", "ncat":
		return shell.LevelHigh, name + " 는 임의 네트워크 연결을 생성합니다"
	case "bash", "sh", "zsh", "ksh", "dash", "csh", "tcsh":
		return shell.LevelHigh, name + " 는 임의 코드를 실행할 수 있습니다"
	case "python", "python3", "python2", "node", "ruby", "perl", "php":
		return shell.LevelHigh, name + " 는 임의 스크립트를 실행합니다"
	case "git":
		return checkGit(args)
	case "docker", "podman":
		return checkDocker(name, args)
	case "mv":
		return shell.LevelMedium, "파일을 이동합니다"
	case "cp":
		return shell.LevelMedium, "파일을 복사합니다"
	case "mkdir":
		return shell.LevelMedium, "디렉토리를 생성합니다"
	case "touch":
		return shell.LevelMedium, "파일을 생성합니다"
	case "tee":
		return shell.LevelMedium, "파일에 씁니다"
	case "truncate":
		return shell.LevelMedium, "파일 크기를 조정합니다"
	case "install":
		return shell.LevelMedium, "파일을 설치합니다"
	case "apt", "apt-get", "yum", "dnf", "pacman", "zypper":
		return shell.LevelMedium, name + " 패키지 관리"
	case "brew":
		return shell.LevelMedium, "brew 패키지 관리"
	case "npm", "yarn", "pnpm", "bun":
		return shell.LevelMedium, name + " 패키지 설치"
	case "pip", "pip3", "cargo", "gem":
		return shell.LevelMedium, name + " 패키지 설치"
	case "systemctl", "service", "launchctl":
		return shell.LevelMedium, name + " 서비스 관리"
	case "kill", "killall", "pkill":
		return shell.LevelMedium, "프로세스를 종료합니다"
	}

	// 미식별 명령 → FAIL-CLOSED
	return shell.LevelUserApproval, name + " 는 알 수 없는 명령입니다"
}

// CheckPipe 는 파이프(|)로 연결된 왼쪽·오른쪽 명령 이름의 위험도를 확인한다.
// curl/wget → bash/sh 패턴(원격 코드 실행)을 Critical 로 탐지한다.
// Ported from: claude_cli/src/utils/bash/ast.ts:checkSemantics (pipe pattern)
func CheckPipe(leftName, rightName string) (shell.Level, string) {
	isDownloader := leftName == "curl" || leftName == "wget" || leftName == "fetch"
	isShell := rightName == "bash" || rightName == "sh" || rightName == "zsh" ||
		rightName == "ksh" || rightName == "dash" || rightName == "eval"
	if isDownloader && isShell {
		return shell.LevelCritical, leftName + " | " + rightName + " 원격 코드 실행 패턴"
	}
	return shell.LevelLow, ""
}

// isSystemPath 는 경로가 시스템 핵심 디렉토리인지 확인한다.
// /home 은 최상위 일치 및 타 사용자 경로를 포함한다.
func isSystemPath(p string) bool {
	if p == "/" || p == "~" || p == "$HOME" {
		return true
	}
	// /home 은 정확히 /home 일 때 시스템 경로
	if p == "/home" || p == "/home/" {
		return true
	}
	// 타 사용자 홈 디렉토리 접근 감지 (예: /home/oracle)
	// 현재 사용자($USER) 환경 변수를 알 수 없는 정적 분석 단계이므로,
	// /home/ 하위 1단계 디렉토리 접근은 기본적으로 위험으로 간주한다.

	// 나머지 시스템 디렉토리는 자신 및 모든 하위 경로를 위험으로 처리
	deepDirs := []string{
		"/etc", "/bin", "/sbin", "/usr", "/lib", "/lib64",
		"/root", "/boot", "/sys", "/proc", "/dev", "/var",
	}
	for _, d := range deepDirs {
		if p == d || strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}

func checkUnzip(args []string) (shell.Level, string) {
	for i, a := range args {
		if a == "-d" && i+1 < len(args) {
			target := args[i+1]
			if isSystemPath(target) {
				return shell.LevelCritical, "unzip 이 시스템 또는 타 사용자 경로에 압축을 풉니다: " + target
			}
		}
	}
	return shell.LevelMedium, "unzip 은 압축을 해제합니다"
}

func checkTar(args []string) (shell.Level, string) {
	hasExtract := false
	for _, a := range args {
		if strings.Contains(a, "x") || a == "extract" || a == "--extract" || a == "--get" {
			hasExtract = true
			break
		}
	}
	if !hasExtract {
		return shell.LevelLow, "tar 조회 또는 생성"
	}
	for i, a := range args {
		if (a == "-C" || a == "--directory") && i+1 < len(args) {
			target := args[i+1]
			if isSystemPath(target) {
				return shell.LevelCritical, "tar 가 시스템 또는 타 사용자 경로에 압축을 풉니다: " + target
			}
		}
	}
	return shell.LevelMedium, "tar 는 압축을 해제합니다"
}

func checkRm(args []string) (shell.Level, string) {
	hasRecursive := false

	for _, a := range args {
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") {
			for _, ch := range a[1:] {
				if ch == 'r' || ch == 'R' {
					hasRecursive = true
				}
			}
		}
		if a == "--recursive" {
			hasRecursive = true
		}
	}

	if !hasRecursive {
		return shell.LevelMedium, "rm 은 파일을 삭제합니다"
	}

	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if isSystemPath(a) {
			return shell.LevelCritical, "rm -r 이 시스템 경로를 삭제합니다: " + a
		}
	}

	return shell.LevelMedium, "rm -r 은 디렉토리를 재귀 삭제합니다"
}

func checkDd(args []string) (shell.Level, string) {
	for _, a := range args {
		if strings.HasPrefix(a, "of=") {
			target := a[3:]
			if strings.HasPrefix(target, "/dev/") {
				return shell.LevelCritical, "dd 가 디바이스에 직접 씁니다: " + target
			}
		}
	}
	return shell.LevelMedium, "dd 는 데이터를 직접 복사합니다"
}

func checkSudo(cmd string, args []string) (shell.Level, string) {
	if len(args) > 0 {
		inner := args[0]
		innerArgs := args[1:]
		if inner == "rm" {
			lvl, reason := checkRm(innerArgs)
			if lvl == shell.LevelCritical {
				return shell.LevelCritical, cmd + " " + reason
			}
		}
		if strings.HasPrefix(inner, "mkfs") {
			return shell.LevelCritical, cmd + " 로 파일시스템을 포맷합니다"
		}
	}
	return shell.LevelHigh, cmd + " 는 관리자 권한으로 실행합니다"
}

func checkChmod(args []string) (shell.Level, string) {
	hasRecursive := false
	has777 := false

	for _, a := range args {
		if a == "-R" || a == "--recursive" {
			hasRecursive = true
		} else if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") {
			for _, ch := range a[1:] {
				if ch == 'R' {
					hasRecursive = true
				}
			}
		}
		if a == "777" || a == "0777" || a == "a+rwx" || a == "ugo+rwx" {
			has777 = true
		}
	}

	if has777 && hasRecursive {
		for _, a := range args {
			if strings.HasPrefix(a, "-") || a == "777" || a == "0777" || a == "a+rwx" || a == "ugo+rwx" {
				continue
			}
			if isSystemPath(a) {
				return shell.LevelCritical, "chmod 777 -R 이 시스템 경로에 적용됩니다: " + a
			}
		}
		return shell.LevelHigh, "chmod 777 -R 은 모든 사용자에게 재귀 쓰기 권한 부여"
	}
	if has777 {
		return shell.LevelHigh, "chmod 777 은 모든 사용자에게 쓰기 권한을 부여합니다"
	}
	return shell.LevelMedium, "chmod 는 파일 권한을 변경합니다"
}

func checkChown(args []string) (shell.Level, string) {
	hasRecursive := false
	ownerSeen := false
	var paths []string

	for _, a := range args {
		if a == "-R" || a == "--recursive" {
			hasRecursive = true
			continue
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") {
			for _, ch := range a[1:] {
				if ch == 'R' {
					hasRecursive = true
				}
			}
			continue
		}
		if !ownerSeen {
			ownerSeen = true // 첫 번째 비플래그 인자는 owner:group
			continue
		}
		paths = append(paths, a)
	}

	if hasRecursive {
		for _, p := range paths {
			if isSystemPath(p) {
				return shell.LevelHigh, "chown -R 이 시스템 경로에 적용됩니다: " + p
			}
		}
	}
	return shell.LevelMedium, "chown 은 파일 소유자를 변경합니다"
}

func checkFirewall(name string, args []string) (shell.Level, string) {
	for _, a := range args {
		if a == "-F" || a == "--flush" {
			return shell.LevelHigh, name + " -F 는 방화벽 규칙을 초기화합니다"
		}
	}
	return shell.LevelMedium, name + " 는 방화벽 규칙을 변경합니다"
}

func checkUfw(args []string) (shell.Level, string) {
	for _, a := range args {
		if a == "disable" || a == "reset" {
			return shell.LevelHigh, "ufw " + a + " 는 방화벽을 비활성화합니다"
		}
	}
	return shell.LevelMedium, "ufw 는 방화벽을 관리합니다"
}

func checkCurl(args []string) (shell.Level, string) {
	for i, a := range args {
		lower := strings.ToLower(a)
		if (lower == "-x" || lower == "--request") && i+1 < len(args) {
			method := strings.ToUpper(args[i+1])
			if method == "POST" || method == "PUT" || method == "DELETE" || method == "PATCH" {
				return shell.LevelHigh, "curl " + method + " 요청은 데이터를 전송합니다"
			}
		}
	}
	return shell.LevelMedium, "curl 은 네트워크 요청을 수행합니다"
}

func checkWget(args []string) (shell.Level, string) {
	for i, a := range args {
		if a == "-O" && i+1 < len(args) && args[i+1] == "-" {
			return shell.LevelHigh, "wget -O - 은 stdout 으로 출력합니다 (curl|sh 패턴)"
		}
		if a == "-O-" || strings.HasPrefix(a, "--output-document=-") {
			return shell.LevelHigh, "wget -O- 은 stdout 으로 출력합니다 (curl|sh 패턴)"
		}
	}
	return shell.LevelMedium, "wget 은 파일을 다운로드합니다"
}

func checkGit(args []string) (shell.Level, string) {
	if len(args) == 0 {
		return shell.LevelUserApproval, "git 서브커맨드가 없습니다"
	}
	switch args[0] {
	case "push":
		return shell.LevelMedium, "git push 는 원격 저장소에 씁니다"
	case "reset":
		for _, a := range args[1:] {
			if a == "--hard" {
				return shell.LevelMedium, "git reset --hard 는 변경사항을 버립니다"
			}
		}
		return shell.LevelMedium, "git reset 은 히스토리를 변경합니다"
	case "clean":
		return shell.LevelMedium, "git clean 은 추적되지 않은 파일을 삭제합니다"
	case "checkout", "restore", "switch":
		return shell.LevelMedium, "git " + args[0] + " 은 파일을 변경합니다"
	case "merge", "rebase", "cherry-pick":
		return shell.LevelMedium, "git " + args[0] + " 은 히스토리를 수정합니다"
	}
	return shell.LevelUserApproval, "git " + args[0] + " 은 알 수 없는 서브커맨드"
}

func checkDocker(name string, args []string) (shell.Level, string) {
	if len(args) == 0 {
		return shell.LevelMedium, name + " 명령"
	}
	sub := args[0]
	switch sub {
	case "run":
		for _, a := range args[1:] {
			if a == "--privileged" {
				return shell.LevelHigh, name + " run --privileged"
			}
		}
		return shell.LevelMedium, name + " run"
	case "rm", "rmi", "volume", "network":
		return shell.LevelMedium, name + " " + sub + " 삭제"
	case "exec", "build", "push", "pull":
		return shell.LevelMedium, name + " " + sub
	case "stop", "kill", "pause", "unpause", "restart":
		return shell.LevelMedium, name + " 컨테이너 상태 변경"
	}
	return shell.LevelMedium, name + " " + sub
}
