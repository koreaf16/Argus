# Phase 6 설계도 — 적응형 학습 + 에러 패턴 학습 + 사용자 도구

## 목표
빌트인에 없는 시스템을 웹 검색 + 서버 탐색으로 학습하여 대응.
**도구 실행 중 발생한 에러와 해결 과정을 자동 학습하여, 같은 실수를 반복하지 않는 자가학습 시스템 구축.**
사용자가 대화로 커스텀 도구 생성.

## 선행: Phase 5 완료

---

## 추가/변경 파일

```
internal/
├── tools/web_search.go       # web_search 도구 (DuckDuckGo Lite)
├── tools/web_fetch.go        # web_fetch 도구 (HTML→마크다운)
├── tools/web_cache.go        # URL 캐싱 (LRU, 15분 TTL)
├── tools/knowledge_search.go # knowledge_search 도구 (FTS5 기반)
├── tools/rag_search.go       # rag_search 도구 (Phase 9 연동 준비)
├── agent/learning.go         # 적응형 학습 로직
├── agent/knowledge.go        # 에러 패턴 학습 + knowledge_base 관리
├── tools/user_tool.go        # 사용자 정의 도구 동적 생성/로딩
└── storage/sqlite.go         # 테이블 추가: user_tools, learned_systems, knowledge_base
```

---

## 웹 검색 도구

### web_search
- DuckDuckGo Lite 또는 검색 API 사용 (API 키 불필요)
- 폴백: 대상 서버에서 curl로 실행
- 결과: HTML → 마크다운 변환, 5000자 제한
- 파라미터: query (필수)
- **IsReadOnly**: true (서버 상태 변경 없음)
- **IsEnabled**: 폐쇄망 감지 시 false로 전환

### web_fetch
- HTTP GET으로 URL 내용 가져오기
- HTML → 마크다운 변환 (`goquery` + 불필요 태그 제거)
  - nav, footer, sidebar, script, style, 광고 태그 제거
  - 코드 블록 보존
- 결과 크기 제한: 기본 8000자 (config에서 조정 가능)
- URL 결과 LRU 캐싱 (15분 TTL, 동일 URL 재요청 방지)
- **IsReadOnly**: true
- **IsEnabled**: web_search와 연동

### web_cache (`internal/tools/web_cache.go`)
```go
type WebCache struct {
    mu      sync.RWMutex
    entries map[string]cacheEntry  // URL → (content, expiry)
    maxSize int                    // LRU 최대 항목 수
}

type cacheEntry struct {
    content string
    expiry  time.Time
}
```

### 폐쇄망 대응
- 인터넷 불가 시 web_search/web_fetch 도구를 비활성화 (`IsEnabled() = false`)
- 설정 또는 자동 감지 (첫 호출 실패 시 자동 비활성화 + slog.Warn)
- 비활성화 후 시스템 프롬프트에서 도구 목록 제거 (LLM이 호출 시도하지 않음)

---

## 웹 지식 처리 파이프라인

단순 "검색 → 결과 반환" 대신, 에이전트 루프에 통합된 파이프라인으로 처리한다:

```
사용자: "ORA-04031 에러 해결방법"
  ↓
1. knowledge_search (FTS5): 로컬 지식 먼저 검색
   → 적중 시: 바로 LLM에 전달 (웹 검색 스킵)
   → 미적중 시: 다음 단계
  ↓
2. web_search("Oracle ORA-04031 shared pool size"): 검색 결과 3~5개 URL
  ↓
3. web_fetch(top 2~3 URL): HTML → 마크다운 변환 + 캐시
  ↓
4. LLM에 원본 질문 + 웹 내용 전달 → 답변 생성
  ↓
5. (자동) execution_logs에 retry_of 체인 감지
   → 실패→성공 패턴 발견 시 knowledge_base에 자동 저장
  ↓
6. 다음 같은 에러 → knowledge_search 적중 → 즉시 해결
```

**Tool Selection 연동 (buildErrorHints):**
도구 실행 실패 시 `loop.go`의 `buildErrorHints()`가 다음 힌트를 추가:
```
[Suggested next steps]
- Use knowledge_search to check if this error has been resolved before.
- Use web_search to find solutions for this error online.
```
LLM이 이 힌트를 보고 knowledge_search → web_search 순서로 자동 실행.

---

## 적응형 학습 흐름

