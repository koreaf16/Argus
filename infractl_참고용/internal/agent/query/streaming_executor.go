package query

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/yourorg/infractl/internal/llm"
)

// StreamingExecutor executes tool batches and emits per-tool events immediately.
type StreamingExecutor struct {
	maxConcurrency int
}

// toolResultInfo carries non-message metadata for each executed tool.
type toolResultInfo struct {
	Name         string
	MetadataJSON string
}

// NewStreamingExecutor creates a new executor.
func NewStreamingExecutor(maxConcurrency int) *StreamingExecutor {
	if maxConcurrency <= 0 {
		maxConcurrency = MaxToolConcurrency()
	}
	return &StreamingExecutor{maxConcurrency: maxConcurrency}
}

// Execute runs batches in order.
func (se *StreamingExecutor) Execute(
	ctx context.Context,
	batches []Batch,
	out chan<- QueryEvent,
	runTool ToolRunner,
) (results []llm.Message, infos []toolResultInfo, aborted bool) {
	for _, batch := range batches {
		var batchResults []llm.Message
		var batchInfos []toolResultInfo
		if batch.Concurrent {
			batchResults, batchInfos, aborted = se.executeConcurrent(ctx, batch.Calls, out, runTool)
		} else {
			batchResults, batchInfos, aborted = se.executeSerial(ctx, batch.Calls, out, runTool)
		}
		results = append(results, batchResults...)
		infos = append(infos, batchInfos...)
		if aborted {
			return results, infos, true
		}
	}
	return results, infos, false
}

func (se *StreamingExecutor) executeConcurrent(
	ctx context.Context,
	calls []llm.ToolCall,
	out chan<- QueryEvent,
	runTool ToolRunner,
) ([]llm.Message, []toolResultInfo, bool) {
	type immediateResult struct {
		idx      int
		tc       llm.ToolCall
		output   string
		isError  bool
		metadata string
	}

	immediateOut := make(chan immediateResult, len(calls))
	msgs := make([]llm.Message, len(calls))
	infos := make([]toolResultInfo, len(calls))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(se.maxConcurrency)

	for i, tc := range calls {
		i, tc := i, tc
		g.Go(func() error {
			send(gctx, out, EventToolUseStart{ID: tc.ID, Name: tc.Function.Name, Input: tc.Function.Arguments})

			var output string
			var isError bool
			var metadata string
			if runTool != nil {
				output, isError, metadata = runTool(gctx, tc)
			} else {
				output = fmt.Sprintf("error: no tool runner for '%s'", tc.Function.Name)
				isError = true
			}
			immediateOut <- immediateResult{i, tc, output, isError, metadata}
			return nil
		})
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = g.Wait()
		close(immediateOut)
	}()

	for r := range immediateOut {
		msgs[r.idx] = llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: r.tc.ID,
			Content:    r.output,
		}
		infos[r.idx] = toolResultInfo{Name: r.tc.Function.Name, MetadataJSON: r.metadata}
		send(ctx, out, EventToolResult{
			ID:           r.tc.ID,
			Name:         r.tc.Function.Name,
			ToolCall:     r.tc,
			Output:       r.output,
			IsError:      r.isError,
			MetadataJSON: r.metadata,
		})
	}

	wg.Wait()
	return msgs, infos, ctx.Err() != nil
}

func (se *StreamingExecutor) executeSerial(
	ctx context.Context,
	calls []llm.ToolCall,
	out chan<- QueryEvent,
	runTool ToolRunner,
) ([]llm.Message, []toolResultInfo, bool) {
	results := make([]llm.Message, 0, len(calls))
	infos := make([]toolResultInfo, 0, len(calls))
	mutationFailed := false
	mutationFailReason := ""

	for _, tc := range calls {
		if ctx.Err() != nil {
			return results, infos, true
		}

		send(ctx, out, EventToolUseStart{ID: tc.ID, Name: tc.Function.Name, Input: tc.Function.Arguments})

		if mutationFailed {
			skipMsg := fmt.Sprintf("skipped due to sibling failure: %s", mutationFailReason)
			send(ctx, out, EventToolResult{
				ID:             tc.ID,
				Name:           tc.Function.Name,
				ToolCall:       tc,
				Output:         skipMsg,
				IsError:        true,
				SiblingSkipped: true,
			})
			results = append(results, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: tc.ID,
				Content:    skipMsg,
			})
			infos = append(infos, toolResultInfo{Name: tc.Function.Name})
			continue
		}

		var output string
		var isError bool
		var metadata string
		if runTool != nil {
			output, isError, metadata = runTool(ctx, tc)
		} else {
			output = fmt.Sprintf("error: no tool runner for '%s'", tc.Function.Name)
			isError = true
		}

		send(ctx, out, EventToolResult{
			ID:           tc.ID,
			Name:         tc.Function.Name,
			ToolCall:     tc,
			Output:       output,
			IsError:      isError,
			MetadataJSON: metadata,
		})
		results = append(results, llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: tc.ID,
			Content:    output,
		})
		infos = append(infos, toolResultInfo{Name: tc.Function.Name, MetadataJSON: metadata})

		if isError {
			mutationFailed = true
			mutationFailReason = fmt.Sprintf("'%s' returned error", tc.Function.Name)
			slog.Warn("serial tool failed, subsequent tools in batch will be skipped",
				"tool", tc.Function.Name,
			)
		}
	}

	return results, infos, false
}
