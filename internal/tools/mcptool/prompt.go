package mcptool

const mcpToolPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- MCP 서버가 제공하는 외부 도구를 호출해야 할 때\n" +
	"- 각 MCP 도구의 용도는 도구 이름과 description을 참조하세요\n" +
	"\n## 사용 노트\n" +
	"- MCP 도구는 활성화된 서버에 따라 동적으로 등록됩니다\n" +
	"- 도구 이름 앞에 서버 이름이 붙을 수 있습니다 (예: server__toolname)\n" +
	"- MCPEnabled가 false이면 이 도구는 사용 불가입니다\n" +
	"\n## 매개변수\n" +
	"- 각 MCP 도구별로 파라미터가 다릅니다. 해당 도구의 description을 확인하세요."

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *MCPTool) GetStaticSystemPromptGuide() string { return mcpToolPrompt }
