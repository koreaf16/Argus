# Phase 1 상세 설계서 — 뼈대 (LLM + 로컬 Bash 실행)

> **선행 문서**: infractl-archacture.md, infractl-build-plan.md
> **원칙**: "애매하면 Claude CLI(Claude Code) 따른다"

---

## 1. 목표 & 범위

### 1.1 목표
`infractl` 실행 → 자연어 입력 → LLM이 bash 명령 생성 → 실행 → 결과를 LLM이 해석 → 사용자에게 답변.
이것만 되면 나머지 Phase는 전부 살을 붙이는 것.

### 1.2 범위
- OpenAI 호환 LLM API 클라이언트 (Ollama, Claude, OpenAI 공통)
- 도구 시스템 기초 (인터페이스 + 레지스트리 + 빌트인 5종)
- Executor 인터페이스 + LocalExecutor (로컬 bash/cmd 실행)
- 에이전트 루프 (입력 → LLM → 도구 → 결과 → 반복)
- 기본 readline REPL (TUI는 Phase 3)
- 설정 파일 (`~/.infractl/config.yaml`)
- `infractl init` 초기 설정 명령

### 1.3 범위 밖 (이후 Phase)
- SSH 원격 실행 (Phase 2)
- SQLite 저장소 (Phase 2)
- TUI (Phase 3)
- 디스커버리/커넥터 (Phase 4)
- 위험도 확인 UI (Phase 5)

---

## 2. 디렉토리 & 파일 구조

```
infractl/
├── cmd/
│   └── infractl/
│       └── main.go                     # 엔트리포인트, CLI 서브커맨드 분기
├── internal/
│   ├── config/
│   │   └── config.go                   # 설정 구조체, 로드, 저장, init 흐름
│   ├── llm/
│   │   ├── client.go                   # Client 인터페이스 정의
│   │   ├── types.go                    # Message, ToolCall, Response 등 타입
│   │   └── openai.go                   # OpenAI 호환 API 구현 (스트리밍 포함)
│   ├── agent/
│   │   ├── loop.go                     # 에이전트 루프 코어 로직
│   │   ├── handler.go                  # EventHandler 인터페이스 (UI 연결)
│   │   └── prompt.go                   # 시스템 프롬프트 빌더
│   ├── executor/
│   │   ├── executor.go                 # Executor 인터페이스 + ExecResult 타입
│   │   └── local.go                    # LocalExecutor 구현
│   ├── tools/
│   │   ├── tool.go                     # Tool 인터페이스 정의
│   │   ├── registry.go                 # 도구 레지스트리 (등록/조회/JSON 변환)
│   │   ├── shell_exec.go              # shell_exec 도구
│   │   ├── file_read.go               # file_read 도구
│   │   ├── file_write.go              # file_write 도구
│   │   ├── process_list.go            # process_list 도구
│   │   └── network_info.go            # network_info 도구
│   └── cli/
│       └── repl.go                     # readline 기반 REPL
├── go.mod
└── go.sum
```

### 파일별 책임

| 파일 | 단일 책임 | 예상 라인 |
|------|-----------|-----------|
| `cmd/infractl/main.go` | CLI 엔트리포인트, 서브커맨드 분기 | ~120 |
| `internal/config/config.go` | 설정 구조체 정의, YAML 로드/저장, init 대화 | ~150 |
| `internal/llm/client.go` | LLM Client 인터페이스 정의 | ~30 |
| `internal/llm/types.go` | LLM 메시지/응답 타입 정의 | ~120 |
| `internal/llm/openai.go` | OpenAI 호환 HTTP 클라이언트 + SSE 스트리밍 | ~280 |
| `internal/agent/loop.go` | 에이전트 루프 (메시지 구성 → LLM 호출 → 도구 실행 → 반복) | ~250 |
| `internal/agent/handler.go` | EventHandler 인터페이스 정의 | ~40 |
| `internal/agent/prompt.go` | 시스템 프롬프트 텍스트 구성 | ~100 |
| `internal/executor/executor.go` | Executor 인터페이스 + ExecResult 타입 | ~40 |
| `internal/executor/local.go` | LocalExecutor (bash/cmd 실행) | ~120 |
| `internal/tools/tool.go` | Tool 인터페이스 정의 | ~40 |
| `internal/tools/registry.go` | 도구 등록/조회, LLM용 JSON Schema 생성 | ~120 |
| `internal/tools/shell_exec.go` | 쉘 명령 실행 도구 | ~80 |
| `internal/tools/file_read.go` | 파일 읽기 도구 | ~80 |
| `internal/tools/file_write.go` | 파일 쓰기 도구 | ~90 |
| `internal/tools/process_list.go` | 프로세스 목록 도구 | ~80 |
| `internal/tools/network_info.go` | 네트워크 정보 도구 | ~80 |
| `internal/cli/repl.go` | readline REPL + 슬래시 명령 | ~180 |

---

## 3. 핵심 타입 정의

### 3.1 LLM 메시지 타입 (`internal/llm/types.go`)

