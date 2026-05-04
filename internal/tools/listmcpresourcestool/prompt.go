package listmcpresourcestool

const listMcpResourcesPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 설정된 MCP 서버에서 사용 가능한 리소스 목록을 확인할 때\n" +
	"- 특정 서버의 리소스를 탐색할 때\n" +
	"\n## 사용 노트\n" +
	"- server 파라미터를 지정하면 해당 서버의 리소스만 반환됩니다\n" +
	"- 반환된 각 리소스 객체에는 server 필드가 포함됩니다\n" +
	"\n## 예시\n" +
	"1. 모든 서버의 리소스 목록: server 파라미터 생략\n" +
	"2. 특정 서버 리소스: server=\"myserver\"\n" +
	"\n## 매개변수\n" +
	"- server: 리소스를 가져올 특정 MCP 서버 이름 (선택)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *ListMcpResourcesTool) GetStaticSystemPromptGuide() string { return listMcpResourcesPrompt }
