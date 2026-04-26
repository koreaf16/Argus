# Phase 4 설계도 — 디스커버리 + 빌트인 커넥터 + MCP 클라이언트

## 목표
서버 접속 시 서비스 자동 탐지 → 빌트인 패턴 매칭 → 접속 성공 시 전용 도구 세트 활성화.
외부 MCP 서버 연결로 GitHub, Grafana 등 서드파티 도구를 코드 변경 없이 에이전트에 통합.

## 선행: Phase 2 완료

---

## 추가/변경 파일

```
internal/
├── discovery/scanner.go       # 서비스 탐색 엔진
├── discovery/patterns.go      # 빌트인 패턴 정의 (Oracle, MySQL 등)
├── connector/manager.go       # 커넥터 매니저 (상태 추적 포함)
├── connector/oracle.go        # Oracle 커넥터 (도구 세트 생성)
├── connector/mysql.go         # MySQL 커넥터
├── connector/postgresql.go    # PostgreSQL 커넥터
├── connector/tomcat.go        # Tomcat 커넥터
├── connector/weblogic.go      # WebLogic 커넥터
├── connector/generic.go       # 제네릭 커넥터 (학습된 서비스용)
├── mcp/
│   ├── client.go              # MCP 클라이언트 (외부 MCP 서버 연결)
│   └── stdio.go               # stdio 전송 (서브프로세스 통신)
└── storage/sqlite.go          # 테이블 추가: discovered_services, connectors
```

---

## 디스커버리 엔진

### 탐색 3단계
```
1. 프로세스 스캔:  ps -eo pid,user,comm,args
2. 포트 스캔:     ss -tlnp 또는 netstat -tlnp
3. 설정파일 탐색:  test -f /etc/oratab 등
```

### 빌트인 패턴 정의

| 서비스 | 프로세스 정규식 | 기본 포트 | 설정 파일 |
|--------|---------------|----------|-----------|
| Oracle | `ora_pmon_(\w+)` | 1521 | /etc/oratab |
| MySQL | `mysqld` | 3306 | /etc/my.cnf, /etc/mysql/my.cnf |
| PostgreSQL | `postgres:\s+` | 5432 | */pg_hba.conf |
| Tomcat | `java.*catalina\|java.*tomcat` | 8080 | - |
| WebLogic | `java.*weblogic\.Server` | 7001 | - |

### 확신도 계산
- 프로세스 매칭: +0.5
- 포트 매칭: +0.2
- 설정파일 존재: +0.3
- 합계 0.7 이상: 높음 (자동 보고)
- 합계 0.5~0.7: 중간 ("맞나요?" 질문)
- 합계 0.5 미만: 낮음 (보고만)

### 미식별 프로세스
- java, python, node 등 리소스 사용량 높은 프로세스 중 빌트인 패턴에 안 걸린 것
- type="unknown"으로 보고 → 사용자에게 질문 또는 Phase 6에서 적응형 학습

---

## 커넥터 시스템

### Connector 인터페이스
```go
type Connector interface {
    ServiceType() string
    // GenerateTools는 접속 성공 시 도구 세트를 생성한다.
    // 생성된 도구들은 IsReadOnly()/IsEnabled()를 올바르게 구현해야 한다.
    GenerateTools(info ServiceInfo, creds Credentials) []tools.Tool
    // ToolNames는 이 커넥터가 생성한 도구 이름 목록을 반환한다.
    // Registry.Unregister() 호출 시 사용.
    ToolNames() []string
    // Status는 현재 연결 상태를 반환한다.
    Status() ConnectorStatus
}

// ConnectorStatus는 커넥터 연결 상태를 나타낸다.
type ConnectorStatus string

const (
    StatusConnected    ConnectorStatus = "connected"
    StatusConnecting   ConnectorStatus = "connecting"
    StatusError        ConnectorStatus = "error"
    StatusDisconnected ConnectorStatus = "disconnected"
)
```

### 커넥터 상태 관리

시작 시 영구 저장된 커넥터를 로드하면서 상태를 추적한다:

