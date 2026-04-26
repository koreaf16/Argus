# Argus — Phase 1 작업 문서

> **상태**: Phase 1 진행 중 (승인·실행 단계)
> **목표**: `C:\Dev\Argus\claude_cli_참고용` (TypeScript Claude CLI) 를 Go 로 점진적으로 복제한다. Phase 1 에서는 **REPL + Query Engine + Shell 실행 + Web Search + Plan 모드** 를 관련 디렉토리의 **모든 파일까지 완전 복제** 한다.

---

## 1. 원칙

### 1.1 완전 복제 원칙
이번 범위에 들어오는 도구·엔진을 만들 때, 해당 기능이 의존하는 **원본 디렉토리의 모든 파일**(메인 클래스, 파서, 검증기, UI 서브컴포넌트, 상수, 프롬프트, 헬퍼 등)을 **1:1 포트**한다. "동작하는 최소 버전" 이 아니라 **원본 수준의 기능 완결성** 을 가지도록.

### 1.2 디렉토리 구조 매핑
TS 의 `src/X/Y.ts` → Go 의 `internal/x/y.go` 로 1:1 매핑. 디렉토리·파일 이름을 보존 (camelCase → snake_case 만 변환). 이후 기능 추가 시 동일한 이름으로 파일만 늘린다.

### 1.3 코딩 규약 (모든 Go 파일에 강제 적용)

1. **파일당 500 라인 초과 금지**. 500 라인에 근접하면 논리 단위로 분할 (예: `engine.go` → `engine.go` + `engine_stream.go` + `engine_tools.go`). 원본 TS 가 한 파일에 많이 담은 경우도 Go 쪽에서는 쪼갠다.

2. **모든 Go 파일 최상단에 블록 주석 헤더** 필수:
   ```go
   // Package <name> — <한 줄 역할>
   //
   // 파일 역할: 이 파일이 담당하는 것을 2~3줄로.
   // 포함 모듈:
   //   - <Type/Func 이름>: 한 줄 설명
   //   - <Type/Func 이름>: 한 줄 설명
   // 호출/사용 방식:
   //   - 누가 호출하는가 (예: internal/query/engine.go 의 SubmitMessage)
   //   - 외부에 노출되는 진입점 함수
   // 연결:
   //   - import 하는 주요 패키지 (internal/...): 왜 연결되는지
   //   - 이 파일을 import 하는 주요 패키지
   //
   package <name>
   ```

3. **`docs/` 동기화 의무**: 기능 추가·제거·변경이 있으면 같은 PR 안에서 `docs/` 하위 문서를 **반드시** 갱신.
   - `docs/architecture.md` — 전체 패키지 다이어그램·의존 그래프
   - `docs/<area>.md` — 영역별 상세 (`docs/query-engine.md`, `docs/shell.md`, `docs/permissions.md`, `docs/planmode.md`, `docs/llm-providers.md`, `docs/tools/<tool>.md`)
   - `docs/CHANGELOG.md` — 날짜·변경 요약·영향 파일 경로

   docs 를 업데이트하지 않은 PR 은 머지 금지.

---

## 2. 확정된 기술 선택

| 항목 | 선택 |
|---|---|
| Go 모듈 | `github.com/koreaf16/argus` |
| Go 버전 | 1.22+ |
| REPL 입력 | `github.com/chzyer/readline` |
| 터미널 스타일 | `github.com/charmbracelet/lipgloss` |
| LLM 프로바이더 | Anthropic / Gemini / OpenAI-호환 (다중 등록 + `/model` 전환) |
| TUI 수준 | Phase 1 에선 readline + lipgloss, full TUI (Bubbletea) 는 Phase 2+ |

### 2.1 LLM 프로바이더 상세

- **Anthropic Claude** — 공식 SDK `github.com/anthropics/anthropic-sdk-go`. tool_use 포맷을 **내부 표준**으로 사용.
- **Google Gemini** — 공식 SDK `google.golang.org/genai`.
- **OpenAI-호환 제네릭** — `net/http` + SSE 직접 구현. base URL 교체로 공식 OpenAI, Codex, DeepSeek, Qwen, Ollama, vLLM 등 모두 커버.
- **Tool call 변환 레이어** — `internal/services/llm/toolcalls.go` 가 Anthropic `tool_use` ↔ OpenAI `function_call` ↔ Gemini `functionCall` 를 쌍방향 정규화.

