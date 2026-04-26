// Package tools
// File: preflight_cmd_scan.go
// Description: shell 명령어의 디스크 변경 패턴 탐지 및 대상 경로 추출
// Responsibility: shell_exec에 전달되는 명령어 문자열을 정규식으로 분석하여
//                 디스크 수정 여부, 위험 수준, 추출된 대상 경로를 반환한다.

package tools

import (
	"regexp"
	"strings"
)

// RiskLevel represents the severity of a disk-modifying operation.
type RiskLevel string

const (
	RiskHigh   RiskLevel = "high"
	RiskMedium RiskLevel = "medium"
	RiskLow    RiskLevel = "low"
)

// CmdScanResult holds the analysis of a shell command string.
type CmdScanResult struct {
	IsDiskModifying bool
	TargetPaths     []string
	Risk            RiskLevel
	Description     string
}

// cmdPattern describes a single pattern to match against a command string.
type cmdPattern struct {
	re          *regexp.Regexp
	risk        RiskLevel
	description string
	extractPath func(matches []string) []string // extracts target paths from subgroups
}

// singlePath returns the first non-empty capture group as a single path slice.
func singlePath(matches []string) []string {
	for _, m := range matches[1:] {
		if m != "" {
			return []string{m}
		}
	}
	return nil
}

// systemRoot returns /  indicating the entire system may be affected.
func systemRoot(_ []string) []string { return []string{"/"} }

// packageInstallRoots returns common package-manager write locations.
func packageInstallRoots(_ []string) []string { return []string{"/", "/var"} }

// noPath returns nil when path extraction is not feasible.
func noPath(_ []string) []string { return nil }

// cmdPatterns is evaluated in order; the first match wins for risk/description,
// but ALL matching patterns contribute to TargetPaths.
var cmdPatterns = []cmdPattern{
	// ── High risk ─────────────────────────────────────────────────────────────
	{
		re:          regexp.MustCompile(`(?i)\brm\s+(-[a-z]*r[a-z]*f[a-z]*|-[a-z]*f[a-z]*r[a-z]*)\s+(\S+)`),
		risk:        RiskHigh,
		description: "recursive force delete (rm -rf)",
		extractPath: func(m []string) []string { return []string{m[len(m)-1]} },
	},
	{
		re:          regexp.MustCompile(`(?i)\brm\s+-r\s+(\S+)`),
		risk:        RiskHigh,
		description: "recursive delete (rm -r)",
		extractPath: singlePath,
	},
	{
		re:          regexp.MustCompile(`(?i)\bdd\b.*\bof=(\S+)`),
		risk:        RiskHigh,
		description: "raw disk write (dd)",
		extractPath: singlePath,
	},
	{
		re:          regexp.MustCompile(`(?i)\b(mkfs|mke2fs|mkswap)\b`),
		risk:        RiskHigh,
		description: "filesystem creation (mkfs/mkswap)",
		extractPath: systemRoot,
	},
	{
		re:          regexp.MustCompile(`(?i)\b(fdisk|parted|gdisk|sgdisk)\b`),
		risk:        RiskHigh,
		description: "disk partition tool (fdisk/parted)",
		extractPath: systemRoot,
	},

	// ── Medium risk ───────────────────────────────────────────────────────────
	{
		re:          regexp.MustCompile(`(?i)\btar\b.*-[a-z]*x[a-z]*.*-C\s+(\S+)`),
		risk:        RiskMedium,
		description: "archive extraction (tar -x -C)",
		extractPath: singlePath,
	},
	{
		re:          regexp.MustCompile(`(?i)\btar\b.*-[a-z]*x`),
		risk:        RiskMedium,
		description: "archive extraction (tar -x)",
		extractPath: noPath,
	},
	{
		re:          regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?(unzip|gunzip|bunzip2|xz\s+-d)\b.*\s+(\S+)`),
		risk:        RiskMedium,
		description: "archive extraction (unzip/gunzip)",
		extractPath: func(m []string) []string { return []string{m[len(m)-1]} },
	},
	{
		re:          regexp.MustCompile(`(?i)\bcp\s+(-[a-z]+\s+)*\S+\s+(\S+)`),
		risk:        RiskMedium,
		description: "file copy (cp)",
		extractPath: func(m []string) []string { return []string{m[len(m)-1]} },
	},
	{
		re:          regexp.MustCompile(`(?i)\bmv\s+(-[a-z]+\s+)*\S+\s+(\S+)`),
		risk:        RiskMedium,
		description: "file move (mv)",
		extractPath: func(m []string) []string { return []string{m[len(m)-1]} },
	},
	{
		re:          regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?(?:/usr/bin/)?install\s+(-[a-z]+\s+)*\S+\s+(\S+)`),
		risk:        RiskMedium,
		description: "file install (install)",
		extractPath: func(m []string) []string { return []string{m[len(m)-1]} },
	},
	{
		re:          regexp.MustCompile(`(?i)\b(yum|dnf|apt|apt-get)\s+install\b`),
		risk:        RiskMedium,
		description: "package install (yum/dnf/apt)",
		extractPath: packageInstallRoots,
	},
	{
		re:          regexp.MustCompile(`(?i)\bwget\s+.*-O\s+(\S+)`),
		risk:        RiskMedium,
		description: "download to file (wget -O)",
		extractPath: singlePath,
	},
	{
		re:          regexp.MustCompile(`(?i)\bcurl\s+.*-o\s+(\S+)`),
		risk:        RiskMedium,
		description: "download to file (curl -o)",
		extractPath: singlePath,
	},
	{
		re:          regexp.MustCompile(`(?i)\btee\b(?:\s+-[-a-zA-Z0-9]+\b)*\s+(\S+)`),
		risk:        RiskMedium,
		description: "write via tee",
		extractPath: singlePath,
	},
	{
		re:          regexp.MustCompile(`(?i)\bmkdir\s+(-[a-z]+\s+)*(\S+)`),
		risk:        RiskMedium,
		description: "directory creation (mkdir)",
		extractPath: func(m []string) []string { return []string{m[len(m)-1]} },
	},

	// ── Low risk ──────────────────────────────────────────────────────────────
	{
		re:          regexp.MustCompile(`(?i)\bchmod\s+\S+\s+(\S+)`),
		risk:        RiskLow,
		description: "permission change (chmod)",
		extractPath: singlePath,
	},
	{
		re:          regexp.MustCompile(`(?i)\bchown\s+\S+\s+(\S+)`),
		risk:        RiskLow,
		description: "ownership change (chown)",
		extractPath: singlePath,
	},
	{
		re:          regexp.MustCompile(`(?i)\b(pip|pip3)\s+install\b`),
		risk:        RiskLow,
		description: "Python package install (pip)",
		extractPath: noPath,
	},
}

