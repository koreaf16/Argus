# TUI 아키텍처

## 1. 핵심 원칙: AltScreen 금지

대부분의 TUI 도구들이 사용하는 `AltScreen` 모드(전체 화면을 차지하고 종료 시 이전 내용이 사라지는 방식)를 **사용하지 않습니다**.

1. **Terminal Scrollback 유지**: 모든 답변은 터미널 스크롤백의 일부가 되어야 합니다.
2. **Dynamic Inline Rendering**: 하단에 상태바(Footer)와 입력창(Input)을 고정하되, 답변 내용은 위로 흐르듯이(Scrolling) 출력됩니다.
3. **UX Continuity**: 사용자가 이전 대화 내용을 터미널 스크롤로 언제든 찾아볼 수 있어야 합니다.

## 2. UI 구조

```
┌────────────────────────────────────────────┐
│  [Terminal Scrollback Area]                │
│  모든 대화 기록이 누적되는 영역             │
│  logo, assistant 응답, tool 결과 등         │
│  ┆                                       │
│  ┆  (스크롤 가능)                         │
│  ┆                                       │
├────────────────────────────────────────────┤ ← Fixed Anchor Area (View() 반환)
│  ┌────────────────────────────────────┐   │
│  │  >  입력창 (Prompt Input)          │   │  ← 멀티라인 지원
│  └────────────────────────────────────┘   │
│  [ARGUS] claude-sonnet | ask | CWD: ...   │   ← Footer/Status Bar
└────────────────────────────────────────────┘
```

## 3. 구현 구조

### BubbleTea Program

```go
// tui.go
parker := NewCursorParkWriter(os.Stdout)
p := tea.NewProgram(model, tea.WithOutput(parker))
// tea.WithAltScreen() 미사용 — 스크롤백 유지
```

### Model ([`model.go`](internal/tui/model.go))

- `Init()`: 초기화 + `ShowCursor` 설정
- `Update(msg)`: 메시지 처리 (키 입력, LLM 스트림, Tool 이벤트)
- `View()`: 하단 고정 영역만 반환 (input + footer)
  - `\x1b[?25l`(커서 숨김) 접두사
  - `lipgloss.JoinVertical`으로 파트 조립

### Transcript ([`transcript.go`](internal/tui/transcript.go))

- 완료된 대화 항목을 `tea.Println()` (scrollbackCmd)으로 스크롤백에 출력
- 각 항목: Markdown 렌더링된 assistant 응답 또는 Tool 결과

### 렌더링 ([`render.go`](internal/tui/render.go))

- `composeFooterStatusLine()`: 상태 정보 footer 내부 통합
- `renderLogoBlock()`: 세션 시작시 Constellation 스플래시

## 4. Windows IME Cursor Parking

[`internal/tui/cursor_park.go`](internal/tui/cursor_park.go) 의 `CursorParkWriter`가 `os.Stdout`을 래핑합니다.

### 파킹 흐름

```
BubbleTea.flush() → Write(frame)
  │
  ├── parked == true → \x1b[?25l + \x1b[u (커서 숨김 + 저장 위치 복원)
  ├── 프레임 내용 터미널에 출력
  └── isFrameEnd(p) == true → parkCursorLocked():
        \x1b[s                    (CSI SCP: 현재 위치 저장)
        \x1b[<linesUp>A           (입력창까지 위로 이동)
        \x1b[<col>G               (캐럿 column으로 이동)
        \x1b[?25h                 (커서 표시 → IME가 이 위치에 앵커)
        parked = true
```

### 프레임 종료 감지 (`isFrameEnd`)

| BubbleTea 버전 | 종료 시퀀스 | 마지막 바이트 |
|---|---|---|
| v0.x | `\r` | `0x0D` |
| **v1.x inline** | `\x1b[<n>D` (CSI CursorLeft) | `'D'` |

## 5. Markdown 렌더링

[`internal/tui/markdown/`](internal/tui/markdown/) 는 터미널에서 Markdown을 렌더링합니다.

| 파일 | 역할 |
|------|------|
| `markdown.go` | Markdown 파싱 및 렌더링 엔진 |
| `codeblock.go` | 코드 블록 (syntax highlighting) |
| `inline.go` | 인라인 형식 (bold, italic, code) |
| `table.go` | 테이블 렌더링 |
| `palette.go` | 색상 팔레트 |
| `width.go` | 너비 계산 |

## 6. ToolUI Renderer

[`internal/tui/toolui/renderer.go`](internal/tui/toolui/renderer.go) 는 도구 전용 UI를 제공합니다.

- `WebFetchUI`: URL/상태 표시
- `WebSearchUI`: 검색 결과 표
- `specialized_renderers.go`: 기타 도구별 렌더러

## 7. 테마 (`theme.go`)

```go
type Theme struct {
    // 색상
    Primary    string
    Secondary  string
    Accent     string
    Error      string
    Warning    string
    // 컴포넌트
    FooterStyle    lipgloss.Style
    InputStyle     lipgloss.Style
    MessageStyle   lipgloss.Style
}
```
