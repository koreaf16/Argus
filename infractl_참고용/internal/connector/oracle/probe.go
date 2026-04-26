// Package oracle
// File: probe.go
// Description: Oracle 접속 직후 CDB 여부, 버전, PDB 목록 자동 탐지
// Responsibility: ProbeMetadata 구현 — sqlplus 1회 호출로 메타데이터를 조회하고 파싱

package oracle

import (
	"fmt"
	"strings"

	"context"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
)

// ProbeMetadata는 Oracle 인스턴스에 접속하여 버전, CDB 여부, PDB 목록을 조회한다.
// 실패해도 활성화를 차단하지 않으므로 부분 결과만 있어도 error는 nil로 반환한다.
// 조회 결과는 map[string]string으로 반환되며 키는 아래와 같다:
//   - oracle_version: "19.0.0.0.0" 형식
//   - cdb: "yes" 또는 "no"
//   - pdbs: "HRPDB:READ WRITE,SALESPDB:READ WRITE" (CDB일 때만, 없으면 키 없음)
func (c *OracleConnector) ProbeMetadata(ctx context.Context, info conn.ServiceInfo, creds conn.Credentials, exec executor.Executor) (map[string]string, error) {
	sid := info.Name
	if sid == "" {
		sid = "ORCL"
	}
	pdb := info.SubInstance
	connStr := buildConnStr(creds.Username, creds.Password, creds.Role,
		info.Details["host"], info.Port, sid, pdb, creds.OSAuth)

	cmd := fmt.Sprintf("ORACLE_SID=%s sqlplus -S '%s' <<'SQLEOF'\n%s\nSQLEOF", sid, connStr, metadataProbeQuery)
	res, err := exec.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("metadata probe execute: %w", err)
	}

	return parseProbeOutput(res.Stdout), nil
}

// parseProbeOutput는 VER:/CDB:/PDB: 프리픽스 기반으로 sqlplus 출력을 파싱한다.
func parseProbeOutput(output string) map[string]string {
	result := make(map[string]string)
	var pdbs []string

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "VER:"):
			result["oracle_version"] = strings.TrimPrefix(line, "VER:")
		case strings.HasPrefix(line, "CDB:"):
			raw := strings.ToUpper(strings.TrimPrefix(line, "CDB:"))
			if raw == "YES" {
				result["cdb"] = "yes"
			} else {
				result["cdb"] = "no"
			}
		case strings.HasPrefix(line, "PDB:"):
			pdbs = append(pdbs, strings.TrimPrefix(line, "PDB:"))
		}
	}

	// CDB 컬럼이 결과에 없으면 (11g 등 ORA-00904) non-CDB로 가정
	if _, ok := result["cdb"]; !ok {
		if strings.Contains(output, "ORA-00904") || strings.Contains(output, "ORA-00942") {
			result["cdb"] = "no"
		}
	}

	if len(pdbs) > 0 {
		result["pdbs"] = strings.Join(pdbs, ",")
	}

	return result
}
