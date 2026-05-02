# aidebug NDJSON Trace Schema

`--aidebug` 모드 활성화 시 stderr와 trace 파일에 NDJSON(Newline-Delimited JSON) 형태로 발행되는 모든 trace record의 타입 카탈로그입니다.

## 공통 Envelope

```json
{
  "ts":         "2026-04-27T10:00:00.000000000Z",
  "type":       "<domain>.<event>",
  "session_id": "uuid",
  "turn_index": 1,
  "seq":        42,
  "provider":   "anthropic",
  "model":      "claude-opus-4-7",
  "call_id":    "tool_abc123",
  "data":       { ... }
}
```

필드 설명:
- `type`: `<domain>.<event>` 형식. 아래 카탈로그 참조.
- `seq`: 세션 내 단조 증가하는 발행 순번.
- `turn_index`: 사용자 메시지 기준 대화 턴 번호 (1-based).
- `call_id`: 도구 호출 ID (도구 관련 trace에만 포함).

---

## Trace Type 카탈로그

명명 규칙: `<domain>.<event>` — domain은 영역, event는 동사/상태.

### session

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `session.start` | aidebug 세션 최초 진입 | `working_dir` |

### turn

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `turn.start` | 사용자 입력 처리 시작 | `user_input` |
| `turn.finish` | 턴 처리 완료 (성공/에러 무관) | `stop_reason`, `is_error`, `duration_ms`, `context_usage?` |

### llm

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `llm.request` | LLM API 요청 직전 | `message_count`, `system_blocks`, `tool_count`, `max_tokens?`, `temperature?`, `thinking?` |
| `llm.provider_chunk` | 스트리밍 청크 수신 시 (어시스턴트 델타 포함) | `delta`, `model`, `stop_reason?` |
| `llm.thinking` | 어시스턴트 thinking 블록 완료 | `done: true` |

### assistant

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `assistant.final` | 어시스턴트 최종 응답 완료 | `text`, `chars`, `stop_reason` |

### tool

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `tool.call.start` | 도구 실행 시작 | `name`, `input` |
| `tool.call.finish` | 도구 실행 완료 | `name`, `duration_ms`, `is_error` |
| `tool.call.output` | 도구 출력 | `name`, `output`, `is_error`, `truncated?` |
| `tool.permission` | 도구 권한 검사 결과 | `name`, `decision` (`allow`/`deny`/`ask`), `rule?` |

### hook

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `hook.dispatch` | UserPromptSubmit 훅 발화 후 | `event`, `continue`, `block_reason`, `duration_ms` |

### context

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `context.consolidate` | 컨텍스트 압축(ForceConsolidate) 실행 | `trigger` (`token_limit`/`snip`), `message` |

### notice

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `notice` | 내부 상태 알림 (tool 실행, web 검증, context snip 등) | `category` (`tool`/`web_verification`/`context`), `message` |

### mcp

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `mcp.server` | MCP 서버 로드/리로드 결과 | `phase` (`load`/`reload`/`fail`), `servers?`, `error?` |

### state

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `state.change` | AppState 주요 필드 변경 | `field`, `before`, `after` |

`field` 가능값:
- `active_model` — 활성 모델 alias 변경
- `effort_level` — effort 레벨 변경 (`high`/`medium`/`low`)
- `context_used_percent` — 컨텍스트 사용률(%) 변경
- `permission_mode` — 권한 모드 변경
- `plan_mode` — 플랜 모드 진입/종료 (`normal`/`plan`)
- `workspace` — 활성 워크스페이스 변경
- `mcp_server_count` — MCP 서버 수 변경
- `skill_count` — 활성 스킬 수 변경

### slash_command

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `slash_command` | 슬래시 명령(`/model`, `/effort` 등) 실행 후 | `name`, `is_error` |

### view

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `view.cleared` | `/clear` 명령으로 화면 초기화 | `reason` (`slash_command`) |

### plan

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `plan.step` | AI-debug 플랜 스텝 실행 | `index`, `total`, `tool`, `prompt`, `decision` (`allow`/`deny`), `output?`, `error?` |

### aidebug

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `aidebug.decision` | aidebug 자동화 결정 (도구 승인, ask_user 응답 등) | `phase` (`tool_approval`/`ask_user`), `tool?`, `prompt?`, `decision` (`allow`/`deny`), `raw_answer?`, `error?` |

### sink

| type | 발생 조건 | data 필드 |
|------|-----------|-----------|
| `sink.dropped` | Sink 버퍼(1000개) 포화로 trace silent drop 발생 | `count` (누적 드랍 수) |

---

## 검증 체크리스트 (수동)

```bash
./Argus.exe --aidebug -p "Read README.md" 2> trace.ndjson

for t in session.start turn.start turn.finish llm.request llm.provider_chunk \
         tool.call.start tool.call.finish tool.call.output tool.permission \
         assistant.final notice mcp.server state.change; do
  count=$(grep -c "\"type\":\"$t\"" trace.ndjson 2>/dev/null || echo 0)
  echo "$t: $count"
done
```

---

## 신규 Trace 추가 가이드

1. `<domain>.<event>` 명명 규칙 준수
2. 반드시 `(*Engine).emitTrace()` 단일 진입점을 통해 발행
3. 본 문서의 카탈로그 표에 추가
4. `presentation.FromUIEvent`의 동등한 case와 1:1 대응 검토
