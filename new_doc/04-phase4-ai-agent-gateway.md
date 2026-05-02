# Phase 4 — AI Agent Gateway · 감사 추적 · 세션 레코딩 (asciicast) ★두꺼운 파트★

> **권장 순서**: 2번째 (사용자 우선순위)
> **영향도**: 매우 높음 / **구현 난이도**: 높음 / **80% 효과 컷**: ✅ 필수

## 목표 한 줄 요약
LLM 기반 자동화 환경에서 모든 도구 호출을 가로채 Lethal Trifecta·prompt injection·tool poisoning을 정책으로 차단하고, 모든 의사결정을 append-only audit + 재생 가능한 asciicast로 보존한다.

## 1. 신규 도입 라이브러리/외부 의존성

- `github.com/golang-infrastructure/go-asciinema-parser` (MIT) — asciicast v2 writer/reader
- `go.opentelemetry.io/otel`, `otel/sdk` (Apache-2.0) — 트레이스
- `github.com/google/cel-go` (Apache-2.0) — 정책 표현식 (CEL)
- `github.com/google/uuid` (Apache-2.0)
- 라이선스 노트: 모두 MIT/Apache. MPL-2.0 라이브러리(`hashicorp/go-uuid`) 회피.

## 2. 신규/수정 파일

### 신규
- `internal/agentguard/gateway.go` — 핵심 게이트웨이
- `internal/agentguard/lethal_trifecta.go` — 3중 위험 감지
- `internal/agentguard/policy/policy.go` — CEL 기반 정책 엔진
- `internal/agentguard/injection.go` — prompt injection 휴리스틱
- `internal/agentguard/sandbox.go` — 도구 출력 → 다음 LLM 입력 사이의 신뢰 경계
- `internal/audit/recorder.go` — append-only JSONL
- `internal/audit/asciicast.go` — 세션 레코딩
- `internal/audit/replay.go` — 재생 CLI

### 수정
- `internal/services/mcp/client.go` 라인 183-214: `CallTool` 호출 전 Gateway.Authorize
- `internal/tools/bash/bash.go`: 호출 전 Gateway.Authorize
- `internal/services/workspace/manager.go` Exec/StartExec: Gateway 통합
- `cmd/argus/main.go`: 부트스트랩

## 3. 핵심 인터페이스/타입 정의