### 2.2 모델 등록·전환 UX (`/model`)

여러 모델을 자유롭게 등록하고 슬래시 커맨드로 전환:

- `/model` — 현재 활성 모델 + 전체 카탈로그(별칭·프로바이더·상태) 표 출력, 번호/별칭으로 전환
- `/model <alias>` — 지정 별칭으로 즉시 전환 (다음 턴부터 적용)
- `/model add` — 대화형 등록 마법사: 프로바이더 → 별칭 → 모델 ID → (openai-compat 만) base URL → env 변수명 → 기능 목록
- `/model rm <alias>` — 카탈로그에서 제거
- `/model show <alias>` — 세부 정보 (컨텍스트 창, 지원 기능) 출력

**영속화**: `~/.argus/models.json` 에 사용자 추가분 저장. API 키는 평문 저장하지 않고 env 변수명만 저장, 런타임에 `os.Getenv` 로 읽음.

**기본 프리셋** (코드 내장, env 키 없으면 자동 비활성 표시):
- Anthropic: `claude-opus-4-7`, `claude-sonnet-4-6`, `claude-haiku-4-5`
- Gemini: `gemini-2.5-pro`, `gemini-2.5-flash`
- OpenAI: `gpt-4.1`, `gpt-4o`, `o4-mini`, `codex-*`
- OpenAI-호환은 프리셋 없음 — 사용자가 자유롭게 다중 등록 (회사 vLLM + 공용 DeepSeek + 로컬 Ollama 동시 보유 가능)

---

## 3. 최종 디렉토리 구조

```
argus/
├── go.mod                                   # module github.com/koreaf16/argus
├── CLAUDE.md                                # 코딩 규약·디렉토리 매핑 (Claude Code 자동 로드)
├── cmd/argus/
│   └── main.go                              # ← src/main.tsx + src/replLauncher.tsx
├── internal/
│   ├── repl/                                # ← src/screens/REPL.tsx + PromptInput 하위
│   │   ├── repl.go                          # 메인 루프
│   │   ├── render.go                        # lipgloss 출력 스타일
│   │   ├── commands/                        # 슬래시 커맨드 디스패치 (/exit /clear /help /model)
│   │   └── promptinput/                     # ← src/components/PromptInput/ (22개 파일)
│   ├── query/                               # ← src/QueryEngine.ts + src/query/ (모두)
│   │   ├── engine.go                        # 메인 턴 루프
│   │   ├── config.go                        # ← query/config.ts
│   │   ├── deps.go                          # ← query/deps.ts
│   │   ├── stop_hooks.go                    # ← query/stopHooks.ts
│   │   ├── token_budget.go                  # ← query/tokenBudget.ts
│   │   ├── context.go                       # 시스템 프롬프트 조립
│   │   ├── stream.go                        # SSE 델타 → UI 이벤트
│   │   └── messages.go                      # mutableMessages 관리
│   ├── tool/
│   │   ├── tool.go                          # ← src/Tool.ts
│   │   ├── context.go                       # ToolUseContext
│   │   └── registry.go                      # ← src/tools.ts
│   ├── tools/
│   │   ├── bash/                            # ← src/tools/BashTool/ (18개 파일 전부)
│   │   ├── websearch/                       # ← src/tools/WebSearchTool/ (3개 파일)
│   │   ├── todowrite/                       # ← src/tools/TodoWriteTool/ (3개 파일)
│   │   ├── enterplanmode/                   # ← src/tools/EnterPlanModeTool/ (4개 파일)
│   │   └── exitplanmode/                    # ← src/tools/ExitPlanModeTool/ (4개 파일)
│   ├── services/
│   │   ├── api/                             # ← src/services/api/ (Claude HTTP 계층 전부)
│   │   └── llm/                             # 멀티 프로바이더 추상화 (신규)
│   │       ├── llm.go
│   │       ├── registry.go
│   │       ├── anthropic.go
│   │       ├── gemini.go
│   │       ├── openai.go
│   │       └── toolcalls.go
│   ├── components/                          # ← src/components/ (이번 범위 것만)
│   ├── utils/
│   │   ├── shell.go, shell_command.go, shell_config.go, prompt_shell_execution.go, collapse_background_bash.go
│   │   ├── shell/                           # ← src/utils/shell/ (10개)
│   │   ├── bash/                            # ← src/utils/bash/ (15개 + specs/)
│   │   ├── powershell/                      # ← src/utils/powershell/ (3개)
│   │   ├── sandbox/                         # ← src/utils/sandbox/ (2개)
│   │   ├── permissions/                     # ← src/utils/permissions/ (26개 전부)
│   │   ├── todo/                            # ← src/utils/todo/
│   │   └── ultraplan/                       # ← src/utils/ultraplan/ (2개)
│   ├── planmode/                            # ← src/utils/planModeV2.ts + plans.ts
│   ├── tasks/localshell/                    # ← src/tasks/LocalShellTask/ (3개)
│   ├── state/                               # ← src/state/ (6개)
│   ├── bootstrap/                           # ← src/bootstrap/
│   ├── hooks/                               # ← src/hooks/ (선별)
│   ├── types/                               # ← src/types/ (8개)
│   └── constants/                           # ← src/constants/ (20개)
└── docs/
    ├── phase1.md                            # (이 문서)
    ├── architecture.md                      # 패키지 의존 그래프
    ├── CHANGELOG.md
    ├── query-engine.md
    ├── shell.md
    ├── permissions.md
    ├── planmode.md
    ├── llm-providers.md
    └── tools/
        ├── bash.md
        ├── websearch.md
        ├── todowrite.md
        ├── enterplanmode.md
        └── exitplanmode.md
```

