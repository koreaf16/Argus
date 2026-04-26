// Package oracle
// File: pdb_tool.go
// Description: Oracle CDB 전용 list_pdbs 도구 — v$pdbs에서 PDB 목록 조회
// Responsibility: makeListPDBsTool 구현 (CDB 인스턴스에만 등록)

package oracle

import (
	"context"
	"fmt"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// makeListPDBsTool은 v$pdbs에서 PDB 목록을 조회하는 CDB 전용 도구를 생성한다.
// non-CDB 인스턴스에서는 등록하지 않으므로 LLM이 잘못 호출할 위험이 없다.
func (c *OracleConnector) makeListPDBsTool(prefix, sid, connStr, pdb string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".list_pdbs",
		fmt.Sprintf("Oracle(%s) PDB 목록 조회.", prefix),
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"target": targetParam()},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			cmd := buildSQLPlusCmd(sid, connStr, pdbsListQuery, pdb)
			res, err := execWithStream(ctx, exec, cmd)
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}
