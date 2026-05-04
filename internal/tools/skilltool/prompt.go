package skilltool

const skillPrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 등록된 스킬을 실행해야 할 때\n" +
	"- 사용자가 `/skillname` 형식으로 슬래시 커맨드를 호출했을 때\n" +
	"- 특정 작업 흐름을 자동화한 스킬을 실행해야 할 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 스킬 이름을 모를 때 → tool_search로 먼저 검색\n" +
	"- 일반 도구 호출로 충분한 경우 (bash, glob 등 직접 호출)\n" +
	"\n## 사용 노트\n" +
	"- `name` 필드는 스킬 이름 (슬래시 없이)\n" +
	"- `args`는 선택 사항이며 스킬에 따라 의미가 다릅니다\n" +
	"- 존재하지 않는 스킬 이름은 오류를 반환합니다\n" +
	"\n## 예시\n" +
	"1. 스킬 실행: name=\"deploy\"\n" +
	"2. 인자와 함께: name=\"test\", args=[\"--verbose\"]\n" +
	"\n## 팁\n" +
	"- 사용 가능한 스킬 목록은 `tool_search`로 확인하세요\n" +
	"\n## 매개변수\n" +
	"- name: 실행할 스킬 이름 (필수)\n" +
	"- args: 스킬에 전달할 인자 목록 (선택)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *SkillTool) GetStaticSystemPromptGuide() string { return skillPrompt }
