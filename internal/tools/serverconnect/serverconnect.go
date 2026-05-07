package serverconnect

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/inventory"
	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/types"
)

type ServerConnectTool struct{}

func NewServerConnectTool() *ServerConnectTool {
	return &ServerConnectTool{}
}

func (t *ServerConnectTool) Name() string {
	return "server_connect"
}

func (t *ServerConnectTool) Description(ctx tool.Context) string {
	return "원격 서버에 연결을 설정합니다. 호스트/사용자 정보가 제공되면 자동 등록을 지원합니다."
}

func (t *ServerConnectTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "서버 별칭 (예: 'sandbox'). host/user 정보가 제공되면 이 별칭으로 자동 등록됩니다.",
			},
			"host": map[string]any{
				"type":        "string",
				"description": "원격 IP 또는 호스트 이름.",
			},
			"user": map[string]any{
				"type":        "string",
				"description": "SSH 사용자 이름.",
			},
			"port": map[string]any{
				"type":        "integer",
				"description": "SSH 포트 (기본값 22).",
			},
			"password": map[string]any{
				"type":        "string",
				"description": "선택적 SSH 로그인 비밀번호.",
			},
			"elevation": map[string]any{
				"type":        "object",
				"description": "서버와 함께 등록할 선택적 권한 상승(sudo/su) 정책입니다. 생략하면 권한 상승은 기본적으로 비활성화(DISABLED)되며, 나중에 /server edit <alias>를 통해 설정해야 합니다.",
				"properties": map[string]any{
					"allowed": map[string]any{
						"type":        "boolean",
						"description": "이 서버에서 sudo/su 허용 여부.",
					},
					"mode": map[string]any{
						"type":        "string",
						"enum":        []string{"password", "reuse_login"},
						"description": "'password': 별도로 등록된 sudo 비밀번호를 사용합니다. 'reuse_login': SSH 로그인 비밀번호를 sudo 비밀번호로 전달합니다.",
					},
					"target_users": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "권한 상승을 특정 대상 사용자(예: ['root', 'postgres'])로 제한합니다. 비어 있으면 모든 사용자를 의미합니다.",
					},
					"sudo_password": map[string]any{
						"type":        "string",
						"description": "캐시할 sudo 비밀번호 (mode='password'인 경우에만 해당). DPAPI로 암호화되어 저장되며, 로그에 남기거나 출력에 표시하지 마십시오.",
					},
				},
			},
		},
		"required": []string{"server"},
	}
}

func (t *ServerConnectTool) IsReadOnly() bool {
	return true
}

func (t *ServerConnectTool) MaxResultSizeChars() int {
	return 2048
}

func (t *ServerConnectTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 1)

	var req struct {
		Server    string `json:"server"`
		Host      string `json:"host"`
		User      string `json:"user"`
		Port      int    `json:"port"`
		Password  string `json:"password"`
		Elevation *struct {
			Allowed      bool     `json:"allowed"`
			Mode         string   `json:"mode"`
			TargetUsers  []string `json:"target_users"`
			SudoPassword string   `json:"sudo_password"`
		} `json:"elevation"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		alias := strings.TrimSpace(req.Server)
		if alias == "" {
			events <- tool.NewErrorEvent(fmt.Errorf("서버 별칭이 필요합니다"))
			return
		}

		if ctx.Workspace == nil {
			events <- tool.NewErrorEvent(fmt.Errorf("워크스페이스 관리자를 사용할 수 없습니다"))
			return
		}

		// [자동 등록 로직] 에일리어스가 없는데 상세 정보가 있다면 즉시 등록
		if strings.TrimSpace(req.Host) != "" {
			user := req.User
			if user == "" {
				user = "root"
			}
			port := req.Port
			if port == 0 {
				port = 22
			}
			entry := workspace.ServerEntry{
				Alias: alias,
				Host:  req.Host,
				Port:  port,
				User:  user,
				Kind:  workspace.ServerKindSSH,
				Auth:  workspace.ServerAuth{AllowPassword: true},
			}
			if req.Elevation != nil {
				entry.Elevation = workspace.Elevation{
					Allowed:     req.Elevation.Allowed,
					Mode:        req.Elevation.Mode,
					TargetUsers: req.Elevation.TargetUsers,
				}
			}
			ctx.Workspace.UpsertServer(entry)

			// Cache sudo password when provided.
			if req.Elevation != nil && req.Elevation.Allowed && req.Elevation.Mode == "password" &&
				strings.TrimSpace(req.Elevation.SudoPassword) != "" {
				ctx.Workspace.SetPassword(alias, "sudo", req.Elevation.SudoPassword)
			}
		}

		// 패스워드 주입
		if strings.TrimSpace(req.Password) != "" {
			ctx.Workspace.SetPassword(alias, "ssh", req.Password)
		}

		// [자율성 보강] 에일리어스가 없는데 IP 형식이라면 스스로 등록 시도
		if strings.Contains(alias, ".") || strings.HasPrefix(alias, "192.") {
			ctx.Workspace.UpsertServer(workspace.ServerEntry{
				Alias: alias,
				Host:  alias,
				Port:  22,
				User:  "sandbox", // 기본 관리자 계정 시도
				Kind:  workspace.ServerKindSSH,
				Auth:  workspace.ServerAuth{AllowPassword: true},
			})
		}

		resolvedAlias, err := ctx.Workspace.ConnectAndActivate(alias)
		if err != nil {
			events <- tool.NewErrorEvent(fmt.Errorf("연결 실패: %w", err))
			return
		}

		// [Eager Priming] 연결 성공 즉시 비동기로 exec channel(lane)을 미리
		// 워밍업한다. Inspect 결과를 기다리는 동안 백그라운드에서 SSH PTY가
		// 초기화되어, 첫 bash 명령의 priming 지연(~15-21초)이 사라진다.
		ctx.Workspace.WarmUpLane(resolvedAlias)

		if ctx.State != nil {
			ctx.State.SetActiveWorkspace(ctx.Workspace.ActiveAlias())
		}

		events <- tool.NewOutputEvent(fmt.Sprintf("[ARGUS_SERVER_CONNECT:connected]\n%s에 연결되었습니다. 환경 정보를 확인 중입니다...\n", resolvedAlias))

		// 인벤토리 스캔 (동기 streaming): header phase ~1s, ready phase ~5-25s.
		// inventoryChannel을 사용하므로 PTY exec lane은 점유하지 않음.
		events <- tool.NewOutputEvent(inventoryScanningMarker + "\n")
		ctx.Workspace.RescanInventoryStreaming(ctx.Context, resolvedAlias, func(phase inventory.InventoryPhase, snap inventory.InventorySnapshot, err error) {
			switch phase {
			case inventory.PhaseHeader:
				cardLines := inventory.FormatUICard(snap)
				if len(cardLines) > 0 {
					events <- tool.NewOutputEvent(inventoryHeaderMarker + "\n" + strings.Join(cardLines, "\n"))
				}
			case inventory.PhaseReady:
				cardLines := inventory.FormatUICard(snap)
				events <- tool.NewOutputEvent(fmt.Sprintf("%s%d]\n%s",
					inventoryReadyPrefix, snap.DurationMs, strings.Join(cardLines, "\n")))
			case inventory.PhaseFailed:
				errMsg := ""
				if err != nil {
					errMsg = err.Error()
				}
				events <- tool.NewOutputEvent(inventoryFailedMarker + "\n" + errMsg)
			}
		})

		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func (t *ServerConnectTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.PermissionResult{Behavior: types.BehaviorAllow}, nil
}
