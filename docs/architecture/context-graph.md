# Episodic Context Graph

`internal/context` 패키지는 단순 선형 기록이 아닌 **그래프 구조**로 문맥을 관리하여 토큰 비용을 최소화하고 검색 효율을 높입니다.

## 1. Node 타입

| NodeKind | Role | 설명 |
|----------|------|------|
| `User` | user | 사용자 메시지 |
| `Assistant` | assistant | 어시스턴트 텍스트 응답 |
| `Thinking` | assistant | 어시스턴트 추론 과정 (thinking block) |
| `ToolUse` | assistant | 도구 호출 (tool_use block) |
| `ToolResult` | user | 도구 실행 결과 (tool_result block) |
| `Summary` | user | 여러 노드를 요약한 합성 노드 |
| `ArtifactRef` | user | 파일로 저장된 대형 출력 참조 |

## 2. Graph 구조

```go
type Graph struct {
    mu    sync.RWMutex
    nodes []*Node
    seq   int  // 다음에 부여할 seq (단조 증가)
}
```

### Node 구조

| 필드 | 타입 | 설명 |
|------|------|------|
| `ID` | string | 고유 ID |
| `Kind` | NodeKind | 노드 타입 |
| `Seq` | int | 시퀀스 번호 |
| `At` | time.Time | 생성 시간 |
| `Role` | string | "user" / "assistant" |
| `Text` | string | 텍스트 내용 |
| `CallID` | string | tool call ID |
| `ToolName` | string | 도구 이름 |
| `ToolInput` | []byte | 도구 입력 JSON |
| `InlineText` | string | context에 포함될 텍스트 |
| `Projection` | Projection | Full/Partial/Summary |
| `IsError` | bool | 에러 여부 |
| `ArtifactID` | string | 아티팩트 ID |
| `ArtifactPath` | string | 아티팩트 파일 경로 |
| `Protected` | bool | 보호 여부 (consolidation 제외) |
| `OriginalChars` | int | 원본 문자 수 |

### Graph 메서드

| 메서드 | 설명 |
|--------|------|
| `AppendUser(text)` | User 노드 추가 |
| `AppendAssistant(text)` | Assistant 노드 추가 |
| `AppendThinking(text)` | Thinking 노드 추가 |
| `AppendToolUse(id, name, input)` | ToolUse 노드 추가 |
| `AppendToolResult(...)` | ToolResult 노드 추가 |
| `AppendSummary(text, replacedSeqs)` | 요약 노드 추가 |
| `MarkProtected(text)` | 최신 2턴 + 마지막 tool 쌍 보호 |
| `ForceConsolidate(text)` | 비보호 노드 일괄 요약 |
| `NonProtectedSeqs()` | 보호되지 않은 시퀀스 반환 |

## 3. RenderForLLM 흐름

```
RenderForLLM(graph, est, systemTokens, contextWindow, currentUserText)
  │
  ├── MarkProtected(currentUserText)
  │     ├── 최신 2 full turn (User+Assistant) 보호
  │     ├── 마지막 tool_use/result 쌍 보호
  │     ├── user prompt 파일 경로 노드 보호
  │     └── plan/todo 관련 노드 보호 (TodoWrite, EnterPlanMode, ExitPlanMode)
  │
  ├── BudgetFor 계산
  │     budget = contextWindow
  │       - systemTokens
  │       - reservedOutput (20%)
  │       - userTokens
  │
  ├── compactNodesForContext()
  │     └── 노드 압축 (중복/인접 병합)
  │
  ├── selectNodes(budget)
  │     ├── 뒤(최신)부터 scan
  │     ├── Protected 노드 항상 포함
  │     └── budget 내 노드 선택
  │
  └── projectToMessages()
        ├── Node → []llm.Message 변환
        ├── tool_use/result 연결
        └── 고아 tool_result 제외
```

## 4. Distiller — 계층적 압축

```go
type Distiller struct {
    store       *ArtifactStore
    manifest    *ArtifactManifest
    summarizeFn func(toolName, content string) (string, error)
}
```

### 압축 단계

| 단계 | 조건 | 동작 |
|------|------|------|
| **Full** | ≤ 8,000 chars | inline 그대로 |
| **Partial** | 8~20,000 chars | Head 80줄 + Tail 60줄 + ArtifactStore 저장 |
| **Summary** | > 20,000 chars | LLM 요약 시도 (최대 250KB) |
| **Extractive** | 요약 실패/과대 | Head 5줄 + Tail 5줄 |

### 아티팩트 저장

- 경로: `~/.argus/session-artifacts/<sessionID>/<seq>-<tool>-<callID_prefix>.txt`
- 세션별 디렉토리 독립
- raw output 전체 보존
- Graph에는 `artifact_id`, `artifact_path`만 참조로 유지

## 5. TokenEstimator (CJK 대응)

### 핵심 공식

```
tokens = ascii/4 + (cjk*2+1)/3
```

### 판별 기준

| 문자 | 토큰 추정 |
|------|-----------|
| ASCII (r < 0x80) | 4 chars/token |
| 한글 (Hangul) | ~1.5 chars/token |
| 한자 (Han) | ~1.5 chars/token |
| 가나 (Hiragana/Katakana) | ~1.5 chars/token |
| 기타 | ASCII 취급 |

**왜 CJK 별도 계산인가?**
- 단순 `chars/4` 로 추정 시 최대 2.5배 과소 추정
- 과소 추정 → Context Overflow

### BudgetFor 공식

```
budget = contextWindow
  - systemTokens
  - reservedOutput (20%)
  - userTokens (3 bytes/token)
```

## 6. Context 초과 처리

1. `CountTokens` 결과 ≥ contextWindow → `ForceConsolidate()` (비보호 노드를 summary로 교체)
2. 재 `RenderForLLM` 후 LLM 호출 재시도
3. `/snip` 명령 → 동일하게 `ForceConsolidate()` 위임
