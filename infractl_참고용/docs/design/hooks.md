# Hook 시스템 설계 (Phase A 즉시 사용 / D 완전 활성 / G CLI + 핫리로드)

> 본 문서는 **Phase A 진입 직전 참고용**. 실행 계획은 [`../../docs_mig/01_phase_a_infrastructure.md`](../../docs_mig/01_phase_a_infrastructure.md), [`04_phase_d_hooks_migration.md`](../../docs_mig/04_phase_d_hooks_migration.md), [`07_phase_g_planmode_todo.md`](../../docs_mig/07_phase_g_planmode_todo.md) 참조.

---

## 1. 왜 Hook 인가

현재 InfraCtl 은 위험 명령 차단을 `internal/preflight/*.go` 에 Go 코드로 박아뒀다. 한계:

- 조직별 정책 반영 = **재컴파일**.
- LLM 검증 / 결정론 / 구조화 yaml 검증이 **각각 다른 파일** 에 흩어짐.
- 외부 시스템 (ITSM, OPA, SIEM) 연동 불가.

claude_cli 의 Hook 시스템은 위 한계를 전부 해소:

- `~/.infractl/hooks.yaml` 로 **재컴파일 없이 정책 변경**.
- **4 가지 backend** 로 command / prompt / http / agent 를 하나의 인터페이스로 통합.
- **이벤트 지점** (PreToolUse, PostToolUse, PreSubagent, ...) 이 명확히 정의됨.

---

## 2. 이벤트 지점 (claude_cli 동등)

| 이벤트 | 시점 | 주요 input | hook 결정 영향 |
|---|---|---|---|
| **PreToolUse** | 도구 호출 **직전** | `{tool, input}` | `approved: false` → 차단 / `newInput: {...}` → 입력 수정 |
| **PostToolUse** | 도구 호출 **직후** | `{tool, input, output}` | 감사 로그, 비동기 통보 (결정에는 영향 X) |
| **PreSubagent** | subagent fork 직전 | `{prompt, tools}` | 차단 / 도구 제한 |
| **PostSubagent** | subagent 종료 직후 | `{prompt, result}` | 감사 |
| **SessionStart** | 새 세션 진입 | `{user, cwd}` | context 추가 (환영 메시지, org-wide 정책 주입) |
| **SessionEnd** | 세션 종료 | `{duration, token_usage}` | 감사, 비용 리포트 |

> Phase A 에서는 **PostToolUse 만** 활성화 (관측용). Phase D 에서 **PreToolUse 활성** + preflight 흡수.

---

## 3. Backend 4 종

### ① command backend
- **언제**: 결정론적 검사 (쉘 스크립트로 충분한 것)
- **실행**: stdin 으로 hook input JSON → stdout 으로 hook output JSON
- **timeout**: default 30s, 설정 가능
- **예시**: `rm -rf /` 차단, `/etc` 보호 경로 검사

```yaml
PreToolUse:
  - matcher: "Bash(rm -rf /*)"
    backend: command
    command: "${infractl_home}/hooks/system_risk.sh"
    timeout: 10s
```

```bash
#!/bin/bash
# system_risk.sh
input=$(cat)
command=$(echo "$input" | jq -r '.input.command')
if echo "$command" | grep -qE '^rm\s+-rf\s+/($|\s)'; then
  echo '{"approved": false, "reason": "rm -rf / 차단"}'
  exit 0
fi
echo '{"approved": true}'
```

### ② prompt backend
- **언제**: LLM 이 자연어 판단 필요 (yaml/json 스키마 검증 등)
- **실행**: hook input + 프롬프트 템플릿 → Fast tier LLM 호출 → 결과 파싱
- **tier**: default Fast (Qwen) — reasoning tier 필요 시 명시
- **예시**: yaml 설정 파일 작성 시 안전성 검토

```yaml
PreToolUse:
  - matcher: "Write(*.yaml)"
    backend: prompt
    prompt: "${infractl_home}/hooks/structured_config.md"
    tier: fast
    timeout: 30s
```

### ③ http backend
- **언제**: 외부 시스템 연동 (ITSM 승인, OPA 정책 평가, SIEM 통보)
- **실행**: hook input → HTTP POST → 응답 JSON 파싱
- **보안**: 헤더에 `$VAR` 보간 허용 (단, `allowedEnvVars` 화이트리스트)
- **예시**: prod-db 재시작은 ITSM 승인 대기

```yaml
PreToolUse:
  - matcher: "Bash(systemctl restart prod-*)"
    backend: http
    url: "https://itsm.internal/approvals"
    method: POST
    headers:
      Authorization: "Bearer $ITSM_TOKEN"
    allowedEnvVars: ["ITSM_TOKEN"]
    timeout: 300s
    async: true   # 응답 대기 대신 폴링
    statusMessage: "ITSM 승인 대기 중..."
```

