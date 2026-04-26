// 파일 역할: RenderForLLM — graph nodes → []llm.Message projection.
//
//	보호 노드를 우선 배치하고, 남은 budget으로 오래된 노드를 채운다.
//	budget 초과 시 비보호 노드를 summary로 축소한다 (synchronous backstop).
package context

import (
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/llm"
)

// RenderForLLM 은 graph를 budget 내의 []llm.Message로 변환한다.
//
//   - systemTokens: systemFn() 호출 결과의 추정 토큰 수
//   - contextWindow: 모델의 최대 context 토큰 수
//   - currentUserText: 현재 사용자 입력 (보호 마킹에 사용)
//
// 반환된 []llm.Message는 LLM API에 직접 전달 가능하다.
func RenderForLLM(g *Graph, est *TokenEstimator, systemTokens, contextWindow int, currentUserText string) []llm.Message {
	if g == nil {
		return nil
	}

	// 보호 마킹 갱신
	g.MarkProtected(currentUserText)

	budget := est.BudgetFor(contextWindow, systemTokens, len(currentUserText))
	if budget <= 0 {
		// budget이 0이면 보호 노드만이라도 포함
		budget = contextWindow / 4
	}

	nodes := g.Nodes()
	nodes = compactNodesForContext(nodes)

	// 1단계: budget 내에서 뒤(최신)부터 노드 선택
	selected := selectNodes(nodes, budget, est)

	// 2단계: 선택된 노드를 시간순으로 정렬하여 llm.Message 생성
	return projectToMessages(selected)
}

// selectNodes 는 budget 내에서 최신 노드부터 포함할 노드를 선택한다.
// 보호 노드는 항상 포함된다.
func selectNodes(nodes []*Node, budgetTokens int, est *TokenEstimator) []*Node {
	remaining := budgetTokens

	// 뒤에서부터 scan — 포함 여부 결정
	include := make([]bool, len(nodes))
	for i := len(nodes) - 1; i >= 0; i-- {
		nd := nodes[i]
		cost := nd.EstimateTokens()
		if cost == 0 {
			cost = 1
		}
		if nd.Protected || remaining >= cost {
			include[i] = true
			if !nd.Protected {
				remaining -= cost
			}
		}
	}

	out := make([]*Node, 0, len(nodes))
	for i, nd := range nodes {
		if include[i] {
			out = append(out, nd)
		}
	}
	return out
}

// projectToMessages 는 선택된 노드를 []llm.Message로 변환한다.
// tool_use / tool_result adjacency 보장: tool_result의 tool_use가 없으면 제외.
func projectToMessages(nodes []*Node) []llm.Message {
	// tool_use callID 추적
	presentCallIDs := map[string]bool{}
	for _, nd := range nodes {
		if nd.Kind == NodeKindToolUse {
			presentCallIDs[nd.CallID] = true
		}
	}

	messages := make([]llm.Message, 0, len(nodes))
	var pending llm.Message // 현재 accumulate 중인 메시지
	hasPending := false

	flush := func() {
		if hasPending {
			messages = append(messages, pending)
			pending = llm.Message{}
			hasPending = false
		}
	}

	for _, nd := range nodes {
		switch nd.Kind {
		case NodeKindUser:
			// user 텍스트는 단독 메시지
			flush()
			messages = append(messages, llm.TextMessage(llm.RoleUser, nd.Text))

		case NodeKindSummary:
			// summary는 user 역할의 텍스트 메시지
			flush()
			messages = append(messages, llm.TextMessage(llm.RoleUser,
				fmt.Sprintf("[대화 요약]\n%s", nd.Text)))

		case NodeKindArtifactRef:
			// artifact ref는 user 역할의 안내 메시지
			flush()
			messages = append(messages, llm.TextMessage(llm.RoleUser,
				fmt.Sprintf("[아티팩트 참조: %s (%d chars)]\n전체 내용: %s",
					nd.ArtifactID, nd.OriginalChars, nd.ArtifactPath)))

		case NodeKindAssistant:
			// assistant 텍스트 (tool_use가 뒤따를 수 있으므로 pending)
			flush()
			pending = llm.Message{
				Role: llm.RoleAssistant,
				Content: []llm.ContentBlock{
					{Type: llm.ContentText, Text: nd.Text},
				},
			}
			hasPending = true

		case NodeKindToolUse:
			// assistant 메시지에 tool_use block 추가
			if !hasPending {
				pending = llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{}}
				hasPending = true
			}
			pending.Content = append(pending.Content, llm.ContentBlock{
				Type:  llm.ContentToolUse,
				ID:    nd.CallID,
				Name:  nd.ToolName,
				Input: nd.ToolInput,
			})

		case NodeKindToolResult:
			// tool_use가 없는 고아 tool_result는 제외
			if !presentCallIDs[nd.CallID] {
				continue
			}
			flush()
			text := nd.InlineText
			if text == "" {
				text = nd.Text
			}
			if nd.ArtifactID != "" && nd.Projection != ProjectionFull && nd.ArtifactPath != "" && !strings.Contains(text, nd.ArtifactPath) {
				text = fmt.Sprintf("%s\n\n[전체 출력: %s]", text, nd.ArtifactPath)
			}
			messages = append(messages, llm.Message{
				Role: llm.RoleUser,
				Content: []llm.ContentBlock{
					{
						Type:      llm.ContentToolResult,
						ToolUseID: nd.CallID,
						Name:      nd.ToolName,
						Text:      text,
						IsError:   nd.IsError,
					},
				},
			})
		}
	}
	flush()

	// 첫 메시지는 반드시 user여야 함 (API 제약)
	for len(messages) > 0 && messages[0].Role != llm.RoleUser {
		messages = messages[1:]
	}

	return messages
}
