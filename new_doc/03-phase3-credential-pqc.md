# Phase 3 — 자격증명 보안 (크로스 플랫폼 keyring + SSH CA + PQC + known_hosts 강화)

> **권장 순서**: 4번째
> **영향도**: 높음 / **구현 난이도**: 중간 / **80% 효과 컷**: ✅ 권장

## 목표 한 줄 요약
비-Windows에서도 OS native keyring을 사용해 평문 저장을 제거하고, SSH CA 단기 인증서를 1차 인증으로 채택하며, OpenSSH 10.0의 PQC hybrid KEX를 기본값으로 채택한다.

## 1. 신규 도입 라이브러리/외부 의존성

- `github.com/99designs/keyring` (MIT) — Keychain/SecretService/WinCred/Pass/File backend 통합
- `golang.org/x/crypto/ssh` (BSD-3) — `mlkem768x25519-sha256` 지원 (Go 1.23+ 가정)
- `github.com/kevinburke/ssh_config` (MIT) — `~/.ssh/config` 파서
- 라이선스 노트: 99designs/keyring(MIT), 안전.

## 2. 신규/수정 파일

### 신규
- `internal/security/keyring/keyring.go`
- `internal/security/keyring/store.go` — `Encrypter` 인터페이스 구현
- `internal/services/workspace/ssh_ca.go` — SSH 인증서 로드/검증/리프레시
- `internal/services/workspace/known_hosts.go` — strict 모드, UpdateHostKeys, CVE-2025-26465 대응
- `internal/services/workspace/algorithms.go` — KEX/Cipher/MAC/HostKey 알고리즘 정책
- `internal/services/workspace/sshconfig_import.go` — `~/.ssh/config` 파서

### 수정
- `internal/security/dpapi.go` — fallback 우선순위 정의
- `internal/services/workspace/credentials.go` — keyring backend 선택 가능

## 3. 핵심 인터페이스/타입 정의

```go
// internal/security/keyring/keyring.go
type Backend string
const (
    BackendAuto         Backend = "auto"
    BackendWinCred      Backend = "wincred"
    BackendKeychain     Backend = "keychain"
    BackendSecretService Backend = "secret_service"
    BackendKWallet      Backend = "kwallet"
    BackendPass         Backend = "pass"
    BackendFileFallback Backend = "file"
)

type Store struct { ring keyring.Keyring; backend Backend }
func Open(service string, prefer Backend) (*Store, error)
func (s *Store) Set(key string, plaintext []byte) error
func (s *Store) Get(key string) ([]byte, error)
func (s *Store) Delete(key string) error
func (s *Store) Backend() Backend

// internal/services/workspace/ssh_ca.go
type SSHCertificate struct {
    PrivateKeyPath  string    // ~/.ssh/id_ed25519 (사용자 키)
    CertificatePath string    // ~/.ssh/id_ed25519-cert.pub
    NotAfter        time.Time
    Principals      []string
    Critical        map[string]string
}
func LoadCertificateAuthMethod(c SSHCertificate) (ssh.AuthMethod, error)
// 만료 임박(15분) 시 외부 CA 엔드포인트(예: HashiCorp Vault, Smallstep)에 갱신 요청
func RefreshIfExpiring(ctx context.Context, c *SSHCertificate, refresher CARefresher) error

type CARefresher interface {
    Sign(ctx context.Context, pub ssh.PublicKey, principals []string) (*ssh.Certificate, error)
}

// internal/services/workspace/algorithms.go
type AlgorithmPolicy struct {
    KexAlgorithms     []string `json:"kex_algorithms"`
    HostKeyAlgorithms []string `json:"host_key_algorithms"`
    Ciphers           []string `json:"ciphers"`
    MACs              []string `json:"macs"`
    PQCMode           string   `json:"pqc_mode"` // "prefer" | "require" | "off"
}
func DefaultPolicy() AlgorithmPolicy // PQC prefer + secure cipher 화이트리스트
func (p AlgorithmPolicy) Apply(cfg *ssh.ClientConfig)
```

## 4. 핵심 의사코드

