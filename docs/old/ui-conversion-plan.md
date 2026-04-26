# Claude 스타일 UI 전환 계획

## 진행 현황 (2026-04-22)

- 기본 TUI 엔진이 `readline` 출력 루프에서 Bubble Tea 기반 full-screen 루프로 전환되었다.
- transcript viewport + multiline input + footer + modal(권한/확인/비밀번호) 기본 구조가 구현되었다.
- `query.Engine` 이벤트 스트림, slash command 출력, plan step 실행 결과가 모두 상태 기반 transcript row로 렌더링되도록 연결되었다.
- transcript 검색(` / `), 매치 이동(`n/N`), transcript mode 토글(`Ctrl+O`), show-all 토글(`Ctrl+E`)이 추가되었다.
- viewport 하단 이탈 시 unseen divider(신규 메시지 구분선) 기본 동작이 추가되었다.
- 입력창에서 `Ctrl+R`로 최근 프롬프트 히스토리를 순환할 수 있다.
- `?` 도움말 오버레이가 추가되어 주요 키바인딩을 풀스크린에서 바로 확인할 수 있다.
- 슬래시 입력(`/<prefix>`) 시 명령 제안 오버레이가 표시되고, `Tab`/`Ctrl+N`/`Ctrl+P`로 선택할 수 있다.
- transcript 우측에 Tasks/Plan 상태 패널이 추가되었고, `Ctrl+B`로 표시를 토글할 수 있다.
- `--legacy-repl` 경로는 호환을 위해 유지된다.

## 목표

Argus에 `Claude Code`와 최대한 비슷한 형태의 UI를 먼저 적용한다.

우선순위는 아래와 같다.

- 내부 구현을 Claude와 똑같이 만드는 것보다, 사용자가 보는 화면 구조와 흐름을 먼저 맞춘다.
- 기존 Argus의 엔진, 툴, 상태 계층은 최대한 유지한다.
- Claude의 React/Ink 렌더러를 1:1로 포팅하려고 하지 않는다.

1차 목표 UI는 다음과 같다.

- 상단 transcript 영역
- 하단 고정 prompt input
- footer 또는 status strip
- 현재 permission mode 표시
- task 또는 todo 요약 표시
- assistant, tool use, tool result, notice, error가 row 단위로 보이는 화면

## 현재 Argus UI 상태

현재 Argus는 fullscreen TUI가 아니라 `readline` 기반 REPL이다.

근거:

- `internal/repl/repl.go`
  - `readline`으로 한 줄씩 입력을 받는다.
  - `query.UIEvent`를 `fmt.Fprint`로 바로 출력한다.
  - 프롬프트는 `[%s] [%s] > ` 수준의 단순 문자열이다.
- `internal/repl/render.go`
  - lipgloss 색상 스타일만 제공한다.
- `internal/query/stream.go`
  - UI에 필요한 기본 이벤트 스트림은 이미 있다.
  - `assistant_delta`, `tool_use`, `tool_result`, `notice`, `error`, `done`, `password_prompt`
- `internal/state/*`
  - model, session, permission mode, todo 상태를 이미 저장하고 있다.
- `cmd/argus/main.go`
  - 전체 부팅 후 `repl.Run`으로 진입한다.

현재 구조의 핵심 한계:

- transcript와 input이 분리된 레이아웃이 없다.
- fullscreen UI가 없다.
- permission modal UI가 없다.
- footer 또는 status line이 없다.
- slash command 결과가 transcript row가 아니라 stdout 텍스트로 출력된다.
- `query.Deps`에는 `ApproveTool.Prompt`가 있지만, `cmd/argus/main.go`에서는 아직 `WorkingDir`만 설정한다.

## Claude 스타일 UI가 의미하는 것

우리가 먼저 가져와야 하는 것은 Claude의 렌더러가 아니라 사용자 경험 구조다.

1. 고정 레이아웃
   - transcript와 input이 서로 다른 영역을 가진다.
   - input 아래 또는 주변에 footer와 hint가 붙는다.

