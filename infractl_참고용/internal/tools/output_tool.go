// Package tools
// File: output_tool.go
// Description: background_output LLM 도구 — 실행 중인 백그라운드 작업의 stdout을 반환
// Responsibility: per-call 8KB · per-job 64KB 제한 적용, 누적 바이트 Manager에 기록

package tools

import (
	"context"
	"fmt"

	"github.com/yourorg/infractl/internal/background"
	"github.com/yourorg/infractl/internal/executor"
)

// OutputTool은 백그라운드 작업의 실시간 stdout 출력을 LLM에 제공하는 도구이다.
// Monitor 설계(8KB/call, 64KB/job)를 적용한다.
type OutputTool struct {
	Manager      *background.Manager
	MaxPerCall   int // 기본 8192 (8KB)
	MaxCumul     int // 기본 65536 (64KB)
}

func (t *OutputTool) Name() string { return "background_output" }

func (t *OutputTool) Description() string {
	return "실행 중인 백그라운드 작업의 stdout 출력을 읽는다. " +
		"job_id와 offset(이전 호출 반환값)을 지정한다. " +
		"done=true이면 작업이 완료된 것이다."
}

func (t *OutputTool) IsReadOnly() bool { return true }
func (t *OutputTool) IsEnabled() bool  { return true }

func (t *OutputTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"job_id": map[string]interface{}{
				"type":        "integer",
				"description": "조회할 백그라운드 작업 ID",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "읽기 시작 byte offset (첫 호출: 0, 이후 반환된 next_offset 사용)",
			},
		},
		"required": []string{"job_id"},
	}
}

func (t *OutputTool) Execute(_ context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	id, err := extractJobID(args)
	if err != nil {
		return ToolOutcome{}, err
	}

	offset := int64(0)
	if v, ok := args["offset"]; ok {
		switch n := v.(type) {
		case float64:
			offset = int64(n)
		case int:
			offset = int64(n)
		case int64:
			offset = n
		}
	}

	job, jobErr := t.Manager.Get(id)
	if jobErr != nil {
		return ToolOutcome{}, fmt.Errorf("background_output: %w", jobErr)
	}

	// 파일 스트리밍이 아닌 작업은 Result 필드로 대체한다.
	if !job.Streamed {
		return t.handleNonStreamed(job)
	}

	storageDir := t.Manager.StorageDir()
	if storageDir == "" {
		return ToolOutcome{Content: "(파일 저장 비활성화 — storageDir 미설정)", Success: true}, nil
	}

	maxPerCall := t.MaxPerCall
	if maxPerCall <= 0 {
		maxPerCall = 8 * 1024
	}
	maxCumul := t.MaxCumul
	if maxCumul <= 0 {
		maxCumul = 64 * 1024
	}

	// 누적 한도 초과 확인
	if job.MonitorBytesEmitted >= maxCumul {
		return ToolOutcome{
			Content: fmt.Sprintf(
				"[cumulative_exceeded] 작업 #%d 의 누적 출력 한도(%d bytes)를 초과했습니다. "+
					"done=%v", id, maxCumul, !job.IsActive()),
			Success: true,
		}, nil
	}

	// 이번 호출에서 읽을 수 있는 최대 바이트 = min(maxPerCall, 남은 누적 여유)
	remaining := maxCumul - job.MonitorBytesEmitted
	readMax := maxPerCall
	if remaining < readMax {
		readMax = remaining
	}

	result, pollErr := background.PollStdout(storageDir, id, offset, readMax)
	if pollErr != nil {
		return ToolOutcome{}, fmt.Errorf("background_output poll: %w", pollErr)
	}

	// 누적 기록
	if len(result.Data) > 0 {
		t.Manager.AddMonitorBytes(id, len(result.Data))
	}

	content := string(result.Data)
	if result.Truncated {
		content += "\n[truncated]"
	}

	return ToolOutcome{
		Content: fmt.Sprintf(
			"job_id=%d offset=%d next_offset=%d done=%v\n%s",
			id, offset, result.NewOffset, result.Done, content),
		Success: true,
	}, nil
}

// handleNonStreamed는 Submit()으로 제출된 작업(파일 없음)의 결과를 반환한다.
func (t *OutputTool) handleNonStreamed(job background.Job) (ToolOutcome, error) {
	if job.IsActive() {
		return ToolOutcome{
			Content: fmt.Sprintf("작업 #%d '%s' 실행 중 (파일 스트리밍 미사용)", job.ID, job.Description),
			Success: true,
		}, nil
	}
	if job.Error != "" {
		return ToolOutcome{
			Content: fmt.Sprintf("작업 #%d 실패:\n%s", job.ID, job.Error),
			Success: true,
		}, nil
	}
	return ToolOutcome{
		Content: fmt.Sprintf("작업 #%d '%s' 완료:\n%s", job.ID, job.Description, job.Result),
		Success: true,
	}, nil
}

func extractJobID(args map[string]interface{}) (int, error) {
	v, ok := args["job_id"]
	if !ok {
		return 0, fmt.Errorf("job_id 파라미터가 필요합니다")
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	}
	return 0, fmt.Errorf("job_id는 정수여야 합니다")
}
