# 수정 계획: 쉘 명령어 렌더링 순서 보장 (Dynamic Inline Rendering)

## Objective
`bash` 등의 쉘 도구 실행 완료 후, AI의 새로운 응답 텍스트가 쉘 박스보다 위쪽 터미널 스크롤백 영역에 먼저 렌더링되는 문제(순서 역전 현상)를 해결합니다.

## Key Files & Context
- **`internal/tui/model.go`**:
  - `scrollbackCmd`: 스트리밍 델타(assistant, thinking)가 도착할 때 터미널에 텍스트를 출력하는 진입점입니다.
  - `forcePromoteUpTo` (새로 추가): 특정 스트림 인덱스(예: AI 답변 시작 인덱스) 이전에 있는 모든 보류 중인 도구(특히 500ms 지연 종료 대기 중인 쉘 박스)를 강제로 완료 상태로 전환하고 스크롤백에 즉시 인쇄하도록 승격(Promote)하는 함수입니다.

## Implementation Steps
1. **`forcePromoteUpTo` 메서드 추가 (`internal/tui/model.go`)**:
   - `streamIdx`를 인자로 받아, 현재 출력되지 않은(`lastPrintedIdx`부터 `streamIdx` 전까지) 모든 엔트리를 검사합니다.
   - 만약 아직 종료 대기 중인 인터랙티브 쉘 박스(`FinishPending == true`)가 있다면 즉시 `SetFinished(true)`를 호출하여 종료 상태로 만듭니다.
   - 그 후, `promotePendingScrollback()`을 호출하여 해당 쉘 박스를 화면 스크롤백(tea.Println) 영역으로 즉시 밀어올립니다.

2. **`scrollbackCmd` 업데이트 (`internal/tui/model.go`)**:
   - `presentation.EventAssistantDelta` 및 `presentation.EventThinkingDelta` 이벤트 처리 시, 스트리밍 텍스트를 출력(`flushStreamingLines`)하기 **직전**에 새로 만든 `forcePromoteUpTo`를 호출하도록 수정합니다.
   - 이를 통해 AI의 텍스트가 화면에 찍히기 전에 항상 쉘 박스가 먼저 터미널 위쪽으로 인쇄(승격)되는 것을 보장합니다.

## Verification & Testing
1. 앱 재기동 후 터미널 명령어(`server_inspect`, `bash` 등)를 실행합니다.
2. 실행 직후 `bash` 쉘 박스가 화면에 표시되고, 그 즉시 AI의 추가 답변(`✦ step` 또는 텍스트)이 쉘 박스 **아래쪽**에 순차적으로 자연스럽게 스트리밍되는지(위쪽으로 텍스트가 덧붙여지지 않는지) 확인합니다.
