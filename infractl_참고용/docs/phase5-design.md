# Phase 5 설계도 — 안전 체계 + 대화 컨텍스트

## 목표
위험 작업 다단계 경고. 대화 세션 영속화/복원. 프롬프트 히스토리. INFRACTL.md.

## 선행: Phase 3, 4 완료

---

## 추가/변경 파일

```
internal/
├── agent/safety.go           # 위험도 판단 + 확인 흐름
├── agent/loop.go             # 시스템 프롬프트에 상태 정보 강화 + 실행 이력 기록
├── storage/sessions.go       # 세션 저장/복원/검색
├── storage/history.go        # 프롬프트 히스토리 저장/검색
├── storage/execlog.go        # 도구 실행 이력 저장/조회
└── cli/repl.go (또는 tui/)   # /sessions, /clear, Ctrl+R 추가
```

---

## 위험도 체계

### 도구별 위험도 태깅

| 위험도 | 확인 방식 | 예시 |
|--------|-----------|------|
| none | 바로 실행 | 상태 조회, 로그 읽기, 프로세스 목록 |
| low | 1회 y/n | 서비스 재시작, 세션 kill, 파일 쓰기 |
| medium | 2회 확인 + 대상 명시 | 데이터 삭제, 설정 변경, cron 등록 |
| high | 3회 확인 + 이름 직접 입력 | DROP/TRUNCATE, 운영 DB 구조 변경, rm -rf |

### LLM 동적 위험도 판단
빌트인 도구는 위험도가 고정이지만, shell_exec 도구의 경우 LLM이 명령 내용을 보고 판단:

시스템 프롬프트에 추가:
```
When using shell_exec, assess the risk of the command:
- READ-ONLY commands (ls, cat, ps, df, SELECT): execute freely
- MODIFICATION commands (restart, kill, echo > file): warn once
- DESTRUCTIVE commands (DROP, TRUNCATE, DELETE, rm -rf, mkfs): warn multiple times
  and require the user to type the target name to confirm

If the user's request is ambiguous (e.g., "전부 다", "아까 그거"), 
ask for clarification BEFORE executing anything.
```

### 확인 흐름 구현
```
low:
  "⚠️ {서버}에서 {명령} 실행합니다. 계속? (y/n)"
  → y → 실행

medium:
  "⚠️ {서버} / {서비스} / {대상} 에 대해 {작업}을 수행합니다. 계속? (y/n)"
  → y → "⚠️ 최종 확인. 대상: {대상}. 진행? (y/n)"
  → y → 실행

high:
  "⚠️⚠️⚠️ 고위험: {서버}에서 {작업}. 되돌릴 수 없습니다! 계속? (y/n)"
  → y → "⚠️ 정말 확실합니까? (y/n)"
  → y → "대상 이름을 직접 입력해주세요:"
  → 이름 일치 → 실행
```

---

## 대화 세션 영속화

### SQLite 테이블
```sql
conversations:  id, title, created_at, updated_at
messages:       id, conversation_id, role, content, tool_calls(JSON), tool_call_id, name, timestamp
```

### 동작
- 매 대화 턴마다 자동 저장 (conversation_id 기준)
- 세션 제목: 첫 사용자 입력 또는 LLM이 요약 생성
- `/sessions` → 최근 세션 목록
- "1번 이어서 해줘" → 해당 세션의 messages 로드 → 에이전트 히스토리 복원

### 세션 복원 시
1. conversations에서 ID로 조회
2. messages 전체 로드
3. agent.history에 설정
4. 시스템 프롬프트에 "이전 세션에서 이어서 진행" 컨텍스트 추가

---

## 프롬프트 히스토리

### SQLite 테이블
```sql
prompt_history:  id, input, session_id, timestamp
```

### 동작
- 모든 사용자 입력을 저장 (비밀번호 마스킹)
- `Ctrl+R` → 역방향 검색 모드 (readline/bubbletea)
- `↑ ↓` → 이전/다음 입력
- 마스킹: 비밀번호, API 키 등 민감 정보 패턴 치환

---

## 도구 실행 이력 (execution_logs)

모든 도구 실행의 상세 이력을 기록하여, 향후 에러 패턴 학습(Phase 6)과 로컬 벡터 검색(Phase 9)의 데이터 기반이 된다.

### SQLite 테이블
```sql
CREATE TABLE execution_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id INTEGER REFERENCES conversations(id),
  tool_name TEXT NOT NULL,           -- 실행된 도구명 (shell_exec, oracle_orcl.query 등)
  target_server TEXT DEFAULT 'localhost',
  input_params TEXT,                 -- JSON: 도구에 전달된 파라미터
  output TEXT,                       -- stdout 결과 (10000자 제한)
  error_message TEXT,                -- stderr 또는 에러 메시지
  exit_code INTEGER,
  duration_ms INTEGER,               -- 실행 소요 시간
  risk_level TEXT DEFAULT 'none',    -- none/low/medium/high
  success BOOLEAN NOT NULL,          -- 성공 여부
  retry_of INTEGER REFERENCES execution_logs(id),  -- 재시도인 경우 원본 실패 로그 ID
  user_prompt TEXT,                  -- 이 실행을 유발한 사용자 입력 원문
  llm_reasoning TEXT,                -- LLM이 이 도구를 선택한 이유 (간략)
  timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_exec_tool ON execution_logs(tool_name);
CREATE INDEX idx_exec_success ON execution_logs(success);
CREATE INDEX idx_exec_error ON execution_logs(error_message) WHERE error_message IS NOT NULL;
```

