package serverinspect

import (
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *ServerInspectTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"등록된 워크스페이스·서버의 연결 상태를 확인할 때",
		"서버 정보(호스트, 플랫폼, 활성 레인 수)를 조회할 때",
		"연결 문제를 진단할 때",
	)

	s.WhenNotToUse(
		"실제 서버 명령을 실행할 때 → bash 사용",
		"파일을 전송할 때 → server_copy 사용",
	)

	s.UsageNotes(
		"server를 지정하지 않으면 모든 등록된 워크스페이스를 반환합니다",
		"연결 상태(connected/disconnected)와 플랫폼 정보를 포함합니다",
	)

	s.Examples(
		"전체 목록: (server 생략)",
		"특정 서버: server=\"prod\"",
	)

	s.Parameters(
		"server: 조회할 서버 alias (선택, 생략 시 전체)",
	)

	return s.Build()
}
