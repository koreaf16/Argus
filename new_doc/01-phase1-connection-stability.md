# Phase 1 — 연결 안정성 기반 (Keep-alive, 재연결, 타임아웃, 컨텍스트 propagation)

> **권장 순서**: 1번째 (가장 먼저 실행)
> **영향도**: 매우 높음 / **구현 난이도**: 낮음 / **80% 효과 컷**: ✅ 필수

## 목표 한 줄 요약
idle disconnect를 0에 가깝게 만들고, 일시적 네트워크 단절에서 자동 복구하며, 모든 SSH I/O를 컨텍스트 취소 가능하게 만든다.

## 1. 신규 도입 라이브러리/외부 의존성

- `github.com/cenkalti/backoff/v4` (MIT) — 지수 백오프 + jitter
- 표준 `golang.org/x/sync/errgroup` (BSD-3) — 이미 호환
- 라이선스 노트: backoff/v4(MIT 호환). GPL 회피.

## 2. 신규/수정 파일 경로

### 신규
- `internal/services/workspace/keepalive.go`
- `internal/services/workspace/reconnect.go`
- `internal/services/workspace/dialer.go` (`net.DialContext` 기반 dial 분리)
- `internal/services/workspace/options.go` — Phase별 옵션 통합
- `internal/services/workspace/ssh_session_test.go`

### 수정
- `internal/services/workspace/ssh_session.go` 라인 94–122: `ssh.Dial` → `ssh.NewClientConn`로 교체, `Timeout: 15*time.Second` 하드코딩 제거
- `internal/services/workspace/manager.go` 라인 133–168: `Connect`에서 supervisor 시작
- `internal/services/workspace/types.go`: `ServerEntry`에 `KeepAlive` 필드 추가

## 3. 핵심 인터페이스/타입 정의

```go
// internal/services/workspace/options.go
package workspace

import "time"

// SSHOptions 는 ssh_config 의 호환 키를 1:1 매핑한다. 모든 필드의 zero value
// 는 OpenSSH 의 기본값(또는 그에 준하는 안전한 값)으로 해석한다.
type SSHOptions struct {
    // ServerAliveInterval (기본 30s, 0=disabled). ssh_config: ServerAliveInterval
    KeepAliveInterval time.Duration `json:"server_alive_interval"`
    // ServerAliveCountMax (기본 3). 연속 실패 횟수가 이 수에 도달하면 연결 종료.
    KeepAliveCountMax int `json:"server_alive_count_max"`
    // ConnectTimeout (기본 15s). ssh_config: ConnectTimeout
    ConnectTimeout time.Duration `json:"connect_timeout"`
    // ReconnectMax 는 자동 재시도 횟수(기본 5, 0=disabled). 초과 시 사용자에게 보고.
    ReconnectMax int `json:"reconnect_max"`
    // ReconnectInitial / ReconnectCeiling 는 backoff 의 시작/최대 간격.
    ReconnectInitial time.Duration `json:"reconnect_initial"`
    ReconnectCeiling time.Duration `json:"reconnect_ceiling"`
}

// internal/services/workspace/keepalive.go
type keepAliveSupervisor struct {
    client      *ssh.Client
    interval    time.Duration
    countMax    int
    onFailure   func(reason error) // 외부에서 reconnect 트리거
    cancel      context.CancelFunc
    done        chan struct{}
}

func startKeepAlive(parent context.Context, c *ssh.Client, opts SSHOptions,
    onFail func(error)) *keepAliveSupervisor

func (k *keepAliveSupervisor) Stop()
```

## 4. 핵심 의사코드

```go
// keepalive.go (scylladb/go-sshtools 패턴 기반)
func (k *keepAliveSupervisor) run(ctx context.Context) {
    defer close(k.done)
    if k.interval <= 0 { return } // disabled
    t := time.NewTicker(k.interval); defer t.Stop()
    failures := 0
    for {
        select {
        case <-ctx.Done(): return
        case <-t.C:
            // SendRequest 자체가 hang 가능 → 별도 고루틴 + deadline
            errCh := make(chan error, 1)
            go func() {
                _, _, err := k.client.SendRequest("keepalive@openssh.com", true, nil)
                errCh <- err
            }()
            select {
            case err := <-errCh:
                if err != nil {
                    failures++
                    if failures >= k.countMax {
                        k.onFailure(fmt.Errorf("keepalive failed %d times: %w", failures, err))
                        return
                    }
                } else { failures = 0 }
            case <-time.After(k.interval / 2):
                failures++
                if failures >= k.countMax {
                    k.onFailure(errors.New("keepalive timeout"))
                    return
                }
            }
        }
    }
}

// reconnect.go
func (m *Manager) reconnectWithBackoff(alias string) {
    bo := backoff.NewExponentialBackOff()
    bo.InitialInterval = m.opts.ReconnectInitial    // 1s
    bo.MaxInterval     = m.opts.ReconnectCeiling     // 60s
    bo.RandomizationFactor = 0.5                     // jitter
    bo.MaxElapsedTime   = 0                          // 무한 (count-based 종료)

    attempts := 0
    operation := func() error {
        if attempts >= m.opts.ReconnectMax { return backoff.Permanent(ErrMaxReconnect) }
        attempts++
        if err := m.reconnectOnce(alias); err != nil {
            // 인증 실패는 재시도 의미 없음
            var authErr *authFailedError
            if errors.As(err, &authErr) { return backoff.Permanent(err) }
            return err
        }
        return nil
    }
    if err := backoff.Retry(operation, bo); err != nil { /* 사용자에 알림 */ }
}

// dialer.go : x/crypto/ssh issue #51926 우회 (Dial 이 ClientConfig.Timeout 무시)
func dialSSH(ctx context.Context, network, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
    d := net.Dialer{Timeout: cfg.Timeout}
    conn, err := d.DialContext(ctx, network, addr)
    if err != nil { return nil, err }
    // 핸드셰이크에도 deadline 적용
    deadline := time.Now().Add(cfg.Timeout)
    _ = conn.SetDeadline(deadline)
    c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
    if err != nil { conn.Close(); return nil, err }
    _ = conn.SetDeadline(time.Time{})
    return ssh.NewClient(c, chans, reqs), nil
}
```

