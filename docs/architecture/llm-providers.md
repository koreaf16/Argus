# LLM Provider 아키텍처

## 1. LLM 인터페이스

[`internal/services/llm/llm.go`](internal/services/llm/llm.go:30) 는 모든 LLM 공급자를 위한 공통 인터페이스를 정의합니다.

```go
type LLM interface {
    Stream(ctx context.Context, req Request) (<-chan Event, error)
    CountTokens(ctx context.Context, req Request) (int, error)
    Capabilities() Caps
    Provider() string
}
```

### Request

```go
type Request struct {
    Model     string
    System    []SystemBlock
    Messages  []Message
    Tools     []ToolSpec
    MaxTokens int
    Thinking  *ThinkingConfig
}
```

### Event

```go
type Event struct {
    Kind    EventKind
    Delta   string
    ToolUse *ToolUseStart
    Stop    *StopReason
}
```

### Capabilities

```go
type Caps struct {
    Tools             bool   // tool use 지원
    Thinking          bool   // extended thinking 지원
    Vision            bool   // 비전 (이미지) 지원
    WebSearch         bool   // 내장 웹 검색
    DefaultToolChoice string // "none" | "auto" | ""
}
```

## 2. 공급자 구현

### Anthropic ([`anthropic.go`](internal/services/llm/anthropic.go))

| 항목 | 값 |
|------|-----|
| API | `POST {baseURL}/messages` |
| Discovery | `GET {baseURL}/models?limit=1000` |
| Header | `x-api-key`, `anthropic-version: 2023-06-01` |
| Tool Use | `tool_use` / `tool_result` content block |
| Thinking | `extended_thinking` 옵션 |

### Gemini ([`gemini.go`](internal/services/llm/gemini.go))

| 항목 | 값 |
|------|-----|
| API | `POST {baseURL}/models/{model}:streamGenerateContent?alt=sse` |
| Discovery | `GET {baseURL}/models?pageSize=1000&key=...` |
| 필터 | `supportedGenerationMethods` includes `generateContent` |
| Tool Use | `functionCall` / `functionResponse` |

### OpenAI-compat ([`openai.go`](internal/services/llm/openai.go))

| 항목 | 값 |
|------|-----|
| API | `POST {baseURL}/chat/completions` |
| Discovery | `GET {baseURL}/models` |
| Auth | `Authorization: Bearer <key>` (optional — no-auth local 지원) |
| Tool Use | `function_call` / `tool_calls` |

### URL 처리 (OpenAI-compat)

- `host:port` → `http://host:port` 자동 정규화
- 경로 없는 URL → `<input>/v1`, `<input>` 순서로 probe
- `GET {baseURL}/models` 성공 시 `base_url` 저장

## 3. 모델 카탈로그 (Registry)

[`internal/services/llm/registry.go`](internal/services/llm/registry.go) 는 `~/.argus/models.json` 을 관리합니다.

### ModelEntry

```go
type ModelEntry struct {
    Alias       string  // 사용자 별칭 (고유)
    ModelID     string  // 실제 API 모델 ID
    Provider    string  // anthropic | gemini | openai-compat
    BaseURL     string  // openai-compat 전용
    APIKeyEnv   string  // env 변수 이름 (평문 저장 X)
    Display     string
    ContextWin  int     // 컨텍스트 윈도우 크기
    Caps        Caps    // 기능
}
```

### 기본 프리셋

| 공급자 | 모델 |
|--------|------|
| Anthropic | `claude-opus-4-7`, `claude-sonnet-4-6`, `claude-haiku-4-5` |
| Gemini | `gemini-2.5-pro`, `gemini-2.5-flash` |
| OpenAI | `gpt-4.1`, `gpt-4o`, `o4-mini` |

## 4. Context Window Enforcement

- estimated prompt tokens ≥ model context window → 턴 거절
- `promptTokens + MaxTokens` > window → `MaxTokens` clamp + notice

## 5. Tool Call 변환 레이어

[`toolcalls.go`](internal/services/llm/toolcalls.go) 는 각 공급자의 tool call 포맷을 내장 표준으로 변환합니다.

```
Anthropic tool_use ←→ OpenAI function_call ←→ Gemini functionCall
         │                      │                      │
         └──────────────────────┴──────────────────────┘
                            ↓
                   내부 표준 ToolSpec/ToolResult
```

## 6. Retry (`retry.go`)

지정된 재시도 정책으로 HTTP 요청 재시도를 처리합니다.
- Exponential backoff
- 5xx 에러 및 rate-limit (429) 대상
