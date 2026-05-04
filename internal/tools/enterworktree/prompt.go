package enterworktree

import (
	tools "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *EnterWorktreeTool) GetSystemPromptGuide(ctx tools.Context) string {
	s := promptkit.New()

	whenItems := []string{
		"기존 코드베이스와 격리된 환경에서 실험적 작업이 필요할 때",
		"같은 저장소의 여러 브랜치를 동시에 작업할 때",
		"PR 검토나 버그 재현을 위해 임시 환경이 필요할 때",
	}
	if ctx.Mode == "plan" {
		whenItems = append(whenItems, "계획 단계에서 격리 작업 환경을 미리 설정할 때")
	}
	s.WhenToUse(whenItems...)

	s.WhenNotToUse(
		"현재 디렉토리에서 바로 작업이 가능한 경우",
		"일회성 파일 수정 — 워크트리 없이 직접 편집",
	)

	s.UsageNotes(
		"워크트리는 git 저장소 외부에 체크아웃된 별도의 작업 디렉토리입니다",
		"enter 후 exit_worktree로 반드시 종료하세요",
		"워크트리 내 변경은 독립적이므로 메인 트리에 영향을 주지 않습니다",
	)

	s.Examples(
		"기본 워크트리 진입: (파라미터 없이)",
		"브랜치 지정: branch=\"feature/new-auth\"",
	)

	s.Tips(
		"작업 완료 후 반드시 exit_worktree를 호출해 정리하세요",
	)

	s.Parameters(
		"branch: 체크아웃할 브랜치 이름 (선택)",
		"path: 워크트리 경로 (선택, 자동 생성됨)",
	)

	return s.Build()
}