---

## 4. Phase 1 대상 영역별 범위

### 4.1 LLM 추상화 + 모델 카탈로그 (`internal/services/llm/`)

**공통 인터페이스** (`llm.go`):
```go
type LLM interface {
    Stream(ctx context.Context, req Request) (<-chan Event, error)
    Capabilities() Caps    // tool use / thinking / vision / web_search 지원 여부
    Provider() string      // "anthropic" | "gemini" | "openai-compat"
}
type Request struct {
    Model     string
    System    []SystemBlock
    Messages  []Message
    Tools     []ToolSpec
    MaxTokens int
    Thinking  *ThinkingConfig
}
type Event struct {
    Kind    EventKind
    Delta   string
    ToolUse *ToolUseStart
    Stop    *StopReason
}
```

**모델 카탈로그** (`registry.go`):
```go
type ModelEntry struct {
    Alias       string   // 사용자 별칭 (고유)
    ModelID     string   // 실제 API 모델 ID
    Provider    string   // anthropic | gemini | openai-compat
    BaseURL     string   // openai-compat 전용
    APIKeyEnv   string   // env 변수 이름 (평문 저장 X)
    Display     string
    ContextWin  int
    Caps        Caps
}
type Registry struct {
    entries []ModelEntry
    active  string
}
```

메서드: `Add`, `Remove`, `List`, `SetActive`, `Build`, `Load`, `Save`.

### 4.2 Tool 시스템 (`internal/tool/`)

```go
type Tool interface {
    Name() string
    Description(ctx ToolContext) string
    InputSchema() ToolInputJSONSchema
    IsReadOnly() bool
    Call(ctx ToolContext, input json.RawMessage) (<-chan ToolEvent, error)
    CheckPermission(ctx ToolContext, input json.RawMessage) (PermissionResult, error)
}
```

`registry.go`: `Register(Tool)`, `Lookup(name)`, `List() []Tool`. 부팅 시 기본 도구 등록.

### 4.3 Query Engine — 완전 복제

`src/QueryEngine.ts:184` 의 `QueryEngine` 클래스 전체 + `src/query/{config,deps,stopHooks,tokenBudget}.ts` 4개 파일 모두 이식.