```go
// internal/agentguard/gateway.go
type ToolCall struct {
    ID         string
    Tool       string             // "Bash", "mcp__github__get_pr", ...
    Args       map[string]any
    SourceMsg  string             // LLM의 원본 메시지 (injection 분석용)
    Origin     ToolCallOrigin     // user / assistant / agent_loop / external
    Capability []string           // 호출 시점 보유 capability (ssh:exec 등)
    Trace      trace.SpanContext
}

type ToolCallOrigin string
const (
    OriginUser        ToolCallOrigin = "user"
    OriginAssistant   ToolCallOrigin = "assistant"
    OriginAgentLoop   ToolCallOrigin = "agent_loop"
    OriginExternal    ToolCallOrigin = "external_input" // ← Lethal Trifecta 조심
)

type Decision struct {
    Action     DecisionAction // allow / deny / ask / transform
    Reason     string
    PolicyID   string
    Risk       RiskScore
    AskPayload *AskUserPayload
    Mutated    map[string]any  // transform 시 인자 수정
}

type Gateway interface {
    Authorize(ctx context.Context, call ToolCall) (Decision, error)
    Record(ctx context.Context, call ToolCall, result ToolResult) error
}

// internal/agentguard/lethal_trifecta.go
type TrifectaContext struct {
    HasUntrustedInput   bool   // 외부 출처 도구 출력이 LLM 컨텍스트에 들어왔음
    HasPrivateData      bool   // 이 호출이 SSH/file-read 등 사적 데이터에 접근 가능
    HasExfilCapability  bool   // 이 호출이 외부로 데이터 보낼 수 있음 (HTTP, slack send 등)
    Sources             []string
}

// 3개 모두 true 면 자동 차단 + 사용자 prompt
func (g *Gateway) DetectTrifecta(ctx context.Context, call ToolCall) TrifectaContext

// internal/agentguard/policy/policy.go
type Rule struct {
    ID         string `json:"id"`
    When       string `json:"when"`        // CEL expression: e.g. "tool=='Bash' && args.cmd.contains('rm -rf')"
    Action     string `json:"action"`      // allow|deny|ask|transform
    Reason     string `json:"reason"`
    Transform  string `json:"transform,omitempty"` // CEL: 새 args 반환
    Severity   string `json:"severity"`    // low|medium|high|critical
}

type Engine struct { rules []Rule; program []cel.Program }
func (e *Engine) Evaluate(ctx context.Context, call ToolCall) (Decision, bool)

// internal/audit/recorder.go
type Event struct {
    Time       time.Time         `json:"time"`
    Type       string            `json:"type"` // tool_call|tool_result|llm_decision|policy_block|user_consent
    SessionID  string            `json:"session_id"`
    SpanID     string            `json:"span_id"`
    Tool       string            `json:"tool,omitempty"`
    Origin     string            `json:"origin,omitempty"`
    Args       json.RawMessage   `json:"args,omitempty"`     // redact.RedactJSON 적용
    Result     json.RawMessage   `json:"result,omitempty"`   // redacted
    Decision   string            `json:"decision,omitempty"`
    PolicyID   string            `json:"policy_id,omitempty"`
    UserAck    bool              `json:"user_ack,omitempty"`
    HashChain  string            `json:"hash_chain"` // prev_hash | sha256(this_event_canonical)
}

type Recorder struct {
    f       *os.File         // append-only, O_APPEND|O_CREATE|0o600
    mu      sync.Mutex
    prevHash string
    asciicast *AsciicastWriter
    otelTracer trace.Tracer
}

// internal/audit/asciicast.go : asciicast v2 호환
type AsciicastWriter struct {
    f         *os.File
    startedAt time.Time
    mu        sync.Mutex
}
func (a *AsciicastWriter) WriteHeader(width, height int, env map[string]string) error
func (a *AsciicastWriter) WriteEvent(t time.Time, kind string, data string) error // "o" output, "i" input, "m" marker
```

## 4. 핵심 의사코드