```go
// keyring/keyring.go (99designs/keyring 어댑터)
func Open(service string, prefer Backend) (*Store, error) {
    cfg := keyring.Config{
        ServiceName:              service,
        AllowedBackends:          backendsFor(prefer),
        FilePasswordFunc:         filePasswordPrompt, // master pw
        FileDir:                  filepath.Join(os.UserConfigDir(), service, "keyring"),
        KeychainTrustApplication: true,
        KeychainSynchronizable:   false,
        LibSecretCollectionName:  service,
    }
    ring, err := keyring.Open(cfg); if err != nil { return nil, err }
    return &Store{ring: ring, backend: detectActive(ring)}, nil
}

// CredentialStore 와 통합: Encrypter 인터페이스 만족
func (s *Store) Encrypt(plain []byte) ([]byte, error) {
    // keyring 자체가 암호화하므로 우리는 unique key를 만들고 그 key 내용물을 plain 으로 둠
    id := uuid.NewString()
    if err := s.Set(id, plain); err != nil { return nil, err }
    return []byte(id), nil // ciphertext = lookup key
}
func (s *Store) Decrypt(blob []byte) ([]byte, error) { return s.Get(string(blob)) }

// known_hosts.go : CVE-2025-26465 대응
func StrictHostKeyCallback(path string, opts SSHOptions) (ssh.HostKeyCallback, error) {
    base, err := knownhosts.New(path); if err != nil { return nil, err }
    return func(host string, remote net.Addr, key ssh.PublicKey) error {
        // VerifyHostKeyDNS 사용 시 SSHFP DNSSEC 검증 + 서명 결과 검사
        // (CVE-2025-26465: client 가 partial signature 응답을 잘못 검증)
        if err := base(host, remote, key); err != nil {
            var keyErr *knownhosts.KeyError
            if errors.As(err, &keyErr) && len(keyErr.Want) > 0 {
                // MITM 가능성 — strict 모드는 절대 자동 갱신 X
                return fmt.Errorf("HOST KEY CHANGED: refusing connection (CVE-2025-26465 권고)")
            }
            // unknown host: strict 모드에서는 사용자 확인 필요
            if opts.StrictHostKeyChecking == "yes" { return err }
            return promptUserAcceptKey(host, key)
        }
        return nil
    }, nil
}

// algorithms.go
func DefaultPolicy() AlgorithmPolicy {
    return AlgorithmPolicy{
        // OpenSSH 10.0 (2025-04) 기본값과 동일
        KexAlgorithms: []string{
            "mlkem768x25519-sha256",        // PQC hybrid (NIST FIPS 203)
            "sntrup761x25519-sha512@openssh.com",
            "curve25519-sha256",
            "curve25519-sha256@libssh.org",
            "diffie-hellman-group16-sha512",
        },
        HostKeyAlgorithms: []string{
            "ssh-ed25519-cert-v01@openssh.com",
            "ssh-ed25519",
            "rsa-sha2-512-cert-v01@openssh.com",
            "rsa-sha2-512",
        },
        Ciphers: []string{ // RC4/CBC 제외
            "chacha20-poly1305@openssh.com",
            "aes256-gcm@openssh.com", "aes128-gcm@openssh.com",
        },
        MACs: []string{
            "hmac-sha2-512-etm@openssh.com",
            "hmac-sha2-256-etm@openssh.com",
        },
        PQCMode: "prefer",
    }
}
```

## 5. OpenSSH 호환성 노트

- 알고리즘 정책 키들은 ssh_config의 `KexAlgorithms`/`HostKeyAlgorithms`/`Ciphers`/`MACs`와 동일.
- `CertificateFile`을 ServerEntry에 추가하여 `IdentityFile`(개인키) + `CertificateFile`(인증서) 조합 지원.
- `UpdateHostKeys yes` 호환 — 서버가 `hostkeys-prove-00@openssh.com`로 추가 키를 advertise하면 자동 추가.
- `~/.ssh/config` import: `Host`, `HostName`, `User`, `Port`, `IdentityFile`, `CertificateFile`, `ProxyJump`, `ServerAliveInterval`, `ConnectTimeout`, `StrictHostKeyChecking` 키 인식.

## 6. 마이그레이션/기존 코드 영향

