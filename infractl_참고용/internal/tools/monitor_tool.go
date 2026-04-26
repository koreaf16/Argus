// Package tools
// File: monitor_tool.go
// Description: monitor LLM 도구 — 백그라운드 작업 stdout을 주기적으로 폴링해 집계 반환
// Responsibility: poll_interval_ms·max_duration_ms 파라미터로 최대 1회 호출 내 폴링 반복

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/background"
	"github.com/yourorg/infractl/internal/executor"
)

const (
	defaultPollIntervalMs = 1000
	defaultMaxDurationMs  = 30000
)

// MonitorTool은 백그라운드 작업 stdout을 반복 폴링해 집계 출력을 반환하는 LLM 도구이다.
// 설계 제한: per-call 8KB, per-job 64KB (OutputTool과 동일한 누적 추적 사용).
type MonitorTool struct {
	Manager      *background.Manager
	MaxPerCall   int // 기본 8192 (8KB)
	MaxCumul     int // 기본 65536 (64KB)
}

func (t *MonitorTool) Name() string { return "monitor" }

func (t *MonitorTool) Description() string {
	return "백그라운드 작업의 stdout을 실시간으로 폴링한다. " +
		"job_id와 offset을 지정한다. poll_interval_ms(기본 1000)마다 확인하고 " +
		"max_duration_ms(기본 30000) 또는 작업 완료 시 종료한다."
}

func (t *MonitorTool) IsReadOnly() bool { return true }
func (t *MonitorTool) IsEnabled() bool  { return true }

func (t *MonitorTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"job_id": map[string]interface{}{
				"type":        "integer",
				"description": "모니터링할 백그라운드 작업 ID",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "읽기 시작 byte offset (첫 호출: 0, 이후 반환된 next_offset 사용)",
			},
			"poll_interval_ms": map[string]interface{}{
				"type":        "integer",
				"description": "폴링 간격 (밀리초, 기본 1000)",
			},
			"max_duration_ms": map[string]interface{}{
				"type":        "integer",
				"description": "최대 모니터링 지속 시간 (밀리초, 기본 30000)",
			},
		},
		"required": []string{"job_id"},
	}
}

func (t *MonitorTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	id, err := extractJobID(args)
	if err != nil {
		return ToolOutcome{}, err
	}

	offset := int64(argInt(args, "offset", 0))
	pollIntervalMs := argInt(args, "poll_interval_ms", defaultPollIntervalMs)
	maxDurationMs := argInt(args, "max_duration_ms", defaultMaxDurationMs)

	if pollIntervalMs < 100 {
		pollIntervalMs = 100 // 최소 100ms — 서버 과부하 방지
	}

	job, jobErr := t.Manager.Get(id)
	if jobErr != nil {
		return ToolOutcome{}, fmt.Errorf("monitor: %w", jobErr)
	}

	// 파일 스트리밍 미사용 작업 — 상태만 반환
	if !job.Streamed || t.Manager.StorageDir() == "" {
		return t.handleNonStreamed(job)
	}

	maxPerCall := t.MaxPerCall
	if maxPerCall <= 0 {
		maxPerCall = 8 * 1024
	}
	maxCumul := t.MaxCumul
	if maxCumul <= 0 {
		maxCumul = 64 * 1024
	}

	deadline := time.Now().Add(time.Duration(maxDurationMs) * time.Millisecond)
	pollInterval := time.Duration(pollIntervalMs) * time.Millisecond

	var sb strings.Builder
	totalRead := 0
	storageDir := t.Manager.StorageDir()

	for {
		// 누적 한도 확인
		job, _ = t.Manager.Get(id)
		if job.MonitorBytesEmitted >= maxCumul {
			sb.WriteString("\n[cumulative_exceeded] 누적 출력 한도 초과")
			break
		}

		remaining := maxCumul - job.MonitorBytesEmitted
		readMax := maxPerCall - totalRead
		if remaining < readMax {
			readMax = remaining
		}
		if readMax <= 0 {
			break
		}

		result, pollErr := background.PollStdout(storageDir, id, offset, readMax)
		if pollErr != nil {
			return ToolOutcome{}, fmt.Errorf("monitor poll: %w", pollErr)
		}

		if len(result.Data) > 0 {
			sb.Write(result.Data)
			totalRead += len(result.Data)
			offset = result.NewOffset
			t.Manager.AddMonitorBytes(id, len(result.Data))
		}

		if result.Done {
			sb.WriteString("\n[done]")
			break
		}

		if result.Truncated || totalRead >= maxPerCall {
			sb.WriteString("\n[truncated]")
			break
		}

		if time.Now().After(deadline) {
			sb.WriteString("\n[timeout]")
			break
		}

		// ctx 취소 확인 후 대기
		select {
		case <-ctx.Done():
			sb.WriteString("\n[cancelled]")
			return ToolOutcome{
				Content: fmt.Sprintf("job_id=%d offset=%d next_offset=%d\n%s",
					id, offset-int64(totalRead), offset, sb.String()),
				Success: true,
			}, nil
		case <-time.After(pollInterval):
		}
	}

	return ToolOutcome{
		Content: fmt.Sprintf("job_id=%d next_offset=%d\n%s", id, offset, sb.String()),
		Success: true,
	}, nil
}

func (t *MonitorTool) handleNonStreamed(job background.Job) (ToolOutcome, error) {
	if job.IsActive() {
		return ToolOutcome{
			Content: fmt.Sprintf("작업 #%d '%s' 실행 중 (파일 스트리밍 미사용 — background_jobs로 결과 확인)", job.ID, job.Description),
			Success: true,
		}, nil
	}
	if job.Error != "" {
		return ToolOutcome{Content: fmt.Sprintf("작업 #%d 실패:\n%s", job.ID, job.Error), Success: true}, nil
	}
	return ToolOutcome{Content: fmt.Sprintf("작업 #%d 완료:\n%s", job.ID, job.Result), Success: true}, nil
}