```go
// Role은 대화 메시지의 역할을 나타낸다.
type Role string

const (
    RoleSystem    Role = "system"
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

// Message는 LLM API의 단일 메시지를 나타낸다.
type Message struct {
    Role       Role       `json:"role"`
    Content    string     `json:"content,omitempty"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant만 사용
    ToolCallID string     `json:"tool_call_id,omitempty"` // tool role만 사용
}

// ToolCall은 LLM이 요청한 도구 호출을 나타낸다.
type ToolCall struct {
    ID       string       `json:"id"`
    Type     string       `json:"type"` // 항상 "function"
    Function FunctionCall `json:"function"`
}

// FunctionCall은 호출할 함수명과 인자를 나타낸다.
type FunctionCall struct {
    Name      string `json:"name"`
    Arguments string `json:"arguments"` // JSON 문자열
}

// Response는 LLM API 응답 결과를 나타낸다.
type Response struct {
    Content      string
    ToolCalls    []ToolCall
    InputTokens  int
    OutputTokens int
}

// ToolDef는 LLM에 전달할 도구 정의를 나타낸다.
// OpenAI function calling 스키마 형식.
type ToolDef struct {
    Type     string      `json:"type"` // "function"
    Function FunctionDef `json:"function"`
}

// FunctionDef는 도구의 이름, 설명, 파라미터 스키마를 나타낸다.
type FunctionDef struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"`
}
```

### 3.2 Executor 타입 (`internal/executor/executor.go`)

```go
// ExecResult는 명령 실행 결과를 나타낸다.
type ExecResult struct {
    Stdout   string
    Stderr   string
    ExitCode int
    Duration time.Duration
}
```

### 3.3 설정 타입 (`internal/config/config.go`)

```go
// Config는 infractl 전역 설정을 나타낸다.
type Config struct {
    LLM LLMConfig `yaml:"llm"`
}

// LLMConfig는 LLM 연결 설정을 나타낸다.
type LLMConfig struct {
    Endpoint string `yaml:"endpoint"`
    Model    string `yaml:"model"`
    APIKey   string `yaml:"api_key"`
    Mode     string `yaml:"mode"`    // "full", "assisted", "basic"
    Timeout  int    `yaml:"timeout"` // 초 단위, 기본 60
}
```

---

## 4. 인터페이스 정의

### 4.1 LLM Client (`internal/llm/client.go`)

```go
// Client는 LLM API와 통신하는 인터페이스이다.
// 구현체: OpenAIClient (OpenAI 호환 API용)
type Client interface {
    // Chat는 동기 방식으로 LLM에 메시지를 전송하고 응답을 받는다.
    Chat(ctx context.Context, messages []Message, tools []ToolDef) (Response, error)

    // ChatStream은 스트리밍 방식으로 LLM에 메시지를 전송한다.
    // onToken 콜백으로 텍스트 토큰을 실시간 전달한다.
    // tool_calls가 포함된 경우 최종 Response에 조합하여 반환한다.
    ChatStream(ctx context.Context, messages []Message, tools []ToolDef, onToken func(token string)) (Response, error)
}
```

### 4.2 Executor (`internal/executor/executor.go`)

```go
// Executor는 쉘 명령을 실행하는 인터페이스이다.
// 구현체: LocalExecutor (로컬), SSHExecutor (Phase 2)
type Executor interface {
    // Execute는 주어진 명령을 실행하고 결과를 반환한다.
    Execute(ctx context.Context, command string) (ExecResult, error)

    // Target은 이 executor가 실행하는 대상을 반환한다.
    // LocalExecutor: "localhost", SSHExecutor: 서버명
    Target() string
}
```

### 4.3 Tool (`internal/tools/tool.go`)

```go
// RiskLevel은 도구의 위험도 수준을 나타낸다.
type RiskLevel string

const (
    RiskNone   RiskLevel = "none"   // 읽기 전용, 바로 실행
    RiskLow    RiskLevel = "low"    // 1회 확인 (y/n)
    RiskMedium RiskLevel = "medium" // 2회 확인 + 대상 명시
    RiskHigh   RiskLevel = "high"   // 3회 확인 + 대상 이름 직접 입력
)

