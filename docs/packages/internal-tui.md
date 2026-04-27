# internal/tui — BubbleTea 기반 TUI

## 파일 구조

| 파일 | 역할 |
|------|------|
| `tui.go` | BubbleTea 프로그램 초기화 및 실행 |
| `model.go` | BubbleTea Model (Init/Update/View) |
| `render.go` | 렌더링 로직 (footer, 상태 줄) |
| `render_footer.go` | Footer 상태 바 렌더링 |
| `transcript.go` | 대화 기록 스크롤백 출력 |
| `modal.go` | 모달 오버레이 |
| `modal_server_form.go` | 서버 설정 폼 모달 |
| `modal_server_list.go` | 서버 목록 모달 |
| `theme.go` | 색상/스타일 테마 |
| `input_history.go` | 입력 히스토리 관리 |
| `slash_suggest.go` | 슬래시 커맨드 제안 |
| `tool_registration.go` | 도구 → TUI 렌더러 연결 |
| `ui_settings.go` | UI 설정 |

## 서브 패키지

### markdown/

| 파일 | 역할 |
|------|------|
| `markdown.go` | Markdown 파싱 및 렌더링 |
| `codeblock.go` | 코드 블록 (syntax highlighting) |
| `inline.go` | 인라인 형식 (bold, italic, code) |
| `table.go` | 테이블 렌더링 |
| `palette.go` | 색상 팔레트 |
| `width.go` | 너비 계산 |
| `wrap_test.go` | 줄 바꿈 테스트 |

### toolui/

| 파일 | 역할 |
|------|------|
| `renderer.go` | `ToolRenderer` 인터페이스, 도구 UI 연결 |
| `specialized_renderers.go` | WebFetch, WebSearch 등 전용 렌더러 |

## Model

```go
type Model struct {
    // BubbleTea
    width, height int

    // Transcript
    transcripts []*TranscriptEntry

    // Input
    input     *TextInput
    history   *InputHistory

    // State
    engine    *query.Engine
    state     *state.AppState

    // Cursor parking (Windows IME)
    linesUp int
    col     int

    // Modal/Overlay
    modal   Modal
    focused bool
}
```

## 렌더링 방식

- `tea.WithAltScreen()` **미사용** — 터미널 스크롤백 유지
- `View()`: 하단 고정 영역만 반환 (input + footer)
- 완료된 항목: `tea.Println()` (scrollbackCmd)으로 직접 출력
- `CursorParkWriter`: Windows IME 대응을 위한 커서 파킹

## Slash Commands

| 명령 | 설명 |
|------|------|
| `/help` | 도움말 |
| `/model` | 모델 전환 |
| `/status` | 상태 조회 |
| `/session` | 세션 관리 |
| `/plan` | 계획 모드 |
| `/memory` | 메모리 검색 |
| `/mcp` | MCP 관리 |
| `/skills` | Skills 목록 |
| `/commit` | Git 커밋 |
| `/diff` | Git diff |
| `/review` | 코드 리뷰 |
| `/config` | 설정 관리 |
| `/init` | 초기화 |
| `/keybindings` | 단축키 |
