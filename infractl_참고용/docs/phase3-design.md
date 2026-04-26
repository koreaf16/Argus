# Phase 3 설계도 — CLI TUI (Claude CLI 스타일)

## 목표
readline REPL을 Claude CLI/Gemini CLI 스타일의 풀스크린 터미널 TUI로 교체.

## 선행: Phase 2 완료

---

## 추가/변경 파일

```
internal/
├── tui/
│   ├── app.go              # bubbletea 메인 앱 (Model, Update, View)
│   ├── statusbar.go        # 상단 상태바 컴포넌트
│   ├── chatview.go         # 중앙 스크롤 영역 (대화 + 실행 박스)
│   ├── inputbar.go         # 하단 고정 입력바
│   ├── cmdbox.go           # 명령 실행 박스 (테두리, 접기/펼치기, 스피너)
│   ├── confirm.go          # 확인 대화상자 (y/n, 선택지, 텍스트 입력)
│   ├── styles.go           # lipgloss 스타일 정의
│   └── handler.go          # EventHandler 구현 (agent ↔ TUI 연결)
├── cli/repl.go             # 기본 REPL 모드용 (TUI 미사용 시)
```

---

## 화면 레이아웃

```
┌─────────────────────────────────────────────────────┐
│ ● infractl v0.1.0 │ qwen3.5:27b │ 3 servers       │ ← StatusBar (1줄, 고정)
├─────────────────────────────────────────────────────┤
│                                                     │
│  (대화 히스토리 + 실행 결과가 위→아래로 쌓임)          │ ← ChatView (스크롤)
│  (명령 실행 박스가 인라인으로 표시)                    │
│                                                     │
├─────────────────────────────────────────────────────┤
│ > █                                                 │ ← InputBar (1~3줄, 고정)
└─────────────────────────────────────────────────────┘
```

---

## 의존성

```
github.com/charmbracelet/bubbletea    # Elm 아키텍처 TUI 프레임워크
github.com/charmbracelet/lipgloss     # 터미널 스타일링
github.com/charmbracelet/glamour      # 마크다운 렌더링
github.com/charmbracelet/bubbles      # textarea, spinner, viewport 등 컴포넌트
```

---

## bubbletea 앱 구조

### Model (상태)
```
AppModel:
  statusBar    StatusBarModel    # 버전, 모델, 서버 수
  chatView     ChatViewModel     # 메시지 리스트, 스크롤 위치
  inputBar     InputBarModel     # 현재 입력 텍스트, 커서
  agent        *agent.Agent      # 에이전트 루프 참조
  width        int               # 터미널 너비
  height       int               # 터미널 높이
  mode         AppMode           # normal | confirming | inputting
```

### Messages (이벤트)
```
TokenMsg           string        # LLM 토큰 수신
ToolStartMsg       {name, target, command}
ToolEndMsg         {name, result, duration, success}
ResponseDoneMsg    string        # 최종 응답 완료
ErrorMsg           error
ConfirmRequestMsg  {message, callback}
InputRequestMsg    {prompt, callback}
WindowSizeMsg      {width, height}  # bubbletea 내장
```

### Update (상태 전이)
- 키 입력 → InputBar 업데이트
- Enter → 입력 전송 → 에이전트 루프 시작 (goroutine)
- 토큰 수신 → ChatView에 추가
- 도구 시작 → ChatView에 실행 박스 추가 (스피너)
- 도구 끝 → 실행 박스 업데이트 (결과, 접기 가능)
- 확인 요청 → mode를 confirming으로 전환

---

## 컴포넌트 상세

### StatusBar
- 배경색 있는 1줄 바
- 왼쪽: `● infractl v0.1.0`
- 중앙: 현재 모델명
- 오른쪽: 연결된 서버 수, 활성 알림 수 (Phase 8)

### ChatView
- bubbles/viewport 사용 (스크롤 가능)
- 내용은 렌더링된 문자열의 리스트:
  - 사용자 입력: `> 입력 내용` (색상 구분)
  - LLM 텍스트 응답: glamour로 마크다운 렌더링
  - 명령 실행 박스: CmdBox 컴포넌트

