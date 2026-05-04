package fslist

const fsListPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 특정 디렉터리의 파일·폴더 목록을 빠르게 확인할 때\n" +
	"- 디렉터리 구조를 파악할 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 파일 내용을 읽으려면 → Read 사용\n" +
	"- 파일 패턴으로 검색하려면 → Glob 사용\n" +
	"\n## 사용 노트\n" +
	"- path는 절대 경로를 사용하세요\n" +
	"- 파일 크기, 수정 시간 등 메타데이터도 함께 반환됩니다\n" +
	"\n## 예시\n" +
	"1. 현재 디렉터리 목록: path=\"/project\"\n" +
	"2. 특정 디렉터리: path=\"/project/internal/tools\"\n" +
	"\n## 매개변수\n" +
	"- path: 목록을 가져올 디렉터리의 절대 경로 (필수)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *FSListTool) GetStaticSystemPromptGuide() string { return fsListPrompt }