// Tool은 에이전트가 사용할 수 있는 도구 인터페이스이다.
type Tool interface {
    // Name은 도구의 고유 이름을 반환한다. (예: "shell_exec")
    Name() string

    // Description은 LLM이 도구를 이해하기 위한 설명을 반환한다.
    Description() string

    // Parameters는 OpenAI function calling 형식의 JSON Schema를 반환한다.
    Parameters() map[string]interface{}

    // RiskLevel은 이 도구의 위험도 수준을 반환한다.
    RiskLevel() RiskLevel

    // IsReadOnly는 이 도구가 읽기 전용(상태 변경 없음)인지 반환한다.
    // true면 병렬 실행 가능, false면 순차 실행.
    // shell_exec 같은 범용 실행 도구는 false를 반환해야 한다.
    IsReadOnly() bool

    // IsEnabled는 이 도구를 현재 컨텍스트에서 LLM에 노출할지 반환한다.
    // false면 ToToolDefs()에 포함되지 않아 LLM이 호출 불가.
    // 폐쇄망에서 web_search 비활성화 등에 활용한다.
    IsEnabled() bool

    // Execute는 도구를 실행한다.
    // args는 LLM이 생성한 JSON 인자를 파싱한 map이다.
    // executor는 명령 실행에 사용할 Executor이다.
    Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error)
}
```

**도구별 IsReadOnly 분류:**

| 도구 | IsReadOnly | 이유 |
|------|-----------|------|
| shell_exec | false | 명령 내용에 따라 상태 변경 가능 |
| file_read | true | 읽기만 수행 |
| file_write | false | 파일 생성/수정 |
| process_list | true | 읽기만 수행 |
| network_info | true | 읽기만 수행 |
| server_add | false | DB + Manager 상태 변경 |
| server_remove | false | DB + Manager 상태 변경 |
| server_list | true | 읽기만 수행 |

### 4.4 EventHandler (`internal/agent/handler.go`)

```go
// EventHandler는 에이전트 루프의 이벤트를 UI로 전달하는 인터페이스이다.
// Phase 1: CLI REPL이 구현. Phase 3: TUI가 구현. Phase 7: Web UI가 구현.
type EventHandler interface {
    // OnThinking은 LLM이 응답 생성을 시작했을 때 호출된다.
    OnThinking()

    // OnToken은 LLM 스트리밍 응답의 토큰 하나를 받았을 때 호출된다.
    OnToken(token string)

    // OnToolStart는 도구 실행이 시작될 때 호출된다.
    OnToolStart(toolName string, args map[string]interface{})

    // OnToolEnd는 도구 실행이 완료되었을 때 호출된다.
    OnToolEnd(toolName string, result string, duration time.Duration, success bool)

    // OnResponse는 LLM의 최종 텍스트 응답이 완성되었을 때 호출된다.
    OnResponse(content string)

    // OnError는 에러가 발생했을 때 호출된다.
    OnError(err error)
}
```

---

## 5. 파일별 구현 스펙

### 5.1 `cmd/infractl/main.go`

**책임**: CLI 엔트리포인트. 서브커맨드 분기.

```
infractl              → runREPL()       # CLI 모드
infractl init         → runInit()       # 초기 설정
infractl version      → 버전 출력
infractl help         → 사용법 출력
infractl daemon       → "Phase 7에서 구현" 메시지 (placeholder)
```

- 서브커맨드 파싱: `os.Args` 직접 파싱 (외부 라이브러리 불필요)
- 설정 로드 실패 시: `infractl init` 안내 메시지
- slog 기본 핸들러 설정 (stderr, INFO 레벨)

### 5.2 `internal/config/config.go`

**책임**: 설정 구조체, YAML 로드/저장, init 대화 흐름.

**주요 함수:**
```go
func DefaultConfigDir() (string, error)    // ~/.infractl 경로 반환
func Load() (*Config, error)               // config.yaml 로드 + 환경변수 오버라이드
func Save(cfg *Config) error               // config.yaml 저장
func RunInit() error                       // 대화형 초기 설정
func Exists() bool                         // config.yaml 존재 여부
```

**환경변수 오버라이드:**
- `INFRACTL_API_KEY` → `llm.api_key`
- `INFRACTL_LLM_ENDPOINT` → `llm.endpoint`
- `INFRACTL_LLM_MODEL` → `llm.model`

**`config.yaml`에서 `${ENV_VAR}` 문법:**
- `api_key: "${INFRACTL_API_KEY}"` → 로드 시 환경변수로 치환

**`infractl init` 흐름:**
1. `~/.infractl/` 디렉토리 생성
2. LLM endpoint 입력 (기본값: `http://localhost:11434/v1`)
3. 모델명 입력 (기본값: `qwen3.5:27b`)
4. API Key 입력 (선택, 빈 값 허용)
5. `config.yaml` 저장
6. 연결 테스트 (LLM에 간단한 메시지 전송)

### 5.3 `internal/llm/openai.go`

**책임**: OpenAI 호환 API HTTP 클라이언트. 스트리밍(SSE) 지원.

**구현 핵심:**

1. **요청 형식** (`POST /v1/chat/completions`):
```json
{
  "model": "qwen3.5:27b",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "디스크 사용량 보여줘"},
    {"role": "assistant", "content": null, "tool_calls": [...]},
    {"role": "tool", "tool_call_id": "call_1", "content": "..."}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "shell_exec",
        "description": "Execute a shell command",
        "parameters": {"type": "object", "properties": {...}, "required": [...]}
      }
    }
  ],
  "stream": true
}
```

2. **SSE 스트리밍 파싱**:
   - `data: {...}` 줄 단위 파싱
   - `data: [DONE]` → 종료
   - `choices[0].delta.content` → 텍스트 토큰 → `onToken()` 콜백
   - `choices[0].delta.tool_calls` → 인덱스별로 버퍼에 누적하여 조합
   - tool_calls의 `arguments`는 여러 chunk에 걸쳐 스트리밍됨 → 전부 모아서 하나의 JSON으로 조합