// ScanCommand analyses a shell command string and returns a CmdScanResult.
// It is a pure function (no I/O) and may be called from any goroutine.
func ScanCommand(command string) CmdScanResult {
	var result CmdScanResult
	seen := map[string]bool{}

	highestRisk := RiskLevel("")
	var descriptions []string

	for _, p := range cmdPatterns {
		matches := p.re.FindStringSubmatch(command)
		if matches == nil {
			continue
		}
		result.IsDiskModifying = true
		descriptions = append(descriptions, p.description)

		if paths := p.extractPath(matches); len(paths) > 0 {
			for _, path := range paths {
				if path != "" && !seen[path] {
					seen[path] = true
					result.TargetPaths = append(result.TargetPaths, path)
				}
			}
		}

		if highestRisk == "" || riskOrdinal(p.risk) > riskOrdinal(highestRisk) {
			highestRisk = p.risk
		}
	}

	// Also extract redirect targets (>, >>).
	for _, m := range redirectRe.FindAllStringSubmatch(command, -1) {
		path := strings.TrimSpace(m[2])
		if path != "" && !seen[path] {
			seen[path] = true
			result.TargetPaths = append(result.TargetPaths, path)
			if !result.IsDiskModifying {
				result.IsDiskModifying = true
				descriptions = append(descriptions, "shell redirect (> or >>)")
				if highestRisk == "" {
					highestRisk = RiskMedium
				}
			}
		}
	}

	if result.IsDiskModifying {
		result.Risk = highestRisk
		result.Description = strings.Join(descriptions, "; ")
	}
	return result
}

// redirectRe matches shell output redirections like > /path or >> /path.
// Excludes 2> (stderr) and heredoc <<.
var redirectRe = regexp.MustCompile(`(?:^|[^<2])(>{1,2})\s*([^&>\s][^\s]*)`)

// systemRiskEntry는 시스템 중단 위험 패턴 하나를 나타낸다.
type systemRiskEntry struct {
	re          *regexp.Regexp
	description string
}