```
미식별 프로세스 발견 (discovery에서 type="unknown")
  또는 사용자가 "Kafka 상태 확인해줘" (빌트인 커넥터 없음)
  ↓
1. LLM에 프로세스 정보 전달:
   "PID 23456, java, kafka.Kafka, 포트 9092"
   → LLM이 자체 지식으로 "Kafka 브로커"로 추정
  ↓
2. 웹 검색으로 관리 방법 학습:
   web_search("Kafka CLI admin commands")
   web_fetch("https://kafka.apache.org/documentation/")
  ↓
3. 서버에서 관련 도구 탐색:
   shell_exec("find / -name 'kafka-topics.sh' 2>/dev/null")
   shell_exec("which kafka-topics 2>/dev/null")
   shell_exec("ls /opt/kafka/bin/ 2>/dev/null")
  ↓
4. 학습한 내용으로 명령 생성 + 실행:
   shell_exec("/opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list")
  ↓
5. 성공 → 커넥터 등록 제안:
   "Kafka 커넥터를 등록할까요?"
   → 도구 세트 생성 (kafka.topics, kafka.groups, kafka.describe 등)
   → learned_systems 테이블에 저장
  ↓
6. 다음부터는 빌트인처럼 즉시 사용
```

### learned_systems 테이블
```sql
CREATE TABLE learned_systems (
  id INTEGER PRIMARY KEY,
  service_type TEXT NOT NULL,     -- "kafka", "redis", "nginx" 등
  cli_path TEXT,                  -- "/opt/kafka/bin/"
  config_path TEXT,               -- "/opt/kafka/config/"
  log_path TEXT,                  -- "/opt/kafka/logs/"
  commands TEXT,                  -- JSON: 학습된 명령 목록
  server_name TEXT,               -- 어느 서버에서 학습했는지
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 에러 패턴 학습 (knowledge_base)

Phase 5의 execution_logs에 기록된 실패→성공 체인을 분석하여,
**"이 상황에서 이 에러가 나면 이렇게 해결한다"** 지식을 자동으로 축적한다.

### knowledge_base 테이블
```sql
CREATE TABLE knowledge_base (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  category TEXT NOT NULL,           -- 'error_pattern' | 'system_knowledge' | 'procedure' | 'tip'
  title TEXT NOT NULL,              -- 지식 제목 (LLM이 자동 생성)
  situation TEXT NOT NULL,          -- 상황 설명 (어떤 맥락에서 발생했는지)
  resolution TEXT NOT NULL,         -- 해결 방법
  tool_name TEXT,                   -- 관련 도구명
  error_pattern TEXT,               -- 에러 메시지 패턴 (키워드 매칭용)
  success_command TEXT,             -- 성공한 명령/쿼리
  embedding BLOB,                   -- 벡터 임베딩 (Phase 9에서 활성화)
  source_execution_id INTEGER REFERENCES execution_logs(id),
  confidence REAL DEFAULT 1.0,      -- 신뢰도 (0.0~1.0)
  use_count INTEGER DEFAULT 0,      -- 이 지식이 참조된 횟수
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_used_at DATETIME
);

CREATE INDEX idx_kb_category ON knowledge_base(category);
CREATE INDEX idx_kb_tool ON knowledge_base(tool_name);
CREATE INDEX idx_kb_error ON knowledge_base(error_pattern);
```

### 자동 학습 흐름

```
에이전트 루프에서 도구 실행 결과 감시
  ↓
execution_logs에서 retry_of 체인 감지:
  실행 #1 (실패) → 실행 #2 (성공, retry_of=#1)  
  ↓
LLM에 실패→성공 쌍을 전달하여 지식 생성 요청:
  "실패 명령: {X}, 에러: {Y}, 성공 명령: {Z}
   이 패턴을 정리해줘"
  ↓
LLM이 구조화된 지식 생성:
  {
    category: "error_pattern",
    title: "Oracle 테이블 접근 시 ORA-00942 에러",
    situation: "sysdba가 아닌 일반 계정으로 DBA 뷰 조회 시도",
    resolution: "DBA_ 뷰 대신 USER_ 뷰를 사용하거나, sysdba 권한으로 접속",
    error_pattern: "ORA-00942",
    success_command: "SELECT ... FROM user_tablespaces"
  }
  ↓
knowledge_base에 저장 (임베딩은 Phase 9에서 추가)
  ↓
사용자에게 알림:
  "💡 새로운 지식을 학습했습니다: ORA-00942 에러 시 DBA_ 뷰 대신 USER_ 뷰 사용"
```

### 에러 패턴 참조 (도구 실행 전)

Phase 9의 로컬 벡터 검색이 활성화되기 전에도,
**키워드 기반 매칭**으로 과거 에러 패턴을 참조할 수 있다:

```
도구 실행 요청
  ↓
knowledge_base에서 error_pattern 키워드 매칭:
  "이전에 이 도구(tool_name)에서 비슷한 에러가 있었습니다:
   상황: {situation}
   해결: {resolution}"
  ↓
LLM에 이 컨텍스트를 함께 전달
  ↓
LLM이 과거 학습을 참고하여 처음부터 올바른 명령 생성
  ↓
성공 → use_count 증가, last_used_at 갱신
```

### 수동 지식 등록

사용자가 직접 지식을 등록할 수도 있다:
```
> 기억해둬: Oracle PROD에서 통계 수집할 때 DBMS_STATS 대신 수동 analyze 쓰지 마

 ✓ 지식 저장:
 카테고리: tip
 내용: "Oracle PROD 통계 수집 시 DBMS_STATS 사용 필수, 수동 ANALYZE 금지"
