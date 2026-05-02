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
			"- For latest/current/news requests, web_search is only the discovery step. You MUST call webfetch on at least one selected result before finalizing.\n"+
			"- For research, comparisons, procedures, checklists, or operational guidance, do not rely on one page unless the user supplied an exact URL or explicitly asked for one source.\n"+
			"\n"+
			"Search strategy:\n"+
			"- For complex or research-style queries, issue 2-4 web_search calls simultaneously in a single response.\n"+
			"  Example: search official docs AND release notes AND community resources in parallel.\n"+
			"- Do NOT wait for one search result before issuing the next when queries are independent.\n"+
			"- After those searches, webfetch at least two high-signal candidate URLs, preferably from distinct domains, before finalizing.\n"+
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
