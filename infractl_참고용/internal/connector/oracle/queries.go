// Package oracle
// File: queries.go
// Description: Oracle 도구에서 사용하는 SQL 쿼리 및 쉘 명령 템플릿
// Responsibility: sqlplus로 실행할 쿼리와 명령을 문자열 상수로 관리

package oracle

import (
	"fmt"
	"strings"
)

// buildConnStr은 sqlplus 접속 문자열을 생성한다.
// host가 있으면 Easy Connect 방식, 없으면 TNSNAMES 방식이다.
// pdb가 지정되면 CDB 대신 PDB 서비스명으로 연결한다.
// osAuth가 true이면 "/ as sysdba" OS 인증 문자열을 반환한다.
func buildConnStr(user, pass, role, host string, port int, cdb, pdb string, osAuth bool) string {
	if osAuth {
		// OS 인증: 사용자 이름/패스워드 없이 sysdba로 접속
		return "/ as sysdba"
	}
	// PDB 서비스명이 있으면 PDB로 직접 연결, 없으면 CDB/SID 사용
	serviceName := cdb
	if pdb != "" {
		serviceName = pdb
	}
	connStr := user + "/" + pass
	if host != "" {
		if port > 0 {
			connStr += fmt.Sprintf("@%s:%d/%s", host, port, serviceName)
		} else {
			connStr += fmt.Sprintf("@%s/%s", host, serviceName)
		}
	} else if serviceName != "" {
		connStr += "@" + serviceName
	}
	if role == "sysdba" {
		connStr += " as sysdba"
	}
	return connStr
}

// buildOSAuthProbeCmd는 Oracle OS 인증 가능 여부를 확인하는 명령을 생성한다.
// ORACLE_SID 환경변수를 설정한 후 "/ as sysdba"로 접속하여 OS_AUTH_OK를 출력한다.
func buildOSAuthProbeCmd(sid string) string {
	return fmt.Sprintf(`ORACLE_SID=%s sqlplus -S '/ as sysdba' <<'SQLEOF'
SET FEEDBACK OFF
SET PAGESIZE 0
SELECT 'OS_AUTH_OK' FROM dual;
EXIT;
SQLEOF`, sid)
}

// buildSQLPlusCmd은 SQL을 실행하는 sqlplus 명령을 생성한다.
// sid가 있으면 ORACLE_SID 환경변수를 앞에 붙인다.
// pdb가 비어있지 않으면 ALTER SESSION SET CONTAINER로 PDB를 변경한다.
func buildSQLPlusCmd(sid, connStr, sql, pdb string) string {
	trimmed := strings.TrimSpace(sql)
	if !strings.HasSuffix(trimmed, ";") {
		trimmed += ";"
	}
	
	// PDB 변경 구문 추가
	if pdb != "" {
		trimmed = fmt.Sprintf("ALTER SESSION SET CONTAINER = %s;\n%s", quoteOracleIdentifier(pdb), trimmed)
	}

	sidPrefix := ""
	if sid != "" {
		sidPrefix = "ORACLE_SID=" + sid + " "
	}
	return fmt.Sprintf(`%ssqlplus -S '%s' <<'SQLEOF'
WHENEVER SQLERROR EXIT FAILURE
SET LINESIZE 200
SET PAGESIZE 50
SET FEEDBACK OFF
%s
EXIT;
SQLEOF`, sidPrefix, connStr, trimmed)
}

func buildSQLProfileSummaryQuery(sqlID string, child int, includeText bool) string {
	textPart := ""
	if includeText {
		textPart = ", sql_text"
	}
	childPart := ""
	if child >= 0 {
		childPart = fmt.Sprintf(" AND child_number = %d", child)
	}
	return fmt.Sprintf(`SELECT sql_id, child_number, plan_hash_value, executions, elapsed_time/1000000 AS elapsed_sec %s
FROM v$sql WHERE sql_id = '%s'%s;`, textPart, sqlID, childPart)
}

func buildSQLProfilePlanQuery(sqlID string, child int) string {
	return fmt.Sprintf(`SELECT * FROM TABLE(dbms_xplan.display_cursor('%s', %d, 'ALLSTATS LAST'));`, sqlID, child)
}

func buildTopSQLQuery(sortBy string, limit int, minExecutions int, schema string) string {
	orderBy := "s.elapsed_time DESC"
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "cpu":
		orderBy = "s.cpu_time DESC"
	case "buffer":
		orderBy = "s.buffer_gets DESC"
	case "executions":
		orderBy = "s.executions DESC"
	}

	if limit <= 0 {
		limit = 20
	}

	var whereParts []string
	if minExecutions > 0 {
		whereParts = append(whereParts, fmt.Sprintf("s.executions >= %d", minExecutions))
	}
	if strings.TrimSpace(schema) != "" {
		whereParts = append(whereParts, fmt.Sprintf("UPPER(s.parsing_schema_name) = UPPER('%s')", strings.ReplaceAll(strings.TrimSpace(schema), "'", "''")))
	}

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " AND " + strings.Join(whereParts, " AND ")
	}

	return fmt.Sprintf(`SELECT * FROM (
  SELECT s.sql_id, s.executions,
         s.elapsed_time,
         s.cpu_time,
         s.buffer_gets,
         s.parsing_schema_name,
         ROUND(s.elapsed_time/1000000,2) AS elapsed_sec,
         ROUND(s.elapsed_time/GREATEST(s.executions,1)/1000000,4) AS avg_elapsed,
         SUBSTR(s.sql_text,1,100) AS sql_text
  FROM v$sql s
  WHERE s.executions > 0%s
  ORDER BY %s
) WHERE ROWNUM <= %d;`, whereClause, orderBy, limit)
}