```go
// gateway.go : 모든 도구 호출이 통과하는 단일 진입점
func (g *Gateway) Authorize(ctx context.Context, call ToolCall) (Decision, error) {
    span := g.tracer.Start(ctx, "agentguard.authorize", trace.WithAttributes(
        attribute.String("tool", call.Tool),
        attribute.String("origin", string(call.Origin)),
    ))
    defer span.End()

    // 1. capability/scope 검증
    if !g.hasCapability(ctx, call) {
        d := Decision{Action: ActionDeny, Reason: "missing capability", PolicyID: "cap.required"}
        g.recordDecision(call, d); return d, nil
    }

    // 2. Lethal Trifecta
    tri := g.DetectTrifecta(ctx, call)
    if tri.HasUntrustedInput && tri.HasPrivateData && tri.HasExfilCapability {
        // 자동 ask + 명시적 confirmation 요구
        d := Decision{Action: ActionAsk, Reason: "Lethal Trifecta detected",
            PolicyID: "trifecta.block",
            AskPayload: &AskUserPayload{
                Title: "잠재적 데이터 유출 위험 도구 호출",
                Detail: fmt.Sprintf("이 도구는 신뢰할 수 없는 입력(%s)을 받은 컨텍스트에서 사적 데이터에 접근하고 외부로 데이터를 보낼 수 있습니다.", strings.Join(tri.Sources, ",")),
                Risk: RiskCritical,
            },
        }
        g.recordDecision(call, d); return d, nil
    }

    // 3. prompt injection 휴리스틱 (도구 출력에서 "ignore previous instructions" 류)
    if call.Origin == OriginExternal && injection.LooksMalicious(call.Args) {
        d := Decision{Action: ActionDeny, Reason: "prompt injection signature", PolicyID: "inj.signature"}
        g.recordDecision(call, d); return d, nil
    }

    // 4. 정책 엔진 (CEL)
    if d, ok := g.policy.Evaluate(ctx, call); ok {
        g.recordDecision(call, d); return d, nil
    }

    // 5. 기본값: 도구별 권한 모드 적용 (기존 PermissionMode 호환)
    return g.fallbackDecision(ctx, call), nil
}

// lethal_trifecta.go
func (g *Gateway) DetectTrifecta(ctx context.Context, call ToolCall) TrifectaContext {
    sess := g.session(ctx)
    out := TrifectaContext{}

    // (a) 최근 N개 도구 결과 중 OriginExternal 출력이 LLM 컨텍스트에 들어갔는지
    for _, r := range sess.RecentResults(20) {
        if r.Origin == OriginExternal { out.HasUntrustedInput = true; out.Sources = append(out.Sources, r.Tool) }
    }

    // (b) 이 호출이 사적 데이터 접근 capability 가지는지
    privateTools := map[string]bool{"Read":true, "Bash":true, "ssh.exec":true, "ssh.read":true,
        "mcp__github__get_pr": true /* 사적 PR 등 */}
    out.HasPrivateData = privateTools[call.Tool] || strings.HasPrefix(call.Tool, "mcp__github__")

    // (c) 이 호출이 exfil 가능한지
    exfilTools := map[string]bool{"WebFetch": true, "Bash": true /* curl/scp 가능 */,
        "mcp__slack__send": true, "mcp__gmail__send": true}
    out.HasExfilCapability = exfilTools[call.Tool]
    return out
}

// injection.go
func LooksMalicious(args map[string]any) bool {
    text := flattenStrings(args)
    patterns := []*regexp.Regexp{
        regexp.MustCompile(`(?i)ignore\s+(all|previous|the)\s+(instructions|rules)`),
        regexp.MustCompile(`(?i)you\s+are\s+now\s+\w+\s+model`),
        regexp.MustCompile(`(?i)<\s*system\s*>`),
        regexp.MustCompile(`(?i)\bexfiltrate\b|\bsend\s+to\s+attacker\b`),
        regexp.MustCompile(`(?i)curl\s+[^\s]*\?[^\s]*\$\{?[A-Z_]+\}?`), // env exfil
    }
    for _, p := range patterns { if p.MatchString(text) { return true } }
    return false
}

// audit/recorder.go : hash chain (tamper-evident)
func (r *Recorder) Append(e Event) error {
    r.mu.Lock(); defer r.mu.Unlock()
    e.HashChain = "" // canonical 계산용
    canon, _ := json.Marshal(e)
    h := sha256.New(); h.Write([]byte(r.prevHash)); h.Write(canon)
    e.HashChain = hex.EncodeToString(h.Sum(nil))
    r.prevHash = e.HashChain
    line, _ := json.Marshal(e); line = append(line, '\n')
    if _, err := r.f.Write(line); err != nil { return err }
    return r.f.Sync() // fsync (감사 무결성 우선)
}

// asciicast.go : v2 포맷
func (a *AsciicastWriter) WriteHeader(w, h int, env map[string]string) error {
    hdr := map[string]any{"version":2, "width":w, "height":h, "timestamp": a.startedAt.Unix(), "env": env}
    b, _ := json.Marshal(hdr); _, err := fmt.Fprintln(a.f, string(b)); return err
}
func (a *AsciicastWriter) WriteEvent(t time.Time, kind, data string) error {
    a.mu.Lock(); defer a.mu.Unlock()
    delta := t.Sub(a.startedAt).Seconds()
    arr := []any{delta, kind, data}
    b, _ := json.Marshal(arr); _, err := fmt.Fprintln(a.f, string(b)); return err
}
```

## 5. OpenSSH 호환성 노트

직접적 ssh_config 매핑은 없으나, audit log는 `LogLevel VERBOSE`처럼 동작. asciicast는 `script`/`scriptreplay` 호환 형식 대신 asciinema 표준 채택 — 재생 도구가 이미 풍부.

## 6. 마이그레이션/기존 코드 영향

- 기존 `internal/utils/permissions/`는 그대로 유지. Gateway가 이를 wrapping. 기존 PermissionMode (`acceptEdits`, `bypassPermissions`)는 그대로 사용 가능.
- ⚠️ **default 모드에서도 Lethal Trifecta 차단을 켤지**: 기본 ON 권장 (사용자 결정 필요). ON 시 일부 자동화 워크플로가 사용자 승인 prompt를 추가 받게 됨.
- `internal/redact/redact.go`는 그대로 사용. Recorder.Append 전에 `redact.RedactJSON(args)` 의무.

