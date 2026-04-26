# Phase 8 설계도 — 고급 기능

## 목표
서브에이전트, 체크포인트, Hooks, 스케줄 대화 등록, 비용 추적, 백그라운드 작업.
**Phase 8 완료 = 꽤 완성된 제품.** (스케줄 데몬 실행과 자동 모드는 Phase 9, 10에서 처리)

## 선행: Phase 6, 7 완료

---

## 추가/변경 파일

```
internal/
├── subagent/subagent.go        # 서브에이전트 시스템
├── checkpoint/checkpoint.go    # 체크포인트/롤백
├── hooks/hooks.go              # 라이프사이클 Hooks
├── schedule/schedule.go        # 스케줄 실행 (cron 등록 부분)
├── cost/tracker.go             # 비용/사용량 추적
├── agent/background.go         # 백그라운드 작업 관리
└── storage/sqlite.go           # 테이블: checkpoints, hooks, schedules, usage_logs
```

---

## 10.1 서브에이전트

### 개념
메인 에이전트가 복잡한 분석 시 전문 서브에이전트에 위임.
Claude Code가 Sonnet(메인) + Haiku(서브) 패턴을 쓰듯, infractl도 동일.

### 서브에이전트 종류

| 서브에이전트 | 전문 분야 | 시스템 프롬프트 특화 |
|---|---|---|
| DB 전문가 | Oracle/MySQL 내부 분석 | AWR, 실행계획, 락 체인, 테이블스페이스, 아카이브 |
| OS 전문가 | 커널/시스템 리소스 | CPU, 메모리, I/O, 네트워크, 프로세스, 커널 파라미터 |
| WAS 전문가 | Tomcat/WebLogic | 스레드 덤프, 커넥션 풀, GC, 배포, 세션 |
| 보안 분석가 | 보안 점검 | 권한, 패치, 취약점, 접근 로그 |

### 동작 방식
```
사용자: "시스템 느려졌는데 종합 분석해줘"
  ↓
메인 에이전트 판단: 복합 문제 → 서브에이전트 투입
  ↓
병렬 실행 (goroutine):
  서브에이전트(DB)  → 별도 LLM 호출 (경량 모델) → Oracle 분석 결과
  서브에이전트(OS)  → 별도 LLM 호출 → CPU/메모리/IO 분석 결과  
  서브에이전트(WAS) → 별도 LLM 호출 → Tomcat 분석 결과
  ↓
메인 에이전트가 결과 종합:
  "원인: Oracle Full Table Scan → I/O 점유 → Tomcat 커넥션 대기"
```

### 구현
- 서브에이전트 = 별도 시스템 프롬프트 + 제한된 도구 + 별도 LLM 호출
- 모델: config.yaml에서 sub_agent_model 설정 (기본: 35B-A3B 또는 더 작은 모델)
- 도구: 서브에이전트 분야에 맞는 도구만 제공 (DB 전문가에게 Oracle 커넥터 도구만)
- 결과: 텍스트로 메인 에이전트에 반환
- 비용: 서브에이전트 호출도 usage_logs에 기록

---

## 10.2 체크포인트 / 롤백

### 자동 체크포인트 생성 시점
- 설정 변경 직전 (ALTER SYSTEM, 커널 파라미터, config 파일 수정)
- 서비스 상태 변경 직전 (재시작, 중지)
- 사용자 도구 실행 직전 (위험도 low 이상)

### 체크포인트 내용
```
checkpoint:
  id: 7
  server: db-server
  description: "undo_retention 변경 (900 → 7200)"
  snapshot:
    parameter: "undo_retention"
    old_value: "900"
    new_value: "7200"
    command_used: "ALTER SYSTEM SET undo_retention=7200 SCOPE=BOTH"
    rollback_command: "ALTER SYSTEM SET undo_retention=900 SCOPE=BOTH"
  created_at: "2026-04-07 14:30:00"
```

### 롤백
```
"방금 한 거 되돌려줘"
  → 최근 체크포인트 조회 → rollback_command 실행
  → 성공 확인

"체크포인트 #5로 돌아가줘"
  → 특정 체크포인트 롤백

"/checkpoints"
  → 목록 조회
```

### SQLite
```sql
CREATE TABLE checkpoints (
  id INTEGER PRIMARY KEY,
  server TEXT,
  description TEXT,
  snapshot TEXT,          -- JSON: old_value, new_value, rollback_command 등
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 10.3 Hooks (라이프사이클 이벤트)

### 이벤트 종류

| 이벤트 | 발생 시점 | 예시 용도 |
|--------|-----------|-----------|
| before_execute | 도구 실행 직전 | 자동 백업, 로그 기록 |
| after_execute | 도구 실행 직후 | 헬스체크, 결과 검증 |
| on_error | 에러 발생 시 | 자동 로그 수집, 알림 |
| on_connect | 서비스 접속 시 | 자동 상태 체크 |
| on_alert | 알림 발생 시 | 자동 진단 스크립트 |
| on_session_start | 세션 시작 시 | 전체 상태 요약 |

### 후크 등록 (대화)
```
"Oracle 세션 kill 하면 항상 로그 남겨줘"
  ↓
hooks 테이블에 저장:
  event: "after_execute"
  condition: "tool_name LIKE 'oracle_%.sessions' AND action='kill'"
  script: "echo $(date) killed SID $SID >> /var/log/infractl_audit.log"
