// Package connector
// File: activate_tool.go
// Description: connector_activate tool
// Responsibility: validate target/discovery alignment and activate connector

package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

// ServiceScanner는 서버에서 서비스를 자동 탐지하여 DiscoveryStore에 저장하는 인터페이스이다.
type ServiceScanner interface {
	ScanAndSave(ctx context.Context, exec executor.Executor, serverName string, ds store.DiscoveryStore) error
}

// ActivateTool activates a connector from discovered service info and credentials.
type ActivateTool struct {
	Manager        *Manager
	ServerStore    store.ServerStore
	DiscoveryStore store.DiscoveryStore
	Disambiguate   DisambiguateHandler // optional UI adapter
	Scanner        ServiceScanner      // optional: discovery 없을 때 자동 탐지 실행
}

var _ tools.Tool = (*ActivateTool)(nil)

func (t *ActivateTool) Name() string { return "connector_activate" }

func (t *ActivateTool) Description() string {
	return "서비스 커넥터를 활성화합니다. " +
		"username/password 없이 호출하면 저장된 크리덴셜로 자동 재연결합니다. " +
		"서비스 탐지 정보가 없거나 만료된 경우 자동으로 탐지를 실행합니다. " +
		"Oracle PDB는 sub_instance에 PDB명을 지정하세요 (예: sub_instance=HRPDB). " +
		"먼저 connector_probe_os_auth로 OS 인증을 시도하고, 실패 시에만 username/password를 사용하세요. " +
		"기본 save_mode는 permanent입니다."
}

func (t *ActivateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server": map[string]interface{}{
				"type":        "string",
				"description": "서버 alias 이름 (server_list에서 확인)",
			},
			"service_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"oracle", "mysql", "postgresql", "tomcat", "weblogic"},
				"description": "서비스 타입",
			},
			"service_name": map[string]interface{}{
				"type":        "string",
				"description": "서비스 이름 (discover_services 결과와 일치해야 함)",
			},
			"username": map[string]interface{}{
				"type":        "string",
				"description": "접속 계정",
			},
			"password": map[string]interface{}{
				"type":        "string",
				"description": "접속 비밀번호",
			},
			"role": map[string]interface{}{
				"type":        "string",
				"description": "Oracle role (예: sysdba)",
			},
			"port": map[string]interface{}{
				"type":        "integer",
				"description": "서비스 포트. 생략 시 discovery 포트를 사용",
			},
			"save_mode": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"permanent", "session", "none"},
				"description": "저장 방식 (기본: session)",
			},
			"sub_instance": map[string]interface{}{
				"type":        "string",
				"description": "서브 인스턴스 이름. Oracle PDB명(예: HRPDB), K8s namespace 등. 선택적.",
			},
		},
		"required": []string{"server", "service_type", "service_name"},
	}
}

func (t *ActivateTool) IsReadOnly() bool { return false }
func (t *ActivateTool) IsEnabled() bool  { return true }

