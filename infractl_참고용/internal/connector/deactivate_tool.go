// Package connector
// File: deactivate_tool.go
// Description: connector_deactivate LLM 도구 — 활성 커넥터 비활성화 및 영구 삭제
// Responsibility: 메모리 상태 제거 + 선택적 SQLite 퍼지

package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

// DeactivateTool은 활성 커넥터를 비활성화하는 LLM 도구이다.
// purge=true이면 SQLite 영구 레코드도 함께 삭제한다.
type DeactivateTool struct {
	Manager     *Manager
	ServerStore store.ServerStore
}

var _ tools.Tool = (*DeactivateTool)(nil)

func (t *DeactivateTool) Name() string { return "connector_deactivate" }

func (t *DeactivateTool) Description() string {
	return "활성 커넥터를 비활성화합니다. " +
		"purge=true이면 저장된 크리덴셜과 영구 레코드도 함께 삭제합니다. " +
		"커넥터를 다시 만들려면 이 도구로 먼저 제거한 뒤 connector_activate를 사용하세요."
}

func (t *DeactivateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server": map[string]interface{}{
				"type":        "string",
				"description": "서버 alias 이름",
			},
			"service_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"oracle", "mysql", "postgresql", "tomcat", "weblogic"},
				"description": "서비스 타입",
			},
			"service_name": map[string]interface{}{
				"type":        "string",
				"description": "서비스 이름 (예: AI26)",
			},
			"sub_instance": map[string]interface{}{
				"type":        "string",
				"description": "서브 인스턴스 이름. Oracle PDB명 등. 생략 시 service_name에서 '/' 구분자로 파싱.",
			},
			"purge": map[string]interface{}{
				"type":        "boolean",
				"description": "true이면 SQLite 영구 레코드도 삭제. false(기본)이면 메모리에서만 제거.",
			},
		},
		"required": []string{"server", "service_type", "service_name"},
	}
}

func (t *DeactivateTool) IsReadOnly() bool { return false }
func (t *DeactivateTool) IsEnabled() bool  { return true }

// Execute는 커넥터를 비활성화하고 필요 시 영구 레코드를 삭제한다.
func (t *DeactivateTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
	server, _ := args["server"].(string)
	serviceType, _ := args["service_type"].(string)
	serviceName, _ := args["service_name"].(string)
	subInstance, _ := args["sub_instance"].(string)
	purge, _ := args["purge"].(bool)

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

	// IP 주소 → alias 자동 변환
	if t.ServerStore != nil {
		if alias, err := resolveHostToAlias(ctx, t.ServerStore, server); err == nil {
			server = alias
		}
	}

	// service_name에 sub_instance가 포함된 경우 분리 후 sub_instance로 사용
	name, sub := splitServiceName(serviceName)
	if subInstance != "" {
		sub = subInstance
	}
	effectiveName := name
	if sub != "" {
		effectiveName = name + "/" + sub
	}

	if err := t.Manager.Deactivate(server, serviceType, effectiveName); err != nil {
		return fail(fmt.Sprintf("커넥터 비활성화 실패: %s", err))
	}

	if purge {
		if err := t.Manager.RemoveSavedConnector(ctx, server, serviceType, effectiveName); err != nil {
			return fail(fmt.Sprintf("비활성화는 완료됐으나 영구 레코드 삭제 실패: %s", err))
		}
		return ok(fmt.Sprintf("✓ %s %s/%s 커넥터 비활성화 + 영구 레코드 삭제 완료", serviceType, server, effectiveName))
	}

	return ok(fmt.Sprintf("✓ %s %s/%s 커넥터 비활성화 완료 (저장 레코드는 유지됨 — 영구 삭제는 purge=true 사용)", serviceType, server, effectiveName))
}
