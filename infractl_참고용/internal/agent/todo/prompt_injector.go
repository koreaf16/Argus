// Package todo
// File: prompt_injector.go
// Description: 사용자 입력 기반 다단계 작업 감지 후 system prompt 에 TodoWrite 사용 지시 주입
// Responsibility: DetectMultiStep + system prompt 한 줄 주입

package todo

const todoPromptHint = "\n\n[시스템] 다단계 작업이 감지되었습니다. 작업을 시작하기 전에 반드시 TodoWrite 도구로 단계를 먼저 작성하세요."

// InjectIfNeeded 는 userInput 에 다단계 신호어가 있으면 basePrompt 에 hint 를 추가한 문자열을 반환한다.
// 신호어가 없으면 basePrompt 를 그대로 반환한다.
func InjectIfNeeded(userInput, basePrompt string) string {
	if !DetectMultiStep(userInput) {
		return basePrompt
	}
	return basePrompt + todoPromptHint
}
