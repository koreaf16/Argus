# Phase 10 설계도 — 모니터링 및 자동 모드

## 목표
daemon에서 SSH 주기적 상태 체크. 원격 cron/백그라운드 등록. 로그 감시. 알림. 자동 모드 규칙 기반 자율 실행.

## 선행: Phase 9 완료

---

## 추가/변경 파일

```
internal/
├── monitor/
│   ├── monitor.go          # 모니터링 매니저 (daemon 루프)
│   ├── healthcheck.go      # 서비스별 헬스체크 로직
│   ├── logwatch.go         # 로그 감시 (키워드 매칭)
│   ├── remote_cron.go      # SSH로 원격 cron/스크립트 등록
│   └── alert.go            # 알림 발송 (webhook, 이메일, WebSocket)
└── storage/sqlite.go       # 테이블 추가: alert_rules, monitor_results
```

---

## 모니터링 매니저

### daemon 내부 루프
```
daemon 시작
  ↓
MonitorManager 초기화
  ↓
등록된 서버별로 goroutine 실행:
  - 주기적 SSH 헬스체크 (30초~5분, 설정 가능)
  - 원격 cron 결과 수집
  - 로그 감시 결과 수집
  ↓
이상 감지 시 → 알림 트리거 / 자동 모드 트리거
```

---

## 헬스체크

### 서비스별 체크 방법

| 서비스 | 헬스체크 명령 | Hang 판단 |
|--------|-------------|-----------|
| Oracle | `echo "SELECT 1 FROM DUAL;" \| sqlplus -S ...` | 응답 시간 > 10초 |
| MySQL | `mysqladmin ping` | 응답 시간 > 5초 |
| Tomcat | `curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/` | HTTP 응답 없음/500 |
| WebLogic | `curl -s http://localhost:7001/console` | 응답 없음 |
| 임의 프로세스 | `kill -0 PID` (프로세스 존재 확인) | PID 없음 |
| OS | `uptime`, `df`, `free` | load average 급등, 디스크 95%+ |

### Hang 감지
- 헬스체크 SSH 명령 자체가 타임아웃 → "SSH 접근 불가" 알림
- 헬스체크 쿼리 타임아웃 → "서비스 hang 의심" 알림
- CPU 100% 고정 (연속 3회) → "프로세스 hang 의심" 알림

---

## 로그 감시

### 방식 1: daemon SSH 폴링
```
주기적 (1~5분)으로:
  SSH → 서버: tail -n 100 {log_path} | grep -E "ORA-|ERROR|FATAL"
  ↓
키워드 매칭 시 → 알림
```

### 방식 2: 원격 cron 등록
```
사용자: "db-server Oracle alert log 감시해줘"
  ↓
LLM이 감시 스크립트 생성:
  #!/bin/bash
  tail -F /path/alert_ORCL.log | while read line; do
    echo "$line" | grep -qE "ORA-|TNS-" && echo "$(date) $line" >> /tmp/infractl_alerts.log
  done
  ↓
SSH로 스크립트 전송 + nohup 실행 또는 crontab 등록
  ↓
daemon이 주기적으로 /tmp/infractl_alerts.log 읽어서 수집
```

### 시스템별 감시 키워드 (빌트인)

| 시스템 | 키워드 패턴 |
|--------|-----------|
| Oracle | `ORA-`, `TNS-`, `checkpoint not complete`, `cannot allocate`, `enq:` |
| WebLogic | `BEA-`, `STUCK`, `OutOfMemoryError`, `JDBC Connection Pool`, `DeadLock` |
| Tomcat | `SEVERE`, `OutOfMemoryError`, `StackOverflowError` |
| MySQL | `ERROR`, `Deadlock`, `Too many connections`, `Slave SQL` |
| OS | `kernel:`, `Out of memory`, `I/O error`, `disk full`, `segfault` |

### 로그 경로 자동 추론
커넥터 활성화 시 서비스별 기본 로그 경로 탐색:
- Oracle: `find $ORACLE_BASE/diag -name "alert_*.log"`
- Tomcat: `find / -name "catalina.out" 2>/dev/null | head -3`
- 찾은 경로를 사용자에게 확인

---

## 3단계 로그 분석 파이프라인

