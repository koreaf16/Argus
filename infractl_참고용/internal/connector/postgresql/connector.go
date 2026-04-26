// Package postgresql
// File: connector.go
// Description: PostgreSQL DB 커넥터 — psql CLI 기반 도구 세트 생성
// Responsibility: PostgreSQL 접속 정보로 query/databases/connections/locks 도구 생성

package postgresql

import (
	"context"
	"fmt"
	"strings"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/pipeline"
	"github.com/yourorg/infractl/internal/tools"
)

// PostgreSQLConnector는 PostgreSQL 전용 커넥터이다.
type PostgreSQLConnector struct {
	info   conn.ServiceInfo
	creds  conn.Credentials
	status conn.ConnectorStatus
	names  []string
}

// New는 PostgreSQLConnector를 생성한다.
func New() *PostgreSQLConnector {
	return &PostgreSQLConnector{status: conn.StatusDisconnected}
}

func (c *PostgreSQLConnector) ServiceType() string          { return "postgresql" }
func (c *PostgreSQLConnector) Status() conn.ConnectorStatus { return c.status }
func (c *PostgreSQLConnector) ToolNames() []string          { return c.names }

// GenerateTools는 PostgreSQL 접속 정보로 도구를 생성한다.
func (c *PostgreSQLConnector) GenerateTools(info conn.ServiceInfo, creds conn.Credentials) []tools.Tool {
	c.info = info
	c.creds = creds
	c.status = conn.StatusConnected

	dbName := info.Name
	if dbName == "" {
		dbName = "postgres"
	}
	prefix := "pg_" + strings.ToLower(dbName)

	host := info.Details["host"]
	if host == "" {
		host = "127.0.0.1"
	}
	port := info.Port
	if port == 0 {
		port = 5432
	}

	toolList := []tools.Tool{
		c.makeQueryTool(prefix, creds.Username, creds.Password, host, port, dbName),
		c.makeDatabasesTool(prefix, creds.Username, creds.Password, host, port),
		c.makeConnectionsTool(prefix, creds.Username, creds.Password, host, port),
		c.makeLocksTool(prefix, creds.Username, creds.Password, host, port),
	}

	c.names = make([]string, len(toolList))
	for i, t := range toolList {
		c.names[i] = t.Name()
	}
	return toolList
}

// buildCmd는 PGPASSWORD 환경변수로 비밀번호를 전달하는 psql 명령을 생성한다.
func buildCmd(user, pass, host string, port int, dbName, sql string) string {
	return fmt.Sprintf(
		`PGPASSWORD='%s' psql -U '%s' -h '%s' -p %d -d '%s' -c "%s" 2>&1`,
		strings.ReplaceAll(pass, "'", "'\\''"),
		user, host, port, dbName, sql,
	)
}

// ProbeOSAuth는 PostgreSQL peer 인증(unix socket)을 시도한다.
func (c *PostgreSQLConnector) ProbeOSAuth(ctx context.Context, info conn.ServiceInfo, exec executor.Executor) (conn.Credentials, error) {
	cmd := `psql -U postgres -c "SELECT 'OS_AUTH_OK';" 2>&1`
	res, err := exec.Execute(ctx, cmd)
	if err != nil {
		return conn.Credentials{}, fmt.Errorf("postgresql peer auth probe: %w", err)
	}
	if !strings.Contains(res.Stdout, "OS_AUTH_OK") {
		return conn.Credentials{}, fmt.Errorf("PostgreSQL peer 인증 실패: 접속 불가")
	}
	return conn.Credentials{Username: "postgres", OSAuth: true}, nil
}

func (c *PostgreSQLConnector) makeQueryTool(prefix, user, pass, host string, port int, dbName string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".query",
		fmt.Sprintf("PostgreSQL(%s) SQL 실행.", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sql":    map[string]interface{}{"type": "string", "description": "실행할 SQL 문"},
				"target": targetParam(),
			},
			"required": []string{"sql"},
		},
		false, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			sql, _ := args["sql"].(string)
			if sql == "" {
				return "sql 인자가 필요합니다", nil
			}

			res, err := exec.Execute(ctx, buildCmd(user, pass, host, port, dbName, sql))
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			output := formatOutput(res.Stdout, res.Stderr, res.ExitCode)
			return validateAndEnrich(ctx, user, pass, host, port, dbName, sql, output, exec), nil
		},
	)
}

func (c *PostgreSQLConnector) makeDatabasesTool(prefix, user, pass, host string, port int) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".databases",
		fmt.Sprintf("PostgreSQL(%s) 데이터베이스 목록 및 크기 조회.", prefix),
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"target": targetParam()},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			sql := `SELECT datname, pg_size_pretty(pg_database_size(datname)) AS size FROM pg_database ORDER BY pg_database_size(datname) DESC;`
			res, err := exec.Execute(ctx, buildCmd(user, pass, host, port, "postgres", sql))
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *PostgreSQLConnector) makeConnectionsTool(prefix, user, pass, host string, port int) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".connections",
		fmt.Sprintf("PostgreSQL(%s) 활성 연결 목록 조회 (pg_stat_activity).", prefix),
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"target": targetParam()},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			sql := `SELECT pid, usename, datname, state, wait_event_type, wait_event, LEFT(query,80) AS query FROM pg_stat_activity WHERE state IS NOT NULL ORDER BY state;`
			res, err := exec.Execute(ctx, buildCmd(user, pass, host, port, "postgres", sql))
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *PostgreSQLConnector) makeLocksTool(prefix, user, pass, host string, port int) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".locks",
		fmt.Sprintf("PostgreSQL(%s) 락 정보 조회 (pg_locks).", prefix),
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"target": targetParam()},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			sql := `SELECT l.pid, l.mode, l.granted, a.usename, a.state, LEFT(a.query,80) FROM pg_locks l JOIN pg_stat_activity a ON l.pid = a.pid WHERE NOT l.granted OR l.pid IN (SELECT blocking_pid FROM pg_stat_activity WHERE cardinality(pg_blocking_pids(pid)) > 0) ORDER BY l.granted;`
			res, err := exec.Execute(ctx, buildCmd(user, pass, host, port, "postgres", sql))
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func targetParam() map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": "대상 서버 이름. 생략 시 커넥터 등록 서버 사용.",
	}
}

// formatOutput은 명령 실행 결과를 포맷한다.
func formatOutput(stdout, stderr string, exitCode int) string {
	out := pipeline.CleanPSQLOutput(stdout)
	if exitCode != 0 && stderr != "" {
		return fmt.Sprintf("[ExitCode: %d]\n%s\n[Stderr]\n%s", exitCode, out, strings.TrimSpace(stderr))
	}
	if out == "" {
		return "(결과 없음)"
	}
	return out
}
