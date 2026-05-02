# Phase 2 — 멀티플렉싱·채널 풀·ProxyJump·동적 포트 포워딩

> **권장 순서**: 5번째
> **영향도**: 중간~높음 / **구현 난이도**: 중간 / **80% 효과 컷**: 🟨 시간 되면

## 목표 한 줄 요약
OpenSSH ControlMaster급 멀티플렉싱(채널 풀), ProxyJump 체인, SOCKS5 동적 포트 포워딩으로 병렬 효율 2~5배 + 격리된 네트워크 접근.

## 1. 신규 도입 라이브러리/외부 의존성

- `github.com/things-go/go-socks5` (Apache-2.0) — SOCKS5 서버 (또는 `armon/go-socks5` MIT)
- 표준 `golang.org/x/crypto/ssh` (BSD-3) — 이미 사용 중
- 라이선스 노트: 모두 MIT/Apache, GPL 회피.

## 2. 신규/수정 파일

### 신규
- `internal/services/workspace/multiplex.go` — `MultiplexedClient`
- `internal/services/workspace/channel_pool.go` — exec/SFTP 별 풀
- `internal/services/workspace/proxy_chain.go` — ProxyJump
- `internal/services/workspace/socks.go` — 동적 포트 포워딩

### 수정
- `internal/services/workspace/ssh_session.go:73,136`: sftpPool 4 → 풀 매니저로 위임
- `internal/services/workspace/ssh_session.go:226-395, 430-657`: `openExecSession`이 `ChannelPool.AcquireExec(ctx)`를 사용
- `internal/services/workspace/types.go`: `ServerEntry.ProxyJump []ProxyHop`

## 3. 핵심 인터페이스/타입 정의

```go
// internal/services/workspace/channel_pool.go
type sessionResource interface { Close() error }

type ChannelPool struct {
    client    *ssh.Client
    maxExec   int
    maxSFTP   int
    execSem   chan struct{} // 슬롯 토큰
    sftpPool  chan *sftp.Client
    metrics   PoolMetrics
}

// AcquireExec 은 ssh.Session 발급 슬롯을 점유한다. ssh.Session 자체는
// 재사용이 불가하므로 (NewSession 마다 새 채널) 슬롯 + 새 세션을 묶어 발급.
func (p *ChannelPool) AcquireExec(ctx context.Context) (*ExecLease, error)

// AcquireSFTP 는 풀에서 sftp.Client 를 꺼낸다. 없으면 새로 만든다.
// 반환 시 Release()로 풀에 복귀.
func (p *ChannelPool) AcquireSFTP(ctx context.Context) (*SFTPLease, error)

type ExecLease struct {
    Session *ssh.Session
    release func()
}
func (e *ExecLease) Close() error // = release + session.Close

// internal/services/workspace/proxy_chain.go
type ProxyHop struct {
    Host           string `json:"host"`
    Port           int    `json:"port"`
    User           string `json:"user"`
    IdentityFile   string `json:"identity_file,omitempty"`
    CertificateFile string `json:"certificate_file,omitempty"` // Phase 3 SSH CA
}

// DialProxyJump 은 OpenSSH 의 ProxyJump (-J) 와 동일하게,
// jump 호스트 SSH 클라이언트 안에서 다음 hop 으로 TCP 채널을 열고
// 그 위에 새 ssh.NewClientConn 을 쌓는다 (체이닝 가능).
func DialProxyJump(ctx context.Context, hops []ProxyHop, target ServerEntry,
    cfg *ssh.ClientConfig) (*ssh.Client, error)

// internal/services/workspace/socks.go
type SOCKSTunnel struct {
    listener net.Listener
    server   *socks5.Server
    client   *ssh.Client
}

// OpenSOCKS 는 OpenSSH 의 -D 와 동일하게 동적 포트 포워딩 SOCKS5
// 서버를 로컬에 띄우고, 모든 요청을 client.Dial 을 통해 원격으로 라우팅.
func (s *sshSession) OpenSOCKS(localAddr string) (*SOCKSTunnel, error)
```

## 4. 핵심 의사코드

