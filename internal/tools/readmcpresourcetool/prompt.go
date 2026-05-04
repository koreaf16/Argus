package readmcpresourcetool

const readMcpResourcePrompt = "## 언제 이 도구를 쓰는가\n" +
	"- MCP 서버의 특정 리소스 내용을 읽어야 할 때\n" +
	"- ListMcpResourcesTool로 URI를 확인한 뒤 상세 내용을 가져올 때\n" +
	"\n## 사용 노트\n" +
	"- server와 uri 모두 필수입니다\n" +
	"- URI는 ListMcpResourcesTool 결과에서 확인하세요\n" +
	"\n## 예시\n" +
	"1. 서버에서 리소스 읽기: server=\"myserver\", uri=\"resource://my-resource\"\n" +
	"\n## 매개변수\n" +
	"- server: MCP 서버 이름 (필수)\n" +
	"- uri: 읽을 리소스의 URI (필수)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *ReadMcpResourceTool) GetStaticSystemPromptGuide() string { return readMcpResourcePrompt }