```go
// ConnectorState는 커넥터의 런타임 상태이다.
type ConnectorState struct {
    Name    string
    Status  ConnectorStatus
    Error   string           // 마지막 에러 메시지
    Tools   []string         // 활성화된 도구 이름 목록
}
```

**상태 표시 (앱 시작 시):**
```
✓ Oracle ORCL — connected (6개 도구 활성)
⟳ MySQL 3306  — connecting...
✗ PostgreSQL  — error: connection refused
  Tomcat 8080 — disconnected (접속 정보 없음)
```

**IsEnabled() 연동**: 커넥터 비활성화 시 해당 도구들의 `IsEnabled()`가 `false`를 반환하도록
하거나, `Registry.Unregister()`로 제거한다.

### 커넥터 활성화 흐름
```
사용자: "Oracle 테이블스페이스 보여줘"
  ↓
LLM: oracle 관련 도구가 없음 → 디스커버리 필요
  ↓
디스커버리 실행 (해당 서버)
  ↓
Oracle 발견 (ora_pmon_ORCL)
  ↓
LLM: "접속 계정이 필요합니다" → 사용자에게 질문
  ? sysdba / 계정입력
  ? 계정: mon_user
  ? 비밀번호: ****
  ↓
접속 테스트 (sqlplus 간단 쿼리)
  ↓
성공 → Oracle 커넥터가 도구 세트 생성:
  - oracle_orcl.query
  - oracle_orcl.tablespace
  - oracle_orcl.sessions
  - oracle_orcl.locks
  - oracle_orcl.alert_log
  - oracle_orcl.top_sql
  ↓
도구 레지스트리에 등록
  ↓
접속 정보 저장 질문: 영구 / 세션 / 안 함
  ↓
LLM이 oracle_orcl.tablespace 도구 호출 → 결과
```

### Oracle 커넥터 생성 도구

| 도구명 | 설명 | 내부 동작 |
|--------|------|-----------|
| oracle_{sid}.query | SQL 실행 | echo "SQL" \| sqlplus -S connStr |
| oracle_{sid}.tablespace | 테이블스페이스 현황 | dba_tablespace_usage_metrics |
| oracle_{sid}.sessions | 활성 세션 | v$session |
| oracle_{sid}.locks | 락 정보 | v$lock + v$session |
| oracle_{sid}.alert_log | 알럿 로그 | tail alert_{sid}.log |
| oracle_{sid}.top_sql | 부하 SQL | v$sql ORDER BY elapsed_time |

MySQL, PostgreSQL, Tomcat, WebLogic도 유사한 패턴으로 각각 도구 세트 정의.

---

## SQLite 테이블 추가

```sql
CREATE TABLE discovered_services (
  id INTEGER PRIMARY KEY,
  server_name TEXT NOT NULL,
  service_type TEXT NOT NULL,
  service_name TEXT,
  details TEXT,           -- JSON
  confidence REAL,
  discovered_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE connectors (
  id INTEGER PRIMARY KEY,
  server_name TEXT NOT NULL,
  service_type TEXT NOT NULL,
  service_name TEXT NOT NULL,
  config TEXT,            -- JSON (접속 설정)
  credentials TEXT,       -- 암호화된 접속 정보
  tools TEXT,             -- JSON (생성된 도구 이름 목록)
  save_mode TEXT,         -- 'permanent', 'session', 'none'
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 앱 시작 시 자동 로딩

1. SQLite에서 save_mode='permanent'인 커넥터 로드
2. 각 커넥터의 접속 정보로 도구 세트 재생성
3. 도구 레지스트리에 등록
4. (접속 테스트는 첫 사용 시 lazy)

시작 시 표시:
```
✓ Oracle ORCL — 저장된 접속 정보 (6개 도구 활성)
✓ Tomcat 8080 — 프로세스 레벨 도구
✗ MySQL 3306 — 접속 정보 없음
```

---

---

## MCP 클라이언트 (외부 MCP 서버 도구 가져오기)

InfraCtl은 MCP **클라이언트 전용**이다 — 외부 MCP 서버에 연결하여 도구를 가져와 에이전트 루프에서 사용한다. MCP 서버를 제공하지 않는다.

### 동작 원리

```
config.yaml의 mcp_servers 목록 읽기
  ↓