```
1차: 키워드 필터 (실시간, 가벼움)
  grep -E "ORA-|ERROR|FATAL" → 매칭 시 알림
  ↓
2차: LLM 분석 (이상 징후 시)
  매칭된 로그 주변 ±50줄을 LLM에 전달
  "이 로그에서 문제가 있는지 분석해주세요"
  → LLM이 원인 분석 + 권장 조치
  ↓
3차: RAG 연동 (Phase 7 구현 후)
  "이 로그 패턴과 유사한 과거 장애?" → RAG 소스 검색
```

---

## 알림

### 알림 규칙 등록 (대화)
```
"Oracle 테이블스페이스 90% 넘으면 알려줘"
  ↓
alert_rules 테이블에 저장:
  server="db-server", condition="tablespace > 90%", action="alert"
```

### 발송 방식

| 방식 | 설정 | 동작 |
|------|------|------|
| Web UI 푸시 | 자동 | WebSocket으로 알림 메시지 전송, 사이드패널에 표시 |
| Webhook | config.yaml alerts.webhook | HTTP POST {server, message, severity, timestamp} |
| 이메일 | config.yaml alerts.email + SMTP 설정 | 제목: [infractl] 서버명 - 알림 내용 |
| CLI | 자동 | CLI 접속 중이면 대화에 알림 삽입 |

### 알림 쿨다운
같은 조건으로 반복 알림 방지. alert_rules.cooldown 필드 (예: "30m", "1h").

---

## 자동 모드

### 개념
daemon에서 조건 매칭 시 사람 확인 없이 자동 실행.
위험도 high 작업은 자동 모드에서도 실행 불가 (안전 장치).

### 설정
```yaml
auto_mode:
  enabled: true
  rules:
    - trigger: "disk_usage > 90%"
      action: "cleanup_archive_logs"
      condition: "30일 이상만"
      max_delete: "10GB"
      cooldown: "1h"
      
    - trigger: "tomcat_health_fail > 3"
      action: "tomcat_restart"
      cooldown: "30m"
      
    - trigger: "oracle_temp > 95%"
      action: "temp_tablespace_cleanup"
      cooldown: "2h"
```

### 동작
```
모니터링 루프에서 조건 매칭
  ↓
cooldown 체크 (마지막 실행 후 충분한 시간 경과?)
  ↓
해당 action을 에이전트 루프에 프로그래밍적으로 전달
  ↓
실행 결과 로그 + 알림
```

### 안전 장치
- auto_mode.enabled = false로 즉시 비활성화 가능
- 위험도 high 작업은 절대 자동 실행 불가
- 모든 자동 실행은 로그 기록 + 알림
- cooldown으로 연쇄 실행 방지

---

## SQLite 테이블

```sql
CREATE TABLE alert_rules (
  id INTEGER PRIMARY KEY,
  server TEXT,
  condition TEXT,
  action TEXT DEFAULT 'alert',
  cooldown TEXT DEFAULT '30m',
  last_fired DATETIME,
  enabled INTEGER DEFAULT 1
);

CREATE TABLE monitor_results (
  id INTEGER PRIMARY KEY,
  server TEXT,
  check_type TEXT,        -- 'health', 'log', 'metric'
  result TEXT,
  status TEXT,            -- 'ok', 'warning', 'critical'
  timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 검증 시나리오

1. daemon 시작 → 등록된 서버 헬스체크 시작 → 정상 로그
2. "db-server Oracle alert log 감시해줘" → 원격 스크립트 등록
3. Oracle alert log에 ORA-에러 발생 → 키워드 매칭 → 알림
4. "테이블스페이스 90% 넘으면 알려줘" → 규칙 등록 → 초과 시 알림
5. Webhook 연동 → Slack 채널에 알림 수신
6. SSH 타임아웃 → "서버 접근 불가" 알림
7. 디스크 90% 초과 → 자동 모드 → 아카이브 정리 → 알림 (자동 모드 동작 검증)

---

## 완료 기준
- [ ] daemon에서 주기적 SSH 헬스체크
- [ ] 원격 cron/스크립트 등록 (대화)
- [ ] 로그 키워드 감시 + LLM 분석
- [ ] Hang 감지 (타임아웃)
- [ ] 알림: Web UI / Webhook / 이메일
- [ ] 알림 규칙 대화로 등록 + 쿨다운
- [ ] 자동 모드 규칙 기반 자율 실행 (안전 장치 포함)
