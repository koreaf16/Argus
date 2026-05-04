package grep

const grepPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 파일 내용에서 특정 텍스트·패턴을 검색할 때\n" +
	"- 함수 정의, 변수 사용처, 오류 메시지 등 코드 심볼을 추적할 때\n" +
	"- 특정 확장자 또는 디렉터리 내에서 패턴을 찾을 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 파일 이름으로 검색할 때 → Glob 사용\n" +
	"- grep 또는 rg를 Bash 명령어로 직접 실행하는 것은 금지 — 이 도구를 사용하세요\n" +
	"- 여러 번 검색·탐색이 필요한 오픈형 작업 → Agent 도구 사용\n" +
	"\n## 사용 노트\n" +
	"- ALWAYS use Grep (NOT grep 또는 rg Bash 명령어). 이 도구는 권한·접근이 최적화돼 있습니다.\n" +
	"- 전체 정규식 문법 지원 (예: \"log.*Error\", \"function\\\\s+\\\\w+\")\n" +
	"- glob 파라미터로 파일 필터링: \"*.go\", \"**/*.tsx\"\n" +
	"- type 파라미터로 파일 타입 필터링: \"go\", \"ts\", \"python\"\n" +
	"- output_mode: \"files_with_matches\" (기본, 경로만), \"content\" (매칭 줄), \"count\" (개수)\n" +
	"- Go 코드에서 interface{} 같은 리터럴 중괄호는 interface\\\\{\\\\} 로 이스케이프 필요\n" +
	"- 기본적으로 단일 줄 매칭. 여러 줄 패턴은 multiline: true 사용\n" +
	"\n## 예시\n" +
	"1. 함수 정의 찾기: pattern=\"func GetUser\", type=\"go\"\n" +
	"2. 오류 메시지 찾기: pattern=\"connection refused\", output_mode=\"content\"\n" +
	"3. 특정 디렉터리 검색: pattern=\"TODO\", glob=\"**/*.ts\", path=\"src/\"\n" +
	"\n## 팁\n" +
	"- context 파라미터로 매칭 줄 전후 N줄을 함께 표시할 수 있습니다\n" +
	"- 파일 수가 많으면 path 또는 glob으로 먼저 범위를 좁히세요\n" +
	"\n## 매개변수\n" +
	"- pattern: 검색할 정규식 패턴 (필수)\n" +
	"- path: 검색 디렉터리 또는 파일 경로 (선택)\n" +
	"- glob: 파일 필터 glob 패턴 (선택)\n" +
	"- type: 파일 타입 필터 (선택)\n" +
	"- output_mode: \"files_with_matches\" | \"content\" | \"count\" (기본: files_with_matches)\n" +
	"- -i: 대소문자 무시 (선택)\n" +
	"- multiline: 여러 줄 매칭 활성화 (선택)\n" +
	"- context: 전후 표시 줄 수 (선택)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *GrepTool) GetStaticSystemPromptGuide() string { return grepPrompt }
