// Package oracle
// File: trace_tool.go
// Description: Oracle 현재 세션의 Trace 파일 경로 조회 도구
// Responsibility: makeTracePathTool 구현 (v$diag_info 사용)

package oracle

import (
	"context"
	"fmt"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

const tracePathQuery = `SET PAGESIZE 0 FEEDBACK OFF VERIFY OFF HEAD OFF
SELECT value FROM v$diag_info WHERE name = 'Default Trace File';
EXIT;`

// makeTracePathTool은 현재 세션의 트레이스 파일 절대 경로를 반환한다.
func (c *OracleConnector) makeTracePathTool(prefix, sid, connStr, pdb string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".trace_path",
		fmt.Sprintf("Oracle(%s) 현재 세션의 Trace 파일 절대 경로 조회 (v$diag_info). tkprof 실행 전 경로 확인용.", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target": targetParam(),
			},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			cmd := buildSQLPlusCmd(sid, connStr, tracePathQuery, pdb)
			res, err := execWithStream(ctx, exec, cmd)
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}
