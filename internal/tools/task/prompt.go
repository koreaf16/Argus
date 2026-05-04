package task

import (
	tools "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *TaskCreateTool) GetSystemPromptGuide(ctx tools.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"3단계 이상의 명확한 하위 작업이 있는 경우",
		"여러 파일·서비스를 순서대로 변경하는 경우",
		"설치·마이그레이션·배포처럼 진행 상황을 사용자에게 보고할 가치가 있는 경우",
		"오래 걸리는 작업으로 완료 여부를 추적해야 하는 경우",
	)

	s.WhenNotToUse(
		"단일 명령이나 파일 하나 수정으로 끝나는 작업 — task_create 없이 바로 실행",
		"파일 읽기, 검색, 정보 조회 등 읽기 전용 작업",
		"사용자가 이미 작업 목록을 제공한 경우 — 중복 등록 금지",
		"이미 등록된 작업의 단계를 세분화하는 것 — task_update로 갱신",
	)

	noteItems := []string{
		"task_create 후 즉시 task_update(status=in_progress)를 호출하세요",
		"완료 시 task_update(status=completed), 중단 시 cancelled",
		"`active_form`은 진행 중 사용자에게 보여줄 현재진행형 문구입니다",
		"1개 요청에 여러 독립 작업이 있으면 각각 별도 task_create를 호출하세요",
	}
	if ctx.Mode == "plan" {
		noteItems = append(noteItems, "⚠ 계획 모드에서는 task_create 대신 계획 파일에 단계를 작성하세요")
	}
	s.UsageNotes(noteItems...)

	s.Examples(
		"Oracle DB 19c 설치 등록: subject=\"Oracle DB 19c 설치\", active_form=\"Oracle DB 설치 중\"",
		"마이그레이션 등록: subject=\"스키마 마이그레이션 실행\", description=\"v2→v3 DDL 적용\"",
	)

	s.Tips(
		"subject는 명령형 동사로 시작하세요 (설치, 배포, 구성, 실행)",
		"단계가 2개 이하라면 task 없이 바로 실행하는 것이 낫습니다",
	)

	s.Parameters(
		"subject: 명령형 짧은 제목 (필수, 예: 'Oracle DB 설치')",
		"description: 수행할 작업의 구체적 내용 (선택)",
		"active_form: in_progress 시 표시할 현재진행형 문구 (선택)",
	)

	return s.Build()
}

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *TaskUpdateTool) GetSystemPromptGuide(ctx tools.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"task_create로 등록한 작업의 상태를 변경할 때",
		"작업 시작 시: status=in_progress",
		"작업 완료 시: status=completed",
		"작업 중단 시: status=cancelled",
	)

	s.WhenNotToUse(
		"새 작업을 등록할 때 → task_create 사용",
		"존재하지 않는 task_id를 갱신하려 할 때",
	)

	s.UsageNotes(
		"task_create 직후 반드시 in_progress로 갱신하세요",
		"task_id는 task_create 결과에서 확인하세요",
		"status 변경 없이 제목/설명만 갱신하는 것도 가능합니다",
	)

	s.Examples(
		"시작: task_id=\"abc123\", status=\"in_progress\"",
		"완료: task_id=\"abc123\", status=\"completed\"",
	)

	s.Parameters(
		"task_id: 갱신할 작업 ID (필수)",
		"status: pending / in_progress / completed / cancelled (선택)",
		"subject: 변경할 제목 (선택)",
		"description: 변경할 설명 (선택)",
		"active_form: 변경할 현재진행형 문구 (선택)",
	)

	return s.Build()
}
