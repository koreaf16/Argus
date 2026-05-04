package shelljob

import (
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *ShellJobTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"백그라운드로 실행 중인 셸 작업의 상태를 확인할 때",
		"bash background=true로 시작한 작업의 출력을 읽을 때",
		"실행 중인 작업 목록을 조회할 때",
	)

	s.WhenNotToUse(
		"작업을 새로 실행할 때 → bash background=true 사용",
		"작업을 중지하거나 입력을 보낼 때 → shell_job_control 사용",
	)

	s.UsageNotes(
		"action: list(전체 목록), status(특정 작업 상태), tail(출력 조회) 중 하나",
		"job_id는 bash 도구가 background=true로 실행 시 반환하는 ID입니다",
		"tail action은 최근 출력 라인을 반환합니다",
	)

	s.Examples(
		"작업 목록: action=\"list\"",
		"상태 확인: action=\"status\", job_id=\"job-abc\"",
		"출력 조회: action=\"tail\", job_id=\"job-abc\", lines=50",
	)

	s.Parameters(
		"action: list / status / tail (필수)",
		"job_id: 대상 작업 ID (status/tail 시 필수)",
		"lines: tail 시 반환할 라인 수, 기본값 100 (선택)",
	)

	return s.Build()
}
