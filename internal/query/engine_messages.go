package query

import (
	"strings"

	"github.com/koreaf16/argus/internal/services/llm"
	ctxpkg "github.com/koreaf16/argus/internal/context"
)

func (e *Engine) Messages() []llm.Message {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return cloneMessages(e.messages)
}

func (e *Engine) TokenSnapshot() (input, output, thinking int) {
	return e.budget.Snapshot()
}

func (e *Engine) CumulativeTokenSnapshot() (input, output, thinking int) {
	return e.budget.CumulativeSnapshot()
}

func (e *Engine) ResetBudget() {
	e.budget.Reset()
}

func (e *Engine) ReplaceMessages(messages []llm.Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages = cloneMessages(messages)
	// Rebuild graph so RenderForLLM has history after a session restore.
	// Tool pairs are reconstructed from flat message content; ToolName is unknown
	// at this level but the text content and pair linkage are preserved.
	e.graph = ctxpkg.NewGraph()
	for _, m := range messages {
		switch m.Role {
		case llm.RoleUser:
			texts := make([]string, 0, len(m.Content))
			for _, cb := range m.Content {
				switch cb.Type {
				case llm.ContentText:
					if cb.Text != "" {
						texts = append(texts, cb.Text)
					}
				case llm.ContentToolResult:
					e.graph.AppendToolResult(cb.ToolUseID, "", cb.Text, ctxpkg.ProjectionFull, cb.IsError, "", "")
				}
			}
			if len(texts) > 0 {
				e.graph.AppendUser(strings.Join(texts, "\n"))
			}
		case llm.RoleAssistant:
			texts := make([]string, 0, len(m.Content))
			for _, cb := range m.Content {
				switch cb.Type {
				case llm.ContentText:
					if cb.Text != "" {
						texts = append(texts, cb.Text)
					}
				case llm.ContentToolUse:
					if len(texts) > 0 {
						e.graph.AppendAssistant(strings.Join(texts, "\n"))
						texts = texts[:0]
					}
					e.graph.AppendToolUse(cb.ID, cb.Name, cb.Input)
				}
			}
			if len(texts) > 0 {
				e.graph.AppendAssistant(strings.Join(texts, "\n"))
			}
		}
	}
}