3. **에러 처리**:
   - HTTP 4xx/5xx → `fmt.Errorf("llm api %d: %s: %w", status, body, err)`
   - 타임아웃 → context 기반, 설정의 `timeout` 값 사용
   - 재시도: **하지 않음** (에이전트 루프에서 LLM에게 에러 전달하여 판단)

4. **API Key 전달**:
   - 비어있지 않으면: `Authorization: Bearer <api_key>` 헤더

### 5.4 `internal/agent/loop.go`

**책임**: 에이전트 루프 코어. 사용자 입력을 받아 LLM과 도구를 반복 호출하여 최종 답변 생성.

**Agent 구조체:**
```go
type Agent struct {
    llmClient        llm.Client
    registry         *tools.Registry
    manager          *executor.Manager
    store            store.ServerStore
    handler          EventHandler
    history          []llm.Message  // 대화 히스토리
    activeServer     *store.Server  // 현재 활성 서버 (단일 서버 모드)
    maxHistory       int            // 최대 보존 턴 수 (기본 50)
    maxToolLoop      int            // 도구 반복 최대 횟수 (기본 20)
    maxContextTokens int            // 컨텍스트 토큰 한계 (compaction.go)
}
```

**주요 함수:**
```go
func New(client llm.Client, registry *tools.Registry, mgr *executor.Manager, handler EventHandler, st store.ServerStore) *Agent
func (a *Agent) Run(ctx context.Context, userInput string) error
func (a *Agent) SetMaxContextTokens(n int)
func (a *Agent) SetActiveServer(srv store.Server)
func (a *Agent) ClearActiveServer()
func (a *Agent) ClearHistory()
```

**`Run()` 상세 흐름:**

```
1. userInput을 Message{Role: RoleUser, Content: userInput}으로 변환, history에 추가
2. compactIfNeeded(ctx) — 토큰 critical/overflow면 LLM 요약으로 history 압축
3. 시스템 프롬프트 구성 (BuildContextual: OS정보, 도구목록, 서버목록, activeServer)
4. toolDefs = registry.ToToolDefs() (IsEnabled=true인 도구만)

[루프 시작] (최대 maxToolLoop 회)
5. handler.OnThinking()
6. response, err = llmClient.ChatStream(ctx, messages, toolDefs, onThinkingToken, onToken)
7. err != nil → handler.OnError(err), return
8. response.ToolCalls가 있으면:
   a. assistant 메시지(tool_calls 포함)를 history에 추가
   b. executeToolCalls(ctx, toolCalls):
      - partitionToolCalls → readOnly 목록, mutation 목록으로 분류
      - readOnly 도구들: sync.WaitGroup으로 병렬 실행 (순서 보존)
      - mutation 도구들: 순차 실행 (원래 순서 유지)
      - 각 도구 실행 시:
        * target 추출 → ExecutorManager에서 Executor 획득
        * tool.Execute(ctx, args, exec)
        * 실패 시: 에러 메시지 + knowledge_search/web_search 힌트 추가
   c. tool 결과 메시지들을 history에 추가
   d. 루프 계속
9. response.ToolCalls가 없으면 (텍스트 응답):
   a. assistant 메시지를 history에 추가
   b. handler.OnResponse(response.Content)
   c. trimHistory() → maxHistory 초과 시 오래된 메시지 제거
   d. 루프 종료
```

**병렬/직렬 실행 분리:**
```go
// indexedToolCall은 원래 순서 인덱스를 보존하는 래퍼이다.
type indexedToolCall struct {
    call          llm.ToolCall
    originalIndex int
}

// partitionToolCalls: IsReadOnly()=true → readOnly, false → mutation
// executeToolCalls: readOnly는 goroutine+WaitGroup, mutation은 for loop
```

**에러 힌트 주입:**
도구 실행 실패 시 에러 메시지에 힌트를 추가하여 LLM이 다음 단계를 판단할 수 있게 한다:
```
Error: <원본 에러>

[Suggested next steps]
- Use knowledge_search to check if this error has been resolved before.
- Use web_search to find solutions for this error online.
```

**히스토리 관리:**
- `trimHistory()`: assistant+tool_calls+tool_results 그룹을 원자적으로 제거
- system 메시지는 매 Run() 호출마다 새로 생성 (히스토리에 포함하지 않음)
- 토큰 기반 관리는 `compaction.go`에서 담당 (5.4-A 참조)

### 5.4-A `internal/agent/compaction.go` (신규)

**책임**: 토큰 기반 컨텍스트 관리. 히스토리 압축으로 LLM 컨텍스트 초과 방지.

**TokenState 상태 4단계:**
```go
type TokenState string
const (
    TokenOK       TokenState = "ok"       // 토큰 사용량 < 80%
    TokenWarning  TokenState = "warning"  // 80~95% — slog.Warn 출력
    TokenCritical TokenState = "critical" // 95~100% — compaction 수행
    TokenOverflow TokenState = "overflow" // 100% 초과 — 즉시 compaction
)
```

**상수:**
```go
defaultMaxContextTokens = 128_000  // 기본 모델 컨텍스트 크기
avgCharsPerToken        = 3        // 한국어/영어 혼합 추정치
tokenWarningThreshold   = 0.80
tokenCriticalThreshold  = 0.95
```