```go
type Engine struct {
    llm       llm.LLM
    registry  *tool.Registry
    state     *state.AppState
    messages  []types.Message
    systemFn  func() []llm.SystemBlock
    cfg       *Config
    deps      *Deps
    stopHooks []StopHook
    budget    *TokenBudget
}
func (e *Engine) SubmitMessage(ctx context.Context, userInput string) (<-chan UIEvent, error)
func (e *Engine) CancelCurrent() error
```

**턴 루프**:
1. user 메시지 append + attachment 처리
2. `systemFn()` 으로 시스템 프롬프트 조립 → prompt cache 마커 부착
3. `llm.Stream` 호출, 델타를 UI 로 전파
4. `tool_use` 블록 완성 시 `registry.Lookup` → `permissions.Check` → plan mode gate → `Call` → `tool_result` append
5. stop hooks 호출, 토큰 예산 갱신
6. `stop_reason == "end_turn"` 까지 반복

### 4.4 Shell 실행 엔진 — 완전 복제

Phase 1 에서 **Shell 주변 디렉토리 전부** 이식:

| Go 경로 | TS 원본 | 파일 수 |
|---|---|---|
| `internal/tools/bash/` | `src/tools/BashTool/` | 18 |
| `internal/utils/bash/` | `src/utils/bash/` | 15 + specs/ |
| `internal/utils/shell/` | `src/utils/shell/` | 10 |
| `internal/utils/powershell/` | `src/utils/powershell/` | 3 |
| `internal/utils/` (shell 계열 루트 파일) | Shell.ts/ShellCommand.ts/shellConfig.ts/promptShellExecution.ts/collapseBackgroundBashNotifications.ts | 5 |
| `internal/tasks/localshell/` | `src/tasks/LocalShellTask/` | 3 |
| `internal/utils/sandbox/` | `src/utils/sandbox/` | 2 |

**Go 포팅 주의**
- `bashParser.ts` / `treeSitterAnalysis.ts` → Go `github.com/smacker/go-tree-sitter` + `tree-sitter-bash` 사용, AST 노드 타입을 원본 파서와 동일 매핑.
- PowerShell parser → Go `regexp` 는 lookahead 미지원 → 해당 패턴만 수동 스캐너로 대체.
- 플랫폼 분기는 `//go:build` 태그 (`shell_windows.go` / `shell_unix.go`).
- Sandbox: Windows 는 `shouldUseSandbox=false`, macOS/Linux 는 `seatbelt`/`bwrap` 호출.

### 4.5 Web Search — 완전 복제

`src/tools/WebSearchTool/` **3개 파일 전부** 를 `internal/tools/websearch/` 로.
- 입력 스키마 (`query` / `allowed_domains` / `blocked_domains`) 원본 그대로.
- 기본 구현은 Anthropic **server-side `web_search_20250305` 도구**를 `llm.Request.Tools` 에 그대로 전달.
- `UI.tsx` → `ui.go` 를 lipgloss 로 재현.

### 4.6 권한 시스템 — 완전 복제

`src/utils/permissions/` **26개 파일 전부** → `internal/utils/permissions/`.

- `PermissionMode` / `PermissionResult` / `PermissionRule` / `PermissionUpdate` 타입 원본 필드 보존
- `bashClassifier` + `yoloClassifier` + `classifierShared` + `classifierDecision` — Bash 위험도 분류 알고리즘
- `dangerousPatterns` — 위험 패턴 리스트
- `shellRuleMatching` + `permissionRuleParser` — 허용 룰 매칭
- `denialTracking` — 연속 거부 추적
- `permissionsLoader` — `~/.argus/settings.json` 에서 룰 로드 (Claude CLI 는 `.claude/` 경로, Argus 는 `.argus/`)
- `shadowedRuleDetection` / `permissionExplainer` / `permissionSetup` 전부 포함
- `bypassPermissionsKillswitch` — yolo 모드 킬스위치

### 4.7 Plan 모드 + TodoWrite — 완전 복제

