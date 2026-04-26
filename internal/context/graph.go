// 파일 역할: Episodic Context Graph — node append, prune, protected window 관리.
package context

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// ProtectedTurns 는 consolidation에서 보호할 최신 full turn 수다.
	ProtectedTurns = 2
)

// Graph 는 대화 전체를 Node 슬라이스로 관리하는 Episodic Context Graph다.
// seq 카운터는 단조 증가하며, 삭제 없이 synthetic node로 교체한다.
type Graph struct {
	mu    sync.RWMutex
	nodes []*Node
	seq   int // 다음에 부여할 seq
}

// NewGraph 는 빈 graph를 반환한다.
func NewGraph() *Graph {
	return &Graph{nodes: make([]*Node, 0, 64)}
}

// Len 은 현재 노드 수를 반환한다.
func (g *Graph) Len() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// Nodes 는 현재 노드 슬라이스의 얕은 복사본을 반환한다.
func (g *Graph) Nodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]*Node, len(g.nodes))
	copy(out, g.nodes)
	return out
}

// AppendUser 는 user 메시지 노드를 추가한다.
func (g *Graph) AppendUser(text string) *Node {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := &Node{
		ID:   g.nextID("user"),
		Kind: NodeKindUser,
		Seq:  g.nextSeq(),
		At:   time.Now(),
		Role: "user",
		Text: text,
	}
	g.nodes = append(g.nodes, n)
	return n
}

// AppendAssistant 는 assistant 메시지 노드를 추가한다.
func (g *Graph) AppendAssistant(text string) *Node {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := &Node{
		ID:   g.nextID("assistant"),
		Kind: NodeKindAssistant,
		Seq:  g.nextSeq(),
		At:   time.Now(),
		Role: "assistant",
		Text: text,
	}
	g.nodes = append(g.nodes, n)
	return n
}

// AppendToolUse 는 tool_use 노드를 추가한다.
func (g *Graph) AppendToolUse(callID, toolName string, input []byte) *Node {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := &Node{
		ID:        g.nextID("tool_use"),
		Kind:      NodeKindToolUse,
		Seq:       g.nextSeq(),
		At:        time.Now(),
		CallID:    callID,
		ToolName:  toolName,
		ToolInput: input,
	}
	g.nodes = append(g.nodes, n)
	return n
}

// AppendToolResult 는 tool_result 노드를 추가한다.
// inlineText 는 distiller가 처리한 뒤 context에 포함될 텍스트다.
// proj 가 FULL이면 원본 전체, PARTIAL이면 head+tail, SUMMARY이면 요약문이다.
func (g *Graph) AppendToolResult(callID, toolName, inlineText string, proj Projection, isError bool, artifactID, artifactPath string) *Node {
	g.mu.Lock()
	defer g.mu.Unlock()
	// 대응하는 tool_use seq 검색
	useSeq := 0
	for i := len(g.nodes) - 1; i >= 0; i-- {
		if g.nodes[i].Kind == NodeKindToolUse && g.nodes[i].CallID == callID {
			useSeq = g.nodes[i].Seq
			break
		}
	}
	n := &Node{
		ID:           g.nextID("tool_result"),
		Kind:         NodeKindToolResult,
		Seq:          g.nextSeq(),
		At:           time.Now(),
		CallID:       callID,
		ToolName:     toolName,
		InlineText:   inlineText,
		Projection:   proj,
		IsError:      isError,
		ArtifactID:   artifactID,
		ArtifactPath: artifactPath,
		ToolUseSeq:   useSeq,
	}
	g.nodes = append(g.nodes, n)
	return n
}

// AppendSummary 는 여러 노드를 대체하는 summary 노드를 추가한다.
// replacedSeqs 에 있는 노드들은 graph에서 제거되고 summary 노드는
// 첫 번째 제거 노드의 위치에 삽입된다(다중 라운드에서 순서 유지).
func (g *Graph) AppendSummary(text string, replacedSeqs []int) *Node {
	g.mu.Lock()
	defer g.mu.Unlock()

	replaced := make(map[int]bool, len(replacedSeqs))
	for _, s := range replacedSeqs {
		replaced[s] = true
	}

	n := &Node{
		ID:   g.nextID("summary"),
		Kind: NodeKindSummary,
		Seq:  g.nextSeq(),
		At:   time.Now(),
		Text: text,
	}

	// 단일 패스: 첫 번째 제거 노드 위치에 summary를 삽입하고 나머지 제거.
	// 다중 라운드 consolidation 시 summary들의 시간 순서를 보존한다.
	out := make([]*Node, 0, len(g.nodes)+1)
	inserted := false
	for _, nd := range g.nodes {
		if replaced[nd.Seq] {
			if !inserted {
				out = append(out, n)
				inserted = true
			}
			continue
		}
		out = append(out, nd)
	}
	if !inserted {
		out = append(out, n)
	}
	g.nodes = out
	return n
}

