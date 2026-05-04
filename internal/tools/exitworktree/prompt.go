package exitworktree

import (
	tools "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *ExitWorktreeTool) GetSystemPromptGuide(ctx tools.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"enter_worktree로 진입한 워크트리 환경을 종료할 때",
		"격리 작업이 완료되어 메인 디렉토리로 돌아올 때",
	)

	s.WhenNotToUse(
		"워크트리 환경에 있지 않은 경우 (오류 반환)",
	)

	s.UsageNotes(
		"변경 사항을 커밋하지 않으면 워크트리 삭제 시 손실됩니다",
		"cleanup=true이면 워크트리 디렉토리를 삭제합니다",
	)

	s.Examples(
		"종료(유지): (파라미터 없이)",
		"종료 및 삭제: cleanup=true",
	)

	s.Parameters(
		"cleanup: 워크트리 디렉토리 삭제 여부, 기본값 false (선택)",
	)

	return s.Build()
}
