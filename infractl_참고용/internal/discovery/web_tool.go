// Package discovery
// File: web_tool.go
// Description: discover_web_servers tool implementation
// Responsibility: run web scanner and report results
package discovery

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// DiscoverWebServersTool is the LLM-facing web infrastructure discovery tool.
type DiscoverWebServersTool struct {
	Scanner *Scanner
}

var _ tools.Tool = (*DiscoverWebServersTool)(nil)

func (t *DiscoverWebServersTool) Name() string { return "discover_web_servers" }

func (t *DiscoverWebServersTool) Description() string {
	return "대상 서버의 웹 인프라(Nginx, Apache, HAProxy, SSL, VHosts)를 탐지합니다. " +
		"병렬 스캔을 통해 웹 서버 상태와 인증서 정보를 한꺼번에 확인합니다."
}

func (t *DiscoverWebServersTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"type":        "string",
				"description": "탐지 대상 서버 이름.",
			},
		},
	}
}

func (t *DiscoverWebServersTool) IsReadOnly() bool { return true }
func (t *DiscoverWebServersTool) IsEnabled() bool  { return true }

func (t *DiscoverWebServersTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
	serverName := exec.Target()
	slog.Info("web_discovery_start", "server", serverName)

	result, err := t.Scanner.ScanWeb(ctx, exec)
	if err != nil {
		slog.Error("web_discovery_failed", "server", serverName, "err", err)
		msg := fmt.Sprintf("웹 서버 탐지 실패: %s", err)
		return tools.ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}

	slog.Info("web_discovery_complete", "server", serverName, "found", len(result.Services))

	if len(result.Services) == 0 {
		return tools.ToolOutcome{Content: fmt.Sprintf("[%s] 탐지된 웹 서버 없음", result.ServerName), Success: true}, nil
	}

	return tools.ToolOutcome{Content: formatScanResult(result), Success: true}, nil
}