// systemRiskPatterns는 디스크 수정과 무관하지만 시스템 서비스·보안·프로세스에
// 심각한 영향을 줄 수 있는 명령 패턴이다.
var systemRiskPatterns = []systemRiskEntry{
	// 시스템 종료/재부팅
	{regexp.MustCompile(`(?i)\b(reboot|halt|poweroff)\b`), "시스템 종료/재부팅 (reboot/halt/poweroff)"},
	{regexp.MustCompile(`(?i)\bshutdown\s+(-[a-z]+\s+)*(now|-h|-r)\b`), "시스템 종료 (shutdown)"},
	{regexp.MustCompile(`(?i)\bsystemctl\s+(reboot|poweroff|halt|kexec)\b`), "systemctl 종료/재부팅"},
	{regexp.MustCompile(`(?i)\binit\s+[06]\b`), "init 0/6 (halt/reboot)"},

	// 서비스 중지 (원격 접속/네트워크 차단 위험)
	{regexp.MustCompile(`(?i)\bsystemctl\s+(stop|disable)\s+(sshd|network|NetworkManager|firewalld)\b`), "핵심 서비스 중지/비활성화 (원격 접속 차단 위험)"},
	{regexp.MustCompile(`(?i)\bservice\s+(sshd|network|NetworkManager|firewalld)\s+stop\b`), "핵심 서비스 중지 (원격 접속 차단 위험)"},

	// 광범위한 권한 파괴
	{regexp.MustCompile(`(?i)\b(chmod|chown)\b.*\s+(-[a-z]*R[a-z]*)\b.*\s+(/|/etc|/boot|/usr|/var)\b`), "시스템 핵심 경로 권한/소유권 재귀적 변경 (시스템 파괴 위험)"},

	// 전체 프로세스 강제 종료
	{regexp.MustCompile(`(?i)\bkill\s+(-9\s+)?-1\b`), "모든 프로세스 강제 종료 (kill -1)"},
	{regexp.MustCompile(`(?i)\bpkill\s+(-9\s+)?-u\s+root\b`), "root 프로세스 전체 kill"},

	// 방화벽/네트워크 보안 해제
	{regexp.MustCompile(`(?i)\biptables\s+-F\b`), "iptables 규칙 전체 초기화"},
	{regexp.MustCompile(`(?i)\bip6tables\s+-F\b`), "ip6tables 규칙 전체 초기화"},
	{regexp.MustCompile(`(?i)\bufw\s+disable\b`), "UFW 방화벽 비활성화"},
	{regexp.MustCompile(`(?i)\bfirewall-cmd\s+.*--remove-all\b`), "firewalld 규칙 전체 삭제"},

	// 크리티컬 시스템 파일 덮어쓰기
	{regexp.MustCompile(`(?i)(:\s*>|truncate\s+-s\s+0)\s+/etc/(passwd|shadow|sudoers|hosts)\b`), "시스템 파일 내용 삭제"},

	// 크론 전체 삭제
	{regexp.MustCompile(`(?i)\bcrontab\s+-r\b`), "crontab 전체 삭제"},

	// 전체 파일시스템 삭제
	{regexp.MustCompile(`(?i)\bfind\s+/\s+.*-delete\b`), "find / -delete (전체 파일 삭제)"},
}

// SystemRiskResult는 시스템 중단 위험 검사 결과다.
type SystemRiskResult struct {
	IsSystemDisruptive bool
	Description        string
}

// ScanSystemRisk는 디스크 수정 외에 시스템 서비스·보안·프로세스에 위험한 명령을 감지한다.
// 순수 함수로 I/O 없이 호출 가능하다.
func ScanSystemRisk(cmd string) SystemRiskResult {
	var descs []string
	for _, p := range systemRiskPatterns {
		if p.re.MatchString(cmd) {
			descs = append(descs, p.description)
		}
	}
	if len(descs) > 0 {
		return SystemRiskResult{
			IsSystemDisruptive: true,
			Description:        strings.Join(descs, "; "),
		}
	}
	return SystemRiskResult{}
}

func riskOrdinal(r RiskLevel) int {
	switch r {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	}
	return 0
}

// privilegeRe matches commands that start with a privilege-escalation wrapper
// (sudo, su, runuser). These wrappers mean the effective user executing the
// payload will be root (or another privileged account), so a write-permission
// pre-flight check performed as the calling (unprivileged) user would produce a
// false-positive failure.
var privilegeRe = regexp.MustCompile(`(?i)^\s*(sudo|su|runuser)\b`)

// CommandImpliesPrivilege reports whether cmd begins with a privilege-escalation
// prefix (sudo / su / runuser). When true, the effective executor will typically
// be root, so write-permission checks on system paths should be skipped.
func CommandImpliesPrivilege(cmd string) bool {
	return privilegeRe.MatchString(cmd)
}
