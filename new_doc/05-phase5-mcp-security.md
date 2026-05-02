# Phase 5 — MCP 보안 강화 + Connector 신뢰 검증 ★두꺼운 파트★

> **권장 순서**: 3번째 (사용자 우선순위)
> **영향도**: 매우 높음 / **구현 난이도**: 중간~높음 / **80% 효과 컷**: ✅ 필수

## 목표 한 줄 요약
MCP의 confused deputy/token passthrough/SSRF/session hijacking을 공식 명세에 맞춰 차단하고, Connector 자동 설치는 서명·SBOM·소스 신뢰도로 검증한다.

## 1. 신규 도입 라이브러리/외부 의존성

- `github.com/sigstore/cosign` (Apache-2.0) — 서명 검증
- `github.com/in-toto/in-toto-golang` (Apache-2.0) — SLSA provenance
- `github.com/coreos/go-oidc/v3` (Apache-2.0) — JWT/OIDC 검증
- `github.com/stripe/smokescreen` 패턴 차용 (구현은 자체) — egress proxy
- 라이선스: 모두 Apache-2.0.

## 2. 신규/수정 파일

### 신규
- `internal/services/mcp/guard.go` — SecurityGuard
- `internal/services/mcp/audience.go` — token audience 검증
- `internal/services/mcp/ssrf.go` — egress firewall
- `internal/services/mcp/sandbox.go` — local server sandbox
- `internal/services/mcp/scope.go` — scope minimization + escalation
- `internal/connector/trust.go` — 서명 검증 + 신뢰도 점수
- `internal/connector/manifest.go` — SBOM 메타데이터

### 수정
- `internal/services/mcp/client.go` 라인 32–57, 183–214: sandbox 적용, audience 검증
- `internal/connector/sources/registry.go`: 서명된 manifest 우선

## 3. 핵심 인터페이스/타입 정의

```go
// internal/services/mcp/guard.go
type SecurityGuard struct {
    audience  *AudienceValidator
    ssrf      *EgressFirewall
    sandbox   *Sandbox
    scopes    *ScopeManager
    gateway   agentguard.Gateway
}

type CallContext struct {
    ServerName string
    ToolName   string
    Args       map[string]any
    UserID     string
    SessionID  string
    Token      string // MCP server에서 제시한 token (있으면)
}
func (g *SecurityGuard) Check(ctx context.Context, c CallContext) (agentguard.Decision, error)

// internal/services/mcp/audience.go
type AudienceValidator struct {
    expectedIssuer   string
    expectedAudience string
    keys             *oidc.RemoteKeySet
}
// 토큰의 aud claim이 우리 서버 자신을 가리키는지 검증.
// 다른 서버용 토큰을 받으면(passthrough) 즉시 거부.
func (v *AudienceValidator) Validate(token string) error

// internal/services/mcp/ssrf.go
type EgressFirewall struct {
    allowList  []netip.Prefix // 명시 허용 CIDR
    denyList   []netip.Prefix // 항상 거부 (private + link-local + loopback)
    redirectMax int
}
func DefaultDeny() []netip.Prefix {
    // RFC1918 + link-local + loopback + IPv6 ULA + IPv6 link-local
    return mustParsePrefixes("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
        "169.254.0.0/16", "127.0.0.0/8", "::1/128", "fc00::/7", "fe80::/10",
        "0.0.0.0/8", "100.64.0.0/10")
}
func (f *EgressFirewall) DialContext(ctx context.Context, network, addr string) (net.Conn, error)

// internal/services/mcp/sandbox.go : local stdio MCP server를 격리
type Sandbox struct {
    profile string // "default" | "strict" | "off"
}
// 운영체제별:
//   - Linux: bubblewrap or systemd-run --user --scope -p ProtectSystem=strict ...
//   - macOS: sandbox-exec
//   - Windows: AppContainer 또는 Job Object + 제한된 토큰
func (s *Sandbox) Start(cfg ServerConfig) (*exec.Cmd, error)

// internal/services/mcp/scope.go : 최소권한 + 단계적 elevation
type Scope string
const (
    ScopeToolsBasic Scope = "mcp:tools-basic"   // read-only 도구
    ScopeToolsWrite Scope = "mcp:tools-write"
    ScopeResources  Scope = "mcp:resources-read"
    ScopeAdmin      Scope = "mcp:admin"
)
type ScopeManager struct{ active map[string]map[Scope]bool /* server -> scopes */ }
func (m *ScopeManager) Require(server string, s Scope) error
func (m *ScopeManager) Elevate(ctx context.Context, server string, s Scope, reason string) error // 사용자 prompt

// internal/connector/trust.go
type TrustLevel int
const (
    TrustUnknown TrustLevel = iota
    TrustCommunity
    TrustVerified  // Cosign 서명 검증됨
    TrustOfficial  // 공식 registry + maintainer 서명
)
type TrustReport struct {
    Level      TrustLevel
    Signature  *cosign.SignatureInfo
    SLSAFrom   string
    Reasons    []string
    Warnings   []string
}
func EvaluateTrust(ctx context.Context, m Manifest) TrustReport
```