### ④ agent backend
- **언제**: 복잡한 LLM 판단 (현재 preflight/validator 동등)
- **실행**: mini-agent fork → 전용 시스템 프롬프트 + probe_* 도구로 자체 조사 → 결과 반환
- **예시**: shell 명령 전반에 대한 AI 검증 (기존 `validatorSystemPrompt` 흡수)

```yaml
PreToolUse:
  - matcher: "Bash(*)"
    if_disk_modifying: true
    backend: agent
    agent: "${infractl_home}/hooks/agent/shell_validator.md"
    tier: fast
    timeout: 90s
    tools: ["probe_command", "probe_path"]
```

---

## 4. Matcher 문법

```
Bash(rm -rf /*)           # 명확한 패턴
Bash(*)                   # 모든 Bash
Write(*.yaml)             # Write 도구 + 특정 확장자
Bash(*) if_disk_modifying # 조건부 (AST 분석 결과가 disk_modifying 인 경우만)
SSH(prod-*)               # SSH 도구 + 호스트 프리픽스 매칭
```

- **정확 일치 우선**, 와일드카드 후순위
- `if_*` 절: 도구가 전달하는 메타데이터 (`disk_modifying`, `network_access`, `read_only` 등) 기반
- Phase E 의 AST 분석 결과가 메타데이터 공급원

---

## 5. Hook Input / Output 스키마

### Input (hook 에게 전달)
```json
{
  "event": "PreToolUse",
  "tool": "Bash",
  "input": {
    "command": "rm -rf /tmp/test",
    "timeout": 30
  },
  "session": {
    "id": "sess_abc123",
    "user": "koreaf21",
    "cwd": "/home/user/project"
  },
  "metadata": {
    "disk_modifying": true,
    "read_only": false,
    "danger_score": "medium"
  }
}
```

### Output (hook 이 반환) — **Phase D 에서 claude_cli 정합으로 교체됨**
```json
{
  "decision": "deny",
  "reason": "내부 로그용 사유",
  "systemMessage": "사용자·LLM 에게 보일 메시지 (옵션)",
  "newInput": { "command": "rm -rf /tmp/test-sandboxed", "timeout": 30 }
}
```

- `decision: "allow"` (또는 미설정) → 도구 실행 허용
- `decision: "deny"` → 도구 실행 차단 + systemMessage/reason 이 tool_result 에 포함
- `decision: "ask"` → 1회 사용자 승인 후 진행 (Phase D 에서는 deny 로 축약, Phase G Plan Mode 에서 실체화)
- `newInput` 있음 → 도구가 수정된 input 으로 호출 (allow/ask 와 병행 가능)
- hook 자체 실패 (timeout / exception) → **fail-closed** (Phase D 기본) — Runner 가 `decision=deny, reason="hook error: ..."` 로 합성

> 과거 `approved: bool` 필드는 Phase D 에서 제거되었다. 하위 호환 레이어 없음.

---

## 6. 보안 규칙 (CLAUDE.md 연계)

- [ ] `~/.infractl/hooks.yaml` 권한 체크 — `group/world writable` 이면 거부 + slog WARN
- [ ] command backend 호출 시 hook input 을 **stdin** 으로만 전달 (쉘 인자 결합 금지)
- [ ] http backend 의 `$VAR` 보간은 `allowedEnvVars` 화이트리스트만
- [ ] hook input 에 있을 수 있는 **크리덴셜 마스킹** (로그 출력 시)
- [ ] 모든 hook 에 **timeout 강제** (default 30s)
- [ ] **fail-closed** 정책 — hook 자체 결함 시 도구 실행 차단 (명시적으로 `fail_open: true` 설정 전까지)
- [ ] slog: hook name, matcher, backend, duration, result 구조화 로그

---

## 7. 디렉토리 배치

```
internal/hooks/
├── types.go              ← Event, Backend, Input, Output 스키마
├── loader.go             ← yaml 파싱
├── registry.go           ← matcher + dispatch
├── runner.go             ← backend 호출, timeout, fail-closed
├── matcher.go            ← matcher 문법 해석
├── backend/
│   ├── command.go
│   ├── prompt.go
│   ├── http.go
│   └── agent.go
├── builtins/             ← 번들 스크립트/프롬프트 (Phase D 미니멀)
│   ├── system_risk.sh    ← fast-path 결정론 차단 1 개로 통합
│   ├── assets.go         ← //go:embed 로 내장 + 언박싱
│   └── agent/
│       └── shell_validator.md   ← LLM 위임 프롬프트 1 개로 통합
├── defaults/
│   └── hooks.yaml.default ← 설치 시 ~/.infractl/hooks.yaml 로 복사
├── watcher.go            ← fsnotify 핫리로드 (Phase G)
└── reloader.go           ← atomic 정책 교체 (Phase G)

~/.infractl/
└── hooks.yaml            ← 사용자 정책 (설치 시 hooks.yaml.default 로 초기화)
```

