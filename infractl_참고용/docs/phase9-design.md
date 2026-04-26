# Phase 9 설계도 — Daemon + Web UI

## 목표
`infractl daemon` → 백그라운드 상주 + 웹 브라우저 챗봇 UI + REST API + 스케줄 데몬 실행.

## 선행: Phase 5 완료, Phase 8 스케줄 등록 구현 완료
## **Phase 9 완료 = 꽤 완성된 제품**

---

## 추가/변경 파일

```
internal/
├── web/
│   ├── server.go           # HTTP 서버 + 라우팅
│   ├── auth.go             # ID/Password 인증, JWT 세션
│   ├── websocket.go        # WebSocket 채팅 핸들러
│   ├── api.go              # REST API 핸들러
│   └── handler.go          # WebSocket용 EventHandler 구현
├── web/frontend/           # React SPA 소스 (빌드 후 embed)
│   ├── index.html
│   ├── app.js              # 채팅 + 사이드패널
│   └── styles.css
├── web/embed.go            # go:embed 지시자로 프론트엔드 포함
└── cmd/infractl/main.go    # daemon 명령 구현
```

---

## 아키텍처

```
infractl daemon
  ├── HTTP 서버 (기본 :9090)
  │   ├── GET  /                → SPA 프론트엔드 제공
  │   ├── POST /api/login       → 인증
  │   ├── GET  /api/status      → 전체 상태 요약
  │   ├── GET  /api/servers     → 서버 목록
  │   ├── GET  /api/tools       → 활성 도구 목록
  │   ├── GET  /api/sessions    → 세션 목록
  │   ├── GET  /api/cost        → 비용 요약
  │   ├── WS   /ws              → WebSocket 채팅
  │   └── static/*             → 프론트엔드 정적 파일
  ├── 에이전트 엔진 (CLI와 동일)
  ├── 스케줄러 루프 (Phase 8에서 등록된 스케줄 실행)
  └── (Phase 10) 모니터링 루프
```

---

## 인증

### 최초 설정
- `infractl daemon` 첫 실행 시 admin 비밀번호 설정
- bcrypt 해시 → SQLite web_auth 테이블 저장

### 로그인 흐름
```
POST /api/login
  Body: { "username": "admin", "password": "****" }
  Response: { "token": "JWT..." }  (24시간 유효)
```

### 세션 관리
- JWT 토큰 방식
- Authorization: Bearer {token} 헤더
- 24시간 타임아웃 (설계 문서 28장 확정 사항)
- WebSocket 연결 시에도 토큰 검증

---

## WebSocket 채팅

### 프로토콜
```
Client → Server:
  { "type": "message", "content": "Oracle 테이블스페이스 보여줘" }
  { "type": "slash", "command": "/servers" }

Server → Client:
  { "type": "thinking" }
  { "type": "token", "content": "테" }
  { "type": "tool_start", "name": "shell_exec", "target": "db-server", "command": "sqlplus..." }
  { "type": "tool_end", "name": "shell_exec", "result": "...", "success": true, "duration": "2.3s" }
  { "type": "response_done", "content": "전체 응답 텍스트" }
  { "type": "error", "message": "에러 메시지" }
  { "type": "confirm", "id": "abc123", "message": "계속할까요?" }
  { "type": "input", "id": "abc123", "prompt": "비밀번호:" }
  { "type": "status_update", "servers": [...], "tools": [...] }

Client → Server (확인 응답):
  { "type": "confirm_response", "id": "abc123", "value": true }
  { "type": "input_response", "id": "abc123", "value": "****" }
```

### WebSocket EventHandler
agent.EventHandler를 WebSocket용으로 구현:
- OnToken → `{"type":"token"}` 전송
- OnToolStart → `{"type":"tool_start"}` 전송
- OnConfirm → `{"type":"confirm"}` 전송 + 응답 대기 (채널)

---

## 프론트엔드 (React SPA)

### 레이아웃
```
┌──────────────────┬──────────────────┐
│                  │  사이드패널       │
│  채팅 영역        │  ├ 서버 목록     │
│  (CLI 출력 스타일) │  ├ 활성 작업     │
│                  │  ├ 최근 알림     │
│                  │  └ 비용 요약     │
├──────────────────┴──────────────────┤
│ > 입력바                            │
└─────────────────────────────────────┘
```

### 채팅 영역
- CLI TUI와 동일한 출력 스타일을 HTML/CSS로 렌더링
- 명령 실행 박스: 접기/펼치기 가능
- 마크다운 렌더링 (marked.js 또는 유사)
- 코드 블록 구문 강조
- 테이블 렌더링

### 사이드패널
- 서버 목록: 이름, 상태 (connected/disconnected), 서비스 수
- 활성 작업: 실행 중인 백그라운드 작업 (Phase 8)
- 최근 알림: 모니터링 알림 (Phase 10)
- 비용 요약: 이번 달 토큰/비용 (Phase 8)

### go:embed로 프론트엔드 임베딩
```go
//go:embed frontend/*
var frontendFS embed.FS
```
빌드 시 프론트엔드 파일이 바이너리에 포함 → 추가 파일 배포 불필요

---

## REST API

| 엔드포인트 | 메서드 | 설명 |
|-----------|--------|------|
| /api/login | POST | 로그인 → JWT 토큰 |
| /api/status | GET | 서버 상태, 도구, 알림 요약 |
| /api/servers | GET | 등록된 서버 목록 |
| /api/tools | GET | 활성 도구 목록 |
| /api/sessions | GET | 대화 세션 목록 |
| /api/cost | GET | 비용/사용량 요약 |
| /api/chat | POST | 비스트리밍 메시지 전송 (외부 연동용) |

모든 API는 JWT 인증 필요 (로그인 제외).
향후 외부 오케스트레이터가 REST API로 infractl에 명령 전달 가능.

---

## Daemon 시작 흐름

```
infractl daemon [--port 9090] [--bind 0.0.0.0]
  ↓
config.yaml 로드
  ↓
SQLite 열기
  ↓
web_auth에 계정 없으면 → 터미널에서 admin 비밀번호 설정
  ↓
에이전트 엔진 초기화 (CLI와 동일)
  ↓
저장된 서버/커넥터 자동 로딩
  ↓
스케줄러 시작 (schedules 테이블 로드 및 cron 데몬 시작)
  ↓
HTTP 서버 시작 → "Web UI: http://0.0.0.0:9090" 출력
  ↓
시그널 대기 (SIGTERM → graceful shutdown)
```

---

## 검증 시나리오

1. `infractl daemon` → admin 비밀번호 설정 → 서버 시작
2. 브라우저에서 접속 → 로그인 → 채팅 UI
3. 채팅으로 "db-server 디스크 확인" → SSH 실행 → 실행 박스 + 결과
4. 사이드패널에 서버 목록 표시
5. REST API `GET /api/status` → JSON 응답
6. CLI와 WebSocket 동시 사용 (별개 세션)
7. Phase 8에서 등록된 스케줄이 지정된 시간에 자동으로 실행되는지 백그라운드 확인

---

## 완료 기준
- [ ] `infractl daemon` 백그라운드 실행
- [ ] 브라우저 챗봇 UI (CLI와 동일한 기능)
- [ ] WebSocket 실시간 스트리밍
- [ ] ID/Password + JWT 인증
- [ ] 사이드패널 (서버, 도구)
- [ ] REST API 기본 엔드포인트
- [ ] 프론트엔드 바이너리 임베딩
- [ ] 스케줄 백그라운드 자동 실행 (cron 데몬 연동)
