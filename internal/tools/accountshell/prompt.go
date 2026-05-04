package accountshell

import (
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *AccountShellTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"현재 열린 계정 범위(account-scoped) 셸 세션 목록을 확인할 때",
		"불필요한 지속 셸 세션을 닫을 때",
	)

	s.WhenNotToUse(
		"새 셸 명령을 실행할 때 → bash/powershell 사용",
		"백그라운드 작업을 관리할 때 → shell_job/shell_job_control 사용",
	)

	s.UsageNotes(
		"action: list(목록 조회), close(세션 닫기) 중 하나",
		"세션은 (server, user, role) 조합으로 식별됩니다",
	)

	s.Examples(
		"세션 목록: action=\"list\"",
		"세션 닫기: action=\"close\", session_id=\"sess-123\"",
	)

	s.Parameters(
		"action: list / close (필수)",
		"session_id: 닫을 세션 ID (close 시 필수)",
	)

	return s.Build()
}