각 MCP 서버에 연결 (stdio: 서브프로세스 spawn)
  ↓
tools/list 요청으로 도구 목록 조회
  ↓
MCP 도구를 Tool 인터페이스 래퍼로 감싸서 Registry에 등록
  ↓
LLM이 MCP 도구 호출 → MCP 서버에 tools/call 전송 → 결과 반환
```

### 설정 (`~/.infractl/config.yaml`)

```yaml
mcp_servers:
  github:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      GITHUB_TOKEN: "${GITHUB_TOKEN}"
  monitoring:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-prometheus"]
    env:
      PROMETHEUS_URL: "http://monitoring:9090"
```

### 도구 이름 규칙

MCP 도구는 `mcp__{serverName}__{toolName}` 형식으로 등록된다:
- `mcp__github__create_issue`
- `mcp__monitoring__query_metrics`

### MCP 연결 상태

MCP 서버도 동일한 `ConnectorStatus` 체계로 관리된다:

```go
// MCPConnection은 외부 MCP 서버의 런타임 상태이다.
type MCPConnection struct {
    Name   string
    Status ConnectorStatus
    Error  string
    Tools  []string  // 해당 서버가 제공하는 도구 목록
}
```

**앱 시작 시 MCP 연결 시도:**
- 실패 시 `Status=error` + 도구 비활성화 (에이전트는 계속 동작)
- `/mcp` 슬래시 명령으로 MCP 서버 상태 조회
- MCP 도구 호출 시 연결 끊어져 있으면 자동 재연결 시도

### 구현 파일: `internal/mcp/`

**`client.go`**: MCP 클라이언트 메인
```go
type MCPClient struct {
    Name   string
    Status ConnectorStatus
    tools  []MCPToolDef
}

func NewMCPClient(name string, cfg MCPServerConfig) *MCPClient
func (c *MCPClient) Connect(ctx context.Context) error
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPToolDef, error)
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
func (c *MCPClient) ToRegistryTools() []tools.Tool
```

**`stdio.go`**: stdio 전송 (서브프로세스와 JSON-RPC 통신)
```go
type StdioTransport struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout *bufio.Reader
}

func NewStdioTransport(command string, args []string, env map[string]string) *StdioTransport
func (t *StdioTransport) Send(ctx context.Context, req JSONRPCRequest) (JSONRPCResponse, error)
```

### Phase 7에서 추가: HTTP/SSE 전송

원격 MCP 서버 연결을 위해 `internal/mcp/http.go` 추가:
```yaml
mcp_servers:
  remote-grafana:
    url: "http://grafana-mcp-server:9200/mcp"
```

---

## 검증 시나리오

1. "db-server에 뭐 떠있어?" → 프로세스/포트/설정파일 스캔 → 서비스 목록
2. "Oracle 테이블스페이스" → 접속 정보 질문 → 커넥터 생성 → 결과
3. 재시작 후 → 영구 저장된 커넥터 자동 로딩
4. 확신도 중간 서비스 → "Tomcat 같은데 맞나요?" 질문
5. config.yaml에 MCP 서버 등록 → 시작 시 자동 연결 → "GitHub 이슈 만들어줘" 동작

---

## 완료 기준
- [ ] 프로세스 + 포트 + 설정파일 통합 스캔
- [ ] 빌트인 5개 서비스 패턴 매칭
- [ ] 커넥터 활성화 시 도구 세트 자동 등록 (IsReadOnly/IsEnabled 포함)
- [ ] 접속 정보 저장 옵션 (영구/세션/없음)
- [ ] 재시작 시 영구 커넥터 자동 복원
- [ ] 커넥터 상태 추적 (connected/connecting/error/disconnected)
- [ ] 커넥터 비활성화 시 Registry.Unregister()로 도구 제거
- [ ] MCP 클라이언트: stdio 전송으로 외부 MCP 서버 연결
- [ ] MCP 도구를 Tool 인터페이스로 래핑하여 Registry에 등록
- [ ] /mcp 슬래시 명령으로 MCP 연결 상태 조회