| Go 경로 | TS 원본 | 파일 수 |
|---|---|---|
| `internal/tools/enterplanmode/` | `src/tools/EnterPlanModeTool/` | 4 |
| `internal/tools/exitplanmode/` | `src/tools/ExitPlanModeTool/` | 4 |
| `internal/tools/todowrite/` | `src/tools/TodoWriteTool/` | 3 |
| `internal/utils/todo/` | `src/utils/todo/` | 1 |
| `internal/planmode/` | `src/utils/planModeV2.ts` + `plans.ts` | 2 |
| `internal/utils/ultraplan/` | `src/utils/ultraplan/` | 2 |

**동작**: `EnterPlanMode` 호출 → 쓰기 계열 도구 차단. `TodoWrite` 로 단계 기록 → 매 턴 후 체크박스 렌더링. `ExitPlanMode` → readline 승인 프롬프트 → 승인 시 기본 모드 복귀 후 순차 실행.

### 4.8 REPL (`internal/repl/`) + CLI 진입점 (`cmd/argus/`)

**바이너리**: `argus.exe` (Windows 빌드 기본). `go build -o argus.exe ./cmd/argus` 로 산출.

**CLI 플래그** (`cmd/argus/main.go` 에서 `flag` 패키지로 파싱):

| 플래그 | 동작 |
|---|---|
| (없음) | REPL 모드 진입 — 기본 동작 |
| `--help` / `-h` | 사용법·서브커맨드·주요 env 변수 목록 출력 후 종료 |
| `--init` | `~/.argus/` 디렉토리 생성, `settings.json` 템플릿·`models.json` 프리셋 작성. 이미 있으면 덮어쓰지 않고 경로만 표시 |
| `--version` / `-v` | 버전·커밋·Go 버전 출력 후 종료 |
| `--model <alias>` | 이번 세션에 한해 활성 모델 지정 (영속 저장은 하지 않음) |
| `--print <prompt>` | REPL 대신 1회 쿼리 후 종료 (파이프·스크립트 용) — Phase 2 확장 여지 |

**REPL 본체**:
- `repl.go` — `for` 루프, `readline.New` 로 히스토리·멀티라인 입력 지원
- `commands/` — 슬래시 커맨드 디스패치 (`/exit`, `/clear`, `/help`, `/model`)
- `promptinput/` — `src/components/PromptInput/` 22개 파일 이식. 상태 표시줄에 `[plan]`, `[sandbox]`, 현재 활성 모델명 뱃지
- `render.go` — lipgloss 스타일 매핑 (assistant, tool_use, tool_result, error, thinking)
- `cmd/argus/main.go` — 플래그 파싱 → env 변수 + `~/.argus/config.json` 로 초기 활성 모델 결정 → `llm.Registry.Build()` → `query.Engine` → `repl.Run`

---

## 5. 의존성 (`go.mod`)

| 패키지 | 용도 |
|---|---|
| `github.com/anthropics/anthropic-sdk-go` | Messages API 스트리밍, tool use |
| `google.golang.org/genai` | Google Gemini |
| `github.com/chzyer/readline` | 라인 편집·히스토리 |
| `github.com/charmbracelet/lipgloss` | 터미널 컬러·정렬 |
| `github.com/google/uuid` | 메시지/툴 호출 ID |
| `github.com/smacker/go-tree-sitter` | Bash AST 파서 |

OpenAI-호환 계열은 외부 SDK 없이 `net/http` + SSE 로 직접 구현 (base URL 교체 유연성·의존성 최소화).

---

## 6. 실행 순서

Phase 1 는 아래 순서로 작업한다. 각 단계 완료 시 `docs/<area>.md` 와 `docs/CHANGELOG.md` 를 갱신한다.

