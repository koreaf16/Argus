package mcpauthtool

const mcpAuthPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- MCP 서버에 인증이 필요할 때\n" +
	"- OAuth 또는 API 키 기반 MCP 서버에 연결하기 전\n" +
	"\n## 사용 노트\n" +
	"- 인증이 필요 없는 MCP 서버에는 사용하지 마세요\n" +
	"- 인증 자격증명은 세션 내에서만 유효합니다\n" +
	"\n## 매개변수\n" +
	"- server: 인증할 MCP 서버 이름 (필수)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *McpAuthTool) GetStaticSystemPromptGuide() string { return mcpAuthPrompt }
