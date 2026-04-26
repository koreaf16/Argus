// Package constants — 시스템 프롬프트 접두사·어트리뷰션 관련 상수.
//
// 파일 역할: API 요청 시 시스템 프롬프트 앞에 붙는 접두사 문자열을 정의한다.
//
//	비대화형(non-interactive) 모드 여부에 따라 접두사를 다르게 선택한다.
//
// 포함 모듈:
//   - CLISyspromptPrefixDefault: 기본 접두사.
//   - CLISyspromptPrefixAgentSDKPreset: Agent SDK Claude Code 프리셋 접두사.
//   - CLISyspromptPrefixAgentSDK: 일반 Agent SDK 접두사.
//   - CLISyspromptPrefixes: 모든 접두사 집합 (역방향 파싱용).
//   - GetCLISyspromptPrefix(): 모드에 따라 접두사를 선택하여 반환.
//
// 호출/사용 방식:
//   - internal/query/context.go 의 시스템 프롬프트 조립 시 참조.
//
// 연결:
//   - 원본: src/constants/system.ts
package constants

// 시스템 프롬프트 접두사 상수.
const (
	CLISyspromptPrefixDefault        = "You are Argus, an interactive CLI AI assistant."
	CLISyspromptPrefixAgentSDKPreset = "You are Argus, an interactive CLI AI assistant, running within the Agent SDK."
	CLISyspromptPrefixAgentSDK       = "You are a Claude agent, built on Anthropic's Claude Agent SDK."
)

// CLISyspromptPrefixes 는 모든 접두사 집합이다. 역방향 파싱 시 접두사 블록 식별에 사용.
var CLISyspromptPrefixes = map[string]bool{
	CLISyspromptPrefixDefault:        true,
	CLISyspromptPrefixAgentSDKPreset: true,
	CLISyspromptPrefixAgentSDK:       true,
}

// GetCLISyspromptPrefix 는 실행 모드에 따라 적절한 시스템 프롬프트 접두사를 반환한다.
func GetCLISyspromptPrefix(isNonInteractive, hasAppendSystemPrompt bool) string {
	if isNonInteractive {
		if hasAppendSystemPrompt {
			return CLISyspromptPrefixAgentSDKPreset
		}
		return CLISyspromptPrefixAgentSDK
	}
	return CLISyspromptPrefixDefault
}
