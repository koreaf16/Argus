# Argus 시스템 견고화·보안 강화·기능 확장 종합 로드맵 (Overview)

## 1. 컨텍스트

Argus는 Go 1.23 기반의 CLI 에이전트로 SSH 워크스페이스, 멀티 서버 셸, MCP(Model Context Protocol) 통합, Connector 자동 설치, Bash 도구 권한 시스템, 자격증명 보관소를 갖추고 있습니다. 그러나 현재 구현은 (a) 주기적 keep-alive 부재로 NAT/방화벽 환경에서 idle disconnect 위험이 있고, (b) 채널 풀·멀티플렉싱 부재로 병렬 명령 실행 효율이 낮으며, (c) 자격증명이 비-Windows에서 평문으로 저장되고, (d) PQC·SSH CA·ProxyJump·동적 포트 포워딩·세션 레코딩 같은 표준 운영 기능이 빠져 있습니다.

무엇보다 Argus가 LLM 에이전트 자동화에 최적화되도록 만든 본래 목적에 맞게, 최근 2025–2026년 위협 환경(Lethal Trifecta, 단일 오염 이메일로 80% 확률 SSH 키 유출, MCP confused deputy/token passthrough/SSRF, tool poisoning, harvest-now-decrypt-later)에 대응할 수 있는 AI Agent Gateway·감사 추적·MCP 보안 강화가 필요합니다.

본 문서 시리즈는 OpenSSH 표준 호환을 우선 가치로 두면서 6단계로 점진적 도입이 가능한 구체적 구조·인터페이스·의사코드·마이그레이션 전략을 정의하여 "보안 강도 + 성능 + AI 에이전트 안전성"을 동시에 끌어올리는 종합 로드맵을 제시합니다.

## 2. 설계 원칙

