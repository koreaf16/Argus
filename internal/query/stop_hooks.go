package query

import (
	"context"

	"github.com/koreaf16/argus/internal/services/llm"
)

type StopSummary struct {
	StopReason llm.StopReason
	ToolCalls  int
	Messages   int
}

type StopHook func(ctx context.Context, summary StopSummary)