### CmdBox (명령 실행 박스)
```
상태: running | completed | failed | collapsed

실행 중:
┌─ ssh → db-server ─ (⠋) ────────────────────────────┐
│ sqlplus -S mon_user/****@PROD ...                   │
└─────────────────────────────────────────────────────┘

완료 (펼침):
┌─ ssh → db-server ─ (2.3초) ✓ ──────────────────────┐
│ sqlplus -S mon_user/****@PROD <<EOF                 │
│   SELECT tablespace_name, used_percent ...          │
│ EOF                                                 │
│                                                     │
│ TABLESPACE_NAME    USED_PERCENT                     │
│ USERS              91.2                             │
│ SYSTEM             72.1                             │
└─────────────────────────────────────────────────────┘

완료 (접힘):
▶ ssh → db-server ─ sqlplus ... (2.3초) ✓
```

- 접기/펼치기: 해당 박스 위에서 Enter 또는 클릭
- 긴 출력(20줄+)은 자동 접힘
- 실행 중일 때 스피너 애니메이션 (bubbles/spinner)

### InputBar
- bubbles/textarea 사용
- 멀티라인 지원 (Shift+Enter로 줄바꿈)
- 하단 고정
- 비밀번호 입력 시 마스킹 모드

### Confirm / Input 대화상자
- mode가 confirming/inputting일 때 InputBar를 대체
- 확인: `? 메시지 (y/n)` → y/n 입력
- 선택: `❯ 1) 옵션A  2) 옵션B` → 숫자 또는 화살표 선택
- 텍스트: `? 프롬프트: ____` → 자유 입력

---

## 스타일 정의 (lipgloss)

```
색상:
  primary     = 터미널 기본 전경
  secondary   = gray/dim
  accent      = cyan/blue
  success     = green
  warning     = yellow
  error       = red

StatusBar:    background=accent, foreground=white, bold
UserInput:    foreground=accent, ">" prefix
CmdBox 테두리: border=rounded, foreground=secondary
CmdBox 헤더:  foreground=secondary, 서버명=accent
스피너:       foreground=accent
성공:         "✓" foreground=success
실패:         "✗" foreground=error
경고:         "⚠️" foreground=warning
```

---

## 키보드 바인딩

| 키 | 동작 |
|----|------|
| Enter | 입력 전송 (일반 모드) / 확인 (확인 모드) |
| Shift+Enter | 줄바꿈 |
| Ctrl+R | 히스토리 검색 모드 진입 (Phase 5에서 완성) |
| Esc Esc (2번) | 실행 중 작업 중단 |
| ↑ ↓ | 히스토리 탐색 (입력 비어있을 때) / 스크롤 (내용 있을 때) |
| Ctrl+C | 종료 |
| Tab | 슬래시 명령 자동완성 |
| PgUp/PgDn | ChatView 스크롤 |

---

## EventHandler → TUI 연결

agent.EventHandler 인터페이스를 TUI용으로 구현:
- OnThinking → ChatView에 "⏺" 추가
- OnToken → ChatView 마지막 항목에 토큰 append (스트리밍)
- OnToolStart → ChatView에 CmdBox 추가 (running 상태)
- OnToolEnd → CmdBox 상태를 completed/failed로 업데이트
- OnResponse → 완료 표시
- OnConfirm → mode를 confirming으로, 결과를 채널로 반환
- OnInput → mode를 inputting으로, 결과를 채널로 반환

핵심: 에이전트 루프는 goroutine에서 돌고, TUI는 bubbletea 메인 루프에서 돌기 때문에
tea.Cmd / tea.Msg로 비동기 통신해야 함.

---

## --tui 옵트인

기본적으로 가벼운 readline REPL 모드로 동작하며, `infractl --tui`로 실행하면 풀스크린 TUI가 실행됨.
TUI 미지원 터미널이나 파이프 입력 시에는 기본 REPL을 사용.

---

## 검증 시나리오

1. 풀스크린 TUI 진입 → 상태바, 입력바 보임
2. 자연어 입력 → 스트리밍 응답 (토큰 단위)
3. 명령 실행 시 → CmdBox 나타남 (스피너 → 결과 → 접기)
4. SSH 명령 → 헤더에 `ssh → 서버명` 표시
5. 스크롤로 이전 대화 확인
6. 비밀번호 입력 시 마스킹
7. Esc Esc로 실행 중 작업 중단

---

## 완료 기준
- [ ] bubbletea 풀스크린 TUI
- [ ] 상단 상태바 + 중앙 스크롤 + 하단 입력바
- [ ] CmdBox (테두리, 스피너, 접기/펼치기)
- [ ] SSH 헤더 구분
- [ ] LLM 스트리밍 출력
- [ ] 확인/입력 대화상자
- [ ] --tui 옵트인
