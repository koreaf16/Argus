// Package discovery
// File: oracle_enrich.go
// Description: Oracle DB 프로세스 탐지 후 CDB/PDB 정보를 추가로 조회하는 Enrichment 로직
// Responsibility: OS Auth(sysdba) 및 lsnrctl status를 통한 메타데이터 수집

package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

var (
	// lsnrctlStatusServiceRegex는 lsnrctl status 출력에서 서비스 이름을 추출한다.
	lsnrctlStatusServiceRegex = regexp.MustCompile(`Service "([^"]+)" has \d+ instance\(s\)`)
)

// enrichOracleService는 탐지된 Oracle 서비스의 메타데이터(CDB 여부, PDB 목록)를 보완한다.
func enrichOracleService(ctx context.Context, exec executor.Executor, svc *DiscoveredService) {
	if svc.Type != ServiceOracle || svc.PID == 0 {
		return
	}

	slog.Info("enriching oracle service", "sid", svc.Name, "pid", svc.PID, "user", svc.User)

	// Step A: ORACLE_HOME 찾기
	oracleHome := findOracleHome(ctx, exec, svc)
	if oracleHome == "" {
		slog.Warn("failed to determine ORACLE_HOME, skipping enrichment", "sid", svc.Name)
		return
	}
	svc.Details["oracle_home"] = oracleHome

	// Step B: Method 1 - OS Auth (sqlplus / as sysdba)
	if enrichViaSQLPlus(ctx, exec, svc, oracleHome) {
		slog.Info("enriched oracle via sqlplus", "sid", svc.Name, "cdb", svc.Details["cdb"])
		return
	}

	// Step C: Method 2 - Fallback to lsnrctl status
	if enrichViaListener(ctx, exec, svc, oracleHome) {
		slog.Info("enriched oracle via listener status", "sid", svc.Name)
	}
}

// findOracleHome은 /proc/<PID>/exe 또는 /etc/oratab을 통해 ORACLE_HOME을 결정한다.
func findOracleHome(ctx context.Context, exec executor.Executor, svc *DiscoveredService) string {
	// 1. /proc/<PID>/exe 조회 (Linux)
	cmd := fmt.Sprintf("readlink -f /proc/%d/exe", svc.PID)
	res, err := exec.Execute(ctx, cmd)
	if err == nil && res.ExitCode == 0 {
		path := strings.TrimSpace(res.Stdout)
		if strings.HasSuffix(path, "/bin/oracle") {
			return strings.TrimSuffix(path, "/bin/oracle")
		}
	}

	// 2. /etc/oratab 파싱
	res, err = exec.Execute(ctx, "cat /etc/oratab")
	if err == nil && res.ExitCode == 0 {
		lines := strings.Split(res.Stdout, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, ":")
			if len(parts) >= 2 && parts[0] == svc.Name {
				return parts[1]
			}
		}
	}

	return ""
}

// enrichViaSQLPlus는 sqlplus / as sysdba 접속을 시도하여 CDB/PDB 정보를 가져온다.
func enrichViaSQLPlus(ctx context.Context, exec executor.Executor, svc *DiscoveredService, oracleHome string) bool {
	sql := `SET HEAD OFF PAGESIZE 0 FEEDBACK OFF
SELECT 'CDB:'||cdb FROM v$database;
SELECT 'PDB:'||name||':'||open_mode FROM v$pdbs WHERE name <> 'PDB$SEED' ORDER BY name;
EXIT;`

	// sudo -u <user> 권한이 필요함
	cmd := fmt.Sprintf("sudo -n -u %s sh -c 'export ORACLE_SID=%s; export ORACLE_HOME=%s; %s/bin/sqlplus -S / as sysdba' <<'EOF'\n%s\nEOF",
		svc.User, svc.Name, oracleHome, oracleHome, sql)

	res, err := exec.Execute(ctx, cmd)
	if err != nil || res.ExitCode != 0 {
		return false
	}

	var pdbs []string
	foundCDB := false
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CDB:") {
			raw := strings.ToUpper(strings.TrimPrefix(line, "CDB:"))
			if raw == "YES" {
				svc.Details["cdb"] = "yes"
			} else {
				svc.Details["cdb"] = "no"
			}
			foundCDB = true
		} else if strings.HasPrefix(line, "PDB:") {
			pdbs = append(pdbs, strings.TrimPrefix(line, "PDB:"))
		}
	}

	if len(pdbs) > 0 {
		svc.Details["pdbs"] = strings.Join(pdbs, ",")
	}
	return foundCDB
}

// enrichViaListener는 lsnrctl status를 파싱하여 등록된 서비스(PDB 가능성 있음) 목록을 가져온다.
func enrichViaListener(ctx context.Context, exec executor.Executor, svc *DiscoveredService, oracleHome string) bool {
	cmd := fmt.Sprintf("sudo -n -u %s sh -c 'export ORACLE_HOME=%s; %s/bin/lsnrctl status'",
		svc.User, oracleHome, oracleHome)

	res, err := exec.Execute(ctx, cmd)
	if err != nil || res.ExitCode != 0 {
		return false
	}

	matches := lsnrctlStatusServiceRegex.FindAllStringSubmatch(res.Stdout, -1)
	if len(matches) == 0 {
		return false
	}

	var foundServices []string
	sidUpper := strings.ToUpper(svc.Name)

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		serviceName := m[1]
		serviceUpper := strings.ToUpper(serviceName)

		// 메인 SID, XDB, 전역 도메인 등 제외 시도
		if serviceUpper == sidUpper ||
			strings.HasSuffix(serviceUpper, "XDB") ||
			strings.HasSuffix(serviceUpper, "_DGMGRL") {
			continue
		}
		foundServices = append(foundServices, serviceName)
	}

	if len(foundServices) > 0 {
		svc.Details["pdbs"] = strings.Join(foundServices, ",")
		if _, ok := svc.Details["cdb"]; !ok {
			svc.Details["cdb"] = "unknown"
		}
		return true
	}

	return false
}
