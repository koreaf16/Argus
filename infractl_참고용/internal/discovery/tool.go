// Package discovery
// File: tool.go
// Description: discover_services tool implementation
// Responsibility: run scanner, persist results, and report conflict hints
package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

// DiscoverServicesTool is the LLM-facing service discovery tool.
type DiscoverServicesTool struct {
	Store       store.DiscoveryStore
	Scanner     *Scanner
	ServerStore store.ServerStore
}

var _ tools.Tool = (*DiscoverServicesTool)(nil)

func (t *DiscoverServicesTool) Name() string { return "discover_services" }

func (t *DiscoverServicesTool) Description() string {
	return "대상 서버에서 실행 중인 서비스를 자동으로 탐지합니다. " +
		"프로세스, 포트, 설정 파일을 분석해 Oracle/MySQL/PostgreSQL/Tomcat/WebLogic 징후를 찾습니다. " +
		"서비스가 발견되면 신뢰도와 함께 목록을 반환합니다. " +
		"서비스 관련 작업 전 먼저 호출하세요."
}

func (t *DiscoverServicesTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"type":        "string",
				"description": "탐지 대상 서버 이름. 생략 시 현재 활성 서버(없으면 localhost)에서 실행.",
			},
		},
		"required": []string{},
	}
}

func (t *DiscoverServicesTool) IsReadOnly() bool { return true }
func (t *DiscoverServicesTool) IsEnabled() bool  { return true }

// Execute scans the target server and returns the formatted result.
func (t *DiscoverServicesTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (tools.ToolOutcome, error) {
	serverName := exec.Target()
	slog.Info("service_discovery_start", "server", serverName)

	result, err := t.Scanner.Scan(ctx, exec)
	if err != nil {
		slog.Error("service_discovery_failed", "server", serverName, "err", err)
		msg := fmt.Sprintf("서비스 탐지 실패: %s", err)
		return tools.ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}

	slog.Info("service_discovery_complete", "server", serverName, "found", len(result.Services))

	if len(result.Services) == 0 {
		return tools.ToolOutcome{Content: fmt.Sprintf("[%s] 탐지된 서비스 없음", result.ServerName), Success: true}, nil
	}

	if t.Store != nil {
		entries := make([]store.DiscoveredServiceEntry, 0, len(result.Services))
		for _, svc := range result.Services {
			entries = append(entries, store.DiscoveredServiceEntry{
				ServerName:  svc.ServerName,
				ServiceType: string(svc.Type),
				ServiceName: svc.Name,
				Port:        svc.Port,
				Details:     svc.Details,
				Confidence:  svc.Confidence,
			})
		}
		if err := t.Store.SaveDiscoveredServices(ctx, result.ServerName, entries); err != nil {
			// Persist failure should not block discovery output.
			_ = err
		}
	}

	output := formatScanResult(result)

	if t.ServerStore != nil {
		servers, listErr := t.ServerStore.List(ctx)
		if listErr == nil {
			output += detectServerNameConflicts(servers, result)
		}
	}

	return tools.ToolOutcome{Content: output, Success: true}, nil
}

// detectServerNameConflicts emits warnings only for real ambiguity cases.
func detectServerNameConflicts(servers []store.Server, result *ScanResult) string {
	serverNames := make(map[string]bool, len(servers))
	for _, s := range servers {
		serverNames[strings.ToLower(s.Name)] = true
	}

	targetServer := strings.ToLower(strings.TrimSpace(result.ServerName))
	seenWarning := make(map[string]bool)
	seenInfo := make(map[string]bool)
	var notes strings.Builder

	for _, svc := range result.Services {
		svcType := strings.ToLower(string(svc.Type))
		if !serverNames[svcType] {
			continue
		}

		if svcType == targetServer {
			if seenInfo[svcType] {
				continue
			}
			seenInfo[svcType] = true
			notes.WriteString(fmt.Sprintf(
				"\nℹ '%s' 서비스는 active SSH 서버 '%s'에서 발견된 결과입니다.\n",
				svc.Type, result.ServerName,
			))
			continue
		}

		if seenWarning[svcType] {
			continue
		}
		seenWarning[svcType] = true
		notes.WriteString(fmt.Sprintf(
			"\n⚠ '%s' 이름의 SSH 서버 alias가 따로 등록되어 있습니다.\n"+
				"  현재 스캔 대상은 '%s'이며, %s 서비스(포트 %d)가 발견되었습니다.\n"+
				"  대상 서버와 서비스 타입을 명확히 구분해 사용하세요.\n",
			svc.Type, result.ServerName, svc.Type, svc.Port,
		))
	}

	return notes.String()
}

// formatScanResult formats scan results for LLM consumption.
func formatScanResult(result *ScanResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] 서비스 탐지 결과 (%s 기준)\n\n",
		result.ServerName, result.ScannedAt.Format("15:04:05")))

	for _, svc := range result.Services {
		icon := confidenceIcon(svc.Level)
		sb.WriteString(fmt.Sprintf("%s [%s] %s", icon, svc.Type, svc.Level))
		if svc.Name != "" {
			sb.WriteString(fmt.Sprintf(" · %s", svc.Name))
		}
		if svc.Port > 0 {
			sb.WriteString(fmt.Sprintf(" (포트 %d)", svc.Port))
		}
		if svc.PID > 0 {
			sb.WriteString(fmt.Sprintf(" PID=%d", svc.PID))
		}
		sb.WriteString(fmt.Sprintf(" 신뢰도 %.0f%%\n", svc.Confidence*100))

		for k, v := range svc.Details {
			sb.WriteString(fmt.Sprintf("   - %s: %s\n", k, v))
		}
	}

	for _, svc := range result.Services {
		if svc.Level == ConfidenceMedium {
			sb.WriteString(fmt.Sprintf("\n? %s 서비스가 감지되었지만 확실하지 않습니다. 맞나요?\n", svc.Type))
		}
	}

	return strings.TrimSpace(sb.String())
}

func confidenceIcon(level ConfidenceLevel) string {
	switch level {
	case ConfidenceHigh:
		return "✓"
	case ConfidenceMedium:
		return "?"
	default:
		return "~"
	}
}
