package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/store"
)

const (
	taskMemoryFetchLimit = 300
	taskMemoryMaxChars   = 1200
)

func prefetchTaskMemoryAsync(
	ctx context.Context,
	knowledgeStore store.KnowledgeStore,
	execStore store.ExecLogStore,
	userInput string,
	activeServer string,
) <-chan string {
	ch := make(chan string, 1)
	if (knowledgeStore == nil && execStore == nil) || strings.TrimSpace(userInput) == "" {
		ch <- ""
		return ch
	}
	go func() {
		ch <- buildTaskMemoryContext(ctx, knowledgeStore, execStore, userInput, activeServer)
	}()
	return ch
}

func buildTaskMemoryContext(
	ctx context.Context,
	knowledgeStore store.KnowledgeStore,
	execStore store.ExecLogStore,
	userInput string,
	activeServer string,
) string {
	taskKey := buildTaskKey(userInput)
	if taskKey == "" {
		return ""
	}
	if knowledgeStore != nil {
		if text := buildTaskMemoryFromKnowledge(ctx, knowledgeStore, taskKey, activeServer); text != "" {
			return text
		}
	}
	if execStore == nil {
		return ""
	}
	logs, err := execStore.ListExecutionLogs(ctx, 0, taskMemoryFetchLimit)
	if err != nil || len(logs) == 0 {
		return ""
	}

	var latestSuccess *store.ExecutionLog
	failures := make([]store.ExecutionLog, 0, 2)
	seenFailure := map[string]struct{}{}
	for i := range logs {
		l := logs[i]
		if l.TaskKey == "" || l.TaskKey != taskKey {
			continue
		}
		if activeServer != "" && l.TargetServer != "" && !strings.EqualFold(l.TargetServer, activeServer) {
			continue
		}
		if l.Success {
			if latestSuccess == nil {
				cp := l
				latestSuccess = &cp
			}
			continue
		}
		key := l.CommandKey + "|" + l.FailureSig
		if _, exists := seenFailure[key]; exists {
			continue
		}
		seenFailure[key] = struct{}{}
		failures = append(failures, l)
		if len(failures) >= 2 {
			break
		}
	}

	if latestSuccess == nil && len(failures) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 관련 작업 메모리\n")
	sb.WriteString("새 명령 실행 전에 이전 결과를 참고 자료로 활용한다.\n\n")
	if latestSuccess != nil {
		sb.WriteString(fmt.Sprintf("- 이전 성공 방법: `%s` on `%s`.\n",
			nonEmpty(latestSuccess.CommandKey, latestSuccess.ToolName),
			nonEmpty(latestSuccess.TargetServer, "localhost"),
		))
	}
	for _, f := range failures {
		sb.WriteString(fmt.Sprintf("- 실패 패턴 회피: `%s` -> `%s`.\n",
			nonEmpty(f.CommandKey, f.ToolName),
			nonEmpty(f.FailureSig, f.ErrorMessage),
		))
	}

	text := strings.TrimSpace(sb.String())
	if len(text) > taskMemoryMaxChars {
		text = text[:taskMemoryMaxChars]
	}
	return text
}

func buildTaskMemoryFromKnowledge(
	ctx context.Context,
	knowledgeStore store.KnowledgeStore,
	taskKey string,
	activeServer string,
) string {
	successes, err := knowledgeStore.SearchTaskMemories(ctx, "task_success", strings.TrimSpace(activeServer), taskKey, "", 3)
	if err != nil {
		return ""
	}
	failures, err := knowledgeStore.SearchTaskMemories(ctx, "task_failure", strings.TrimSpace(activeServer), taskKey, "", 2)
	if err != nil {
		return ""
	}
	if len(successes) == 0 && len(failures) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 관련 작업 메모리\n")
	sb.WriteString("새 명령 실행 전에 이전 결과를 참고 자료로 활용한다.\n\n")
	for _, s := range successes {
		cmd := strings.TrimSpace(s.SuccessCommand)
		if cmd == "" {
			cmd = nonEmpty(s.CommandKey, s.ToolName)
		}
		sb.WriteString(fmt.Sprintf("- 이전 성공 방법: `%s` on `%s`.\n",
			cmd,
			nonEmpty(s.ServerName, "localhost"),
		))
	}
	for _, f := range failures {
		pattern := strings.TrimSpace(f.ErrorPattern)
		if pattern == "" {
			pattern = strings.TrimSpace(f.Resolution)
		}
		sb.WriteString(fmt.Sprintf("- 실패 패턴 회피: `%s` -> `%s`.\n",
			nonEmpty(f.CommandKey, f.ToolName),
			nonEmpty(pattern, "known failure"),
		))
	}

	text := strings.TrimSpace(sb.String())
	if len(text) > taskMemoryMaxChars {
		text = text[:taskMemoryMaxChars]
	}
	return text
}

func nonEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}