// Execute validates request consistency, hydrates discovery data, and activates connector.
func (t *ActivateTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
	server, _ := args["server"].(string)
	serviceType, _ := args["service_type"].(string)
	serviceName, _ := args["service_name"].(string)
	username, _ := args["username"].(string)
	password, _ := args["password"].(string)
	subInstance, _ := args["sub_instance"].(string)

	server = strings.TrimSpace(server)
	serviceType = strings.ToLower(strings.TrimSpace(serviceType))
	serviceName = strings.TrimSpace(serviceName)
	subInstance = strings.TrimSpace(subInstance)

	ok := func(msg string) (tools.ToolOutcome, error) {
		return tools.ToolOutcome{Content: msg, Success: true}, nil
	}
	fail := func(msg string) (tools.ToolOutcome, error) {
		return tools.ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}

	if server == "" || serviceType == "" || serviceName == "" {
		return fail("server, service_type, service_name는 필수입니다")
	}

	if t.ServerStore != nil {
		// IP 주소가 들어온 경우 워크스페이스 alias로 자동 변환
		if alias, err := resolveHostToAlias(ctx, t.ServerStore, server); err == nil {
			server = alias
		}
		resolved, _, _ := checkNameConflict(ctx, t.ServerStore, t.Disambiguate, server, serviceType, exec.Target())
		server = resolved
	}

	// 크리덴셜이 없을 때: 저장된 커넥터로 자동 재연결 시도
	if username == "" && password == "" {
		if savedInfo, savedCreds, okConn := t.Manager.GetSavedConnector(ctx, server, serviceType, serviceName, subInstance); okConn {
			if err := t.Manager.Activate(ctx, savedInfo, savedCreds, SavePermanent, exec); err != nil {
				return fail(fmt.Sprintf("저장된 크리덴셜로 재연결 실패: %s\nusername/password를 직접 입력하세요.", err))
			}
			states := t.Manager.States()
			for _, s := range states {
				key := connectorKey(savedInfo.ServerName, savedInfo.ServiceType, savedInfo.Name, savedInfo.SubInstance)
				if s.Name == key {
					return ok(fmt.Sprintf("✓ %s %s — 저장된 크리덴셜로 자동 재연결 완료%s (%d개 도구 등록)\n활성 도구: %v",
						savedInfo.ServiceType, savedInfo.Name, formatConnectorMeta(s), len(s.Tools), s.Tools))
				}
			}
			return ok(fmt.Sprintf("✓ %s %s — 저장된 크리덴셜로 자동 재연결 완료", serviceType, serviceName))
		}
	}

	if err := validateDiscoveryTarget(exec.Target(), server); err != nil {
		return fail(err.Error())
	}

	entry, err := validateRecentDiscovery(ctx, t.DiscoveryStore, server, serviceType, serviceName)
	if err != nil {
		if t.Scanner != nil && t.DiscoveryStore != nil {
			if scanErr := t.Scanner.ScanAndSave(ctx, exec, server, t.DiscoveryStore); scanErr != nil {
				return fail(fmt.Sprintf("자동 탐지 실패: %s", scanErr))
			}
			entry, err = validateRecentDiscovery(ctx, t.DiscoveryStore, server, serviceType, serviceName)
		}
		if err != nil {
			return fail(err.Error())
		}
	}

	role, _ := args["role"].(string)
	saveModeStr, _ := args["save_mode"].(string)
	if strings.TrimSpace(saveModeStr) == "" {
		saveModeStr = string(SavePermanent) // 기본값: 영구 저장
	}
	port := parseOptionalInt(args["port"])

	info := buildServiceInfoFromDiscovery(server, serviceType, serviceName, port, entry)
	info.SubInstance = subInstance
	creds := Credentials{
		Username: username,
		Password: password,
		Role:     role,
	}
	saveMode := SaveMode(saveModeStr)

	if err := t.Manager.Activate(ctx, info, creds, saveMode, exec); err != nil {
		return fail(fmt.Sprintf("커넥터 활성화 실패: %s", err))
	}

	states := t.Manager.States()
	for _, s := range states {
		if s.ServerName == info.ServerName && s.Type == info.ServiceType && s.ServiceName == info.Name && s.SubInstance == info.SubInstance {
			return ok(fmt.Sprintf("✓ %s %s 커넥터 활성화 완료%s (%d개 도구 등록)\n저장 방식: %s\n활성 도구: %v",
				info.ServiceType, info.Name, formatConnectorMeta(s), len(s.Tools), saveMode, s.Tools))
		}
	}

	return ok(fmt.Sprintf("✓ %s %s 커넥터 활성화 완료 (save_mode: %s)", info.ServiceType, info.Name, saveMode))
}

// formatConnectorMeta는 ConnectorState.Details에서 Oracle CDB/버전/PDB 정보를
// 활성화 메시지에 포함할 수 있는 짧은 문자열로 변환한다.
// Oracle 이외의 커넥터는 Details가 비어 있으므로 빈 문자열을 반환한다.
func formatConnectorMeta(s ConnectorState) string {
	if len(s.Details) == 0 {
		return ""
	}
	ver := s.Details["oracle_version"]
	cdb := s.Details["cdb"]
	pdbs := s.Details["pdbs"]
	if ver == "" && cdb == "" {
		return ""
	}

	var parts []string
	if ver != "" {
		parts = append(parts, "Oracle "+ver)
	}
	if cdb == "yes" {
		cdbLabel := "CDB"
		if pdbs != "" {
			pdbList := strings.Split(pdbs, ",")
			names := make([]string, 0, len(pdbList))
			for _, p := range pdbList {
				if idx := strings.Index(p, ":"); idx > 0 {
					names = append(names, p[:idx])
				} else {
					names = append(names, p)
				}
			}
			cdbLabel = fmt.Sprintf("CDB (PDB %d개: %s)", len(names), strings.Join(names, ", "))
		}
		parts = append(parts, cdbLabel)
	} else if cdb == "no" {
		parts = append(parts, "non-CDB")
	}

	if len(parts) == 0 {
		return ""
	}
	return " — " + strings.Join(parts, ", ")
}

func parseOptionalInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

// resolveHostToAlias는 server가 IP/호스트명인 경우 ServerStore에서 alias를 역조회한다.
// 이미 alias인 경우에도 동일하게 alias를 반환한다.
func resolveHostToAlias(ctx context.Context, ss store.ServerStore, server string) (string, error) {
	servers, err := ss.List(ctx)
	if err != nil {
		return server, fmt.Errorf("server list: %w", err)
	}
	norm := strings.ToLower(strings.TrimSpace(server))
	for _, s := range servers {
		if strings.ToLower(s.Name) == norm || strings.ToLower(s.Host) == norm {
			return s.Name, nil
		}
	}
	return server, fmt.Errorf("no alias for %q", server)
}
