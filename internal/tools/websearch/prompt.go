// Package websearch — prompt/description strings for the web search tool.
//
// 파일 역할: tool description과 모델용 사용 가이드를 중앙에서 관리한다.
// UI, tool registry, 시스템 프롬프트에서 동일한 문구를 사용하도록 한다.
// 포함 모듈:
//   - GetWebSearchDescription: 툴 목록에 노출되는 한 줄 설명.
//   - GetWebSearchPrompt: 모델에 주입할 상세 사용 규칙.
//
// 호출/사용 방식:
//   - internal/tools/websearch/websearch.go 의 Description()에서 호출한다.
//   - 필요한 경우 시스템 프롬프트 조립 계층에서 GetWebSearchPrompt()를 사용할 수 있다.
//
// 연결:
//   - import 하는 주요 패키지: internal/constants.
//   - 이 파일을 import 하는 주요 패키지: internal/tools/websearch.
package websearch

import (
	"fmt"

	"github.com/koreaf16/argus/internal/constants"
)

func GetWebSearchDescription() string {
	return GetWebSearchPrompt()
}

func GetWebSearchPrompt() string {
	currentMonthYear := constants.GetLocalMonthYear()
	return fmt.Sprintf(
		"- Search the web for up-to-date information.\n"+
			"- Returns structured results with title, URL, and snippet.\n"+
			"- For latest/current/news requests, prefer snippets if they contain definitive facts (e.g., current temperature, stock price).\n"+
			"- Only call webfetch if snippets are incomplete, contradictory, or lack necessary depth.\n"+
			"\n"+
			"Search strategy:\n"+
			"- If the user explicitly names a site (for example Hugging Face, GitHub, Docker Hub), prioritize that site first.\n"+
			"- Use broad search terms initially. Avoid overly specific constraints unless necessary.\n"+
			"- If a search returns no results, DO NOT immediately conclude the information does not exist. Try again with broader or alternative search terms.\n"+
			"- Prefer official and primary sources when available (release notes, vendor docs, model cards, official blogs).\n"+
			"\n"+
			"Critical requirement:\n"+
			"- Include absolute dates for time-sensitive claims.\n"+
			"- Include a \"Sources:\" section in the final answer.\n"+
			"- List all relevant URLs as markdown links.\n"+
			"\n"+
			"Usage notes:\n"+
			"- Supports allowed_domains and blocked_domains filters.\n"+
			"- Current month/year is %s; use this year in recent-information queries.",
		currentMonthYear,
	)
}
