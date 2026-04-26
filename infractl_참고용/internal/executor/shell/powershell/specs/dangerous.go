// Package specs
// File: dangerous.go
// Description: PowerShell 위험 명령 패턴 정의
// Responsibility: CheckDangerous — 알려진 파괴적/고위험 PowerShell 패턴 탐지

// Ported from: claude_cli/src/utils/shell/powershellProvider.ts (danger patterns)

package specs

import (
	"strings"

	"github.com/yourorg/infractl/internal/executor/shell"
)

// DangerPattern 은 단일 위험 패턴을 정의한다.
type DangerPattern struct {
	// Substring 은 명령어 문자열에서 검색할 부분 문자열 (대소문자 무시)
	Substring string
	// Level 은 이 패턴의 위험도
	Level shell.Level
	// Reason 은 위험 이유 설명
	Reason string
}

// patterns 는 PowerShell 위험 명령 패턴 목록이다.
// Ported from: claude_cli/src/utils/shell/powershellProvider.ts (dangerous patterns)
var patterns = []DangerPattern{
	// Critical — 즉각적·회복불가 파괴
	{Substring: "remove-item -recurse -force c:\\", Level: shell.LevelCritical, Reason: "C 드라이브 전체 삭제"},
	{Substring: "remove-item -recurse -force /", Level: shell.LevelCritical, Reason: "루트 경로 재귀 삭제"},
	{Substring: "format-volume", Level: shell.LevelCritical, Reason: "볼륨 포맷"},
	{Substring: "clear-disk", Level: shell.LevelCritical, Reason: "디스크 초기화"},

	// Critical — 원격 코드 실행
	{Substring: "invoke-expression", Level: shell.LevelCritical, Reason: "Invoke-Expression 은 임의 코드를 실행합니다"},
	{Substring: "iex ", Level: shell.LevelCritical, Reason: "IEX (Invoke-Expression 별칭) 임의 코드 실행"},
	{Substring: "invoke-webrequest | invoke-expression", Level: shell.LevelCritical, Reason: "원격 스크립트 다운로드 후 즉시 실행"},
	{Substring: "irm | iex", Level: shell.LevelCritical, Reason: "curl|sh 패턴 (PowerShell)"},
	{Substring: "invoke-restmethod | invoke-expression", Level: shell.LevelCritical, Reason: "원격 스크립트 다운로드 후 즉시 실행"},

	// High — 보안 설정 변경
	{Substring: "set-executionpolicy unrestricted", Level: shell.LevelHigh, Reason: "스크립트 실행 정책을 Unrestricted 으로 변경"},
	{Substring: "set-executionpolicy bypass", Level: shell.LevelHigh, Reason: "스크립트 실행 정책을 Bypass 로 변경"},
	{Substring: "disable-windowsoptionalfeature", Level: shell.LevelHigh, Reason: "Windows 기능 비활성화"},
	{Substring: "stop-service", Level: shell.LevelHigh, Reason: "서비스 중지"},
	{Substring: "disable-service", Level: shell.LevelHigh, Reason: "서비스 비활성화"},

	// High — 네트워크
	{Substring: "invoke-webrequest", Level: shell.LevelHigh, Reason: "네트워크 요청 (Invoke-WebRequest)"},
	{Substring: "invoke-restmethod", Level: shell.LevelHigh, Reason: "네트워크 요청 (Invoke-RestMethod)"},
	{Substring: "start-bitstransfer", Level: shell.LevelHigh, Reason: "BITS 파일 전송"},
}

// CheckDangerous 는 PowerShell 명령 문자열의 위험도를 반환한다.
// FAIL-CLOSED: 패턴 미일치 → LevelUserApproval (AST 없으므로 미식별)
// Ported from: claude_cli/src/utils/shell/powershellProvider.ts
func CheckDangerous(command string) (shell.Level, string) {
	lower := strings.ToLower(command)

	maxLevel := shell.LevelLow
	var maxReason string

	for _, p := range patterns {
		if strings.Contains(lower, p.Substring) {
			if p.Level == shell.LevelCritical {
				return shell.LevelCritical, p.Reason
			}
			if p.Level > maxLevel {
				maxLevel = p.Level
				maxReason = p.Reason
			}
		}
	}

	if maxLevel > shell.LevelLow {
		return maxLevel, maxReason
	}

	// PowerShell 은 AST 분석이 없으므로 미식별 명령 → FAIL-CLOSED
	return shell.LevelUserApproval, "PowerShell 명령 — 수동 검토 필요"
}