2. 상태가 보이는 입력창
   - 현재 mode, model, permission mode, task 상태가 prompt 근처에 보인다.

3. row 기반 transcript
   - user
   - assistant
   - tool use
   - tool result
   - notice 또는 error
   - plan 또는 todo 관련 row

4. permission 상호작용의 UI화
   - 단순 stdin 확인이 아니다.
   - 어떤 툴이 왜 실행되려는지 보여주는 dialog 형태가 필요하다.

5. multiline input의 기본 지원
   - 한 줄 입력이 아니라 textarea 중심 입력이어야 한다.

## 권장 기술 방향

### `readline`을 유지하지 말고 fullscreen TUI로 전환

현재 REPL 위에 스타일만 더 입히는 방식으로는 Claude 형태를 제대로 만들기 어렵다.

그 이유는 아래 기능이 필요하기 때문이다.

- 하단 고정 input
- 스크롤 가능한 transcript viewport
- modal overlay
- footer 또는 status strip
- 전체 키 입력 제어

권장 스택:

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles/viewport`
- `github.com/charmbracelet/bubbles/textarea`
- `github.com/charmbracelet/bubbles/help`
- 기존 `github.com/charmbracelet/lipgloss`

### Claude의 Ink 트리를 1:1로 포팅하지 않는다

로컬 참고 소스 `claude_cli_참고용`은 React/Ink 기반으로 UI가 매우 크게 나뉘어 있다.

- `Messages.tsx`
- `VirtualMessageList.tsx`
- `PromptInput.tsx`
- `PromptInputFooter.tsx`
- permission 관련 컴포넌트
- overlay, footer context

Argus는 내부 컴포넌트 개수를 따라갈 필요가 없다.
대신 아래 식으로 개념만 매핑하는 편이 맞다.

- Claude `Messages.tsx` + `VirtualMessageList.tsx`
  - Argus transcript viewport
- Claude `PromptInput.tsx`
  - Argus textarea 기반 prompt composer
- Claude `PromptInputFooter.tsx` + status line
  - Argus footer 또는 status renderer
- Claude permission components
  - Argus permission modal

## 권장 패키지 구조

1차 구현 기준 권장 구조:

```text
internal/tui/
  app.go
  model.go
  layout.go
  keys.go
  messages.go
  transcript.go
  input.go
  footer.go
  permissions.go
  commands.go