## 4. 핵심 의사코드

```go
// guard.go
func (g *SecurityGuard) Check(ctx context.Context, c CallContext) (agentguard.Decision, error) {
    // 1. token audience 검증 (token passthrough 방지)
    if c.Token != "" {
        if err := g.audience.Validate(c.Token); err != nil {
            return agentguard.Decision{Action: agentguard.ActionDeny, Reason: "token audience mismatch (passthrough?)", PolicyID:"mcp.token.aud"}, nil
        }
    }

    // 2. scope 검증
    required := requiredScopeFor(c.ToolName) // tool 메타데이터에서 추출
    if err := g.scopes.Require(c.ServerName, required); err != nil {
        // 자동 elevate 안 함 — 사용자 prompt
        if err := g.scopes.Elevate(ctx, c.ServerName, required, "tool '"+c.ToolName+"' 호출"); err != nil {
            return agentguard.Decision{Action: agentguard.ActionDeny, Reason: err.Error(), PolicyID:"mcp.scope"}, nil
        }
    }

    // 3. URL/host 인자 정규화 후 SSRF 검사
    for _, v := range walkURLs(c.Args) {
        if err := g.ssrf.Validate(v); err != nil {
            return agentguard.Decision{Action: agentguard.ActionDeny, Reason: err.Error(), PolicyID:"mcp.ssrf"}, nil
        }
    }

    // 4. 기존 AgentGateway 통과 (Lethal Trifecta + 정책)
    return g.gateway.Authorize(ctx, agentguard.ToolCall{
        Tool: "mcp__"+c.ServerName+"__"+c.ToolName, Args: c.Args, Origin: agentguard.OriginAgentLoop,
    })
}

// ssrf.go : redirect-aware
func (f *EgressFirewall) Validate(rawURL string) error {
    u, err := url.Parse(rawURL); if err != nil { return err }
    if u.Scheme != "https" && u.Scheme != "http" { return errInvalidScheme } // mcp는 https 권장
    host := u.Hostname()
    ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
    if err != nil { return err }
    for _, ip := range ips {
        a, _ := netip.AddrFromSlice(ip.IP)
        for _, p := range f.denyList { if p.Contains(a) { return fmt.Errorf("SSRF: %s -> %s blocked", host, a) } }
        if len(f.allowList) > 0 {
            ok := false; for _, p := range f.allowList { if p.Contains(a) { ok = true; break } }
            if !ok { return fmt.Errorf("SSRF: %s not in allowList", a) }
        }
    }
    return nil
}
func (f *EgressFirewall) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
    host, _, _ := net.SplitHostPort(addr)
    if err := f.Validate("https://"+addr); err != nil { return nil, err }
    return (&net.Dialer{}).DialContext(ctx, network, addr)
}

// sandbox.go (Linux 예)
func (s *Sandbox) startLinux(cfg ServerConfig) (*exec.Cmd, error) {
    if s.profile == "off" { return exec.Command(cfg.Command, cfg.Args...), nil }
    args := []string{
        "--ro-bind", "/usr", "/usr",
        "--ro-bind", "/lib", "/lib", "--ro-bind", "/lib64", "/lib64",
        "--proc", "/proc", "--dev", "/dev",
        "--unshare-all", "--share-net", // net는 SSRF firewall이 책임
        "--die-with-parent",
        "--cap-drop", "all",
        "--", cfg.Command,
    }
    args = append(args, cfg.Args...)
    return exec.Command("bwrap", args...), nil
}

// connector/trust.go
func EvaluateTrust(ctx context.Context, m Manifest) TrustReport {
    rep := TrustReport{Level: TrustUnknown}
    if m.SignatureURL != "" {
        info, err := cosign.Verify(ctx, m.ArtifactURL, m.SignatureURL, cosign.IdentityPolicy{
            Issuer: "https://accounts.google.com", // 또는 GitHub Actions OIDC
            SubjectRegExp: m.MaintainerEmailRegex,
        })
        if err == nil { rep.Level = TrustVerified; rep.Signature = info }
    }
    if m.Source == "registry.modelcontextprotocol.io" && rep.Level == TrustVerified {
        rep.Level = TrustOfficial
    }
    if m.Source == "github" && m.RepoStars < 50 {
        rep.Warnings = append(rep.Warnings, "github 레포 스타 < 50: 신뢰도 낮음")
    }
    return rep
}
```

