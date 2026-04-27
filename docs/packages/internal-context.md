# internal/context — 컨텍스트 관리

## 파일 구조

| 파일 | 역할 |
|------|------|
| `node.go` | `Node` 구조체, `NodeKind`(7종), `Projection`(4종) |
| `graph.go` | `Graph`: append/prune/protect/ForceConsolidate |
| `token_estimator.go` | `TokenEstimator`: chars→token 추정, budget 계산 |
| `renderer.go` | `RenderForLLM()`: Graph → `[]llm.Message` |
| `artifact.go` | `ArtifactStore`, `ArtifactManifest` |
| `distiller.go` | `Distiller`: FULL/PARTIAL/SUMMARY 3단계 압축 |

## Node

```go
type Node struct {
    ID           string
    Kind         NodeKind     // User, Assistant, Thinking, ToolUse, ToolResult, Summary, ArtifactRef
    Seq          int
    At           time.Time
    Role         string       // "user" / "assistant"
    Text         string
    CallID       string
    ToolName     string
    ToolInput    []byte
    InlineText   string
    Projection   Projection   // Full, Partial, Summary, Error
    IsError      bool
    ArtifactID   string
    ArtifactPath string
    Protected    bool
    OriginalChars int
}
```

## Graph

| 메서드 | 설명 |
|--------|------|
| `NewGraph()` | 빈 그래프 생성 |
| `AppendUser(text)` | User 노드 추가 |
| `AppendAssistant(text)` | Assistant 노드 추가 |
| `AppendThinking(text)` | Thinking 노드 추가 |
| `AppendToolUse(id, name, input)` | ToolUse 노드 추가 |
| `AppendToolResult(...)` | ToolResult 노드 추가 |
| `AppendSummary(text, replacedSeqs)` | Summary 노드 추가 |
| `MarkProtected(text)` | 최신 2턴 + tool 쌍 보호 |
| `ForceConsolidate(text)` | 비보호 노드 일괄 요약 |

## TokenEstimator

```
tokens = ascii/4 + (cjk*2+1)/3
```

CJK 문자(한글, 한자, 가나)는 ASCII보다 더 많은 토큰을消費하므로 별도 계산합니다.

## Distiller

| 단계 | 조건 | 동작 |
|------|------|------|
| Full | ≤ 8,000 chars | inline 그대로 |
| Partial | 8~20,000 chars | Head 80줄 + Tail 60줄 |
| Summary | > 20,000 chars | LLM 요약 (최대 250KB) |
| Extractive | 요약 실패 | Head 5 + Tail 5 줄 |

## ArtifactStore

- 경로: `~/.argus/session-artifacts/<sessionID>/<seq>-<tool>-<callID>.txt`
- 원본 raw output 전체 보존
- Graph에는 ID와 path만 참조

## RenderForLLM

```
RenderForLLM(graph, est, systemTokens, contextWindow, currentUserText)
  → MarkProtected()
  → BudgetFor()
  → compactNodesForContext()
  → selectNodes(budget)
  → projectToMessages()
  → []llm.Message
```