---

## 8. 생명주기

```
[Phase A] ✅ 구현 완료
  hooks 패키지 신설: event/config/snapshot/matcher/runner/timeout/backend (4종)
  기존 internal/hooks/ → internal/lifecycle/ 이동
  PostToolUse 활성 (관측용, 차단 X) — tool_exec.go 삽입 완료
  ~/.infractl/hooks.yaml 권한 체크 + fail-closed 정책 적용
  matcher_test.go: 28+ 케이스 통과

[Phase D] ✅ claude_cli 정합 방향으로 확정
  HookOutput 스키마 교체: approved → decision(allow|deny|ask) + systemMessage
  PreToolUse 활성화 + metadata (DiskModifying/ReadOnly/NetworkAccess/DangerScore) 주입
  builtins 최소화: system_risk.sh (fast-path 결정론) + shell_validator.md (LLM 위임)
    - 기존 tar/mkdir/protected_paths/structured_guard 의 결정론 스크립트 **신설 안 함**
    - 대신 shell_validator 프롬프트가 LLM 위임으로 통합 판단
  hooks.yaml.default + ~/.infractl/builtins/ 부트스트랩 (embed 언박싱)
  TUI 간결화: OnPreflightResult/warn/RequiresConfirmation 모달 제거
    - deny 시 tool_result 로 systemMessage 통합 전달 (claude_cli 방식)
  동등성 50/50 (수동 큐레이션 testdata/equivalence/*.yaml) → 통과 후 preflight/* 제거

[Phase G] ✅
  fsnotify 핫리로드 (500ms debounce, editor atomic save 대응) ✅
  CLI: infractl hooks {test, list, validate, reload} ✅
  org-wide hooks bundle 공유 준비 (향후 확장)
```

---

## 9. CLI (Phase G) ✅

```bash
# 현재 등록된 hook 목록
infractl hooks list

# 특정 이벤트 시뮬레이션
infractl hooks test \
  --event PreToolUse \
  --tool Bash \
  --input '{"command":"rm -rf /"}'

# yaml 유효성 검증
infractl hooks validate ~/.infractl/hooks.yaml

# 강제 재로드 (watcher 외)
infractl hooks reload
```

### 구현 완료 (Phase G)

- `internal/hooks/watcher.go` — fsnotify 기반 감시 루프 (ctx 취소 시 정리, 500ms debounce)
- `internal/hooks/reloader.go` — yaml 재파싱 + `snapshotMu.Lock()` atomic swap
- `cmd/infractl/hooks.go` — list/test/validate/reload 서브커맨드 dispatcher
- `main.go` — TUI 시작 시 `hooks.Watch(ctx, hooksYAML)` goroutine 구동

---

## 10. 테스트 전략

### 단위
- 각 backend 정식 케이스 + timeout / 예외
- matcher 문법 (정확 일치 / 와일드카드 / 조건부)
- loader (유효 yaml / 잘못된 yaml / 권한 체크)

### 통합 (`//go:build integration`)
- 실제 hooks.yaml + 실제 command 스크립트
- 실제 HTTP mock 서버 (로컬 테스트서버)
- 실제 Fast tier LLM 으로 prompt/agent backend

### E2E
- 시나리오 1: `rm -rf /etc/*` → command hook 차단 + reason 표시
- 시나리오 2: `systemctl restart prod-db` → http hook (mock ITSM) 승인 대기 → 승인 후 실행
- 시나리오 3: shell_validator agent → 50+ 회귀 케이스 통과
- 시나리오 4: hooks.yaml 실시간 변경 (fsnotify) → <1s 내 반영 (Phase G)
- 시나리오 5: hook timeout → fail-closed 차단 + 로그

---

## 11. 참고 (claude_cli 원본)

- `claude_cli/src/schemas/hooks.ts:211-222` — hook config 스키마
- `claude_cli/src/utils/hooks/execShellCommand.ts` — command backend 구현
- `claude_cli/src/utils/hooks/execPromptHook.ts` — prompt backend + `$ARGUMENTS` 보간
- `claude_cli/src/utils/hooks/execHttpHook.ts` — http backend + `$VAR` 화이트리스트
- `claude_cli/src/utils/hooks/execAgentHook.ts` — agent backend (mini-agent fork)
- `claude_cli/src/services/tools/toolExecution.ts` — PreToolUse 통합 지점

> Phase A / D 시작 전 위 파일 Read 필수 (포팅 정확도).

---

## 끝.