```

### 후크 실행 로직
```
도구 실행 전:
  hooks 테이블에서 event='before_execute' 매칭 → 스크립트 실행

도구 실행:
  실제 도구 실행

도구 실행 후:
  hooks 테이블에서 event='after_execute' 매칭 → 스크립트 실행

에러 시:
  hooks 테이블에서 event='on_error' 매칭 → 스크립트 실행
```

### 저장
```sql
CREATE TABLE hooks (
  id INTEGER PRIMARY KEY,
  event TEXT NOT NULL,
  condition TEXT,          -- 매칭 조건 (tool_name, server, action 등)
  script_path TEXT,        -- ~/.infractl/hooks/ 아래 스크립트
  enabled INTEGER DEFAULT 1
);
```

---

## 10.4 스케줄 등록 (실행은 Phase 9)

### 등록 (대화)
```
"매일 아침 9시에 전체 헬스체크 리포트"
  ↓
schedules 테이블:
  name: "daily_health"
  cron_expr: "0 9 * * *"
  prompt: "전체 서버 상태를 확인하고 리포트를 작성해줘. 이상이 있으면 상세히."
```

### 구현
- `robfig/cron/v3` 라이브러리로 Go 내부 cron 스케줄러 로직 구현 (실제 백그라운드 실행은 Phase 9의 daemon에서 담당)
- 스케줄 대화 등록 및 관리 기능

### 관리
```
/schedules          → 목록
/schedules enable 1 → 활성화
/schedules disable 1 → 비활성화
대화: "1번 스케줄 삭제해줘"
```

### SQLite
```sql
CREATE TABLE schedules (
  id INTEGER PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  cron_expr TEXT NOT NULL,
  prompt TEXT NOT NULL,
  last_run DATETIME,
  last_result TEXT,
  enabled INTEGER DEFAULT 1
);
```

---

## 10.5 비용/사용량 추적

### 기록 대상
- 매 LLM API 호출: 모델, 입력 토큰, 출력 토큰, 예상 비용
- 서브에이전트 호출 별도 기록
- 스케줄/자동 모드 호출도 기록

### 비용 계산
모델별 단가 테이블 (설정 가능):
```yaml
cost_per_1m_tokens:
  qwen3.5:27b:
    input: 0.10
    output: 0.30
  qwen3.5:35b:
    input: 0.05
    output: 0.15
```

### 조회
```
/cost              → 이번 달 요약
/cost week         → 이번 주
/cost detail       → 일별 상세
```

Web UI 사이드패널에 실시간 누적 표시.

### SQLite
```sql
CREATE TABLE usage_logs (
  id INTEGER PRIMARY KEY,
  model TEXT,
  input_tokens INTEGER,
  output_tokens INTEGER,
  estimated_cost REAL,
  source TEXT,              -- 'user', 'subagent', 'schedule', 'auto_mode'
  timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 10.6 백그라운드 작업

### 개념
장시간 작업을 백그라운드로 돌리면서 다른 명령 처리.

### 대상
- AWR 리포트 생성 (수 분)
- 대량 로그 분석
- 통계 수집
- 백업/복구
- 사용자 도구 중 장시간 실행

### 동작
```
"Oracle AWR 리포트 생성해줘 지난 7일"
  ↓
에이전트: "시간이 걸릴 수 있습니다. 백그라운드로 실행합니다. (작업 #1)"
  ↓
goroutine으로 실행, 에이전트는 다른 입력 대기
  ↓
완료 시: "✓ 작업 #1 완료: AWR 리포트 생성됨"
```

### 관리
```
"작업 목록" → #1 AWR 리포트 (실행 중, 3분 경과) / #2 로그 분석 (완료)
"작업 1 결과" → 결과 표시
"작업 1 중지" → 취소
```

### 구현
- 작업 맵: map[int]*BackgroundJob (ID → goroutine + 결과 채널)
- 완료 시 EventHandler에 알림 전달
- Web UI 사이드패널에 실행 중 작업 표시

---

## 전체 Phase 완료 후 최종 구조

```
~/.infractl/
├── config.yaml
├── infractl.db
├── INFRACTL.md
├── tools/
├── hooks/
├── learned/
└── logs/

infractl
├── infractl           → CLI TUI
├── infractl init      → 초기 설정
├── infractl daemon    → Web UI + 모니터링 + 스케줄 + 자동 모드 (Phase 9, 10)
├── infractl version
└── infractl help
```

---

## 검증 시나리오

1. "시스템 종합 분석" → 서브에이전트 3개 병렬 → 종합 결론
2. "undo_retention 변경" → 체크포인트 생성 → 변경 → "되돌려줘" → 롤백
3. "kill 후에 로그 남겨" → Hook 등록 → kill 실행 → 자동 로그
4. "매일 9시 헬스체크" → 스케줄 등록 → DB 저장 확인 (실행은 Phase 9 검증)
5. "/cost" → 이번 달 토큰 2.8M, $4.27
6. "AWR 리포트" → 백그라운드 실행 → 다른 명령 처리 → 완료 알림

---

## 완료 기준
- [ ] 서브에이전트 병렬 분석
- [ ] 체크포인트 자동 생성 + 롤백
- [ ] Hooks 등록 + 이벤트별 자동 실행
- [ ] 스케줄 대화 등록 (daemon 자동 실행은 Phase 9)
- [ ] 비용/사용량 추적 + /cost
- [ ] 백그라운드 작업 관리
- [ ] **Phase 8 완료 = 꽤 완성된 제품**