**주요 함수:**
```go
func (a *Agent) checkTokenState() TokenState   // 현재 토큰 사용률 계산
func (a *Agent) compactIfNeeded(ctx context.Context)  // critical/overflow면 압축
func (a *Agent) requestCompactionSummary(ctx context.Context) (string, error)
func estimateTokens(messages []llm.Message) int  // len(content)/3 추정
```

**compaction 흐름:**
```
checkTokenState() → critical 또는 overflow
  ↓
requestCompactionSummary(): LLM에 "대화를 2000토큰 이내로 요약해줘" 요청
  - 인프라 컨텍스트, 발견된 문제, 수행한 작업, 현재 상태, 핵심 발견을 포함
  - 실패 시 → trimHistory()로 폴백
  ↓
history를 요약 2개 메시지로 교체:
  [User]      "[Previous conversation summary]\n{요약}\n\n[Continue from here]"
  [Assistant] "Understood. I have the context from our previous conversation."
```

**SetMaxContextTokens()**: 모델 변경 시 호출하여 compaction 임계값 조정.

---

### 5.5 `internal/agent/prompt.go`

**책임**: LLM에 전달하는 시스템 프롬프트 구성.

**주요 함수:**
```go
func BuildContextual(toolList []tools.Tool, infractlMD string, servers []store.Server, activeServer *store.Server) string
func LoadInfractlMD() string
```

**activeServer가 있을 때 (단일 서버 세션):**
- ACTIVE SESSION CONTEXT 섹션으로 대상 서버 명시
- OS/EnvProfile 기반 명령어 가이드라인 주입 (Windows/Ubuntu/CentOS)
- 사용자가 서버명을 명시하지 않아도 해당 서버에서 동작

**activeServer가 없을 때 (다중 서버 관리):**
- 로컬 환경 정보 출력 (OS, hostname, cwd)
- Known Servers Pool 목록 표시

**시스템 프롬프트 섹션 구성:**
```
1. 역할 선언
2. [ACTIVE SESSION CONTEXT] 또는 [Current Environment] — 컨텍스트에 따라 선택
3. ## Available Tools — IsEnabled()=true인 도구 목록
4. ## Known Servers Pool — 등록된 서버 목록
5. ## Tool Selection Guidelines — 도구 선택 의사결정 트리
6. ## Behavior Rules — 행동 원칙
7. ## Project-Specific Instructions — INFRACTL.md 내용 (있으면)
```

**Tool Selection Guidelines (도구 선택 의사결정 트리):**

```
Information Gathering 우선순위:
1. 이미 알면 → 바로 답변 (도구 불필요)
2. 서버 데이터 필요 → shell_exec 사용
3. 알려진 에러 패턴 → knowledge_search 먼저 (사용 가능 시)
4. knowledge_search 미적중 + 인터넷 가능 → web_search
5. URL 상세 내용 필요 → web_fetch

Error Resolution Flow (도구 실패 시):
1. 에러 메시지 스스로 분석
2. knowledge_search로 유사 패턴 검색
3. 미적중 → web_search
4. 해결 성공 → knowledge_base 저장 제안

Service Discovery Flow (미지의 서비스):
1. process_list + network_info로 스캔
2. 서비스 식별 → 해당 커넥터 활성화
3. 미식별 → web_search로 관리 명령 학습

Multi-Server Operations:
- 읽기 전용 (ps, df, SELECT) → 여러 서버 병렬 가능
- 변경 명령 → 서버 하나씩, 확인 후 실행
```

**INFRACTL.md 로딩 (`LoadInfractlMD()`):**
- `~/.infractl/INFRACTL.md` 파일을 읽어 반환
- 없으면 빈 문자열 (에러 아님, `slog.Debug`만 출력)

### 5.6 `internal/executor/local.go`

**책임**: 로컬 쉘 명령 실행.

**핵심 구현:**
```go
func (e *LocalExecutor) Execute(ctx context.Context, command string) (ExecResult, error) {
    // OS별 분기
    // Linux/Mac: exec.CommandContext(ctx, "bash", "-c", command)
    // Windows:   exec.CommandContext(ctx, "cmd", "/C", command)

    // stdout, stderr를 bytes.Buffer로 캡처
    // cmd.Run() 실행
    // ExitCode 추출: cmd.ProcessState.ExitCode()
    // 실행 시간 측정: time.Since(start)
}
```

**주의사항:**
- `context`로 타임아웃 제어 (도구에서 context에 timeout 설정)
- 프로세스 kill 시 자식 프로세스도 함께 종료 (process group)
- stdout/stderr 크기 제한: 최대 64KB (초과 시 truncate + 안내 메시지)

### 5.7 도구 구현 (`internal/tools/`)

#### `shell_exec.go`
```
이름: shell_exec
설명: Execute a shell command on the target system
파라미터:
  - command (string, 필수): 실행할 쉘 명령
  - timeout (integer, 선택): 타임아웃 초 (기본 30)
위험도: none
동작: executor.Execute(ctx, command) 호출, stdout+stderr+exitCode 반환
반환 형식:
  [Exit Code: 0]
  <stdout 내용>

  [Stderr]
  <stderr 내용 (있으면)>
```