## 7. 테스트 전략

- **단위**: 모든 Lethal Trifecta 조합 (8가지) 테스트.
- **Property-based**: CEL 정책 무한 변형 fuzz, panic 없음.
- **Adversarial**: AgentDojo 유사 prompt-injection 데이터셋 100개 케이스로 false-negative <5% 목표.
- **통합**: asciicast 파일을 `asciinema play`로 외부 도구로 재생 검증.
- **Hash chain**: 임의의 1줄 수정 후 검증 도구가 무결성 깨짐을 보고.
- **OpenTelemetry**: Jaeger/Tempo 백엔드로 trace 시각화.

## 8. 위험 및 완화

1. **정책 false-positive로 정상 자동화 차단** → 정책 dry-run 모드(`policy.dry_run=true`) + 카운터.
2. **asciicast 파일 비대화 (수십 MB/세션)** → 시간 기반 회전 + gzip.
3. **Gateway가 LLM 응답 지연 ↑** → Authorize는 1~3ms 이내 목표 (CEL precompile).
4. **Recorder fsync로 SSD 마모** → 배치 sync(100ms 또는 64KB) 옵션.
5. **prompt-injection 휴리스틱 우회** → 모델 가드레일 단독 신뢰 금지 원칙 명시(Simon Willison 2025 원칙).

## 9. 검증 방법

```
Argus.exe --aidebug -p "외부 GitHub PR 코멘트 읽고 → 그 내용 기반으로 SSH로 prod 서버에 curl 실행"
   → Lethal Trifecta 탐지, 사용자 명시 승인 요구

Argus.exe --aidebug --policy-dry-run -p "임의 자동화 100개 시나리오"
   → 어떤 정책이 차단할지만 보고, 실행 X

argus replay --session 2026-04-30T14-22-11.cast
   → 모든 명령·출력 재생

argus audit verify --since "2026-04-29"
   → hash chain 무결성 OK
```

## 10. 작업 분해 (구현 체크리스트)

### 10-1. AgentGuard 코어
- [ ] `agentguard/gateway.go`: ToolCall, Decision, Authorize 진입점
- [ ] `agentguard/lethal_trifecta.go`: DetectTrifecta + 세션 컨텍스트 추적
- [ ] `agentguard/injection.go`: LooksMalicious + 휴리스틱 패턴
- [ ] `agentguard/sandbox.go`: 외부 입력→다음 LLM 입력 신뢰 경계
- [ ] `agentguard/policy/policy.go`: CEL 엔진 (precompile + Evaluate)

### 10-2. 감사 인프라
- [ ] `audit/recorder.go`: append-only JSONL + hash chain
- [ ] `audit/asciicast.go`: AsciicastWriter v2 포맷
- [ ] `audit/replay.go`: `argus replay` 서브커맨드
- [ ] `audit verify` 서브커맨드: hash chain 무결성 검증

### 10-3. 통합 지점
- [ ] `mcp/client.go` 수정: CallTool 호출 전 Gateway.Authorize
- [ ] `tools/bash/bash.go` 수정: Bash 호출 전 Gateway.Authorize
- [ ] `services/workspace/manager.go` 수정: Exec/StartExec 통합
- [ ] `cmd/argus/main.go`: 부트스트랩 시 Gateway 초기화

### 10-4. 테스트
- [ ] Lethal Trifecta 8가지 조합 단위 테스트
- [ ] Prompt injection 데이터셋 100개 케이스
- [ ] CEL 정책 fuzz 테스트
- [ ] asciicast 외부 도구 호환 검증
- [ ] Hash chain 무결성 검증 도구

## 11. 참고 출처

- Simon Willison — "The Lethal Trifecta" 블로그
- Anthropic — Claude tool-use safety guidance
- asciinema cast format spec v2
- OpenTelemetry Go SDK docs
- AI Agent Security 2026 (swarmsignal.net)