```go
// channel_pool.go
func NewChannelPool(c *ssh.Client, maxExec, maxSFTP int) *ChannelPool {
    return &ChannelPool{
        client:   c,
        maxExec:  maxExec, // 기본 8
        maxSFTP:  maxSFTP, // 기본 4
        execSem:  make(chan struct{}, maxExec),
        sftpPool: make(chan *sftp.Client, maxSFTP),
    }
}

func (p *ChannelPool) AcquireExec(ctx context.Context) (*ExecLease, error) {
    select {
    case p.execSem <- struct{}{}:
    case <-ctx.Done(): return nil, ctx.Err()
    }
    // x/crypto/ssh issue #26643 : NewSession() 이 hang 가능 → 별도 고루틴 + ctx
    type r struct{ s *ssh.Session; err error }
    ch := make(chan r, 1)
    go func() { s, err := p.client.NewSession(); ch <- r{s, err} }()
    select {
    case res := <-ch:
        if res.err != nil { <-p.execSem; return nil, res.err }
        atomic.AddInt64(&p.metrics.ActiveExec, 1)
        return &ExecLease{Session: res.s, release: func(){
            <-p.execSem
            atomic.AddInt64(&p.metrics.ActiveExec, -1)
        }}, nil
    case <-ctx.Done():
        <-p.execSem
        return nil, ctx.Err()
    }
}

// proxy_chain.go : OpenSSH ProxyJump 호환
func DialProxyJump(ctx context.Context, hops []ProxyHop, t ServerEntry, finalCfg *ssh.ClientConfig) (*ssh.Client, error) {
    if len(hops) == 0 { return dialSSH(ctx, "tcp", net.JoinHostPort(t.Host, strconv.Itoa(t.Port)), finalCfg) }

    // 1. 첫 hop 으로 직접 dial
    first := hops[0]
    firstCfg := buildAuthConfig(first) // identity/cert 포함
    cur, err := dialSSH(ctx, "tcp", net.JoinHostPort(first.Host, strconv.Itoa(first.Port)), firstCfg)
    if err != nil { return nil, fmt.Errorf("jump1 %s: %w", first.Host, err) }

    // 2. 중간 hop 들을 cur.Dial 로 연결
    for i := 1; i < len(hops); i++ {
        h := hops[i]
        innerConn, err := cur.DialContext(ctx, "tcp", net.JoinHostPort(h.Host, strconv.Itoa(h.Port)))
        if err != nil { cur.Close(); return nil, fmt.Errorf("jump%d %s: %w", i+1, h.Host, err) }
        innerCfg := buildAuthConfig(h)
        c, chans, reqs, err := ssh.NewClientConn(innerConn, h.Host, innerCfg)
        if err != nil { cur.Close(); return nil, err }
        cur = ssh.NewClient(c, chans, reqs)
    }

    // 3. 마지막 hop 안에서 target 으로 연결
    targetConn, err := cur.DialContext(ctx, "tcp", net.JoinHostPort(t.Host, strconv.Itoa(t.Port)))
    if err != nil { cur.Close(); return nil, err }
    c, chans, reqs, err := ssh.NewClientConn(targetConn, t.Host, finalCfg)
    if err != nil { cur.Close(); return nil, err }
    final := ssh.NewClient(c, chans, reqs)
    // jump client 는 final 이 닫힐 때 함께 닫는다 (wrapper 도입)
    return wrapWithChainCleanup(final, cur), nil
}

// socks.go (armon/go-socks5)
func (s *sshSession) OpenSOCKS(localAddr string) (*SOCKSTunnel, error) {
    conf := &socks5.Config{
        Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
            return s.client.DialContext(ctx, network, addr) // SSH 채널 통해 원격 dial
        },
        // SSRF 가드: private CIDR 차단 (Phase 5 와 공통)
        Rules: argusSOCKSRules{},
    }
    server, err := socks5.New(conf); if err != nil { return nil, err }
    ln, err := net.Listen("tcp", localAddr); if err != nil { return nil, err }
    go server.Serve(ln)
    return &SOCKSTunnel{listener: ln, server: server, client: s.client}, nil
}
```

## 5. OpenSSH 호환성 노트