#### `file_read.go`
```
이름: file_read
설명: Read contents of a file
파라미터:
  - path (string, 필수): 파일 경로
  - lines (integer, 선택): 읽을 최대 줄 수 (기본 전체, 최대 500)
위험도: none
동작: shell_exec으로 cat 또는 head -n 실행
```

#### `file_write.go`
```
이름: file_write
설명: Write content to a file
파라미터:
  - path (string, 필수): 파일 경로
  - content (string, 필수): 쓸 내용
  - append (boolean, 선택): true면 추가, false면 덮어쓰기 (기본 false)
위험도: low
동작: shell_exec으로 echo/cat heredoc 실행
```

#### `process_list.go`
```
이름: process_list
설명: List running processes with resource usage
파라미터:
  - filter (string, 선택): 프로세스 이름 필터 (grep)
위험도: none
동작:
  Linux/Mac: ps aux (+ grep filter)
  Windows: tasklist
```

#### `network_info.go`
```
이름: network_info
설명: Show network information (listening ports or connections)
파라미터:
  - type (string, 선택): "listen" (기본) 또는 "connections"
위험도: none
동작:
  Linux: ss -tlnp 또는 ss -tnp
  Mac: netstat -an
  Windows: netstat -an
```

**공통 규칙:**
- 모든 도구에 `target` 파라미터를 포함하되, Phase 1에서는 무시 (Phase 2에서 SSH 라우팅)
- 도구 실행 결과는 항상 string으로 반환 (LLM이 해석)
- 에러 발생 시 에러 메시지를 string으로 반환 (LLM이 에러 분석)

### 5.8 `internal/tools/registry.go`

**책임**: 도구 등록/조회/해제, LLM에 전달할 JSON Schema 변환.

```go
type Registry struct {
    tools map[string]Tool
}

func NewRegistry() *Registry
func (r *Registry) Register(tool Tool) error          // 중복 이름 체크
func (r *Registry) Unregister(name string)            // 커넥터 비활성화 시 도구 제거
func (r *Registry) Has(name string) bool              // 등록 여부 확인
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) List() []Tool                      // 전체 도구 (이름순 정렬)
func (r *Registry) GetEnabled() []Tool                // IsEnabled()=true인 도구만
func (r *Registry) ToToolDefs() []llm.ToolDef         // GetEnabled() 기반 LLM API용 변환
```

**`GetEnabled()` vs `List()`:**
- `List()`: 전체 도구 목록 (관리용, 시스템 프롬프트에는 사용 안 함)
- `GetEnabled()`: 현재 컨텍스트에서 활성화된 도구만 (LLM에 전달, 시스템 프롬프트에 사용)
- `ToToolDefs()`는 내부적으로 `GetEnabled()`를 호출

**커넥터 활성화/비활성화 패턴:**
```go
// 커넥터 도구 세트 활성화
for _, tool := range connector.GenerateTools(serviceInfo, creds) {
    registry.Register(tool)
}

// 커넥터 도구 세트 비활성화
for _, name := range connector.ToolNames() {
    registry.Unregister(name)
}
```

**`ToToolDefs()` 변환:**
각 Tool의 Name(), Description(), Parameters()를 OpenAI function calling 형식으로 변환:
```json
{
  "type": "function",
  "function": {
    "name": "shell_exec",
    "description": "Execute a shell command on the target system",
    "parameters": {
      "type": "object",
      "properties": {
        "command": {"type": "string", "description": "The shell command to execute"},
        "timeout": {"type": "integer", "description": "Timeout in seconds (default: 30)"},
        "target": {"type": "string", "description": "Target server name (default: localhost)"}
      },
      "required": ["command"]
    }
  }
}
```

### 5.9 `internal/cli/repl.go`

**책임**: readline 기반 대화형 입력/출력.

**핵심 동작:**
1. 배너 출력: 버전, LLM 모델, 연결 상태
2. `> ` 프롬프트로 입력 대기
3. 슬래시 명령 처리:
   - `/help` → 도움말 출력
   - `/quit` 또는 `/exit` → 종료
   - `/tools` → 등록된 도구 목록
   - `/clear` → 대화 히스토리 초기화
   - `/model` → 현재 LLM 모델 표시
4. 일반 입력 → `agent.Run(ctx, input)`
5. `Ctrl+C` → 실행 중이면 중단, 대기 중이면 종료
6. `Ctrl+D` → 종료 (EOF)

**EventHandler 구현 (REPLHandler):**
```
OnThinking()     → "⏺ Thinking..." 출력
OnToken(token)   → 토큰을 즉시 stdout에 출력 (줄바꿈 없이)
OnToolStart()    → "  ▶ [도구명] 실행 중..." 출력
OnToolEnd()      → "  ✓ 완료 (0.5s)" 또는 "  ✗ 실패" 출력
OnResponse()     → 최종 응답 후 빈 줄
OnError()        → "Error: ..." 빨간색 출력
```

---

## 6. 에이전트 루프 상세 흐름도