func buildHealthCheckQuery() string {
	return `SELECT 'DB_STATUS:'||status FROM v$instance;
SELECT 'SESSION_COUNT:'||count(*) FROM v$session;
SELECT 'LOCK_COUNT:'||count(*) FROM v$lock WHERE block=1 OR request>0;`
}

func buildAWRReportQuery(beginID, endID int, reportType string) string {
	if reportType == "html" {
		return fmt.Sprintf("SELECT * FROM TABLE(dbms_workload_repository.awr_report_html(1, 1, %d, %d));", beginID, endID)
	}
	return fmt.Sprintf("SELECT * FROM TABLE(dbms_workload_repository.awr_report_text(1, 1, %d, %d));", beginID, endID)
}

func buildASHSummaryQuery(minutes int) string {
	return fmt.Sprintf(`SELECT event, count(*) AS ash_count FROM v$active_session_history 
WHERE sample_time > sysdate - interval '%d' minute 
GROUP BY event ORDER BY ash_count DESC;`, minutes)
}

func buildFullScanObjectsQuery(sqlID string, child int) string {
	return fmt.Sprintf(`SELECT object_owner||'|'||object_name FROM v$sql_plan 
WHERE sql_id='%s' AND child_number=%d AND operation='TABLE ACCESS' AND options='FULL';`, sqlID, child)
}

func buildCandidateColumnQuery(owner, table string) string {
	return fmt.Sprintf(`SELECT column_name FROM dba_tab_columns 
WHERE owner='%s' AND table_name='%s' AND num_distinct > 100 
ORDER BY num_nulls ASC;`, owner, table)
}

func quoteOracleIdentifier(id string) string {
	return `"` + strings.ReplaceAll(id, `"`, `""`) + `"`
}


// tablespaceQuery는 테이블스페이스 사용량 조회 쿼리이다.
const tablespaceQuery = `
SELECT
  d.tablespace_name,
  ROUND(d.allocated_space/1024/1024,1) AS allocated_mb,
  ROUND(d.used_space/1024/1024,1) AS used_mb,
  ROUND((d.allocated_space-d.used_space)/1024/1024,1) AS free_mb,
  ROUND(d.used_percent,1) AS used_pct
FROM dba_tablespace_usage_metrics d
ORDER BY d.used_percent DESC;`

// sessionsQuery는 활성 세션 조회 쿼리이다.
const sessionsQuery = `
SELECT
  s.sid, s.serial#, s.username, s.status, s.machine,
  s.program, s.sql_id, s.event
FROM v$session s
WHERE s.type = 'USER'
ORDER BY s.status, s.username;`

// locksQuery는 락 정보 조회 쿼리이다.
const locksQuery = `
SELECT
  s.sid, s.serial#, s.username, l.type, l.lmode, l.request,
  l.block, s.status, s.machine
FROM v$lock l JOIN v$session s ON l.sid = s.sid
WHERE l.block = 1 OR l.request > 0
ORDER BY l.block DESC;`

// topSQLQuery는 부하 SQL 조회 쿼리이다.
const topSQLQuery = `
SELECT * FROM (
  SELECT sql_id, executions,
         ROUND(elapsed_time/1000000,2) AS elapsed_sec,
         ROUND(elapsed_time/GREATEST(executions,1)/1000000,4) AS avg_elapsed,
         SUBSTR(sql_text,1,100) AS sql_text
  FROM v$sql
  WHERE executions > 0
  ORDER BY elapsed_time DESC
) WHERE ROWNUM <= 20;`

// metadataProbeQuery는 접속 직후 Oracle 버전, CDB 여부, PDB 목록을 한 세션에 조회한다.
// VER:/CDB:/PDB: 프리픽스로 라인 구분한다.
// v$database.cdb는 12c 이전(11g 등)에 존재하지 않아 ORA-00904가 발생할 수 있으므로
// probe.go 파서는 해당 컬럼 누락을 non-CDB로 해석한다.
const metadataProbeQuery = `SET FEEDBACK OFF
SET PAGESIZE 0
SET LINESIZE 300
SELECT 'VER:'||version FROM v$instance;
SELECT 'CDB:'||cdb FROM v$database;
SELECT 'PDB:'||name||':'||open_mode FROM v$pdbs WHERE name <> 'PDB$SEED' ORDER BY name;
EXIT;`

// pdbsListQuery는 list_pdbs 도구에서 사용하는 PDB 목록 쿼리이다.
const pdbsListQuery = `
SELECT con_id, name, open_mode, restricted
FROM v$pdbs
ORDER BY con_id;`

// alertLogCmd는 alert 로그 마지막 N줄을 읽는 명령이다.
func alertLogCmd(oracleHome, sid string, lines int) string {
	return fmt.Sprintf(
		`find %s/diag/rdbms -name 'alert_%s.log' 2>/dev/null | head -1 | xargs -I{} tail -n %d {}`,
		oracleHome, sid, lines,
	)
}
