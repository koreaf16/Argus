package glob

const globPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 파일 이름 패턴으로 파일을 찾아야 할 때 (예: 특정 확장자, 디렉터리 내 모든 파일)\n" +
	"- 수정 시간 순 정렬이 필요할 때\n" +
	"- 여러 디렉터리에 걸쳐 특정 패턴의 파일을 나열할 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 파일 내용을 검색할 때 → Grep 사용\n" +
	"- 여러 번 Glob·Grep 왕복이 필요한 오픈형 탐색 → Agent 도구 사용\n" +
	"- 이미 경로를 알고 있는 단일 파일 읽기 → Read 사용\n" +
	"\n## 사용 노트\n" +
	"- ALWAYS use Glob (NOT find 또는 ls 명령어). Glob는 권한·접근이 최적화돼 있습니다.\n" +
	"- \"**/*.ts\" 같은 재귀 패턴을 지원합니다\n" +
	"- 결과는 최신 수정 시간 순으로 정렬돼 반환됩니다\n" +
	"- path 파라미터로 검색 기준 디렉터리를 좁힐 수 있습니다\n" +
	"\n## 예시\n" +
	"1. 모든 Go 파일: pattern=\"**/*.go\"\n" +
	"2. 특정 디렉터리 내 TypeScript 파일: pattern=\"**/*.ts\", path=\"src/\"\n" +
	"3. 최근 수정된 설정 파일: pattern=\"**/*.json\"\n" +
	"\n## 팁\n" +
	"- 결과가 너무 많으면 path를 좁히거나 패턴을 구체화하세요\n" +
	"- 파일이 존재하는지 확인하는 빠른 방법으로도 활용 가능합니다\n" +
	"\n## 매개변수\n" +
	"- pattern: glob 패턴 문자열 (필수). 예: \"**/*.go\"\n" +
	"- path: 검색 기준 디렉터리 (선택). 생략 시 현재 작업 디렉터리"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *GlobTool) GetStaticSystemPromptGuide() string { return globPrompt }
