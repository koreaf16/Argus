# Phase 2 설계도 — SSH + 멀티서버

## 목표
원격 서버에 SSH로 명령 실행. 서버명 없으면 로컬, 있으면 SSH.

## 선행: Phase 1 완료

---

## 추가/변경 파일

```
internal/
├── connector/ssh/           # SSH 클라이언트, 설정, Executor 구현
├── store/sqlite.go          # SQLite 저장소 (서버 정보, 접속 정보 암호화)
├── crypto/crypto.go         # AES-256-GCM 암호화 및 머신 고유 키 파생
├── tools/server_add.go      # SSH 서버 등록 도구
└── cli/commands.go          # /servers 슬래시 명령 추가
```

---

## SSHExecutor 설계

### 접속 방식
- `golang.org/x/crypto/ssh` 라이브러리
- SSH 키 인증 + 비밀번호 인증 지원
- 커넥션 풀: 서버당 1개 SSH 연결 유지, 재사용
- keep-alive 패킷으로 연결 유지
- 연결 끊어지면 자동 재연결

### Executor 라우팅
- 에이전트가 도구 실행 시 `target` 파라미터 확인
- target 없음 / "localhost" / "local" → LocalExecutor
- target이 서버명 → ExecutorManager에서 SSHExecutor 조회
- 미등록 서버명 → 에러 메시지 + 등록 안내

### ExecutorManager
```
ExecutorManager:
  local     LocalExecutor
  remotes   map[서버명]SSHExecutor

  Get(target) → Executor
  Register(name, SSHExecutor)
  Remove(name)
  ListRemotes() → []ServerInfo
```

---

## SQLite 저장소 설계

경로: `~/.infractl/infractl.db`

### 의존성
```
modernc.org/sqlite   # 순수 Go SQLite (CGO 불필요)
```

### servers 테이블
```sql
CREATE TABLE servers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT UNIQUE NOT NULL,
  host TEXT NOT NULL,
  port INTEGER DEFAULT 22,
  user TEXT NOT NULL,
  auth_type TEXT NOT NULL,    -- 'key' | 'password'
  credential TEXT,            -- SSH 키 경로 또는 암호화된 비밀번호
  os TEXT DEFAULT '',         -- 스캔된 운영체제 정보
  env_profile TEXT DEFAULT '',-- 자동 스캔된 주요 환경 정보
  created_at DATETIME NOT NULL
);
```

### 암호화
- 비밀번호: AES-256-GCM 암호화 → Base64 인코딩 → credential 컬럼에 저장
- 암호화 키: SHA256(hostname + machineID + salt) 방식으로 생성 (OS별 Machine ID 활용, 파일에 저장하지 않고 런타임 파생)
- SSH 키 경로: 평문 저장 (경로만, 키 내용 아님)

---

## 서버 등록 흐름 (LLM 도구 기반)

```
사용자: "192.168.1.50 서버 추가해줘"
  ↓
LLM 판단: 서버 등록 요청
  ↓
LLM이 정보가 부족할 경우 사용자에게 질문하여 정보 수집 (이름, IP, 사용자, 인증방식, 키/비밀번호)
  ↓
LLM이 `server_add` 도구 호출
  ↓
SSH 접속 테스트 (echo ok)
  ↓
성공 → SQLite 저장 + ExecutorManager 등록
실패 → 에러 메시지 반환
```

### 전용 도구(server_add) 활용
- 초기 설계와 달리 명시적인 LLM 도구(`server_add`)를 생성하여 서버 등록 과정을 일원화했습니다.
- LLM이 필요한 매개변수를 수집한 뒤 도구를 실행하여 접속 테스트와 저장을 원자적으로 수행합니다.

---

## 시스템 프롬프트 변경

Phase 1의 시스템 프롬프트에 추가:
```
## Available Servers
- localhost (local)
- db-server (admin@192.168.1.50:22) [ssh-key]
- was-server (root@192.168.1.51:22) [password]

When the user mentions a server name, use the "target" parameter in tool calls.
When no server is specified, execute locally.
```

---

## 슬래시 명령 추가

| 명령 | 동작 |
|------|------|
| /servers | 등록된 서버 목록 출력 |
| /servers remove <이름> | 서버 삭제 |

*(참고: 서버 추가는 CLI 명령어가 아닌 LLM과의 자연어 대화 및 `server_add` 도구를 통해 수행됩니다)*

---

## 앱 시작 시 서버 자동 로딩

1. SQLite에서 서버 목록 로드
2. 각 서버에 대해 SSHExecutor 생성
3. ExecutorManager에 등록
4. (접속 테스트는 하지 않음 — 첫 명령 실행 시 lazy connect)

---

## 검증 시나리오

1. "서버 추가할게 192.168.1.50" → 대화형 정보 수집 → `server_add` 도구 실행 → SSH 테스트 → 성공
2. "/servers" → 등록된 서버 목록 출력
3. "db-server 디스크 확인해줘" → SSH로 df -h → 결과
4. "디스크 확인해줘" → 로컬 df -h (타겟 미지정)
5. "전체 서버 메모리 보여줘" → localhost + db-server 병렬 조회 → 비교 테이블
6. infractl 재시작 → 서버 자동 로딩 → "db-server 상태" 바로 동작
7. SSH 접속 실패 시 → 명확한 에러 메시지 (포트, 키, 비밀번호 문제 등)

---

## 완료 기준
- [x] SSH 키/비밀번호로 원격 명령 실행
- [x] 서버 대화형 등록/삭제 (server_add 도구 및 /servers remove 명령)
- [x] SQLite 암호화 저장 + 재시작 시 자동 로딩 (MachineID 기반 키 파생)
- [x] target 파라미터로 로컬/SSH 자동 전환
- [x] 여러 서버 병렬 조회
- [x] /servers 명령
