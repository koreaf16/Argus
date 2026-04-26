# Windows IME Cursor Parking

## 문제

Windows Terminal에서 한글(CJK IME) 입력 시, 조합 중인 문자("하" 등)가 **터미널 좌측 하단**(프롬프트 초기 위치)에 먼저 렌더링된 뒤 입력창 커서 위치로 이동하는 깜빡임이 발생했다.

**원인 체인**:

1. Windows Terminal + IME는 **하드웨어 커서 위치**를 실시간으로 추적하여 그 위치에 조합 중 문자를 렌더한다.
2. BubbleTea v1.x 인라인 렌더러는 매 프레임을 `\x1b[<width>D` (CSI CursorLeft) 시퀀스로 종료한다. 이 시점에 커서는 프레임 마지막 줄의 column 0 (좌하단)에 있다.
3. 커서를 입력창 위치로 재이동("파킹")하는 `CursorParkWriter`가 있었지만, 프레임 종료 감지를 `\r` (carriage return)로만 했기 때문에 **BubbleTea v1.x에서는 한 번도 동작하지 않았다**.
4. 결과적으로 매 프레임 후 커서가 좌하단에 그대로 남아, IME 조합 문자도 좌하단에 표시됐다.

---

## 해결 구조: CursorParkWriter

`internal/tui/cursor_park.go`의 `CursorParkWriter`가 `os.Stdout`을 래핑하여 BubbleTea 렌더 출력을 인터셉트한다.

```
tea.NewProgram(model, tea.WithOutput(parker))
```

### 파킹 흐름 (수정 후)

```
BubbleTea.flush() → Write(frame)
  ↓
CursorParkWriter.Write():
  1. parked == true 이면: \x1b[?25l + \x1b[u (커서 숨김 + 저장 위치 복원)
  2. 프레임 내용 terminal에 출력
  3. isFrameEnd(p) == true 이면 parkCursorLocked():
       \x1b[s                    (CSI SCP: 현재 위치 저장 = 프레임 마지막 줄 column 0)
       \x1b[<linesUp>A           (입력창까지 위로 이동)
       \x1b[<col>G               (캐럿 column으로 이동)
       \x1b[?25h                 (커서 표시 → IME가 이 위치에 앵커)
       parked = true
```

### 프레임 종료 감지: `isFrameEnd()`

| BubbleTea 버전 | 프레임 종료 시퀀스 | 마지막 바이트 |
|---|---|---|
| v0.x | `\r` (carriage return) | `0x0D` |
| **v1.x inline** | `\x1b[<n>D` (CSI CursorLeft) | `'D'` |

```go
func isFrameEnd(p []byte) bool {
    if p[len(p)-1] == '\r' { return true }
    // \x1b[<digits>D 패턴
    if p[len(p)-1] != 'D' { return false }
    i := len(p) - 2
    for i >= 0 && p[i] >= '0' && p[i] <= '9' { i-- }
    return i >= 1 && p[i] == '[' && p[i-1] == '\x1b'
}
```

### ANSI 시퀀스 선택: CSI SCP/RCP vs DECSC/DECRC

| 방식 | 저장 코드 | 복원 코드 | 특징 |
|---|---|---|---|
| DECSC/DECRC | `\x1b7` | `\x1b8` | 커서 위치 + SGR 속성 + charset 상태까지 저장 |
| **CSI SCP/RCP** | `\x1b[s` | `\x1b[u` | 커서 위치만 저장 (더 단순, BubbleTea SGR 관리와 충돌 없음) |

DECSC는 SGR(색상, 굵기 등) 상태까지 저장하기 때문에 BubbleTea의 자체 SGR 리셋 경로와 간섭할 수 있다. CSI SCP를 사용한다.

---

## 커서 위치 계산: `updateCursorTarget()`

`model.go`의 `updateCursorTarget()`이 매 `View()` 호출마다 파킹 목표 좌표를 계산한다.

### linesUp (수직 거리)

프레임 마지막 줄(column 0)에서 입력창 캐럿 줄까지 올라가야 할 줄 수:

```
linesUp = 1 (bottom emptyLine)
        + footerH (footer 높이)
        + 1 (input box bottom border)
        + (contentLinesCount - 1 - textareaRow)  // 캐럿 아래 content 줄 수
```

### col (수평 위치, 1-indexed)

```
col = 3 (prefix width: " " + "> ")
    + runewidth.StringWidth(text before cursor)
    + 1 (ANSI 1-indexed 보정)
```

한글 등 CJK 문자는 터미널에서 2칸 폭이므로 `runewidth.StringWidth()`로 시각적 폭을 계산한다.

---

## View() 커서 숨김 접두사

BubbleTea가 프레임을 재렌더할 때 (cursor-up → clear → re-draw), 하드웨어 커서가 visible 상태로 화면을 훑으면 IME가 그 순간의 위치에 조합 문자를 그린다. 이를 방지하기 위해 `View()` 반환값 앞에 `\x1b[?25l`(커서 숨김)을 붙인다.

```go
// model.go View() 반환
return ansiHideCursor + lipgloss.JoinVertical(lipgloss.Left, parts...) + "\x1b[J"
```

렌더링 동안 커서가 불가시 → 파킹 완료 후 `\x1b[?25h`(커서 표시)로만 visible 해짐 → IME 앵커가 입력창 캐럿에만 고정.

---

## 핵심 파일

| 파일 | 역할 |
|---|---|
| `internal/tui/cursor_park.go` | CursorParkWriter, isFrameEnd(), ANSI 상수 |
| `internal/tui/model.go` | View() 커서 숨김 접두사, updateCursorTarget(), Init() ShowCursor |
| `internal/tui/tui.go:111` | `parker := NewCursorParkWriter(os.Stdout)` 초기화 |

---

## BubbleTea v1.x 업그레이드 시 주의

`standard_renderer.go`의 프레임 종료 시퀀스가 변경되면 `isFrameEnd()`를 수정해야 한다. 현재 감지 대상:

```go
// github.com/charmbracelet/bubbletea@v1.2.4 standard_renderer.go:285
buf.WriteString(ansi.CursorLeft(r.width))  // = \x1b[<width>D
```
