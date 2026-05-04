package shelljobcontrol

import (
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *ShellJobControlTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"실행 중인 백그라운드 작업을 중지해야 할 때",
		"대화형 프로세스에 입력을 보내야 할 때",
		"작업이 응답하지 않거나 종료해야 할 때",
	)

	s.WhenNotToUse(
		"작업 상태를 읽기만 할 때 → shell_job 사용",
		"새 작업을 실행할 때 → bash 사용",
	)

	s.UsageNotes(
		"action: stop(중지), send_input(입력 전송) 중 하나",
		"stop은 SIGTERM을 먼저 보내고 타임아웃 후 SIGKILL을 보냅니다",
		"send_input은 대화형 프로세스(Python REPL, 쉘 등)에 텍스트를 전송합니다",
	)

	s.Examples(
		"작업 중지: action=\"stop\", job_id=\"job-abc\"",
		"입력 전송: action=\"send_input\", job_id=\"job-abc\", input=\"yes\\n\"",
	)

	s.Parameters(
		"action: stop / send_input (필수)",
		"job_id: 대상 작업 ID (필수)",
		"input: 전송할 입력 텍스트 (send_input 시 필수)",
	)

	return s.Build()
}