```

기존 REPL 코드는 아래처럼 다루는 것이 좋다.

- `internal/repl/commands`는 우선 재사용한다.
- `internal/repl/repl.go`는 전환이 끝날 때까지 유지한다.
- TUI 경로가 안정화된 뒤 기존 REPL을 deprecate 한다.

## 데이터 흐름

권장 흐름:

1. 사용자가 textarea에 입력한다.
2. Enter로 제출한다.
3. `/`로 시작하면 slash command 경로로 보낸다.
4. 아니면 `query.Engine.SubmitMessage`를 호출한다.
5. engine goroutine이 `query.UIEvent`를 방출한다.
6. TUI가 이를 `tea.Msg`로 변환한다.
7. transcript, footer, task strip, modal은 모두 UI state에서 렌더링한다.

핵심 변화는 이거다.

- 현재: stdout 직접 출력
- 목표: 상태 기반 렌더링

## 필요한 리팩터링

### A. 입력 처리와 렌더링 분리

지금은 두 기능이 `repl.Run` 안에 같이 들어 있다.

Claude 스타일 UI를 만들려면 아래 상태가 분리되어야 한다.

- input state
- screen state
- render state

즉, 제어 흐름을 `readline.Readline()` 중심에서 `tea.Model.Update()` 중심으로 바꿔야 한다.

### B. slash command 결과를 구조화

지금은 대부분의 slash command가 `ctx.Stdout`에 바로 출력한다.

새 UI에서는 이 결과도 transcript row로 들어가야 한다.

실용적인 1차 방법:

- `commands.Dispatch()`는 유지한다.
- TUI용 writer를 만들어 stdout을 버퍼링한다.
- 실행이 끝나면 버퍼 내용을 system 또는 notice row로 transcript에 추가한다.

장기적으로는 아래 방향이 더 낫다.

- slash command가 `CommandResult` 같은 구조체를 반환하도록 개선

### C. permission 승인 경로를 UI에 연결

Argus는 permission mode와 permission 판단 로직은 이미 갖고 있다.
하지만 승인을 처리하는 UI 경로는 아직 약하다.

즉시 필요한 변경:

- `cmd/argus/main.go`에서 `query.Deps.ApproveTool.Prompt` 연결
- TUI permission modal이 allow 또는 deny를 반환
- permission 요청 상태를 transcript와 footer 모두에 반영

### D. query event 표면을 약간 넓히기

1차 구현은 현재 이벤트로도 가능하지만, 2차에서는 이벤트 구조가 더 풍부하면 좋다.

후속으로 있으면 좋은 이벤트:

- tool execution started 또는 stopped
- thinking placeholder 또는 block
- approval requested
- slash command output
- todo updated

## 1차 범위

이 단계의 산출물은 "Argus가 Claude 형태로 바뀌었다"는 느낌을 주는 수준이어야 한다.

포함:

- fullscreen alt-screen TUI
- 스크롤 가능한 transcript viewport
- multiline textarea input
- footer 표시 항목
  - permission mode
  - active model
  - session id 일부
  - cwd basename
  - todo count
- transcript row 종류
  - user
  - assistant
  - tool use
  - tool result
  - notice
  - error
- `Shift+Tab` permission mode cycle
- 기본 slash command 실행
- tool permission modal
- password prompt modal

제외:

- vim input mode
- transcript virtualization
- mouse selection 및 click routing
- Claude식 suggestion overlay
- transcript search UI
- 고급 background task panel
- custom statusline command

## 2차 범위

이 단계에서는 "형태만 비슷함"을 넘어 Claude와 더 닮은 사용감까지 가져간다.

- footer suggestion row
- `?` help menu
- transcript detail toggle
- task panel, plan panel
- history search
- custom status line hook
- 긴 transcript에 대한 virtualization
- grouped tool rows
- richer permission rule editing UI

## 이 순서가 맞는 이유

이 순서가 현실적이다.

- 사용자는 레이아웃 변화부터 체감한다.
- 레이아웃 변화의 핵심은 transcript, input, footer 분리다.
- multiline input과 permission modal이 들어와야 Claude 같은 느낌이 난다.
- virtualization, vim, search, overlay는 그 다음 문제다.

즉, 처음부터 Claude 내부 구조를 따라가기보다,
Argus에서 가장 큰 체감 차이를 만드는 축부터 먼저 옮기는 것이 맞다.

## 바로 다음 작업

권장 구현 순서:

1. `internal/tui` 스켈레톤 추가
2. `cmd/argus/main.go`에 `tui.Run` 진입 경로 추가
3. `query.UIEvent`를 `tea.Msg`로 브릿지
4. 3분할 레이아웃 구현
   - transcript viewport
   - textarea input
   - footer
5. slash command stdout을 버퍼 writer로 래핑
6. `ApproveTool.Prompt`를 통한 permission modal 연결

여기까지 오면 Argus는 외형과 흐름 모두에서 Claude Code에 훨씬 가까워진다.

## 로컬 참고 소스

- `claude_cli_참고용/src/components/PromptInput/PromptInput.tsx`
- `claude_cli_참고용/src/components/PromptInput/PromptInputFooter.tsx`
- `claude_cli_참고용/src/components/Messages.tsx`
- `claude_cli_참고용/src/components/VirtualMessageList.tsx`
- `claude_cli_참고용/src/ui4.md`

## 공식 참고 문서

- https://code.claude.com/docs/en/interactive-mode
- https://code.claude.com/docs/en/statusline
- https://code.claude.com/docs/en/team
