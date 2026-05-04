package powershell

import (
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *PowerShellTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	whenItems := []string{
		"Windows 환경에서 PowerShell 명령 실행이 필요한 경우",
		"Get-*, Set-*, New-*, Remove-* 등 PS Cmdlet 사용 시",
		"Windows 서비스·레지스트리·이벤트 로그 조작",
		"Windows 네이티브 도구(netsh, sfc, chkdsk 등) 실행",
	}
	if ctx.Workspaces.Count > 1 {
		whenItems = append(whenItems, "Windows 원격 서버 명령 — `server` 파라미터로 대상 지정 필수")
	}
	s.WhenToUse(whenItems...)

	s.WhenNotToUse(
		"Unix/Linux 서버에서 bash 명령 실행 → bash 도구 사용",
		"파일 읽기 → Read 도구, 검색 → Grep/Glob 도구",
		"단순 파일 쓰기 → FileWrite 도구",
	)

	usageItems := []string{
		"PowerShell 5.1(powershell.exe) — &&, || 파이프 체이닝 미지원, ; 또는 if ($?) {} 사용",
		"파일 경로에 공백이 있으면 반드시 이중 따옴표로 감싸세요",
		"환경변수는 $env:VAR_NAME 형식으로 참조하세요",
		"멀티라인 문자열: here-string @'...'@ 사용 (단일 인용부 사용 권장)",
	}
	if ctx.Workspaces.Count > 1 {
		aliases := ctx.Workspaces.Aliases
		usageItems = append(usageItems, "다중 워크스페이스 ("+strings.Join(aliases, ", ")+"): `server` 파라미터 필수")
	}
	s.UsageNotes(usageItems...)

	s.Examples(
		"서비스 목록: command=\"Get-Service | Where-Object {$_.Status -eq 'Running'}\"",
		"파일 복사: command=\"Copy-Item -Path 'C:\\src\\file.txt' -Destination 'D:\\dst\\'\"",
		"레지스트리 읽기: command=\"Get-ItemProperty -Path 'HKLM:\\SOFTWARE\\...'\"",
	)

	tipItems := []string{
		"find/grep/cat 대신 Glob/Grep/Read를 우선 사용하세요",
		"출력이 많으면 | Select-Object -First N 또는 | Out-File로 제한하세요",
	}
	if ctx.Mode == "plan" {
		tipItems = append(tipItems, "⚠ 계획 모드: 실행 전 exit_plan_mode를 먼저 호출하세요")
	}
	s.Tips(tipItems...)

	paramItems := []string{
		"command: 실행할 PowerShell 명령 (필수)",
		"timeout: 타임아웃(ms), 기본 120000",
		"description: 명령 요약 (승인 UI 표시용)",
	}
	if ctx.Workspaces.Count > 1 {
		paramItems = append(paramItems, "server: 대상 서버 alias (다중 워크스페이스 시 필수)")
	}
	s.Parameters(paramItems...)

	return s.Build()
}
