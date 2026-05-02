# 위협 모델 매트릭스 + 우선순위

## 1. 위협 모델 매트릭스

각 위협별로 어떤 Phase가 어떤 메커니즘으로 대응하는지 정리합니다.

| 위협 | 대응 Phase | 핵심 메커니즘 |
|---|---|---|
| **Lethal Trifecta** (사적 데이터 + 미신뢰 입력 + exfil) | Phase 4 | AgentGateway DetectTrifecta + 자동 ask + 정책 |
| **Prompt injection** ("ignore previous") | Phase 4 | injection.LooksMalicious 휴리스틱 + 도구 출력 sandbox |
| **Tool poisoning** (악성 MCP 도구 메타데이터) | Phase 5 | Cosign 서명 검증 + TrustReport |
| **MCP token passthrough** | Phase 5 | AudienceValidator (aud claim) |
| **MCP confused deputy** | Phase 5 | per-client consent + state 검증 |
| **SSRF** (169.254.169.254, RFC1918) | Phase 5 | EgressFirewall denyList + redirect-aware |
| **Session hijacking (MCP)** | Phase 5 | 비결정적 session ID + `<user>:<sid>` 키 |
| **MITM (CVE-2025-26465 VerifyHostKeyDNS)** | Phase 3 | StrictHostKeyCallback + DNS off by default |
| **Agent forwarding 악용 (CVE-2023-38408)** | Phase 2 | ProxyJump 권장, agent forwarding deprecation 경고 |
| **Harvest-now-decrypt-later** | Phase 3 | mlkem768x25519-sha256 hybrid KEX |
| **Cred 평문 디스크 노출** | Phase 3 | OS native keyring (DPAPI/Keychain/SecretService) |
| **Idle disconnect** | Phase 1 | KeepAlive ticker + reconnect backoff |
| **LLM 자동화 audit 부재** | Phase 4 | append-only JSONL + hash chain + asciicast |
| **Connector 공급망 공격** | Phase 5 | TrustLevel + SLSA provenance + 사용자 confirm |
| **Goroutine 누수 / hang (issue #21478, #26643, #51926)** | Phase 1, 2 | dialSSH + ChannelPool ctx-aware |
| **동적 포트 차단 (방화벽)** | Phase 2 | OpenSOCKS via SSH 채널 |
| **Bash sanitization 우회** (백틱, $()) | Phase 4 | AgentGateway 정책 (CEL) + Bash 추가 검사 |

## 2. 위협별 심층 분석

### 2-1. Lethal Trifecta (Phase 4)

**위협 정의**: 한 도구가 ① 사적 데이터에 접근할 수 있고, ② 신뢰할 수 없는 외부 입력의 영향 아래 있고, ③ 외부로 데이터를 송출할 수 있을 때 발생.

**현실 사례 (2026)**: 1월 2026 연구에서 단일 오염된 이메일이 GPT-4o로 하여금 SSH 키를 80% 확률로 유출시킴.

**Argus 대응**:
- 모든 도구 호출에서 `DetectTrifecta` 실행
- 3중 조건 모두 만족 시 자동 ask 모달
- 사용자 결정 + 영구 정책 저장 옵션
- `policies.json`에 거부 규칙 추가 가능

### 2-2. MCP Token Passthrough (Phase 5)

**위협 정의**: MCP 서버가 클라이언트 토큰을 검증 없이 다운스트림 API에 그대로 전달.

**위험**: rate limiting 우회, audit 추적 불가, 신뢰 경계 붕괴.

**Argus 대응**:
- `AudienceValidator.Validate(token)`: aud claim이 우리 서버를 가리키는지 강제 검증
- 다른 audience 토큰은 즉시 거부
- OIDC keysets 정기 갱신

### 2-3. SSRF (Phase 5)

**위협 정의**: MCP 클라이언트가 OAuth 메타데이터 디스커버리 중 내부 리소스(예: 169.254.169.254 클라우드 메타데이터)에 접근.

**Argus 대응**:
- `EgressFirewall.DefaultDeny()`: RFC1918, link-local, loopback, IPv6 ULA 모두 차단
- redirect chain도 매 hop 검증
- DNS rebinding 방어 (TOCTOU 회피)
- HTTPS 강제 (loopback 예외)

### 2-4. Harvest-now-decrypt-later (Phase 3)

**위협 정의**: 공격자가 현재 SSH 트래픽을 저장해두었다가 양자컴퓨터로 미래에 복호화.

**Argus 대응**:
- OpenSSH 10.0+의 `mlkem768x25519-sha256` hybrid KEX 기본 채택
- ML-KEM (NIST FIPS 203) + X25519 결합
- `PQCMode=prefer`(기본) 또는 `require`(엄격)
- 75% 이상의 인터넷 OpenSSH가 PQC 미지원이므로 `prefer` 권장

### 2-5. Idle Disconnect (Phase 1)

**위협 정의**: NAT/방화벽이 idle TCP 연결을 끊어 작업 도중 세션 유실.

**Argus 대응**:
- 30초 주기 `keepalive@openssh.com` 요청
- 3회 연속 실패 시 자동 재연결
- 지수 백오프 + jitter (1s → 60s)
- 인증 실패는 재시도 무의미하므로 즉시 영구 실패

## 3. 종합 일정·우선순위 매트릭스

| Phase | 영향도 | 구현 난이도 | 권장 순서 | 80% 효과 컷 |
|---|---|---|---|---|
| Phase 1 (안정성) | 매우 높음 | 낮음 | 1 | ✅ 필수 |
| Phase 4 (AI Gateway + 감사) | 매우 높음 | 높음 | 2 | ✅ 필수 (사용자 우선순위) |
| Phase 5 (MCP 보안) | 매우 높음 | 중간~높음 | 3 | ✅ 필수 (사용자 우선순위) |
| Phase 3 (자격증명 + PQC) | 높음 | 중간 | 4 | ✅ 권장 |
| Phase 2 (멀티플렉싱·ProxyJump) | 중간~높음 | 중간 | 5 | 🟨 시간 되면 |
| Phase 6 (OTel + Persistent) | 중간 | 높음 | 6 | 🟨 선택 |

### 80% 효과 라인

**Phase 1 + Phase 4 + Phase 5의 핵심 부분**(Lethal Trifecta + audit + audience + SSRF)만으로도 보안·안정성 효과의 약 80%를 확보할 수 있습니다.

Phase 3은 PQC가 "현재 시점에서 위협이 미래"라 다소 후순위지만 12개월 안에는 처리 권장. 자격증명 keyring은 비-Windows 사용자가 있다면 즉시 처리 권장.

## 4. ⚠️ 사용자가 결정해야 할 항목

| 항목 | 옵션 | 권장 | 영향 |
|---|---|---|---|
| PQC 정책 | `prefer` / `require` | `prefer` | `require`는 구형 서버 연결 차단 |
| Lethal Trifecta 기본값 | `ask` / `deny` / `allow` | `ask` | `deny`는 일부 자동화 워크플로 차단 |
| MCP local server 샌드박스 | `default` / `strict` / `off` | `default` | `strict`는 bwrap 없는 Linux에서 실패 |
| known_hosts mismatch | `accept-new` / `yes` | `accept-new` | `yes`는 첫 접속 시 사용자 prompt |
| audit 로그 fsync | 즉시 / 100ms 배치 | 즉시 | SSD 마모 vs 무결성 보장 |
| Tailscale 통합 | 메인 빌드 / build tag | build tag | 메인 빌드는 바이너리 비대화 |
| AgentGuard policy_dry_run | true / false | 1주일 `true` 후 `false` | dry_run은 차단 X, 관찰만 |

## 5. 타임라인 권장

```
M0 (Day 0)    Phase 1 시작 — 단순/저위험
M1 (Day 14)   Phase 1 완료 → Phase 4 시작
M2 (Day 28)   Phase 4 dry_run 모드로 1주일 운영
M3 (Day 35)   Phase 4 enforce 모드 + Phase 5 시작
M4 (Day 56)   Phase 5 완료 → Phase 3 시작 (자격증명+PQC)
M5 (Day 77)   Phase 3 완료 → Phase 2 시작
M6 (Day 98)   Phase 2 완료 → Phase 6 (선택)
M7 (Day 120)  전체 통합 검증 + 문서 정비
```

각 Phase는 약 2주 단위 sprint로 잡고, dry_run 운영 기간을 둠으로써 사용자 경험을 점진 검증합니다.