// AppendArtifactRef 는 artifact 참조 노드를 추가한다.
func (g *Graph) AppendArtifactRef(artifactID, artifactPath string, originalChars int) *Node {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := &Node{
		ID:            g.nextID("artifact_ref"),
		Kind:          NodeKindArtifactRef,
		Seq:           g.nextSeq(),
		At:            time.Now(),
		ArtifactID:    artifactID,
		ArtifactPath:  artifactPath,
		OriginalChars: originalChars,
	}
	g.nodes = append(g.nodes, n)
	return n
}

// MarkProtected 는 최신 ProtectedTurns 개의 full turn과 마지막 tool 쌍을 보호 표시한다.
// currentUserText 에서 언급된 파일 경로를 포함하는 노드도 보호한다.
func (g *Graph) MarkProtected(currentUserText string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, nd := range g.nodes {
		nd.Protected = false
	}

	// 보호할 파일 path 집합 (간단히 "/" 또는 "\" 포함 단어 추출)
	protectedPaths := extractPaths(currentUserText)

	// 뒤에서부터 scan
	turnCount := 0
	lastToolUseSeq := -1

	// 1패스: 보호 여부 결정
	for i := len(g.nodes) - 1; i >= 0; i-- {
		nd := g.nodes[i]

		// 최신 2 full turn 보호 (user 노드 기준으로 턴 카운트)
		if nd.Kind == NodeKindUser {
			turnCount++
		}
		if turnCount <= ProtectedTurns {
			nd.Protected = true
		}

		// 마지막 tool_use → tool_result 쌍 보호
		if nd.Kind == NodeKindToolResult && lastToolUseSeq < 0 {
			nd.Protected = true
			lastToolUseSeq = nd.ToolUseSeq
		}
		if nd.Kind == NodeKindToolUse && nd.Seq == lastToolUseSeq {
			nd.Protected = true
		}

		// 현재 user prompt 언급 파일 path 포함 노드 보호
		if len(protectedPaths) > 0 && nd.Kind == NodeKindToolResult {
			for _, p := range protectedPaths {
				if strings.Contains(nd.InlineText, p) || strings.Contains(nd.ArtifactPath, p) {
					nd.Protected = true
					break
				}
			}
		}

		// plan/todo 관련 노드 보호
		if nd.Kind == NodeKindToolUse &&
			(strings.EqualFold(nd.ToolName, "todowrite") ||
				strings.EqualFold(nd.ToolName, "enterplanmode") ||
				strings.EqualFold(nd.ToolName, "exitplanmode")) {
			nd.Protected = true
		}
	}
}

// NonProtectedSeqs 는 보호되지 않은 노드의 Seq 목록을 오래된 순으로 반환한다.
func (g *Graph) NonProtectedSeqs() []int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]int, 0, len(g.nodes))
	for _, nd := range g.nodes {
		if !nd.Protected {
			out = append(out, nd.Seq)
		}
	}
	return out
}

// ForceConsolidate 는 전체 비보호 노드를 summary 하나로 교체한다.
// summary 텍스트는 호출자가 제공한다.
func (g *Graph) ForceConsolidate(summaryText string) {
	seqs := g.NonProtectedSeqs()
	if len(seqs) == 0 {
		return
	}
	g.AppendSummary(summaryText, seqs)
}

// EstimatedTokens 는 현재 모든 노드의 추정 토큰 수다 (chars/4 근사).
func (g *Graph) EstimatedTokens() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	total := 0
	for _, nd := range g.nodes {
		total += nd.EstimatedChars()
	}
	return total / 4
}

// FindByCallID 는 callID에 해당하는 tool_result 노드를 반환한다.
func (g *Graph) FindByCallID(callID string) *Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for i := len(g.nodes) - 1; i >= 0; i-- {
		if g.nodes[i].CallID == callID {
			return g.nodes[i]
		}
	}
	return nil
}

// --- internal helpers ---

func (g *Graph) nextSeq() int {
	g.seq++
	return g.seq
}

func (g *Graph) nextID(kind string) string {
	return fmt.Sprintf("%d-%s", g.seq+1, kind)
}

// extractPaths 는 텍스트에서 파일 경로처럼 보이는 단어를 추출한다.
func extractPaths(text string) []string {
	words := strings.Fields(text)
	out := make([]string, 0)
	for _, w := range words {
		if strings.ContainsAny(w, "/\\") && len(w) > 2 {
			out = append(out, w)
		}
	}
	return out
}
