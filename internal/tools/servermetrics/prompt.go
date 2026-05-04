package servermetrics

import (
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *ServerMetricsTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"서버의 CPU·메모리·디스크 사용량을 확인할 때",
		"시스템 리소스 과부하 여부를 진단할 때",
		"여러 서버의 메트릭을 한 번에 조회할 때",
	)

	s.WhenNotToUse(
		"실시간 연속 모니터링 → bash + watch 또는 ShellJob 사용",
		"애플리케이션 레벨 메트릭 → 해당 도구 사용",
	)

	s.UsageNotes(
		"server를 지정하지 않으면 모든 연결된 서버의 메트릭을 반환합니다",
		"반환값: cpu_percent, mem_used_mb, mem_total_mb, disk_used_gb, disk_total_gb",
	)

	s.Examples(
		"전체 서버 메트릭: (server 생략)",
		"특정 서버: server=\"prod-db\"",
	)

	s.Parameters(
		"server: 대상 서버 alias (선택, 생략 시 전체)",
	)

	return s.Build()
}
