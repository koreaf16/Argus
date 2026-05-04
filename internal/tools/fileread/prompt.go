package fileread

const fileReadPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 로컬 파일 시스템의 파일을 읽어야 할 때\n" +
	"- 코드, 설정, 로그, 이미지, PDF 등 모든 파일 형식\n" +
	"- 스크린샷·임시 파일 경로가 주어졌을 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 디렉터리 목록을 보려면 → Bash ls 또는 FSList 사용\n" +
	"- 파일 내용을 검색하려면 → Grep 사용\n" +
	"- cat/head/tail/sed/awk Bash 명령어로 파일을 읽는 것은 금지 — 이 도구를 사용하세요\n" +
	"\n## 사용 노트\n" +
	"- file_path는 절대 경로를 사용해야 합니다 (상대 경로 불가)\n" +
	"- 기본적으로 파일 시작부터 최대 2000줄을 읽습니다\n" +
	"- 긴 파일은 offset·limit 파라미터로 필요한 부분만 읽으세요\n" +
	"- 이미지 파일(PNG, JPG 등)은 시각적으로 표시됩니다\n" +
	"- PDF 파일: 10페이지 초과 시 pages 파라미터로 범위 지정 필수 (최대 20페이지/요청)\n" +
	"- Jupyter 노트북(.ipynb)은 모든 셀과 출력을 통합해 반환됩니다\n" +
	"- 존재하지 않는 파일을 읽어도 괜찮습니다 — 오류가 반환됩니다\n" +
	"- 빈 파일은 빈 내용 경고가 표시됩니다\n" +
	"\n## 예시\n" +
	"1. 전체 파일 읽기: file_path=\"/home/user/project/main.go\"\n" +
	"2. 대용량 파일 일부만 읽기: file_path=\"...\", offset=100, limit=50\n" +
	"3. PDF 특정 페이지: file_path=\"...\", pages=\"3-7\"\n" +
	"\n## 팁\n" +
	"- 같은 파일을 여러 번 읽지 마세요. 대화 내 이전 Read 결과를 참조하세요.\n" +
	"- 어느 부분이 필요한지 알고 있으면 offset/limit으로 불필요한 토큰 낭비를 줄이세요\n" +
	"\n## 매개변수\n" +
	"- file_path: 읽을 파일의 절대 경로 (필수)\n" +
	"- offset: 읽기 시작 줄 번호 (선택, 기본: 0)\n" +
	"- limit: 읽을 최대 줄 수 (선택, 기본: 2000)\n" +
	"- pages: PDF 페이지 범위 (선택, 예: \"1-5\", PDF 전용)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *FileReadTool) GetStaticSystemPromptGuide() string { return fileReadPrompt }
