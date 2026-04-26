// Package precheck
// File: engine.go
// Description: 여러 Checker를 병렬로 실행하고 결과를 집계하는 엔진
// Responsibility: context 전파, goroutine 관리, CheckSummary 조립을 담당한다.

package precheck

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/yourorg/infractl/internal/executor"
)

// Run executes all checkers in parallel and returns a consolidated CheckSummary.
// Checks with SeverityBlock that fail will set summary.Blocked = true.
// Windows executors are skipped (POSIX-only checks).
func Run(ctx context.Context, exec executor.Executor, checks []Checker) CheckSummary {
	if executor.CommandPlatform(exec) == executor.PlatformWindows {
		return CheckSummary{}
	}
	if len(checks) == 0 {
		return CheckSummary{}
	}

	results := make([]CheckResult, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func(idx int, ch Checker) {
			defer wg.Done()
			r := ch.Run(ctx, exec)
			results[idx] = r
			slog.Debug("precheck", "kind", r.Kind, "subject", r.Subject, "ok", r.OK, "severity", r.Severity)
		}(i, c)
	}
	wg.Wait()

	return buildSummary(results)
}

// buildSummary aggregates CheckResults into a CheckSummary.
func buildSummary(results []CheckResult) CheckSummary {
	summary := CheckSummary{Results: results}
	var blockMsgs []string

	for _, r := range results {
		if r.OK || r.Message == "" {
			continue
		}
		switch r.Severity {
		case SeverityBlock:
			summary.Blocked = true
			blockMsgs = append(blockMsgs, r.Message)
		case SeverityWarn:
			summary.Warnings = append(summary.Warnings, "[pre-check warning] "+r.Message)
		}
	}

	if summary.Blocked {
		summary.BlockMsg = strings.Join(blockMsgs, "; ")
	}
	return summary
}