```
┌─ 사용자 ─┐
│  입력     │
└────┬──────┘
     ↓
┌─ REPL ───────────────────────────────────────────────────┐
│  슬래시 명령? ─ 예 → 처리 (help, quit, tools 등)          │
│       ↓ 아니오                                            │
│  agent.Run(ctx, input) 호출                               │
└────┬──────────────────────────────────────────────────────┘
     ↓
┌─ Agent Loop ─────────────────────────────────────────────┐
│                                                           │
│  1. user 메시지를 history에 추가                           │
│  2. compactIfNeeded() — 토큰 >95%면 LLM 요약으로 압축     │
│  3. system 프롬프트 구성 (도구 목록 + 서버 목록 + 가이드)  │
│  4. messages = [system] + history                         │
│                                                           │
│  ┌─ LLM 호출 루프 (최대 20회) ────────────────────────┐   │
│  │                                                     │   │
│  │  5. LLM.ChatStream(messages, tools, onToken)        │   │
│  │          ↓                                          │   │
│  │  6. tool_calls 있음?                                │   │
│  │     ├─ 예 → partitionToolCalls()                    │   │
│  │     │       ├─ IsReadOnly=true → goroutine 병렬 실행│   │
│  │     │       └─ IsReadOnly=false → 순차 실행          │   │
│  │     │       결과를 tool 메시지로 추가 (순서 보존)     │   │
│  │     │       루프 계속 ─────────────────→ (5번으로)   │   │
│  │     │                                               │   │
│  │     └─ 아니오 → 텍스트 응답                          │   │
│  │                 history에 추가                       │   │
│  │                 handler.OnResponse()                 │   │
│  │                 trimHistory()                        │   │
│  │                 루프 종료                             │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                           │
└───────────────────────────────────────────────────────────┘
     ↓
  다음 입력 대기
```

---

## 7. LLM API 연동 스펙

### 7.1 엔드포인트

- `POST {endpoint}/chat/completions`
- Content-Type: `application/json`
- Authorization: `Bearer {api_key}` (api_key가 비어있지 않을 때만)

### 7.2 스트리밍 SSE 파싱 규칙

```
1. 응답 Body를 줄 단위로 읽음
2. "data: " 접두사를 떼고 JSON 파싱
3. "data: [DONE]" → 스트림 종료
4. choices[0].delta 파싱:
   - delta.content → 텍스트 토큰 → onToken() 콜백 + 버퍼에 누적
   - delta.tool_calls → 인덱스별 버퍼에 누적
     - tool_calls[i].id → 새 도구 호출 시작
     - tool_calls[i].function.name → 함수명 설정
     - tool_calls[i].function.arguments → 기존 arguments에 이어붙임
5. 스트림 종료 후:
   - 텍스트 버퍼 → Response.Content
   - tool_calls 버퍼 → Response.ToolCalls
   - usage → Response.InputTokens, OutputTokens
```

### 7.3 tool_calls 스트리밍 조합 예시

```
chunk 1: delta.tool_calls = [{"index":0, "id":"call_abc", "type":"function", "function":{"name":"shell_exec", "arguments":""}}]
chunk 2: delta.tool_calls = [{"index":0, "function":{"arguments":"{\"co"}}]
chunk 3: delta.tool_calls = [{"index":0, "function":{"arguments":"mmand"}}]
chunk 4: delta.tool_calls = [{"index":0, "function":{"arguments":"\": \"df -h\"}"}}]

→ 최종 조합 결과:
ToolCall{ID: "call_abc", Type: "function", Function: {Name: "shell_exec", Arguments: "{\"command\": \"df -h\"}"}}
```

---

## 8. 설정 파일

### 8.1 경로
`~/.infractl/config.yaml`

### 8.2 최소 설정
```yaml
llm:
  endpoint: "http://localhost:11434/v1"
  model: "qwen3.5:27b"
  api_key: ""
  mode: "full"
  timeout: 60
```

### 8.3 외부 API 예시
```yaml
llm:
  endpoint: "https://api.openai.com/v1"
  model: "gpt-4o"
  api_key: "${INFRACTL_API_KEY}"
  mode: "full"
  timeout: 120
```

---

## 9. 메인 엔트리포인트 분기

```
infractl              → config 로드 → Agent 생성 → REPL 시작
infractl init         → RunInit() (대화형 설정)
infractl version      → "infractl v0.1.0" 출력
infractl help         → 사용법 출력
infractl daemon       → "Not implemented. Coming in Phase 7." 출력
```

---

## 10. 의존성

```
github.com/chzyer/readline   # REPL 입력 (히스토리, 라인 에디팅)
gopkg.in/yaml.v3             # YAML 설정 파일 파싱
```

- LLM API 호출: 표준 `net/http`로 직접 구현 (외부 SDK 의존 없음)
- JSON 파싱: 표준 `encoding/json`
- SSE 파싱: `bufio.Scanner` + `strings.TrimPrefix`로 직접 구현
- 로깅: 표준 `log/slog`

---

## 11. 구현 순서

각 단계는 이전 단계의 결과물에 의존한다.