1. **OpenSSH 호환성 우선**: 신규 옵션 키 이름은 ssh_config 키와 1:1 매칭(`ServerAliveInterval`, `ServerAliveCountMax`, `ControlMaster`, `ControlPath`, `ControlPersist`, `ProxyJump`, `IdentityFile`, `KexAlgorithms`, `HostKeyAlgorithms`, `MACs`, `Ciphers`, `CertificateFile`, `IdentityAgent`, `KnownHostsCommand`, `UpdateHostKeys`)을 따르고, `~/.ssh/config`를 import 가능하게 만든다.
2. **점진적 도입**: 모든 신규 동작은 기존 동작을 깨지 않는 "off-by-default" 또는 "backwards-compatible default"로 출시한다. Phase별로 feature flag(`features.ssh_keepalive=true` 등)를 둔다.
3. **AI Agent Gateway 패턴**: 모델 가드레일은 불충분하다는 전제. 모든 도구 호출(SSH `Exec`, MCP `tools/call`, Bash, FileEdit, Write 등)은 Gateway 레이어를 통과해 정책·Lethal Trifecta 검사·리스크 평가 후 허용/차단/사용자 승인된다.
4. **최소 권한 + Capability 토큰**: 도구별 scope(예: `mcp:tools-basic`, `ssh:exec`, `ssh:tunnel`)를 발급, wildcard 금지, 세션마다 단계적 elevation.
5. **감사 가능성 (Auditability First)**: 모든 LLM 의사결정·도구 호출·SSH 명령·MCP 호출은 append-only audit log + 선택적 asciicast 기록으로 남는다. 사후 재생 가능.
6. **암호 미래 보장 (PQC-ready)**: OpenSSH 10.0+에 맞춰 `mlkem768x25519-sha256` hybrid KEX를 기본 우선순위에 두고, harvest-now-decrypt-later를 방어한다.
7. **컨텍스트 주도 동시성**: 모든 SSH/MCP/Connector 호출은 `context.Context`를 받아 취소·타임아웃·deadline propagation을 강제한다(`x/crypto/ssh` issue #26643, #51926 우회).

## 3. 목표 아키텍처

```
                                  ┌──────────────────────────────────┐
                                  │            TUI / CLI Layer       │
                                  │    (Argus.exe --aidebug 등)      │
                                  └───────────────┬──────────────────┘
                                                  │
                       ┌──────────────────────────┴──────────────────────────┐
                       │              AI Agent Gateway (신규)                │
                       │  - Lethal Trifecta 검사                             │
                       │  - per-tool capability/scope 검증                   │
                       │  - prompt-injection 휴리스틱 + 출력 sandboxing      │
                       │  - 정책 매트릭스 enforcement                         │
                       └──┬──────────────┬───────────────┬──────────────────┘
                          │              │               │
                          ▼              ▼               ▼
                ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐
                │  Bash/Edit   │  │  MCP Bridge  │  │ Workspace        │
                │  Tools       │  │  (보안 확장) │  │ Manager          │
                └──────┬───────┘  └──────┬───────┘  └────────┬─────────┘
                       │                 │                   │
                       │                 ▼                   ▼
                       │      ┌────────────────────┐  ┌────────────────────────────┐
                       │      │ MCP SecurityGuard  │  │ MultiplexedClient (신규)   │
                       │      │  - audience claim  │  │  ┌──────────────────────┐  │
                       │      │  - SSRF firewall   │  │  │ ChannelPool          │  │
                       │      │  - sandbox runner  │  │  │ (Exec/SFTP/Tunnel)   │  │
                       │      │  - scope enforce   │  │  └──────────┬───────────┘  │
                       │      └────────┬───────────┘  │             │              │
                       │               │              │  ┌──────────▼───────────┐  │
                       │               ▼              │  │ KeepAlive + Reconnect│  │
                       │      ┌────────────────────┐  │  │ Supervisor           │  │
                       │      │ MCP Servers (stdio │  │  └──────────┬───────────┘  │
                       │      │ / streamable-http) │  │             │              │
                       │      └────────────────────┘  │  ┌──────────▼───────────┐  │
                       │                              │  │ ProxyChain (신규):   │  │
                       │                              │  │ ssh.Dial -> Jump ->  │  │
                       │                              │  │ Target (cert auth)   │  │
                       │                              │  └──────────┬───────────┘  │
                       │                              │             │              │
                       │                              │  ┌──────────▼───────────┐  │
                       │                              │  │ x/crypto/ssh.Client  │  │
                       │                              │  └──────────────────────┘  │
                       │                              └────────────────────────────┘
                       │
                       ▼
            ┌────────────────────────────────────────────────────────────┐
            │ AuditRecorder (신규):                                       │
            │   append-only JSONL + asciicast (.cast)                     │
            │   OpenTelemetry 트레이스 export                              │
            └─┬───────────────────────────────────────────────────────┬──┘
              │                                                       │
              ▼                                                       ▼
   ┌────────────────────┐                        ┌────────────────────────────────┐
   │ KeyringStore (신규)│                        │ OpenTelemetry Collector        │
   │  - 99designs/keyring│                       │  (OTLP/HTTP, OTLP/gRPC)        │
   │  - WinCred/macOS    │                       └────────────────────────────────┘
   │   Keychain/Secret   │
   │   Service/Pass/File │
   │  - SSH CA helper    │
   └────────────────────┘
```

신규 컴포넌트:
- `MultiplexedClient` (`internal/services/workspace/multiplex.go`)
- `ChannelPool` (`internal/services/workspace/channel_pool.go`)
- `KeyringStore` (`internal/security/keyring/`)
- `AgentGateway` (`internal/agentguard/`)
- `AuditRecorder` (`internal/audit/`)
- `ProxyChain` (`internal/services/workspace/proxy_chain.go`)
- `MCP SecurityGuard` (`internal/services/mcp/guard.go`)
- `SessionRecorder` (`internal/audit/asciicast.go`)

## 4. 6단계 Phase 요약

| Phase | 문서 | 영향도 | 구현 난이도 | 권장 순서 | 80% 효과 컷 |
|---|---|---|---|---|---|
| Phase 1 — 연결 안정성 기반 | [01-phase1-connection-stability.md](./01-phase1-connection-stability.md) | 매우 높음 | 낮음 | 1 | ✅ 필수 |
| Phase 4 — AI Agent Gateway · 감사 추적 | [04-phase4-ai-agent-gateway.md](./04-phase4-ai-agent-gateway.md) | 매우 높음 | 높음 | 2 | ✅ 필수 |
| Phase 5 — MCP 보안 강화 + Connector 신뢰 | [05-phase5-mcp-security.md](./05-phase5-mcp-security.md) | 매우 높음 | 중간~높음 | 3 | ✅ 필수 |
| Phase 3 — 자격증명 + PQC + known_hosts | [03-phase3-credential-pqc.md](./03-phase3-credential-pqc.md) | 높음 | 중간 | 4 | ✅ 권장 |
| Phase 2 — 멀티플렉싱·ProxyJump·SOCKS | [02-phase2-multiplex-proxyjump.md](./02-phase2-multiplex-proxyjump.md) | 중간~높음 | 중간 | 5 | 🟨 시간 되면 |
| Phase 6 — OTel + Persistent + Tailscale | [06-phase6-observability-persistent.md](./06-phase6-observability-persistent.md) | 중간 | 높음 | 6 | 🟨 선택 |

**80% 효과 라인**: Phase 1 + Phase 4 + Phase 5의 핵심 부분(Lethal Trifecta + audit + audience + SSRF)만으로도 보안·안정성 효과의 약 80%를 확보할 수 있습니다. Phase 3은 PQC가 "현재 시점에서 위협이 미래"라 다소 후순위지만 12개월 안에는 처리 권장.

## 5. 부속 문서

- [07-config-schemas.md](./07-config-schemas.md) — 신규 설정 파일 스키마 (settings.json/ssh_config.json/mcp.json 확장)
- [08-tui-changes.md](./08-tui-changes.md) — TUI/UI 변경 (렌더링 예상도 3종)
- [09-threat-model-matrix.md](./09-threat-model-matrix.md) — 위협 모델 매트릭스 + 우선순위
- [10-e2e-validation.md](./10-e2e-validation.md) — End-to-End 검증 시나리오 10개

## 6. ⚠️ 사용자가 결정해야 할 항목 모음

- PQC를 `prefer`(권장)로 둘지 `require`(엄격)로 둘지
- Lethal Trifecta 기본값을 `ask`(권장)로 둘지 `deny`(엄격)로 둘지
- MCP local server 샌드박스를 `default`(있으면 사용)로 둘지 `strict`(필수)로 둘지
- known_hosts mismatch 시 자동 prompt 허용할지(`accept-new`) vs 절대 거부(`yes`)
- audit 로그 fsync 주기: 즉시 vs 100ms 배치
- Tailscale/WireGuard 통합을 메인 빌드에 포함할지, build tag로 분리할지

## 7. 참고 자료

- OpenSSH 10.0 release notes (2025-04) — mlkem768x25519-sha256 default
- modelcontextprotocol.io security best practices
- Simon Willison — "The Lethal Trifecta"
- scylladb/go-sshtools — keep-alive ticker 패턴
- 99designs/keyring — cross-platform credential storage
- NIST FIPS 203 (ML-KEM)
- CVE-2025-26465 (VerifyHostKeyDNS), CVE-2023-38408 (agent forwarding)
