// Package context
// File: system_prompt.go
// Description: 정적 시스템 프롬프트 조각(Part) 정의 및 수집 인터페이스
// Responsibility: PromptPart 타입 정의, 각 섹션 수집 함수 선언

package context

// PromptPart 는 시스템 프롬프트의 한 섹션이다.
type PromptPart struct {
	Key     string
	Content string
}

// StaticParts 는 변경이 드문 정적 프롬프트 조각 목록을 반환한다.
// 실제 콘텐츠는 호출자(agent.BuildContextual 등)가 주입한다.
func StaticParts(userMD, envBlock string) []PromptPart {
	return []PromptPart{
		{Key: "user_context", Content: userMD},
		{Key: "env_info", Content: envBlock},
	}
}