### 기록 시점
- **매 도구 실행 완료 시** 자동 기록 (에이전트 루프 내부)
- 성공/실패 모두 기록
- 에러 후 LLM이 수정해서 재시도할 때 `retry_of`로 원본 실패와 연결

### 재시도 체인 기록
```
도구 실행 #1 (실패)
  → error: "ORA-00942: table or view does not exist"
  → LLM이 원인 분석 + 수정
  ↓
도구 실행 #2 (성공, retry_of=#1)
  → 수정된 쿼리로 성공
  → Phase 6에서 이 패턴을 knowledge_base에 자동 저장
```

### 실행 이력 조회
- LLM이 필요 시 "최근 실패 이력" 참조 가능
- `/history` 슬래시 명령으로 최근 실행 이력 조회
- 세션별, 도구별, 서버별 필터링

---

## 시스템 프롬프트 강화

Phase 5에서 시스템 프롬프트에 추가되는 정보:
```
## Current Server Status
- localhost: RHEL 8.9, 4코어, 32GB
- db-server: connected (Oracle ORCL 활성, 6개 도구)
- was-server: connected (Tomcat 8080 활성)

## Active Connectors
- oracle_orcl: query, tablespace, sessions, locks, alert_log, top_sql
- tomcat_8080: status, thread_dump

## Safety Rules
- 위험 작업은 반드시 경고 후 확인
- 애매한 요청은 명확화 질문
- INFRACTL.md 운영 규칙 준수
```

---

## INFRACTL.md 자동 로딩

경로 우선순위:
1. `~/.infractl/INFRACTL.md` (전역)
2. `/etc/infractl/INFRACTL.md` (서버별 — 로컬 실행 시)

시작 시 자동 읽어서 시스템 프롬프트에 포함.
내용 예: "Oracle 재시작 전 배치 스케줄러 먼저 중지", "22~23시 재시작 금지" 등

### @include 지시자 (Phase 5 추가)

INFRACTL.md에서 다른 파일을 포함할 수 있다:

```markdown
# INFRACTL.md
@include ./runbooks/oracle-restart.md
@include ./runbooks/incident-checklist.md

## 일반 규칙
- 작업 전 항상 티켓 번호 확인
```

`LoadInfractlMD()`가 `@include` 지시자를 처리하여 파일 내용을 인라인으로 삽입한다.
상대 경로는 INFRACTL.md 위치 기준으로 해석한다.

---

## knowledge_base 키워드 검색 (Phase 9 RAG 전까지)

Phase 6에서 구축하는 knowledge_base를 Phase 9의 벡터 검색 이전까지
SQLite FTS5로 키워드 검색한다.

```sql
-- FTS5 가상 테이블 (knowledge_base와 동기화)
CREATE VIRTUAL TABLE knowledge_fts USING fts5(
    title, situation, resolution, error_pattern,
    content='knowledge_base', content_rowid='id'
);

-- 키워드 검색
SELECT kb.*
FROM knowledge_base kb
JOIN knowledge_fts ON kb.id = knowledge_fts.rowid
WHERE knowledge_fts MATCH 'ORA-04031 shared pool'
ORDER BY rank;
```

**에이전트 루프 연동 (Phase 6의 buildErrorHints와 연계):**
도구 실행 실패 시, 에러 메시지로 knowledge_fts를 검색하여 관련 지식이 있으면
힌트 메시지에 포함하여 LLM에 전달한다.

---

## 검증 시나리오

1. "Oracle 세션 142 kill" → low 확인 1회 → 실행
2. "테이블 DROP" → high 확인 3회 + 이름 입력 → 실행 또는 취소
3. "전부 다 지워줘" → LLM이 "어떤 것을 말씀하시는 건가요?" 질문
4. `/sessions` → 목록 → "1번 이어서" → 복원 → 컨텍스트 유지 확인
5. `Ctrl+R` → "oracle" 검색 → 이전 입력 찾기
6. INFRACTL.md에 "22시 이후 재시작 금지" → 22시에 "Tomcat 재시작" → 경고

---

## 완료 기준
- [ ] 위험도별 다단계 확인
- [ ] LLM 동적 위험도 판단 (shell_exec 내용 분석)
- [ ] 애매한 요청 명확화 질문
- [ ] 세션 SQLite 저장 + /sessions 복원
- [ ] Ctrl+R 히스토리 검색
- [ ] INFRACTL.md 자동 로딩
- [ ] INFRACTL.md @include 지시자 처리
- [ ] 도구 실행 이력 자동 기록 (execution_logs)
- [ ] 재시도 체인 기록 (retry_of 연결)
- [ ] /history 실행 이력 조회
- [ ] knowledge_base FTS5 가상 테이블 생성 (Phase 6 준비)
- [ ] **Phase 5 완료 = 실무 사용 가능**
