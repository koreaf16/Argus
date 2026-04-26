// Package context — 장기 세션/대형 출력 관리를 위한 Episodic Context Graph.
//
// 파일 역할: Node 타입 계층 정의.
//
//	LLM 대화에서 발생하는 모든 이벤트를 구조화된 노드로 표현한다.
//	Anthropic/Gemini/OpenAI wire format은 RenderForLLM()의 projection으로만 생성된다.
package context

import (
	"encoding/json"
	"time"
)

// NodeKind 는 graph node의 종류다.
type NodeKind string

const (
	// Organic nodes — 실제 대화에서 생성됨
	NodeKindUser       NodeKind = "user"
	NodeKindAssistant  NodeKind = "assistant"
	NodeKindToolUse    NodeKind = "tool_use"
	NodeKindToolResult NodeKind = "tool_result"

	// Synthetic nodes — context management에서 생성됨
	NodeKindSummary     NodeKind = "summary"
	NodeKindArtifactRef NodeKind = "artifact_ref"
)

// Projection 은 tool_result node가 LLM에 전달될 때의 압축 수준이다.
type Projection string

const (
	ProjectionFull     Projection = "FULL"     // 전체 내용 inline
	ProjectionPartial  Projection = "PARTIAL"  // head+tail+symbol 유지
	ProjectionSummary  Projection = "SUMMARY"  // LLM/extractive 요약
	ProjectionExcluded Projection = "EXCLUDED" // ref만 남기고 내용 제외
)

// Node 는 graph의 기본 단위다.
type Node struct {
	// ID 는 turn-seq 기반 고유 식별자다 (예: "4-bash-abc123").
	ID   string    `json:"id"`
	Kind NodeKind  `json:"kind"`
	Seq  int       `json:"seq"` // graph 내 순서 (1부터)
	At   time.Time `json:"at"`

	// Organic fields
	Role string `json:"role,omitempty"` // "user" | "assistant"
	Text string `json:"text,omitempty"` // user/assistant/summary 본문

	// Tool fields
	ToolName   string          `json:"tool_name,omitempty"`
	CallID     string          `json:"call_id,omitempty"`
	ToolInput  json.RawMessage `json:"tool_input,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
	ToolUseSeq int             `json:"tool_use_seq,omitempty"` // 대응하는 tool_use의 Seq

	// tool_result 압축 관련
	InlineText string     `json:"inline_text,omitempty"` // context에 포함될 텍스트
	Projection Projection `json:"projection,omitempty"`  // 압축 수준
	ArtifactID string     `json:"artifact_id,omitempty"` // ArtifactStore의 키

	// artifact_ref 전용
	ArtifactPath  string `json:"artifact_path,omitempty"`
	OriginalChars int    `json:"original_chars,omitempty"`

	// Protected 는 이 노드가 consolidation에서 보호되는지 여부다.
	Protected bool `json:"protected,omitempty"`
}

// TurnID 는 이 노드가 속한 turn의 식별자를 반환한다.
// user 노드의 Seq을 기준으로 동일 turn의 노드를 묶는다.
func (n *Node) TurnID() int {
	return n.Seq
}

// IsOrganic 은 실제 대화에서 생성된 노드인지 반환한다.
func (n *Node) IsOrganic() bool {
	switch n.Kind {
	case NodeKindUser, NodeKindAssistant, NodeKindToolUse, NodeKindToolResult:
		return true
	}
	return false
}

// IsSynthetic 은 context management에서 생성된 노드인지 반환한다.
func (n *Node) IsSynthetic() bool {
	return !n.IsOrganic()
}

// EstimatedChars 는 이 노드가 LLM에 전달될 때의 예상 문자(바이트) 수다.
func (n *Node) EstimatedChars() int {
	switch n.Kind {
	case NodeKindUser, NodeKindAssistant:
		return len(n.Text)
	case NodeKindToolUse:
		return len(n.ToolName) + len(n.ToolInput) + 20
	case NodeKindToolResult:
		if n.InlineText != "" {
			return len(n.InlineText)
		}
		return len(n.Text)
	case NodeKindSummary:
		return len(n.Text)
	case NodeKindArtifactRef:
		return len(n.ArtifactPath) + 60
	}
	return 0
}

// EstimateTokens 는 CJK 문자를 인식해 이 노드의 추정 토큰 수를 반환한다.
// EstimatedChars()/4 보다 한글·한자 등이 포함된 텍스트에서 정확도가 높다.
func (n *Node) EstimateTokens() int {
	switch n.Kind {
	case NodeKindUser, NodeKindAssistant, NodeKindSummary:
		return estimateRunes(n.Text)
	case NodeKindToolUse:
		return estimateRunes(n.ToolName+string(n.ToolInput)) + 5
	case NodeKindToolResult:
		if n.InlineText != "" {
			return estimateRunes(n.InlineText)
		}
		return estimateRunes(n.Text)
	case NodeKindArtifactRef:
		return estimateRunes(n.ArtifactPath) + 15
	}
	return 0
}
