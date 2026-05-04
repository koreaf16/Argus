package filewrite

const fileWritePrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 새 파일을 생성할 때\n" +
	"- 파일 전체를 새 내용으로 완전히 교체할 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 기존 파일의 일부만 수정할 때 → Edit 도구 사용 (diff만 전송, 훨씬 효율적)\n" +
	"- echo, cat <<EOF 등 Bash 명령어로 파일을 쓰는 것은 금지 — 이 도구를 사용하세요\n" +
	"- 문서(*.md) 또는 README 파일은 사용자가 명시적으로 요청한 경우에만 생성\n" +
	"\n## 사용 노트\n" +
	"- 기존 파일이 있으면 덮어씁니다\n" +
	"- 기존 파일을 수정하는 경우, 반드시 먼저 Read 도구로 현재 내용을 읽으세요\n" +
	"  (Read 없이 쓰면 도구가 실패합니다)\n" +
	"- 새 파일 생성에만 이 도구를 사용하세요. 부분 수정은 Edit을 사용하세요.\n" +
	"- 사용자가 명시적으로 요청하지 않는 한 이모지를 파일에 작성하지 마세요\n" +
	"\n## 예시\n" +
	"1. 새 Go 파일 생성: file_path=\"/project/handler.go\", content=\"package main...\"\n" +
	"2. 설정 파일 생성: file_path=\"/project/.env.example\"\n" +
	"\n## 팁\n" +
	"- 기존 파일 수정 시 Edit을 먼저 고려하세요 — 토큰을 절약하고 실수를 줄입니다\n" +
	"- 대용량 파일 전체 교체가 필요한 경우에만 Write를 사용하세요\n" +
	"\n## 매개변수\n" +
	"- file_path: 쓸 파일의 절대 경로 (필수)\n" +
	"- content: 파일에 쓸 내용 (필수)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *FileWriteTool) GetStaticSystemPromptGuide() string { return fileWritePrompt }
