package lsptool

const lspPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 함수·클래스·변수의 정의 위치를 찾을 때 (goToDefinition)\n" +
	"- 심볼의 모든 참조를 찾을 때 (findReferences)\n" +
	"- 타입 정보·문서를 확인할 때 (hover)\n" +
	"- 파일 내 모든 심볼을 나열할 때 (documentSymbol)\n" +
	"- 워크스페이스 전체에서 심볼을 검색할 때 (workspaceSymbol)\n" +
	"- 인터페이스 구현체를 찾을 때 (goToImplementation)\n" +
	"- 함수 호출 계층을 탐색할 때 (prepareCallHierarchy, incomingCalls, outgoingCalls)\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 단순 텍스트 검색 → Grep 사용 (LSP 서버 불필요, 더 빠름)\n" +
	"- 파일 존재 여부 확인 → Glob 사용\n" +
	"\n## 사용 노트\n" +
	"- 모든 작업에 filePath, line(1-based), character(1-based)가 필요합니다\n" +
	"- 해당 파일 타입에 맞는 LSP 서버가 설정돼 있어야 합니다\n" +
	"- LSP 서버가 없으면 오류가 반환됩니다\n" +
	"\n## 예시\n" +
	"1. 정의로 이동: operation=\"goToDefinition\", filePath=\"/proj/main.go\", line=42, character=15\n" +
	"2. 참조 찾기: operation=\"findReferences\", filePath=\"/proj/types.go\", line=10, character=8\n" +
	"3. 워크스페이스 심볼 검색: operation=\"workspaceSymbol\", query=\"UserHandler\"\n" +
	"\n## 매개변수\n" +
	"- operation: 수행할 LSP 작업 (필수)\n" +
	"- filePath: 대상 파일의 절대 경로 (필수)\n" +
	"- line: 줄 번호 (1-based, 필수)\n" +
	"- character: 문자 오프셋 (1-based, 필수)\n" +
	"- query: 워크스페이스 심볼 검색어 (workspaceSymbol 전용)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *LSPTool) GetStaticSystemPromptGuide() string { return lspPrompt }
