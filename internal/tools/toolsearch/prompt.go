package toolsearch

const toolSearchPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 어떤 도구를 써야 할지 불확실할 때\n" +
	"- 특정 작업(파일 탐색, 서버 연결 등)에 맞는 도구를 찾을 때\n" +
	"- 증거 카테고리(local_code, server_state 등)로 도구를 필터링할 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 이미 어떤 도구를 써야 할지 알고 있을 때 → 해당 도구를 직접 호출\n" +
	"- 단순히 사용 가능한 모든 도구 목록이 필요한 경우 → 시스템 프롬프트 참조\n" +
	"\n## 사용 노트\n" +
	"- `query`는 원하는 작업이나 기능을 자연어로 설명\n" +
	"- `evidence_categories`로 필터링하면 더 정확한 결과\n" +
	"- 기본 반환 수: 8개, 최대: 20개\n" +
	"\n## 예시\n" +
	"1. 파일 검색 도구 찾기: query=\"파일에서 텍스트 검색\"\n" +
	"2. 카테고리 필터: query=\"서버 상태 확인\", evidence_categories=[\"server_state\"]\n" +
	"\n## 팁\n" +
	"- 이 도구는 도구를 찾기만 할 뿐 실행하지 않습니다\n" +
	"- 찾은 도구는 name 필드로 직접 호출하세요\n" +
	"\n## 매개변수\n" +
	"- query: 찾고자 하는 작업이나 기능 설명 (선택)\n" +
	"- evidence_categories: 필터링할 증거 카테고리 목록 (선택)\n" +
	"- limit: 반환할 최대 도구 수, 기본값 8 (선택)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *Tool) GetStaticSystemPromptGuide() string { return toolSearchPrompt }