```

### /knowledge 슬래시 명령
```
/knowledge           → 저장된 지식 목록 (최근순)
/knowledge search X  → 키워드 X로 검색
/knowledge delete N  → N번 지식 삭제
```

---

## 사용자 정의 도구 생성

### 흐름
```
사용자: "아카이브 로그 30일 이상된거 정리하는 도구 만들어줘"
  ↓
LLM이 스크립트 생성:
  #!/bin/bash
  find /archive_log -name "*.arc" -mtime +30 -print  # 먼저 목록 출력
  read -p "삭제할까요? (y/n) " confirm
  [ "$confirm" = "y" ] && find /archive_log -name "*.arc" -mtime +30 -delete
  ↓
사용자에게 스크립트 보여주고 확인
  ↓
LLM이 위험도 자동 판단: "파일 삭제 → medium"
  ↓
사용자 확인 → ~/.infractl/tools/cleanup_archive.sh 저장
  ↓
user_tools 테이블에 등록:
  name="cleanup_archive", description="30일 이상 아카이브 로그 정리", 
  script_path="~/.infractl/tools/cleanup_archive.sh", risk_level="medium"
  ↓
도구 레지스트리에 동적 등록
  ↓
이후 "아카이브 로그 정리해줘" → cleanup_archive 도구 자동 호출
```

### user_tools 테이블
```sql
CREATE TABLE user_tools (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  description TEXT,
  script_path TEXT,
  risk_level TEXT DEFAULT 'none',
  parameters TEXT,            -- JSON: 파라미터 정의
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 사용자 도구 실행 방식
- 생성된 스크립트를 shell_exec로 실행
- target 파라미터 지원 (로컬 또는 SSH)
- SSH 실행 시: 스크립트를 /tmp에 전송 후 실행, 완료 후 삭제

---

## 커스텀 커넥터 (REST API)

```
사용자: "사내 API 서버 연결하고 싶어"
  ↓
LLM 질문:
  ? 베이스 URL: https://internal.company.com
  ? 엔드포인트:
    - /api/health (GET) → 상태 확인
    - /api/deploy (GET) → 배포 목록
  ? 인증: bearer token
  ? 토큰: ****
  ↓
Generic HTTP 커넥터 생성:
  - internal_api.health → curl -H "Authorization: Bearer ..." URL/api/health
  - internal_api.deploys → curl -H "Authorization: Bearer ..." URL/api/deploy
```

---

## INFRACTL.md

`~/.infractl/INFRACTL.md` 파일이 있으면 시스템 프롬프트에 자동 포함.
에이전트 시작 시 + 주기적으로(변경 감지) 리로드.

---

## 검증 시나리오

1. "이 서버에 9092 포트 프로세스 뭔지 확인" → Kafka 감지 → 웹 검색 → CLI 찾기 → 상태 조회
2. "Kafka 커넥터 등록할까요?" → y → 다음부터 "Kafka 토픽 목록" 즉시 실행
3. "로그 정리 도구 만들어줘" → 스크립트 생성 → 확인 → 저장 → 이후 자동 호출
4. 인터넷 안 되는 환경 → web_search 자동 비활성 → LLM 자체 지식으로 대응

---

## 검증 시나리오 (에러 패턴 학습)

5. "db-server Oracle 테이블스페이스" → ORA-00942 실패 → LLM 수정 → 성공 → 자동 학습
6. 다음 세션에서 같은 요청 → 학습된 지식 참조 → 처음부터 올바른 쿼리 → 한번에 성공
7. "기억해둬: Oracle 재시작 전에 리스너 먼저 확인" → knowledge_base에 tip 저장
8. `/knowledge` → 학습된 지식 목록 확인
9. `/knowledge search ORA` → ORA 관련 학습 내용 검색

---

## 완료 기준
- [ ] web_search, web_fetch 동작 (HTML→마크다운 변환)
- [ ] web_cache URL 캐싱 (LRU, 15분 TTL)
- [ ] 폐쇄망 자동 감지 → IsEnabled()=false → 시스템 프롬프트에서 자동 제거
- [ ] knowledge_search 도구 (FTS5 기반 키워드 검색)
- [ ] 도구 실패 시 knowledge_search → web_search 순서 힌트 (buildErrorHints 연동)
- [ ] 미식별 시스템 → 웹 검색 → CLI 탐색 → 자동 학습 → 커넥터 등록
- [ ] 사용자 도구 대화로 생성 + 위험도 자동 판단
- [ ] 학습 결과 SQLite 저장 + 재시작 시 복원
- [ ] 실패→성공 체인 자동 감지 → knowledge_base 저장
- [ ] 에러 패턴 FTS5 매칭으로 과거 지식 참조
- [ ] 수동 지식 등록 ("기억해둬" 패턴)
- [ ] /knowledge 관리 명령