- `CredentialStore`의 `Encrypter` 필드 (DPAPI)를 keyring backend로 우선 시도, 실패 시 기존 DPAPI(Windows) 또는 file-AES(`age` MIT 권장) fallback.
- 기존 `~/.Argus/ssh_credentials.json`은 자동 마이그레이션: 시작 시 기존 entry를 keyring으로 옮기고 파일 권한 0o600 유지(legacy 호환), 다음 부팅에서 안전하게 빈 파일로 트림.
- `tofuHostKeyCallback`(`ssh_session.go:1238`)을 `StrictHostKeyCallback`로 대체. 기본 모드 `accept-new`(unknown은 prompt, mismatch는 거부) — 기존 silent TOFU에서 거동 변경. ⚠️ **breaking-ish**, 사용자 알림 필요.
- ⚠️ **PQC 기본값으로 강제할지 여부**: 현재 권장은 `prefer`(서버가 지원 안 하면 fallback). `require`로 강제할지 사용자 선택.

## 7. 테스트 전략

- **단위**: 99designs/keyring의 mock backend로 set/get/delete.
- **Cross-platform CI** (GitHub Actions matrix: windows-latest, macos-latest, ubuntu-latest) — 각 OS native backend 동작 확인.
- **SSH CA**: smallstep CLI(`step ssh certificate`)로 테스트용 인증서 발급, NotAfter 30초 뒤 만료, 자동 갱신 확인.
- **Fuzz**: known_hosts 파일에 임의의 garbage 라인 → 파싱 안전.
- **알고리즘 정책**: `gliderlabs/ssh`로 PQC만 advertise하는 서버 vs 비-PQC 서버 모두 연결 가능.

## 8. 위험 및 완화

1. **Linux Headless 환경(libsecret 데몬 미실행)** → `BackendFileFallback`로 자동 전환, 파일 마스터 패스워드는 첫 사용 시 입력.
2. **PQC 강제 시 구형 OpenSSH(8.x) 서버 연결 실패** → `prefer` 모드 기본.
3. **SSH CA 인증서 시계 skew** → 클라이언트 NTP 검사 + 1분 grace.
4. **CVE-2025-26465**: `VerifyHostKeyDNS` 사용 시 결함 — Argus는 기본적으로 DNS SSHFP 사용 안 함, 명시 enable 시 경고.
5. **keyring 마스터 비밀번호 잊음** → 복구 코드 / 재인증 플로우 docs 명시.

## 9. 검증 방법

```
Argus.exe --aidebug -p "Linux 노트북에서 keyring set/get 동작 확인 (Secret Service)"
Argus.exe --aidebug -p "smallstep CA로 발급한 인증서로 server1에 인증, 만료 5분 전 자동 갱신"
Argus.exe --aidebug -p "PQC 강제 모드에서 OpenSSH 8.5 서버 연결 시 명확한 에러 + suggestion"
ssh-keygen -R server1 후 재접속 → 자동 갱신 X, 사용자 prompt 확인
```

## 10. 작업 분해 (구현 체크리스트)

- [ ] `keyring/keyring.go`: 99designs/keyring 어댑터, Backend 자동 선택
- [ ] `keyring/store.go`: Encrypter 인터페이스 구현
- [ ] `credentials.go` 수정: keyring 우선, DPAPI fallback
- [ ] 기존 `ssh_credentials.json` 자동 마이그레이션 로직
- [ ] `algorithms.go`: DefaultPolicy + Apply(cfg) — PQC prefer
- [ ] `known_hosts.go`: StrictHostKeyCallback (CVE-2025-26465)
- [ ] `ssh_ca.go`: LoadCertificateAuthMethod + RefreshIfExpiring
- [ ] `sshconfig_import.go`: kevinburke/ssh_config 파서 통합
- [ ] CI matrix (windows/macos/ubuntu) 추가
- [ ] smallstep으로 SSH CA 통합 검증

## 11. 참고 출처

- 99designs/keyring (GitHub README — backend matrix)
- OpenSSH 10.0 release notes — mlkem768x25519-sha256
- NIST FIPS 203 (ML-KEM)
- CVE-2025-26465 advisory
- smallstep — SSH certificate workflow