1. `C:\Dev\Argus\docs\phase1.md` 생성 (이 문서)
2. `C:\Dev\Argus\docs\architecture.md` 초안 — 패키지 의존 그래프 + TS ↔ Go 매핑 표
3. `C:\Dev\Argus\docs\CHANGELOG.md` 스켈레톤
4. `C:\Dev\Argus\CLAUDE.md` — 코딩 규약, docs 동기화 규칙, 디렉토리 매핑 요약
5. `go.mod` 초기화 + 의존성 추가
6. 상수·타입 이식 — `internal/constants/`, `internal/types/` (다른 패키지가 모두 의존)
7. 권한·Shell 유틸 — `internal/utils/permissions/`, `internal/utils/shell/`, `internal/utils/bash/`, `internal/utils/powershell/`, `internal/utils/sandbox/`
8. LLM 추상화 — `internal/services/api/` → `internal/services/llm/`
9. Tool 인터페이스·레지스트리 — `internal/tool/`
10. 도구 구현 순서: Bash → WebSearch → TodoWrite → EnterPlanMode → ExitPlanMode (각 디렉토리 원본 파일 전부 1:1 이식)
11. 상태·bootstrap·hooks — `internal/state/`, `internal/bootstrap/`, `internal/hooks/`
12. Plan mode — `internal/planmode/`, `internal/utils/ultraplan/`
13. Query Engine — `internal/query/` 전체
14. REPL + PromptInput + 컴포넌트 — `internal/repl/`, `internal/components/`
15. `cmd/argus/main.go` — 와이어링, env 읽기, 기본 모델 선택, 레지스트리 로드, `repl.Run`
16. End-to-End 검증 (§7)

---

## 7. 검증 (End-to-End)

1. `go mod tidy && go build ./...` — 전 패키지 컴파일
2. `ANTHROPIC_API_KEY=... go run ./cmd/argus` → `>` 프롬프트, 상태줄에 현재 활성 모델 표시
3. **쿼리 엔진**: `hello` 입력 → 스트리밍 응답이 색상 적용되어 한 줄씩 출력
4. **모델 전환**:
   - `/model` → 카탈로그 표 출력, 활성 모델 하이라이트
   - `/model add` → DeepSeek 엔드포인트를 별칭 `ds` 로 등록 (OpenAI-호환, base URL/env 변수명 입력)
   - `/model add` → Ollama 로컬 (`http://localhost:11434/v1`) 을 `local` 별칭으로 추가 (**두 개 이상 동시 등록 확인**)
   - `/model ds` / `/model local` / `/model claude-opus-4-7` → 자유롭게 전환
   - Argus 재실행 시 `~/.argus/models.json` 에서 사용자 추가분 유지 확인
5. **Shell 실행**: `현재 디렉토리의 파일 목록을 보여줘` → `Bash` 도구 호출 + 권한 프롬프트 → 승인 시 결과 표시
6. **Web Search** (Anthropic 활성 시): `오늘 날씨 서울 검색해줘` → 서버 측 `web_search` 호출 이벤트가 UI 에 표시, 결과 요약 출력. OpenAI-호환/Gemini 활성 시에는 "현재 모델은 web_search 미지원" 안내
7. **Plan 모드**: `파이썬 설치하는 계획을 세워줘` → `EnterPlanMode` → `TodoWrite` → `ExitPlanMode` 승인 → 각 단계 순차 실행, TODO 체크박스 진행 반영
8. `/exit` 로 정상 종료, `~/.argus_history` 생성 확인

---

## 8. Phase 2+ 백로그 (이번에 안 함)

- `src/tools/` 의 File* (Read/Edit/Write/Glob/Grep), LSP, Task, Agent, Skill, MCP 도구들
- `src/commands/` 슬래시 커맨드 전체 (50+)
- Bubbletea 기반 full TUI (`internal/ink/` 이전)
- `src/services/mcp/` MCP 클라이언트
- `src/services/skills/` + `internal/skills/`
- `src/remote/`, `src/bridge/` 원격/브리지
- 영구 저장 (SQLite) — 세션·히스토리·TODO 지속화
- OpenAI tool use 완전 호환 (function calling 쌍방향 변환)
---

## Session Resume Update (2026-04-22)

- CLI now supports `--resume` / `-r <id>` to continue a saved session by ID.
- Session snapshots are autosaved per turn and on exit under `.Argus/sessions/<session-id>.json`.
- On normal exit (REPL and `--print`), Argus prints `session_id: <id>` to stderr.
