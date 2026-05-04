package webfetch

import (
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *WebFetchTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"web_search 결과에서 특정 URL의 상세 내용이 필요할 때",
		"공식 문서·릴리스 노트·API 레퍼런스를 직접 읽어야 할 때",
		"사용자가 특정 URL을 직접 제공한 경우",
		"검색 결과 스니펫만으로 확정하기 어려운 최신 정보를 검증할 때",
	)

	s.WhenNotToUse(
		"아직 어떤 URL을 가져올지 모를 때 → web_search로 먼저 탐색",
		"로컬 파일 읽기 → Read 도구 사용",
		"SSRF 방지 대상인 내부 네트워크 주소 (localhost, 10.x, 172.16.x, 192.168.x)",
	)

	usageItems := []string{
		"url은 https:// 또는 http:// 로 시작해야 합니다",
		"대용량 페이지는 자동 요약됩니다 — 전체가 필요하면 max_length를 늘리세요",
		"인증이 필요한 페이지는 일반적으로 접근 불가합니다",
	}
	if ctx.Caps.WebSearch {
		usageItems = append(usageItems, "모델 네이티브 웹 검색이 활성화된 경우 일부 URL은 직접 요청하지 않아도 됩니다")
	}
	s.UsageNotes(usageItems...)

	s.Examples(
		"공식 문서 조회: url=\"https://docs.example.com/api\"",
		"GitHub 릴리스 노트: url=\"https://github.com/org/repo/releases\"",
	)

	s.Tips(
		"web_search → webfetch 패턴으로 검색 후 상위 2개 이상 URL을 확인하세요",
		"JSON API 엔드포인트도 직접 조회할 수 있습니다",
	)

	s.Parameters(
		"url: 가져올 URL (필수)",
		"max_length: 최대 응답 길이 (선택)",
		"start_index: 응답 시작 위치, 페이지네이션 시 사용 (선택)",
	)

	return s.Build()
}
