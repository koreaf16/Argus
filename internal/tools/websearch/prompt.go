// Package websearch — prompt/description strings for the web search tool.
package websearch

import (
	"fmt"

	"github.com/koreaf16/argus/internal/constants"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

func GetWebSearchDescription() string {
	return GetWebSearchPrompt()
}

func GetWebSearchPrompt() string {
	currentMonthYear := constants.GetLocalMonthYear()
	return fmt.Sprintf(
		"- 최신 정보를 얻기 위해 웹을 검색합니다.\n"+
			"- 제목, URL 및 스니펫이 포함된 구조화된 결과를 반환합니다.\n"+
			"- 최신/현재/뉴스 요청의 경우 web_search는 발견 단계일 뿐입니다. 답변을 확정하기 전에 반드시 선택한 결과 중 하나 이상에 대해 webfetch를 호출해야 합니다.\n"+
			"- 조사, 비교, 절차, 체크리스트 또는 운영 가이드의 경우, 사용자가 정확한 URL을 제공하거나 명시적으로 하나의 소스만 요청하지 않는 한 한 페이지에만 의존하지 마십시오.\n"+
			"\n"+
			"검색 전략:\n"+
			"- 복잡하거나 조사 스타일의 쿼리의 경우 한 번의 응답으로 2~4개의 web_search 호출을 동시에 실행하십시오.\n"+
			"  예: 공식 문서, 릴리스 노트 및 커뮤니티 리소스를 병렬로 검색합니다.\n"+
			"- 쿼리가 독립적인 경우 다음 검색 결과를 기다리지 말고 다음 검색을 수행하십시오.\n"+
			"- 해당 검색 후, 답변을 확정하기 전에 서로 다른 도메인에서 최소 2개의 고신호 후보 URL을 webfetch하십시오.\n"+
			"- 사용자가 명시적으로 사이트(예: Hugging Face, GitHub, Docker Hub)를 지정하면 해당 사이트를 우선순위로 두십시오.\n"+
			"- 처음에는 광범위한 검색어를 사용하십시오. 필요한 경우가 아니면 지나치게 구체적인 제약 조건은 피하십시오.\n"+
			"- 검색 결과가 없는 경우 즉시 정보가 존재하지 않는다고 결론 내리지 마십시오. 더 넓은 범위의 검색어나 대체 검색어로 다시 시도하십시오.\n"+
			"- 가능한 경우 공식 및 기본 소스(릴리스 노트, 벤더 문서, 모델 카드, 공식 블로그)를 선호하십시오.\n"+
			"\n"+
			"중요 요구 사항:\n"+
			"- 시간에 민감한 주장의 경우 절대 날짜를 포함하십시오.\n"+
			"- 최종 답변에 \"출처(Sources):\" 섹션을 포함하십시오.\n"+
			"- 모든 관련 URL을 마크다운 링크로 나열하십시오.\n"+
			"\n"+
			"사용 참고 사항:\n"+
			"- allowed_domains 및 blocked_domains 필터를 지원합니다.\n"+
			"- 현재 월/년은 %s입니다. 최근 정보 쿼리에 이 연도를 사용하십시오.",
		currentMonthYear,
	)
}

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *WebSearchTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"최신 정보·뉴스·공지가 필요한 경우",
		"라이브러리 버전·릴리스 노트·공식 문서 탐색",
		"여러 소스를 비교하는 조사형 질문",
	)

	s.WhenNotToUse(
		"로컬 코드·파일 검색 → Grep/Glob 사용",
		"모델 학습 데이터로 충분히 알 수 있는 일반 지식",
		"사용자가 특정 URL을 제공한 경우 → webfetch 직접 호출",
	)

	notes := []string{
		"검색 후 반드시 상위 결과 중 1~2개 URL을 webfetch로 확인하세요",
		"복합 조사는 2~4개의 독립 검색을 병렬로 실행하세요",
		"allowed_domains/blocked_domains 필터로 소스를 제한할 수 있습니다",
		fmt.Sprintf("현재 날짜: %s — 최신 정보 쿼리에 활용하세요", constants.GetLocalMonthYear()),
	}
	if ctx.Caps.WebSearch {
		notes = append(notes, "모델 네이티브 웹 검색이 활성화되어 있습니다 — 간단한 검색은 네이티브 기능도 활용 가능")
	}
	s.UsageNotes(notes...)

	s.Examples(
		"라이브러리 최신 버전 확인: query=\"golang 1.23 release notes\"",
		"공식 문서 탐색: query=\"PostgreSQL 16 partitioning\"",
	)

	s.Tips(
		"출처(Sources) 섹션을 최종 답변에 반드시 포함하세요",
		"결과 없으면 검색어를 넓게 바꿔 재시도하세요",
	)

	s.Parameters(
		"query: 검색 쿼리 (필수)",
		"allowed_domains: 허용할 도메인 목록 (선택)",
		"blocked_domains: 차단할 도메인 목록 (선택)",
	)

	return s.Build()
}
