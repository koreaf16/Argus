# Query Engine Design

## 상태

Phase B 완료 기준 문서다. 기존 `loop_llm.go` 기반 반복 루프는 제거하고, `internal/agent/query` 패키지의 state machine + streaming event channel 경로로 교체했다.

## 목표

- LLM 호출, 스트리밍 토큰, 도구 실행, terminal reason을 하나의 명시적 상태기계로 관리한다.
- 도구 결과는 완료되는 즉시 `QueryEvent`로 yield한다.
- 읽기 전용 도구는 병렬 실행하고, mutation 도구는 순차 실행한다.
- mutation 도구 실패 시 같은 serial batch의 후속 mutation 도구를 sibling skip 처리한다.
- 분류 단계는 query 진입 전에 1회만 수행한다.
- TUI는 query event sink를 통해 streaming/token/tool/terminal 이벤트를 소비한다.

## 실행 흐름

1. `Agent.Run()`이 사용자 입력을 history에 추가한다.
2. `runSelfClassification()`이 `classify_request`를 1회 호출한다.
3. 분류 결과로 tool group, prompt section, model tier를 결정한다.
4. `BuildContextualLayoutAt()`이 system prompt를 조합한다.
5. `runWithEngine()`이 `query.Params`를 만들고 `query.Engine.Run()`을 호출한다.
6. `consumeEngineEvents()`가 `QueryEvent`를 history, session store, TUI sink, legacy handler callback으로 라우팅한다.

## QueryEvent

`Engine.Run()`은 `<-chan QueryEvent`를 반환한다. sender만 채널을 닫고, 마지막 이벤트는 항상 `EventTerminal`이어야 한다.

- `EventStreamStart`: LLM stream 요청 시작
- `EventAssistantChunk`: thinking/token chunk
- `EventAssistantResponse`: 한 turn의 assistant 응답과 tool call 목록
- `EventToolUseStart`: 도구 실행 시작
- `EventToolResult`: 도구 결과 즉시 yield
- `EventError`: model/tool 경로의 비정상 이벤트
- `EventTerminal`: `completed`, `interrupted`, `model_error`, `max_turns`, `aborted_tools` 등 종료 사유

## State Machine

`Engine.runLoop()`은 최대 turn 수까지 반복한다.

1. context 취소 확인
2. stream start event 발행
3. `Client.ChatStream()` 호출
4. assistant chunk event 발행
5. tool call이 없으면 `TerminalCompleted`
6. tool call이 있으면 `PartitionToolCalls()`로 batch 구성
7. `StreamingExecutor.Execute()`로 batch 실행
8. assistant/tool message를 query state에 반영하고 다음 turn으로 이동
9. 최대 turn 도달 시 `TerminalMaxTurns`

Phase C 전까지 compaction은 noop 위치만 유지한다.

## Tool Execution

`PartitionToolCalls()`는 registry의 `Tool.IsReadOnly()`를 기준으로 batch를 나눈다.

- 연속 read-only tool: concurrent batch
- 연속 mutation tool: serial batch
- registry가 없거나 알 수 없는 tool: 보수적으로 serial batch

`StreamingExecutor`는 concurrent batch에서 결과 순서를 강제하지 않고, 완료 즉시 `EventToolResult`를 발행한다. serial batch에서는 첫 실패 이후 같은 batch의 후속 mutation을 실행하지 않고 `SiblingSkipped=true` 결과를 발행한다.

## Hook 자리

`query.ToolInvoker`는 Phase B에서 PreToolUse/PostToolUse 호출 자리를 마련한다.

- `RunPreToolUse()`가 deny하면 tool 실행 없이 error result를 반환한다.
- `NewInput`이 있으면 tool arguments를 교체한다.
- 실제 정책 활성화와 preflight 흡수는 Phase D 범위다.

## TUI Adapter

`internal/tui/query_adapter.go`의 `TUIQueryEventSink`가 query events를 bubbletea message로 변환한다.

- stream start -> thinking start
- assistant chunk -> token/thinking token
- error -> error message
- terminal/tool start/tool result는 현재 `consumeEngineEvents()`와 기존 `executeSingleTool()` callback이 담당한다.

## 검증 매트릭스

| 요구 시나리오 | 검증 |
|---|---|
| 단순 도구 1회 -> `completed` | `TestGolden_01_EchoOnce`, `TestPhaseB_E2EAgentRunUsesQueryEngineAndClassifiesOnce` |
| 기존 사용자 흐름과 동일한 Agent 경로 | `TestPhaseB_E2EAgentRunUsesQueryEngineAndClassifiesOnce` |
| read-only 5개 병렬, 결과 모두 도착 | `TestPhaseB_E2EReadOnlyFiveParallelAllResultsArrive` |
| mutation 실패 -> sibling skip | `TestGolden_04_MutationFailSiblingSkip` |
| 100 turn 장기 세션 안정성 | `TestPhaseB_E2ELongSession100TurnsStable` |
| ctx cancel -> `interrupted` | `TestGolden_05_CtxCancelInterrupted` |
| query 진입 전 classification 1회 | `TestPhaseB_E2EAgentRunUsesQueryEngineAndClassifiesOnce` |
| interactive/PTY shell path 보존 | `TestPhaseB_E2EInteractiveShellExecStillUsesPTYStream` |

통과 확인 명령:

```powershell
go test ./internal/agent/query
go test ./internal/agent
go test ./internal/tui
go build -o bin\infractl.exe ./cmd/infractl/
```
