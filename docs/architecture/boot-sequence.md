# 부트 시퀀스 (Boot Sequence)

Argus의 시작부터 첫 대화 입력까지의 초기화 흐름입니다.

## 1. 시작 흐름

```
main() (cmd/argus/main.go:104)
  │
  ├── Windows UTF-8 코드페이지 설정 (CP 65001)
  │
  ├── SSH GUI 방지 (SSH_ASKPASS/DISPLAY 제거)
  │
  ├── parseFlags()  ← 플래그 파싱
  │     ├── --help / -h
  │     ├── --version / -v
  │     ├── --init
  │     ├── --model <alias>
  │     ├── --print / -p <prompt>
  │     ├── --resume / -r <session_id>
  │     ├── --aidebug
  │     └── --auto-approve
  │
  ├── init()  ← --init 플래그 시 ~/.argus 설정 파일 생성
  │
  └── bootstrap()  ← 모든 서비스 초기화
        │
        ├── memdir.EnsureAll()  ← ~/.argus 디렉토리 생성
        ├── llm.NewRegistry()   ← 모델 카탈로그 로드
        ├── state.NewAppState() ← 앱 상태 생성
        ├── session.NewID()     ← 세션 UUIDv4 생성
        ├── tool.NewRegistry()  ← Tool 레지스트리 생성
        ├── 도구 등록 (23개 내장 + MCP 브리지)
        ├── mcp.NewManager()    ← MCP 설정 로드
        ├── lsp.NewManager()    ← LSP 매니저
        ├── skills.NewRegistry() ← Skills 로드
        ├── hooks.NewDispatcher() ← 훅 디스패처
        ├── query.NewEngine()   ← 쿼리 엔진 생성
        └── session.Load()      ← --resume 시 세션 복원
        │
        └── Mode Router
              ├── TUI Mode  ← 기본 (tui.Run())
              ├── AIDebug   ← --aidedebug (NDJSON 트레이스)
              └── Print     ← --print (싱글 프롬프트)
```

## 2. Bootstrap 상세

### 2.1 Memdir Bootstrap
[`internal/memdir`](internal/memdir/) 은 `~/.argus` 디렉토리 구조를 초기화합니다.

```
~/.argus/
├── settings.json          # 앱 설정
├── models.json            # 모델 카탈로그
├── mcp.json               # MCP 서버 설정
├── permissions/           # 권한 규칙
├── sessions/              # 세션 스냅샷 (NDJSON)
├── memory/                # 메모리 저장소
├── todos/                 # Todo 영속화
├── plans/                 # Plan 파일
├── worktrees/             # 작업 트리
├── scheduled-tasks/       # 스케줄드 작업
└── session-artifacts/     # Distiller 아티팩트
```

### 2.2 LLM Registry
[`internal/services/llm/registry.go`](internal/services/llm/registry.go) 는 `~/.argus/models.json` 을 로드합니다.

- **발견 (Discovery)**: `GET /models` (OpenAI), Anthropic API, Gemini API
- **런타임**: `POST /chat/completions` (OpenAI), `/messages` (Anthropic), `/models/{id}:streamGenerateContent` (Gemini)
- **프리셋**: Anthropic (Claude), Gemini, OpenAI 내장 — env 키 없으면 비활성

### 2.3 Tool Registry 초기화
`cmd/argus/main.go` bootstrap에서 내장 도구들을 등록합니다:

- 셸 도구: `bash`, `powershell`, `shelljob`, `shelljobcontrol`
- 파일 도구: `fileread`, `filewrite`, `fslist`, `glob`, `grep`
- 웹 도구: `webfetch`, `websearch`
- 계획 도구: `todowrite`, `todoread`, `enterplanmode`, `exitplanmode`
- 외부 통합: `mcptool`, `lsptool`, `listmcpresources`, `readmcpresource`, `mcpauth`
- 서버 관리: `serverconnect`, `servercopy`, `serverinspect`, `servermetrics`, `servertunnel`
- 기타: `skilltool`, `sniptool`, `askuser`, `enterworktree`, `exitworktree`

MCP 브리지는 `~/.argus/mcp.json` 설정을 읽어 `mcp__{server}__{tool_name}` 패턴으로 동적 등록합니다.

## 3. 세션 복원 (--resume / -r)

```
initializeSession(sessionID)
  │
  ├── memStore.LoadSession(sessionID)
  │     └── NDJSON 파일 로드 (~/.argus/sessions/{id}.ndjson)
  │
  ├── Snapshot 역직렬화
  │     ├── v1: Messages[]만
  │     └── v2: Messages[] + Graph[] + Artifacts[]
  │
  ├── engine.ReplaceMessages()
  │     └── graph 재구축, 노드 시퀀스 복원
  │
  └── 다음 Turn 계속
```

## 4. TUI 초기화

[`internal/tui/tui.go`](internal/tui/tui.go) 는 BubbleTea 프로그램을 실행합니다.

- `tea.WithAltScreen()` **사용하지 않음** (스크롤백 유지)
- `CursorParkWriter`로 os.Stdout 래핑 (Windows IME 대응)
- `model.go` 의 `View()`는 하단 고정 영역만 반환 (input + footer)
- 완료된 대화는 `tea.Println()`으로 스크롤백에 직접 출력
