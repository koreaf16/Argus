package connectortool

import (
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *ConnectorSuggestTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	whenItems := []string{
		"현재 없는 특수 도구(DB 커넥터, Kubernetes, GitHub 등)가 필요할 때",
		"사용자가 특정 서비스 연동 MCP 커넥터를 요청한 경우",
	}
	if !ctx.MCPEnabled {
		whenItems = append(whenItems, "⚠ 현재 MCP가 비활성화 상태입니다 — MCP를 먼저 활성화하세요")
	}
	s.WhenToUse(whenItems...)

	s.WhenNotToUse(
		"이미 설치된 MCP 도구가 있는 경우 → 해당 도구를 직접 사용",
		"bash/powershell로 충분히 처리 가능한 경우",
	)

	s.UsageNotes(
		"query로 필요한 기능을 설명하면 레지스트리에서 후보를 검색합니다",
		"install=true이면 선택한 커넥터를 자동 설치합니다",
		"설치 후 MCP 서버가 재시작됩니다",
	)

	s.Examples(
		"PostgreSQL 커넥터 검색: query=\"PostgreSQL database connector\"",
		"Kubernetes 커넥터 설치: query=\"kubernetes\", install=true",
	)

	s.Parameters(
		"query: 필요한 커넥터 설명 (필수)",
		"install: 선택한 커넥터를 설치할지 여부, 기본값 false (선택)",
	)

	return s.Build()
}