```
Step 1: 프로젝트 초기화
  go.mod, 디렉토리 구조, 의존성

Step 2: 타입 & 인터페이스 정의
  llm/types.go, llm/client.go, executor/executor.go,
  tools/tool.go, agent/handler.go

Step 3: 설정
  config/config.go (로드/저장/init)

Step 4: Executor
  executor/local.go

Step 5: 도구 시스템
  tools/registry.go, tools/shell_exec.go,
  tools/file_read.go, tools/file_write.go,
  tools/process_list.go, tools/network_info.go

Step 6: LLM 클라이언트
  llm/openai.go (비스트리밍 먼저, 스트리밍 추가)

Step 7: 에이전트 루프
  agent/prompt.go, agent/loop.go

Step 8: REPL
  cli/repl.go

Step 9: 메인 엔트리포인트
  cmd/infractl/main.go

Step 10: 통합 테스트 & 검증
```

---

## 12. 검증 시나리오

### 12.1 기본 동작
```
$ infractl init
  LLM Endpoint [http://localhost:11434/v1]:
  Model [qwen3.5:27b]:
  API Key []:
  ✓ config.yaml 저장 완료
  ✓ LLM 연결 테스트 성공

$ infractl
  ● infractl v0.1.0 | qwen3.5:27b | localhost

  > 현재 디스크 사용량 보여줘
  ⏺ Thinking...
    ▶ [shell_exec] 실행 중...
    ✓ 완료 (0.3s)

  디스크 사용량:
  ┌──────────────┬───────┬───────┬──────┐
  │ Filesystem   │ Size  │ Used  │ Use% │
  ├──────────────┼───────┼───────┼──────┤
  │ /dev/sda1    │ 100G  │ 45G   │ 45%  │
  │ /dev/sdb1    │ 500G  │ 320G  │ 64%  │
  └──────────────┴───────┴───────┴──────┘
```

### 12.2 컨텍스트 유지
```
  > /tmp/test.txt 만들어줘 내용은 hello world
    ▶ [file_write] 실행 중...
    ✓ 완료
  ✓ /tmp/test.txt 파일을 생성했습니다.

  > 방금 만든 파일 보여줘
    ▶ [file_read] 실행 중...
    ✓ 완료
  /tmp/test.txt 내용:
  hello world
```

### 12.3 에러 처리
```
  > 존재하지 않는 파일 보여줘 /nonexistent/file.txt
    ▶ [file_read] 실행 중...
    ✗ 실패 (exit code 1)

  파일을 찾을 수 없습니다. 경로를 확인해주세요:
  /nonexistent/file.txt — No such file or directory
```

### 12.4 멀티 도구 호출
```
  > CPU와 메모리 사용량 같이 보여줘
    ▶ [shell_exec] 실행 중... (top -bn1 | head -5)
    ✓ 완료 (1.2s)
    ▶ [shell_exec] 실행 중... (free -h)
    ✓ 완료 (0.1s)

  CPU: 23.4% 사용 중
  메모리: 16GB 중 8.2GB 사용 (51%)
```

### 12.5 슬래시 명령
```
  > /help
  사용 가능한 명령:
    /help    — 도움말
    /tools   — 등록된 도구 목록
    /clear   — 대화 히스토리 초기화
    /model   — 현재 LLM 모델
    /quit    — 종료

  > /tools
  등록된 도구 (5개):
    shell_exec     — Execute a shell command
    file_read      — Read contents of a file
    file_write     — Write content to a file
    process_list   — List running processes
    network_info   — Show network information
```

---

## 13. 완료 기준

- [ ] `infractl init`으로 config.yaml 생성
- [ ] `infractl`로 REPL 진입, 배너 출력
- [ ] 자연어 → LLM → bash 명령 생성 → 실행 → 결과 해석 루프 동작
- [ ] 대화 컨텍스트 유지 (최근 50턴, "아까 그 파일" 참조 가능)
- [ ] 에러 발생 시 LLM이 에러 분석하여 재시도 또는 설명
- [ ] LLM 스트리밍 출력 (토큰 단위 실시간)
- [ ] tool_calls 스트리밍 조합 정상 동작
- [ ] 빌트인 도구 5종 동작 (shell_exec, file_read, file_write, process_list, network_info)
- [ ] 슬래시 명령 동작 (/help, /tools, /clear, /quit)
- [ ] Ollama + qwen3.5:27b로 동작 확인
- [ ] 환경변수 오버라이드 동작 (INFRACTL_API_KEY 등)
- [ ] Ctrl+C / Ctrl+D 정상 종료
- [ ] slog 기반 로깅 (stderr)
- [ ] 모든 소스 파일에 DocBlock 헤더 포함
- [ ] 파일당 300라인 이내
- [ ] Tool 인터페이스 IsReadOnly()/IsEnabled() 구현 (모든 빌트인 도구)
- [ ] 읽기 전용 도구 병렬 실행, 상태 변경 도구 순차 실행
- [ ] 토큰 기반 컨텍스트 관리 (compaction.go): 4단계 상태, critical 시 LLM 요약
- [ ] 시스템 프롬프트에 Tool Selection Guidelines 포함
- [ ] Registry.Unregister(), Has(), GetEnabled() 동작
