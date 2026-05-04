// Package enterplanmode — EnterPlanMode prompt.
package enterplanmode

const enterPlanModePrompt = "## 언제 이 도구를 쓰는가\n" +
	"- 여러 파일을 수정하거나 여러 단계가 필요한 복잡한 작업\n" +
	"- 아키텍처 결정이나 큰 리팩터링이 포함된 경우\n" +
	"- 실행 전 사용자의 명시적 승인이 필요한 경우\n" +
	"- 잘못 실행하면 되돌리기 어려운 변경이 수반되는 경우\n" +
	"- 사용자가 \"계획\", \"플랜\", \"설계\" 등 계획 수립을 명시적으로 요청한 경우\n" +
	"\n## 언제 쓰지 않는가\n" +
	"- 한 번의 명령이나 파일 수정으로 해결되는 단순한 작업 → 바로 실행\n" +
	"- 정보 조회, 파일 읽기, 검색 등 읽기 전용 작업\n" +
	"- 이미 계획 모드인 상태에서 중첩 호출 (불필요)\n" +
	"- 사용자가 \"그냥 해줘\", \"바로 해줘\"라고 명확히 요청한 경우\n" +
	"\n## 사용 노트\n" +
	"- 계획 모드 진입 후 LLM은 마크다운 계획 파일을 작성해야 합니다\n" +
	"- 계획 파일 경로: ~/.claude/plans/<slug>.md\n" +
	"- 계획에는 단계별 목록, 수정할 파일, 예상 결과를 포함하세요\n" +
	"- 계획 완성 후 반드시 exit_plan_mode를 호출해 사용자 승인을 받으세요\n" +
	"- 승인 없이 계획을 실행하지 마세요\n" +
	"\n## 좋은 계획 작성 팁\n" +
	"- 각 단계를 구체적으로 작성 (\"파일 수정\" 대신 \"X 파일의 Y 함수에 Z 추가\")\n" +
	"- 순서 의존성이 있는 단계는 명시적으로 표시\n" +
	"- 부작용이나 롤백 방법도 함께 기술\n" +
	"- 불필요한 추상화 없이 실제로 할 작업만 포함\n" +
	"\n## 매개변수\n" +
	"- 없음 (파라미터 불필요)"

// GetStaticSystemPromptGuide implements tools.StaticSystemPromptContributor.
func (t *EnterPlanModeTool) GetStaticSystemPromptGuide() string { return enterPlanModePrompt }
