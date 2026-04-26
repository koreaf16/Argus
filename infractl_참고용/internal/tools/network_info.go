// Package tools
// File: network_info.go
// Description: 네트워크 구성 정보 조회 도구
// Responsibility: IP, 인터페이스 상태, 라우팅 테이블 등 수집
package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/infractl/internal/executor"
)

// NetworkInfoTool은 네트워크 구성 정보를 조회한다.
type NetworkInfoTool struct{}

func (t *NetworkInfoTool) Name() string { return "network_info" }

func (t *NetworkInfoTool) Description() string {
	return "Retrieve network configuration, IP addresses, interface status, and routing table."
}

func (t *NetworkInfoTool) IsReadOnly() bool { return true }
func (t *NetworkInfoTool) IsEnabled() bool  { return true }

func (t *NetworkInfoTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target server name.",
			},
		},
	}
}

func (t *NetworkInfoTool) Execute(ctx context.Context, _ map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	platform := executor.CommandPlatform(exec)
	tasks := []string{"interfaces", "routing", "arp"}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		parts = make(map[string]string)
		errs  []string
	)

	slog.Info("network_info_start", "server", exec.Target())

	for _, task := range tasks {
		wg.Add(1)
		go func(tk string) {
			defer wg.Done()
			EmitOutput(ctx, fmt.Sprintf("task_start task=%s", tk))
			slog.Info("task_start", "task", tk)
			start := time.Now()

			cmd := buildNetworkCommand(tk, platform)
			if cmd == "" {
				slog.Warn("network_info unknown task, skipping", "task", tk)
				EmitOutput(ctx, fmt.Sprintf("task_skip task=%s reason=unknown", tk))
				return
			}
			result, err := exec.Execute(ctx, cmd)
			dur := time.Since(start)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Sprintf("[%s] Error: %s", tk, err))
				EmitOutput(ctx, fmt.Sprintf("task_fail task=%s dur=%s", tk, dur))
				slog.Info("task_fail", "task", tk, "dur", dur)
				return
			}
			parts[tk] = strings.TrimSpace(result.Stdout)
			EmitOutput(ctx, fmt.Sprintf("task_done task=%s dur=%s", tk, dur))
			slog.Info("task_done", "task", tk, "dur", dur)
		}(task)
	}

	wg.Wait()

	var output []string
	for _, tk := range tasks {
		if content, ok := parts[tk]; ok {
			output = append(output, fmt.Sprintf("[%s]\n%s", tk, content))
		}
	}
	output = append(output, errs...)

	return ToolOutcome{Content: strings.Join(output, "\n\n"), Success: true}, nil
}

func buildNetworkCommand(task string, platform executor.Platform) string {
	if platform == executor.PlatformWindows {
		switch task {
		case "interfaces":
			return "ipconfig /all"
		case "routing":
			return "route print"
		case "arp":
			return "arp -a"
		default:
			return ""
		}
	} else {
		switch task {
		case "interfaces":
			return "ip addr show || ifconfig -a"
		case "routing":
			return "ip route show || route -n"
		case "arp":
			return "ip neigh show || arp -a"
		default:
			return ""
		}
	}
}