## 5. OpenSSH 호환성 노트

MCP는 OpenSSH 와 무관. 단 MCP 스트리머블 HTTP 엔드포인트가 SSH 터널 위에 올라갈 수 있도록 Phase 2의 SOCKS와 연동 가능.

## 6. 마이그레이션/기존 코드 영향

- `mcpClient.call`(`client.go:90`)이 sync.Mutex로 순차 처리 — 변경 없음, 추후 Phase 6에서 채널 분리 검토.
- 기본 `permission: "ask"` 유지. 다만 신뢰 등급이 `Unknown`인 connector는 자동 설치 X, 사용자 prompt 추가.
- `internal/services/mcp/manager.go:NewManager`에 `WithSecurityGuard(g)` option 추가.
- ⚠️ **샌드박스 강제할지 여부**: Linux에서 bwrap이 없는 환경이 흔함 → `default`은 "있으면 사용, 없으면 경고 + 일반 실행". 사용자 결정 필요.

## 7. 테스트 전략

- **단위**: SSRF — 100개 URL (10/8, AWS metadata 169.254.169.254, IPv6 ULA, redirect chain) 모두 차단.
- **Audience**: aud=otherserver 토큰 → 즉시 거부. aud=ourserver → 허용.
- **Cosign**: 가짜 서명 → 거부. 정상 서명 → 통과.
- **Sandbox**: bwrap 없는 환경 → graceful fallback.
- **Confused deputy 시나리오**: 외부 사이트가 우리 OAuth client_id로 redirect하고 cookie consent 재사용 → 거부.

## 8. 위험 및 완화

1. **SSRF 정상 사내 사용 차단 (allowList 미설정)** → 기본 denyList만 적용, allowList는 opt-in.
2. **샌드박스 호환성 부족** → 점진적: Linux→macOS→Windows.
3. **cosign 의존성 무거움** → 선택적 컴파일 태그.
4. **scope elevation 사용자 피로** → "이 세션 동안 기억" 옵션 제공.
5. **confused deputy**: per-client consent storage → cookie/session 재사용 금지.

## 9. 검증 방법

```
Argus.exe --aidebug -p "MCP github 서버에서 PR 본문 가져와 가공"
   → 처음 호출 시 mcp:tools-basic scope 필요 → 사용자 승인 → 이후 자동

Argus.exe --aidebug -p "MCP 도구가 http://169.254.169.254/latest/meta-data 호출"
   → SSRF 즉시 차단

Argus.exe --aidebug -p "connector add github-mcp"
   → 서명 없는 패키지 → 경고 + 명시적 'I trust this'
```

## 10. 작업 분해 (구현 체크리스트)

### 10-1. MCP 보안 가드
- [ ] `mcp/guard.go`: SecurityGuard.Check 진입점
- [ ] `mcp/audience.go`: AudienceValidator (OIDC JWT + aud claim)
- [ ] `mcp/ssrf.go`: EgressFirewall (DefaultDeny CIDR, DialContext, Validate, redirect-aware)
- [ ] `mcp/sandbox.go`: 운영체제별 샌드박스 (Linux bwrap, macOS sandbox-exec, Windows AppContainer)
- [ ] `mcp/scope.go`: ScopeManager + Require + Elevate (사용자 prompt)
- [ ] `mcp/client.go` 수정: 모든 CallTool 전 Guard.Check

### 10-2. Connector 신뢰
- [ ] `connector/trust.go`: TrustReport + EvaluateTrust (Cosign + SLSA provenance)
- [ ] `connector/manifest.go`: SBOM 메타데이터 + SignatureURL
- [ ] `connector/sources/registry.go` 수정: 서명된 manifest 우선
- [ ] 신뢰 등급 Unknown은 자동 설치 차단

### 10-3. 통합·테스트
- [ ] `manager.go` 수정: WithSecurityGuard 옵션
- [ ] SSRF 100개 URL 케이스 테스트
- [ ] Confused deputy 시나리오 통합 테스트
- [ ] Cosign 서명 검증 통합 테스트
- [ ] 샌드박스 fallback 검증 (bwrap 없음 환경)

## 11. 참고 출처

- modelcontextprotocol.io security best practices (공식 사양)
- Stripe Smokescreen — egress filtering pattern
- Cosign keyless verification docs
- OWASP SSRF prevention cheat sheet
- The Vulnerable MCP Project (vulnerablemcp.info)
