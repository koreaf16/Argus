# 전체 데이터 흐름 (End-to-End Data Flow)

## 사용자 → 최종 응답까지의 흐름

```
User                 TUI              Engine
 │                    │                 │
 │── 입력 ────────────▶│                 │
 │                    │── SubmitMessage ──▶│
 │                    │                 │── HookDispatch
 │                    │                 │   (UserPrompt)
 │                    │                 │── graph.AppendUser
 │                    │                 │
 │                    │                 │── MarkProtected
 │                    │                 │── RenderForLLM
 │                    │                 │   ┌──────────────┐
 │                    │                 │   │ selectNodes  │
 │                    │                 │   │ projectToMsg │
 │                    │                 │   └──────────────┘
 │                    │                 │
 │                    │                 │── LLM.Stream()───▶ LLM Provider
 │                    │                 │   │               │
 │                    │◀────────────────│───┤               │
 │                    │  ThinkingDelta  │   │               │
 │                    │◀────────────────│───┤               │
 │                    │  TextDelta      │   │               │
 │                    │◀────────────────│───┤               │
 │                    │  ToolUseStart   │   │               │
 │                    │                 │   │── Stop ──────▶│
 │                    │                 │   │               │
 │                    │                 │◀──┘               │
 │                    │                 │
 │                    │                 │── ToolRegistry.Lookup
 │                    │                 │── CheckPermission
 │                    │                 │── Tool.Call()────▶ Tool
 │                    │                 │   │               │
 │                    │◀────────────────│───┤ ToolEvent     │
 │                    │  ToolDelta      │   │               │
 │                    │                 │   │               │
 │                    │                 │◀──┘               │
 │                    │                 │
 │                    │                 │── Distiller.Distill
 │                    │                 │   ┌──────────────┐
 │                    │                 │   │ ArtifactStore│
 │                    │                 │   │ .Save()      │
 │                    │                 │   └──────────────┘
 │                    │                 │── graph.AppendToolResult
 │                    │                 │
 │                    │◀────────────────│── UIEventToolResult
 │                    │                 │
 │                    │◀────────────────│── UIEventDone
 │◀── 최종 응답 ──────│                 │
```

## 계층별 흐름

### 1. 입력 계층 (TUI)
1. 사용자가 텍스트 입력 + Enter
2. `model.go`의 `Update()`가 키 이벤트를 포착
3. `SubmitMessage()` 호출

### 2. 훅 계층 (Hooks)
1. `HookDispatcher.Dispatch(UserPrompt)` 실행
2. 조건부 훅 필터링 → once 체크 → async/동기 실행
3. `Continue=false`면 차단, `true`면 계속

### 3. 컨텍스트 계층 (Context)
1. `graph.AppendUser(text)` — User 노드 추가
2. `MarkProtected()` — 최신 2턴 + tool 쌍 보호
3. `RenderForLLM()` — budget 내 Node 선택 → `[]llm.Message`

### 4. LLM 계층 (Services/LLM)
1. `client.Stream(req)` — SSE 스트림 시작
2. 이벤트 처리:
   - `EventThinkingDelta` → TUI에 추론 표시
   - `EventTextDelta` → TUI에 텍스트 표시
   - `EventToolUseStart` → Tool 호출 큐에 추가
   - `EventStop` → stopReason 추출

### 5. Tool 계층 (Tools)
1. `Registry.Lookup(name)` — 도구 조회
2. `CheckPermission()` — 권한 체크
3. `Tool.Call(input)` — 도구 실행
4. `<-chan ToolEvent` — 스트리밍 결과 수신

### 6. 압축 계층 (Context/Distiller)
1. `Distiller.Distill()` — 3단계 압축
   - Full (≤8K): inline 그대로
   - Partial (8~20K): head+tail
   - Summary (>20K): LLM 요약
2. `ArtifactStore.Save()` — 원본 파일 저장
3. `graph.AppendToolResult()` — 결과 노드 추가

### 7. 출력 계층 (TUI)
1. `UIEventToolResult` → Tool 결과를 transcript에 출력
2. `UIEventDone` → 턴 종료 표시
3. 다음 입력 대기
