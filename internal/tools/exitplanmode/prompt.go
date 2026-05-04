// Package exitplanmode — ExitPlanMode prompt.
package exitplanmode

const exitPlanModePrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 계획 파일 작성이 완료되고 사용자 검토/승인을 받을 준비가 됐을 때\n" +
	"- 계획 수정 후 재승인을 요청할 때\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 계획이 아직 미완성인 경우 → 계획을 먼저 완성하세요\n" +
	"- 계획 모드가 아닌 상태에서 호출 (오류 반환)\n" +
	"\n## 사용 노트\n" +
	"- 이 도구를 호출하면 작성한 계획 파일 내용이 사용자에게 표시됩니다\n" +
	"- 사용자가 승인하면 계획 실행 단계로 진입합니다\n" +
	"- 사용자가 거부하거나 수정을 요청하면 계획 모드로 다시 돌아갑니다\n" +
	"- `plan_file_path`에 enter_plan_mode로 작성한 계획 파일 경로를 전달하세요\n" +
	"\n## 흐름\n" +
	"1. enter_plan_mode 호출\n" +
	"2. 계획 파일 작성 (~/.claude/plans/<slug>.md)\n" +
	"3. exit_plan_mode 호출 → 사용자 승인 대기\n" +
	"4. 승인 시 실행 / 거부 시 계획 수정 반복\n" +
	"\n## 매개변수\n" +
	"- plan_file_path: 작성한 계획 파일의 전체 경로 (필수)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *ExitPlanModeTool) GetStaticSystemPromptGuide() string { return exitPlanModePrompt }