- `ControlMaster auto` ↔ Argus의 `MultiplexedClient`는 항상 auto. ssh_config의 `ControlPath`는 사용하지 않으나 동일 의미를 가진 in-process pool로 대체.
- `ControlPersist 10m` ↔ 우리는 connection-lifetime persistent (manager가 들고 있는 동안 유지).
- `ProxyJump user@bastion:22,user2@bastion2:22` ↔ `ServerEntry.ProxyJump []ProxyHop` 슬라이스.
- `DynamicForward 1080` ↔ `OpenSOCKS`.
- OpenSSH 10.0 변경(scp/sftp가 기본 ControlMaster no)을 의식해 SFTP는 ChannelPool에서 명시적으로 분리(Argus 내부에서만 공유).

## 6. 마이그레이션/기존 코드 영향

- 기존 `s.sftpPool chan *sftp.Client` 4개를 `ChannelPool.sftpPool`로 흡수.
- 기존 `openExecSession` → `ChannelPool.AcquireExec(ctx)` 호출. 호출부 4곳 (`Exec`, `StartExec`, `warmupSudo`, `account_shell.go`) 모두 `defer lease.Close()`로 감쌈.
- `sshSession.client` 필드는 유지하지만 외부 접근은 `s.pool.Client()`로 캡슐화.
- 기존 정적 터널(`openTunnel`)은 그대로 두고, OpenSOCKS는 추가 옵션.

## 7. 테스트 전략

- **단위**: `gliderlabs/ssh` 멀티 인스턴스로 ProxyJump 체인 (A→B→C) 검증, `nc -lk` 대신 in-process echo 서버로 SOCKS5 검증.
- **동시성**: `go test -race`로 ChannelPool에 16 goroutine 동시 acquire/release.
- **Fuzz**: ProxyJump hop 0~10개, 각 hop에 잘못된 호스트 → 부분 정리 (모든 jump client close).
- **통합**: `Argus --aidebug -p "bastion 통해 db1, db2 동시 ls 5초 안에"`.

## 8. 위험 및 완화

1. **ProxyJump 중간 client 누수** → `wrapWithChainCleanup` 으로 단일 Close 시 chain 전체 정리.
2. **SOCKS 통한 SSRF (LAN 대상)** → 기본 차단, 화이트리스트 설정 필요. (Phase 5와 공유)
3. **채널풀 크기 부족 시 deadlock** → `AcquireExec`에 ctx deadline + 메트릭 노출.
4. **agent forwarding 동시 사용 시 CVE-2023-38408 위험** → ProxyJump 권장, agent forwarding은 별도 phase에서 deprecation 경고.

## 9. 검증 방법

```
Argus.exe --aidebug -p "bastion.example.com을 거쳐서 internal-db.lan에 ssh -> 'select 1' 실행"
Argus.exe --aidebug -p "DynamicForward 1080: curl --socks5 localhost:1080 https://internal-only.lan"
Argus.exe --aidebug -p "동시에 6개 명령 stat 실행. 채널풀이 8/8 활용되는지 metric 확인"
```

## 10. 작업 분해 (구현 체크리스트)

- [ ] `channel_pool.go`: ChannelPool 구조체, AcquireExec, AcquireSFTP
- [ ] `multiplex.go`: MultiplexedClient (ChannelPool 래퍼)
- [ ] `proxy_chain.go`: DialProxyJump + wrapWithChainCleanup
- [ ] `socks.go`: OpenSOCKS + argusSOCKSRules (Phase 5 EgressFirewall과 통합 지점 표시)
- [ ] `ssh_session.go` 수정: openExecSession → AcquireExec
- [ ] `types.go` 수정: ServerEntry.ProxyJump
- [ ] 호출부 4곳에 defer lease.Close() 적용
- [ ] race 검출 통합 테스트
- [ ] ProxyJump A→B→C 체인 통합 테스트
- [ ] SOCKS5 통합 테스트 (curl --socks5 검증)

## 11. 참고 출처

- blacknon/go-sshlib — Go 에서 ProxyJump 구현 패턴
- armon/go-socks5 — Dial 콜백 인터페이스
- OpenSSH 10.0 release notes (ControlMaster 변경)