## 5. OpenSSH 호환성 노트

- `KeepAliveInterval` ↔ `ServerAliveInterval` (단위 초)
- `KeepAliveCountMax` ↔ `ServerAliveCountMax`
- `ConnectTimeout` ↔ `ConnectTimeout`
- `~/.ssh/config` 파일에서 위 키들을 발견하면 자동 import (Phase 3에서 마무리)

## 6. 마이그레이션/기존 코드 영향

- `ssh_session.go:98`의 `Timeout: 15*time.Second`를 `cfg.Timeout = opts.ConnectTimeout (default 15s)`로 변경 — 외부 동작 동일.
- `ssh.Dial` → `dialSSH(ctx, ...)`로 교체. `ensureSession`에 `ctx context.Context` 인자를 추가하지 않고 우선 internal context로 시작 (Phase 1 비파괴), Phase 2에서 caller signature 변경.
- `Connect`가 keep-alive supervisor를 시작; `Disconnect`가 supervisor.Stop().

## 7. 테스트 전략

- **단위 테스트**: `gliderlabs/ssh`(MIT)로 in-memory SSH 서버를 띄워 `keepalive@openssh.com` 처리 + 일정 시간 후 close → reconnect 검증.
- **단위 테스트**: `dialSSH`에 `iptables -A INPUT -p tcp --dport 22 -j DROP` 시뮬레이션 (test-only `net.Listener`로 read를 hang 시킴) → ConnectTimeout 동작 확인.
- **회귀 테스트**: 기존 password/agent/key 인증 경로 모두 통과.
- **Fuzz**: keep-alive interval 0~3600 범위 + countMax 0~100에서 panic 없음.

## 8. 위험 및 완화

1. **너무 자주 keep-alive를 보내면 서버가 차단할 수 있음** → 기본값 30초, ssh_config 표준과 동일.
2. **재연결 시 SFTP/터널 핸들 stale** → `client.Close()` 시 채널풀·터널 자동 close (이미 부분 구현, Phase 2에서 강화).
3. **백오프 무한루프** → `MaxElapsedTime`이 아닌 `attempts >= ReconnectMax` 종료 조건 사용.
4. **Goroutine 누수** (issue #47541) → context cancel + done channel 명시적 종료 검증.

## 9. 검증 방법

```
Argus.exe --aidebug -p "test@server1: 30분 idle 후에도 연결 유지 + ls /tmp"
Argus.exe --aidebug -p "test@server1: 네트워크 끊기 후 30초 내 자동 재연결 확인"
```

실제 서버에서 `tcpkill` 또는 라우터 `iptables DROP`로 단절 시뮬레이션, 30초 내 자동 복구 + UI 토스트 "재연결됨".

## 10. 작업 분해 (구현 체크리스트)

- [ ] `options.go` 작성: SSHOptions 구조체, default 값 함수
- [ ] `dialer.go` 작성: dialSSH 함수 (ctx + Deadline)
- [ ] `keepalive.go` 작성: keepAliveSupervisor + run()
- [ ] `reconnect.go` 작성: reconnectWithBackoff
- [ ] `manager.go` 수정: Connect()에서 supervisor 시작, Disconnect()에서 Stop()
- [ ] `ssh_session.go` 수정: ssh.Dial → dialSSH, Timeout 하드코딩 제거
- [ ] `types.go` 수정: ServerEntry.KeepAlive 필드
- [ ] 단위 테스트 작성 (gliderlabs/ssh mock)
- [ ] Fuzz 테스트 작성
- [ ] `--aidebug` 시나리오로 수동 검증

## 11. 참고 출처

- scylladb/go-sshtools — keep-alive ticker 패턴
- golang/go issue #21478, #51926
- OpenSSH ssh_config(5)
