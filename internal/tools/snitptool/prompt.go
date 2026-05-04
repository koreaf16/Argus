package snitptool

const snipPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 파일에서 특정 줄 범위만 추출해야 할 때\n" +
	"- 대용량 파일에서 필요한 코드 블록만 빠르게 확인할 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 파일 전체를 읽어야 할 때 → Read 사용\n" +
	"- 내용으로 위치를 검색할 때 → Grep 사용\n" +
	"\n## 사용 노트\n" +
	"- file_path는 절대 경로를 사용하세요\n" +
	"- start_line과 end_line은 1-based 줄 번호입니다\n" +
	"\n## 예시\n" +
	"1. 함수 본체만 추출: file_path=\"/proj/main.go\", start_line=42, end_line=80\n" +
	"\n## 매개변수\n" +
	"- file_path: 대상 파일의 절대 경로 (필수)\n" +
	"- start_line: 시작 줄 번호 (1-based, 필수)\n" +
	"- end_line: 종료 줄 번호 (1-based, 필수)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *SnipTool) GetStaticSystemPromptGuide() string { return snipPrompt }
