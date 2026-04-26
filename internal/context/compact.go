package context

import (
	"fmt"
	"strings"
)

const (
	// ShellRecentFullCount keeps only the latest N shell tool results with fuller context.
	ShellRecentFullCount = 2
	// OldToolResultClearedPlaceholder marks tool outputs compacted out of context.
	OldToolResultClearedPlaceholder = "[Old tool result content cleared]"
	defaultToolResultBudgetChars    = 3200
	shellRecentBudgetChars          = 6000
)

func compactNodesForContext(nodes []*Node) []*Node {
	out := cloneNodes(nodes)
	shellSeen := 0
	for i := len(out) - 1; i >= 0; i-- {
		nd := out[i]
		if nd == nil || nd.Kind != NodeKindToolResult {
			continue
		}
		text := strings.TrimSpace(nd.InlineText)
		if text == "" {
			text = strings.TrimSpace(nd.Text)
		}
		if text == "" {
			continue
		}

		if isShellTool(nd.ToolName) {
			shellSeen++
			if shellSeen > ShellRecentFullCount {
				nd.InlineText = buildClearedPlaceholder(nd)
				nd.Projection = ProjectionExcluded
				continue
			}
			nd.InlineText = applyToolBudget(text, shellRecentBudgetChars)
			if len(nd.InlineText) < len(text) && nd.Projection == ProjectionFull {
				nd.Projection = ProjectionPartial
			}
			continue
		}

		nd.InlineText = applyToolBudget(text, defaultToolResultBudgetChars)
		if len(nd.InlineText) < len(text) && nd.Projection == ProjectionFull {
			nd.Projection = ProjectionPartial
		}
	}
	return out
}

func applyToolBudget(text string, budget int) string {
	if budget <= 0 || len(text) <= budget {
		return text
	}
	keepHead := budget / 2
	keepTail := budget - keepHead
	return strings.TrimSpace(text[:keepHead]) + fmt.Sprintf(
		"\n\n...[context-compacted %d chars]...\n\n",
		len(text)-budget,
	) + strings.TrimSpace(text[len(text)-keepTail:])
}

func buildClearedPlaceholder(nd *Node) string {
	lines := []string{OldToolResultClearedPlaceholder}
	if strings.TrimSpace(nd.ArtifactPath) != "" {
		lines = append(lines, fmt.Sprintf("[Full output: %s]", strings.TrimSpace(nd.ArtifactPath)))
	} else if strings.TrimSpace(nd.ArtifactID) != "" {
		lines = append(lines, fmt.Sprintf("[Artifact: %s]", strings.TrimSpace(nd.ArtifactID)))
	}
	return strings.Join(lines, "\n")
}

func cloneNodes(nodes []*Node) []*Node {
	out := make([]*Node, 0, len(nodes))
	for _, nd := range nodes {
		if nd == nil {
			continue
		}
		cp := *nd
		if len(nd.ToolInput) > 0 {
			cp.ToolInput = make([]byte, len(nd.ToolInput))
			copy(cp.ToolInput, nd.ToolInput)
		}
		out = append(out, &cp)
	}
	return out
}
