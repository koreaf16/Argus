# 신규 설정 파일 스키마

> 본 문서는 모든 Phase가 도입한 신규 설정 키를 종합하여 `~/.Argus/`의 설정 파일들에 어떻게 반영되는지를 정의합니다.

## 1. `~/.Argus/settings.json` 확장

기존 settings.json에 다음 섹션이 추가됩니다.

```json
{
  "permissions": {
    "default_mode": "default",
    "agentguard": {
      "enabled": true,
      "policy_dry_run": false,
      "lethal_trifecta": "ask",
      "prompt_injection": "deny",
      "audit_log": "~/.Argus/audit/events.jsonl",
      "asciicast_dir": "~/.Argus/audit/casts"
    },
    "scopes": {
      "mcp": ["mcp:tools-basic"],
      "ssh": ["ssh:exec", "ssh:read"]
    }
  },
  "ssh_defaults": {
    "ServerAliveInterval": 30,
    "ServerAliveCountMax": 3,
    "ConnectTimeout": 15,
    "ReconnectMax": 5,
    "ReconnectInitial": "1s",
    "ReconnectCeiling": "60s",
    "PQCMode": "prefer",
    "StrictHostKeyChecking": "accept-new"
  },
  "keyring": {
    "backend": "auto",
    "service": "argus"
  },
  "observability": {
    "otel_endpoint": "",
    "sample_rate": 0.1
  }
}
```

### 키 설명

| 키 | 도입 Phase | 의미 | 기본값 |
|---|---|---|---|
| `permissions.agentguard.enabled` | Phase 4 | AI Agent Gateway 활성화 | `true` |
| `permissions.agentguard.policy_dry_run` | Phase 4 | 정책 평가만 하고 실제 차단은 X (관찰용) | `false` |
| `permissions.agentguard.lethal_trifecta` | Phase 4 | Lethal Trifecta 감지 시 행동 (`ask`/`deny`/`allow`) | `ask` |
| `permissions.agentguard.prompt_injection` | Phase 4 | injection.LooksMalicious 감지 시 행동 | `deny` |
| `permissions.agentguard.audit_log` | Phase 4 | append-only JSONL 경로 | `~/.Argus/audit/events.jsonl` |
| `permissions.agentguard.asciicast_dir` | Phase 4 | 세션 레코딩 디렉토리 | `~/.Argus/audit/casts` |
| `permissions.scopes.mcp` | Phase 5 | 기본 MCP scope | `["mcp:tools-basic"]` |
| `permissions.scopes.ssh` | Phase 4·5 | 기본 SSH scope | `["ssh:exec", "ssh:read"]` |
| `ssh_defaults.ServerAliveInterval` | Phase 1 | keep-alive 간격(초) | `30` |
| `ssh_defaults.ServerAliveCountMax` | Phase 1 | 연속 실패 임계 | `3` |
| `ssh_defaults.ConnectTimeout` | Phase 1 | 연결 타임아웃(초) | `15` |
| `ssh_defaults.ReconnectMax` | Phase 1 | 재시도 횟수 | `5` |
| `ssh_defaults.ReconnectInitial` | Phase 1 | backoff 시작 간격 | `1s` |
| `ssh_defaults.ReconnectCeiling` | Phase 1 | backoff 최대 간격 | `60s` |
| `ssh_defaults.PQCMode` | Phase 3 | `prefer`/`require`/`off` | `prefer` |
| `ssh_defaults.StrictHostKeyChecking` | Phase 3 | `yes`/`accept-new`/`no` | `accept-new` |
| `keyring.backend` | Phase 3 | `auto`/`wincred`/`keychain`/`secret_service`/`kwallet`/`pass`/`file` | `auto` |
| `keyring.service` | Phase 3 | keyring service 이름 | `argus` |
| `observability.otel_endpoint` | Phase 6 | OTLP/HTTP 엔드포인트 | `""` (off) |
| `observability.sample_rate` | Phase 6 | trace 샘플링 비율 | `0.1` |

## 2. `~/.Argus/ssh_config.json` (신규)

OpenSSH `ssh_config(5)` 형식과 1:1 매칭되는 키들을 그대로 사용합니다.

```json
{
  "imports": ["~/.ssh/config"],
  "hosts": [
    {
      "alias": "prod-db",
      "HostName": "10.0.5.21",
      "User": "ops",
      "Port": 22,
      "IdentityFile": "~/.ssh/id_ed25519",
      "CertificateFile": "~/.ssh/id_ed25519-cert.pub",
      "ProxyJump": ["bastion.example.com:22"],
      "ServerAliveInterval": 20,
      "DynamicForward": "127.0.0.1:1080",
      "KexAlgorithms": ["mlkem768x25519-sha256", "curve25519-sha256"],
      "MACs": ["hmac-sha2-512-etm@openssh.com"]
    }
  ]
}
```

