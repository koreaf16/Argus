package servertunnel

import (
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *ServerTunnelTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"원격 서버의 포트를 로컬로 포워딩해야 할 때 (SSH 터널)",
		"원격 DB·웹 서비스에 로컬에서 직접 접근할 때",
		"활성 터널 목록을 확인하거나 터널을 닫을 때",
	)

	s.WhenNotToUse(
		"서버에 명령만 실행할 때 → bash 사용",
		"파일만 전송할 때 → server_copy 사용",
	)

	s.UsageNotes(
		"action: open(열기), close(닫기), list(목록) 중 하나",
		"open 시: server, remote_port, local_port 필수",
		"터널은 세션 종료 시 자동으로 닫힙니다",
	)

	s.Examples(
		"터널 열기: action=\"open\", server=\"prod-db\", remote_port=5432, local_port=15432",
		"목록 조회: action=\"list\"",
		"터널 닫기: action=\"close\", tunnel_id=\"t-123\"",
	)

	s.Parameters(
		"action: open / close / list (필수)",
		"server: 대상 서버 alias (open 시 필수)",
		"remote_port: 원격 포트 (open 시 필수)",
		"local_port: 로컬 바인딩 포트 (open 시 필수)",
		"tunnel_id: 닫을 터널 ID (close 시 필수)",
	)

	return s.Build()
}
