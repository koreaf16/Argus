# Phase 6 — 관측성(OpenTelemetry) + mosh-style 영속 세션 + Tailscale/WireGuard 연동(선택)

> **권장 순서**: 6번째 (선택)
> **영향도**: 중간 / **구현 난이도**: 높음 / **80% 효과 컷**: 🟨 선택

## 목표 한 줄 요약
모든 SSH/MCP/Tool 호출에 분산 트레이스를 부여하고, 네트워크 변경에도 끊기지 않는 영속 세션을 제공하며, 기업 zero-trust 백본(Tailscale)과 통합한다.

## 1. 신규 도입 라이브러리/외부 의존성

- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`, `otlpmetrichttp` (Apache-2.0)
- `tailscale.com/tsnet` (BSD-3) — 임베디드 Tailscale 클라이언트(선택)
- `golang.zx2c4.com/wireguard` (MIT) — WireGuard userspace(선택)
- mosh 자체는 GPL — Argus는 mosh 프로토콜을 직접 구현하지 않고, x/crypto/ssh 위에 "무손실 입력 큐 + roaming key + AES-128-GCM" 형태의 **Argus persistent session**으로 자체 구현.
- 라이선스 노트: GPL인 mosh는 import 안 함, 핵심 아이디어만 차용.

## 2. 신규/수정 파일

### 신규
- `internal/observability/otel.go`
- `internal/observability/metrics.go`
- `internal/services/workspace/persistent.go` — roaming session
- `internal/services/workspace/network/tailscale.go` (build tag `+build tailscale`)

### 수정
- `cmd/argus/main.go`: OTel SDK 초기화

## 3. 핵심 인터페이스/타입 정의

```go
// internal/observability/otel.go
type Config struct {
    Endpoint   string  // OTLP/HTTP endpoint
    Headers    map[string]string
    Insecure   bool
    SampleRate float64 // 0.0~1.0
    Resource   *resource.Resource
}
func Init(ctx context.Context, c Config) (Shutdown, error)
type Shutdown func(ctx context.Context) error

// internal/services/workspace/persistent.go
type PersistentSession struct {
    underlying  *MultiplexedClient
    inputQueue  *deque.Deque[byte]    // ack 안 된 입력
    outputSeq   uint64
    lastAcked   uint64
    aead        cipher.AEAD            // AES-128-GCM, mosh 스타일
    roamMu      sync.Mutex
}
// 네트워크 IP 변경 감지 → 새 UDP/SSH-over-UDP 세션 협상
// 슬립/wake 후 5초 안에 자동 복구
func (p *PersistentSession) HandleRoam(newAddr net.Addr) error
```

## 4. 핵심 의사코드

```go
// observability/otel.go : 이미 산업 표준
func Init(ctx context.Context, c Config) (Shutdown, error) {
    exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(c.Endpoint), otlptracehttp.WithHeaders(c.Headers))
    if err != nil { return nil, err }
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exp),
        sdktrace.WithSampler(sdktrace.TraceIDRatioBased(c.SampleRate)),
        sdktrace.WithResource(c.Resource),
    )
    otel.SetTracerProvider(tp)
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{}, propagation.Baggage{}))
    return tp.Shutdown, nil
}

// persistent.go : roaming
func (p *PersistentSession) HandleRoam(newAddr net.Addr) error {
    p.roamMu.Lock(); defer p.roamMu.Unlock()
    if err := p.underlying.RebindTo(newAddr); err != nil { return err }
    // 미응답 입력 재전송
    for _, b := range p.inputQueue.All() { _ = p.underlying.WriteRaw(b) }
    return nil
}
```

## 5. OpenSSH 호환성 노트

- OTel은 OpenSSH 와 무관, 다만 `LogLevel=VERBOSE`보다 풍부한 trace를 제공.
- mosh-style은 `~/.ssh/config`에 표현 안 됨 — Argus 전용 옵션 `PersistentMode=on` (기본 off).

## 6. 마이그레이션/기존 코드 영향

- 비파괴: OTel은 기본 off, env `OTEL_EXPORTER_OTLP_ENDPOINT`가 있으면 자동 활성화.
- Tailscale은 build tag `+build tailscale` 필요 — 기본 빌드에 포함 X.

## 7. 테스트 전략

- **단위**: OTel mock exporter로 span 생성 검증.
- **통합**: docker-compose로 jaeger 띄우고 trace 시각화.
- **Persistent**: `tc qdisc add dev eth0 root netem loss 50%` 환경에서 명령 실행 누락 없이 완료 확인.

## 8. 위험 및 완화

1. **OTel 트래픽으로 비용 ↑** → 샘플링 기본 0.1.
2. **Persistent 모드는 복잡도 ↑** → 기본 off, 명시적 opt-in.
3. **Tailscale 라이선스/배포 정책** → 별도 배포물 (build tag로 분리).

## 9. 검증 방법

```
Argus.exe --aidebug --otel-endpoint=http://localhost:4318 -p "test"
   → Jaeger 에서 trace 확인

Argus.exe --aidebug --persistent -p "원격 서버에 vim 작업 (오해 X — 실은 스트리밍 명령 시뮬레이션)"
   → 와이파이 끊었다가 5G로 전환해도 세션 유지
```

## 10. 작업 분해 (구현 체크리스트)

### 10-1. OpenTelemetry
- [ ] `observability/otel.go`: Init + Shutdown
- [ ] `observability/metrics.go`: SSH/MCP/Tool 메트릭
- [ ] `cmd/argus/main.go`: OTEL_EXPORTER_OTLP_ENDPOINT env 자동 감지
- [ ] AgentGuard·MCP Guard·SSH 호출에 span 추가
- [ ] docker-compose Jaeger 통합 검증

### 10-2. Persistent Session
- [ ] `services/workspace/persistent.go`: PersistentSession 구조
- [ ] 입력 큐 + ack 시퀀스 관리
- [ ] HandleRoam: 네트워크 변경 감지 + 재바인딩
- [ ] netem 패킷 손실 환경 통합 테스트

### 10-3. Tailscale 연동 (선택)
- [ ] `network/tailscale.go` (build tag `+build tailscale`)
- [ ] tsnet 임베디드 클라이언트로 ACL 적용 호스트 자동 디스커버리
- [ ] 별도 배포 빌드 파이프라인

## 11. 참고 출처

- mosh.org — "Mobile shell" 논문
- OpenTelemetry Go OTLP exporter docs
- Tailscale tsnet docs
- WireGuard userspace Go implementation