### 키 설명

| 키 | 도입 Phase | OpenSSH 키 | 비고 |
|---|---|---|---|
| `imports` | Phase 3 | — | `~/.ssh/config`를 자동 import |
| `alias` | Phase 1+ | (Argus 고유) | Argus 내부 식별자 |
| `HostName` | — | `HostName` | 동일 |
| `User` | — | `User` | 동일 |
| `Port` | — | `Port` | 동일 |
| `IdentityFile` | Phase 3 | `IdentityFile` | 동일 |
| `CertificateFile` | Phase 3 | `CertificateFile` | SSH CA 인증서 경로 |
| `ProxyJump` | Phase 2 | `ProxyJump` | 슬라이스 (chain 가능) |
| `ServerAliveInterval` | Phase 1 | `ServerAliveInterval` | 동일 |
| `DynamicForward` | Phase 2 | `DynamicForward` | SOCKS5 listen 주소 |
| `KexAlgorithms` | Phase 3 | `KexAlgorithms` | 동일 |
| `MACs` | Phase 3 | `MACs` | 동일 |
| `Ciphers` | Phase 3 | `Ciphers` | 동일 |
| `HostKeyAlgorithms` | Phase 3 | `HostKeyAlgorithms` | 동일 |
| `StrictHostKeyChecking` | Phase 3 | `StrictHostKeyChecking` | 동일 |
| `IdentityAgent` | Phase 3 | `IdentityAgent` | 동일 |
| `KnownHostsCommand` | Phase 3 | `KnownHostsCommand` | 동일 |
| `UpdateHostKeys` | Phase 3 | `UpdateHostKeys` | 동일 |

## 3. `~/.Argus/mcp.json` 확장

기존 servers 항목에 보안 메타데이터가 추가됩니다.

```json
{
  "servers": [
    {
      "name": "github",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {"GITHUB_TOKEN": "${env:GITHUB_TOKEN}"},
      "trust": {
        "level": "verified",
        "cosign_identity_regex": "^maintainer@modelcontextprotocol\\.io$"
      },
      "scopes_required": ["mcp:tools-basic", "mcp:tools-write"],
      "sandbox": "default",
      "egress": {
        "allow": ["api.github.com", "*.githubusercontent.com"]
      },
      "audience": {
        "expected_aud": "argus-mcp-github",
        "issuer": "https://token.actions.githubusercontent.com"
      }
    }
  ]
}
```

### 키 설명

| 키 | 도입 Phase | 의미 |
|---|---|---|
| `trust.level` | Phase 5 | `unknown`/`community`/`verified`/`official` |
| `trust.cosign_identity_regex` | Phase 5 | Cosign keyless verify 시 maintainer 이메일 정규식 |
| `scopes_required` | Phase 5 | 호출 가능한 도구의 최대 scope 집합 |
| `sandbox` | Phase 5 | `off`/`default`/`strict` |
| `egress.allow` | Phase 5 | EgressFirewall allowList (호스트 패턴) |
| `audience.expected_aud` | Phase 5 | OIDC token aud claim 기대값 |
| `audience.issuer` | Phase 5 | OIDC issuer URL |

## 4. 새 설정 파일

| 파일 | 도입 Phase | 용도 |
|---|---|---|
| `~/.Argus/audit/events.jsonl` | Phase 4 | append-only audit log (hash chain) |
| `~/.Argus/audit/casts/<sessionID>.cast` | Phase 4 | asciicast v2 세션 레코딩 |
| `~/.Argus/keyring/` | Phase 3 | 99designs/keyring File backend (fallback 시) |
| `~/.Argus/policies.json` | Phase 4 | CEL 기반 AgentGuard 정책 (사용자 작성) |
| `~/.Argus/ssh_config.json` | Phase 1+ | SSH 호스트 정의 (위 참조) |

## 5. 마이그레이션 우선순위

1. **Phase 1**: `ssh_defaults` 섹션 추가. 기존 동작과 호환(zero value = 기존 default).
2. **Phase 2**: `ssh_config.json`의 `ProxyJump`, `DynamicForward` 키 인식 추가.
3. **Phase 3**: `keyring`, `KexAlgorithms`/`StrictHostKeyChecking` 등 보안 설정. 기존 `ssh_credentials.json`은 첫 부팅 시 자동 keyring 이동.
4. **Phase 4**: `permissions.agentguard` 섹션. 기본 ON이지만 dry_run 모드를 1주일 운영 후 enforce.
5. **Phase 5**: `mcp.json`의 `trust`/`sandbox`/`egress`/`audience` 섹션. 기존 servers 항목은 자동으로 `trust.level=unknown`으로 분류 → 사용자 승인 prompt.
6. **Phase 6**: `observability` 섹션. env `OTEL_EXPORTER_OTLP_ENDPOINT` override 가능.
