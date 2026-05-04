package serverconnect

import (
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *ServerConnectTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	// When to use — include connection-intent routing rules
	whenItems := []string{
		"사용자가 서버에 연결·전환·접속을 요청할 때 (예: '접속해', 'connect to X', 'X로 바꿔', 'X 워크스페이스로')",
		"새 원격 서버에 처음 연결할 때 (host/user 정보 제공 시 자동 등록)",
		"연결 끊긴 워크스페이스에 재연결할 때",
	}
	s.WhenToUse(whenItems...)

	s.WhenNotToUse(
		"이미 연결된 서버에 명령을 실행할 때 → bash/powershell 사용",
		"서버 상태·메트릭만 조회할 때 → server_inspect/server_metrics 사용",
		"ssh 명령을 직접 실행하려 할 때 → server_connect를 사용하세요",
	)

	notesItems := []string{
		"별칭이 이미 등록되어 있으면 `server`=alias 만으로 호출하세요 (host/user 불필요)",
		"별칭이 등록되어 있지 않고 host/user를 제공받은 경우 server+host+user로 자동 등록·연결",
		"비밀번호는 세션 내에서만 캐시됩니다",
	}
	if len(ctx.Workspaces.Aliases) > 0 {
		notesItems = append(notesItems,
			"등록된 별칭: "+strings.Join(ctx.Workspaces.Aliases, ", ")+" — 이 이름을 언급하면 즉시 server=alias로 호출하세요",
			"등록되지 않은 별칭을 언급하면 server_connect를 호출하지 말고 사용자에게 목록을 알려주세요",
		)
	}
	notesItems = append(notesItems,
		"사용자가 '로컬 머신/내 PC'를 언급하면 bash/powershell의 server=\"local\"을 사용하세요",
	)
	s.UsageNotes(notesItems...)

	s.Examples(
		"등록된 별칭 연결: server=\"prod\"",
		"신규 서버 등록·연결: server=\"db1\", host=\"10.0.0.5\", username=\"admin\"",
		"포트 지정: server=\"staging\", host=\"staging.example.com\", username=\"deploy\", port=2222",
	)

	s.Tips(
		"사용자가 별칭을 언급하면 재질문 없이 바로 server_connect를 호출하세요",
		"키 기반 인증이 설정된 경우 password 없이 연결 가능합니다",
	)

	s.Parameters(
		"server: 서버 alias (필수)",
		"host: 서버 IP 또는 도메인 (신규 등록 시 필수)",
		"username: SSH 사용자 이름 (신규 등록 시 필수)",
		"port: SSH 포트, 기본값 22 (선택)",
		"password: 비밀번호 (선택, 키 인증 권장)",
	)

	return s.Build()
}
