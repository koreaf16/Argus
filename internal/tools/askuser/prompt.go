package askuser

const askUserPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 작업 진행에 필수적인 정보를 사용자에게 물어봐야 할 때\n" +
	"- 여러 선택지 중 사용자의 선호를 확인해야 할 때\n" +
	"- 예/아니요 확인이 필요한 중요한 결정을 내려야 할 때\n" +
	"- 파일명, 서버 주소 등 LLM이 추론할 수 없는 정보가 필요할 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 코드나 컨텍스트에서 추론할 수 있는 정보를 물어볼 때 → 직접 추론\n" +
	"- 작업과 무관한 호기심 질문 → 하지 마세요\n" +
	"- 이미 대화에서 명확히 밝혀진 정보를 재확인할 때\n" +
	"- YOLO 모드에서는 가능하면 LLM이 스스로 결정합니다\n" +
	"\n## 사용 노트\n" +
	"- 단일 질문: `question`, `type`, `placeholder` 사용\n" +
	"- 다중 질문: `questions` 배열로 한 번에 전달 (여러 번 호출보다 효율적)\n" +
	"- type: text(자유 입력) / choice(선택지) / yesno(예/아니요)\n" +
	"- `required: true`이면 빈 답변 시 재입력을 요청합니다\n" +
	"\n## 예시\n" +
	"1. 텍스트 입력: question=\"배포 환경을 입력하세요\", type=\"text\"\n" +
	"2. 선택지: type=\"choice\", options=[{value:\"prod\"},{value:\"staging\"}]\n" +
	"3. 예/아니요: question=\"계속 진행할까요?\", type=\"yesno\"\n" +
	"4. 다중 질문: questions=[{question:\"환경?\"},{question:\"버전?\"}]\n" +
	"\n## 팁\n" +
	"- 질문은 구체적이고 간결하게 작성하세요\n" +
	"- 기본값(`default`)을 제공하면 사용자가 빠르게 진행할 수 있습니다\n" +
	"\n## 매개변수\n" +
	"- question: 단일 질문 내용 (단일 질문 시 필수)\n" +
	"- type: 질문 유형 — text / choice / yesno (선택, 기본값: text)\n" +
	"- options: 선택지 목록 (type=choice 시 필수)\n" +
	"- default: 기본값 (선택)\n" +
	"- required: 필수 답변 여부, 기본값 true (선택)\n" +
	"- questions: 다중 질문 배열 (다중 질문 시 사용)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *AskUserTool) GetStaticSystemPromptGuide() string { return askUserPrompt }
