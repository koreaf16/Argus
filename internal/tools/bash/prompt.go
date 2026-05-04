package bash

import (
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *BashTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	// When to use
	whenItems := []string{
		"셸 명령 실행이 필요한 작업 (빌드, 테스트, 패키지 설치 등)",
		"파일 시스템 조작 (복사, 이동, 삭제, 권한 변경)",
		"프로세스 시작·종료, 서비스 관리",
		"파이프라인·스크립트 실행",
		"Git 커맨드 (커밋, 브랜치, 머지 등)",
	}
	if ctx.Workspaces.Count > 1 {
		whenItems = append(whenItems, "원격 서버 명령 — `server` 파라미터로 대상 지정 필수")
	}
	s.WhenToUse(whenItems...)

	// When NOT to use
	s.WhenNotToUse(
		"파일 내용 읽기 → Read 도구 사용",
		"파일/디렉토리 탐색 → Glob 도구 사용",
		"텍스트 검색 → Grep 도구 사용",
		"파일 쓰기/편집 → FileWrite·Edit 도구 사용",
		"단순 정보 조회로 셸이 불필요한 경우",
	)

	// Usage notes
	usageItems := []string{
		"특수문자(!, &, $, 공백 포함 경로)는 작은따옴표(')로 감싸세요",
		"timeout: 기본 120초, 장시간 작업은 명시 지정",
		"비밀번호·시크릿을 명령 인라인에 직접 넣지 마세요",
		"sudo 명령 전 워크스페이스 elevation 설정이 ENABLED인지 확인하세요",
		"여러 독립 명령은 병렬 채널(role/channel)로 실행하면 효율적입니다",
	}
	if ctx.Workspaces.Count > 1 {
		aliases := ctx.Workspaces.Aliases
		usageItems = append(usageItems, "다중 워크스페이스 ("+strings.Join(aliases, ", ")+"): `server` 파라미터 필수")
	}
	s.UsageNotes(usageItems...)

	// Examples
	s.Examples(
		"패키지 설치: command=\"apt-get install -y nginx\"",
		"Git 커밋: command=\"git commit -m '메시지'\"",
		"백그라운드 실행: command=\"./server &\", background=true",
	)
	if ctx.Workspaces.Count > 1 {
		s.AppendRaw("- 원격 실행: command=\"systemctl status nginx\", server=\"prod-web\"")
	}

	// Tips
	tipItems := []string{
		"find/grep/cat/echo 대신 Glob/Grep/Read/FileWrite를 우선 사용하세요",
		"장시간 명령은 background=true + ShellJob으로 추적하세요",
		"권한 레인 진입: `sudo -i` 또는 `sudo -i -u <user>` 또는 `su - <user>` — 이후 명령이 해당 레인에서 실행됨",
		"단발성 권한 실행(레인 유지 안 함): `sudo -u <user> <body>` 사용",
		"각 (server, role, channel) 조합은 독립 채널 — cwd·환경변수가 다른 채널에 영향 안 줌",
	}
	if ctx.Mode == "plan" {
		tipItems = append(tipItems, "⚠ 계획 모드: 실행 전 exit_plan_mode를 먼저 호출하세요")
	}
	s.Tips(tipItems...)

	// Parameters
	paramItems := []string{
		"command: 실행할 셸 명령 (필수)",
		"timeout: 타임아웃(ms), 기본 120000",
		"background: true이면 비동기 실행 (선택)",
		"description: 명령 요약 (승인 UI 표시용)",
	}
	if ctx.Workspaces.Count > 1 {
		paramItems = append(paramItems, "server: 대상 서버 alias (다중 워크스페이스 시 필수)")
		paramItems = append(paramItems, "role: 권한 역할 (선택)")
		paramItems = append(paramItems, "channel: 격리 채널 이름 (선택)")
	}
	s.Parameters(paramItems...)

	return s.Build()
}
