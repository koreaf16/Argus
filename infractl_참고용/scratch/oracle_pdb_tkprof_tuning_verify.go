//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/store"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	dbPath := ".infractl/infractl.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}
	serverName := "oracle"
	if len(os.Args) > 2 {
		serverName = os.Args[2]
	}

	st, err := store.NewSQLiteStore(ctx, dbPath)
	if err != nil {
		panic(err)
	}
	defer st.Close()

	srv, err := st.Get(ctx, serverName)
	if err != nil {
		panic(err)
	}

	cfg := &sshconn.Config{
		Host:         srv.Host,
		Port:         srv.Port,
		User:         srv.User,
		AuthType:     string(srv.AuthType),
		WorkspaceDir: srv.WorkspaceDir,
		Timeout:      10 * time.Minute,
	}
	if srv.AuthType == store.AuthTypeKey {
		cfg.KeyPath = srv.Credential
	} else {
		cfg.Password = srv.Credential
	}

	client := sshconn.NewClient(cfg)
	defer client.Close()

	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
	exec.SetTimeout(720)

	command := "bash -s <<'BASH'\n" + oraclePDBTKProfTuningScript() + "\nBASH"
	start := time.Now()
	res, err := exec.Execute(ctx, command)
	duration := time.Since(start).Round(time.Millisecond)

	logPath := filepath.Join("scratch", fmt.Sprintf("oracle_pdb_tkprof_tuning_%s.log", time.Now().Format("20060102_150405")))
	_ = os.WriteFile(logPath, []byte(res.Stdout+"\n[stderr]\n"+res.Stderr), 0600)

	fmt.Printf("target=%s host=%s duration=%s exit=%d err=%v log=%s\n", srv.Name, srv.Host, duration, res.ExitCode, err, logPath)
	if strings.TrimSpace(res.Stdout) != "" {
		fmt.Println(res.Stdout)
	}
	if strings.TrimSpace(res.Stderr) != "" {
		fmt.Println("[stderr]")
		fmt.Println(res.Stderr)
	}
	if err != nil {
		os.Exit(1)
	}
	if res.ExitCode != 0 {
		os.Exit(res.ExitCode)
	}
}

func oraclePDBTKProfTuningScript() string {
	return `set -Eeuo pipefail

RUN_ID=$(date +%Y%m%d%H%M%S)
LAB_USER=INFRACTL_LAB
LAB_PASS='Infractl_Lab_2026#'
ROWS=${INFRACTL_LAB_ROWS:-120000}
OUT_DIR=/tmp/infractl_oracle_tune_${RUN_ID}
mkdir -p "$OUT_DIR"

SQLPLUS=$(command -v sqlplus || true)
if [ -z "$SQLPLUS" ]; then
  for p in /u01/app/oracle/product/*/dbhome*/bin/sqlplus /opt/oracle/product/*/dbhome*/bin/sqlplus /home/oracle/app/oracle/product/*/bin/sqlplus /home/oracle/*/bin/sqlplus; do
    if [ -x "$p" ]; then SQLPLUS="$p"; break; fi
  done
fi
if [ -z "$SQLPLUS" ]; then echo "NO_SQLPLUS"; exit 3; fi

if [ -z "${ORACLE_HOME:-}" ]; then
  ORACLE_HOME=$(cd "$(dirname "$SQLPLUS")/.." && pwd)
  export ORACLE_HOME
fi
if [ -z "${ORACLE_SID:-}" ]; then
  ORACLE_SID=$(ps -ef | awk '/[o]ra_pmon_/ {sub(/^ora_pmon_/,"",$8); print $8; exit}')
  export ORACLE_SID
fi
if [ -z "${ORACLE_SID:-}" ]; then echo "NO_ORACLE_SID"; exit 4; fi
export PATH="$ORACLE_HOME/bin:$PATH"

TKPROF=$(command -v tkprof || true)
if [ -z "$TKPROF" ]; then echo "NO_TKPROF"; exit 5; fi

trim_sqlplus() {
  awk '{
    gsub(/^[ \t]+|[ \t]+$/, "", $0)
    if ($0 != "" && $0 !~ /^SQL>/) print
  }'
}

echo "== ENV =="
echo "OS_USER=$(whoami)"
echo "ORACLE_HOME=$ORACLE_HOME"
echo "ORACLE_SID=$ORACLE_SID"
echo "SQLPLUS=$SQLPLUS"
echo "TKPROF=$TKPROF"
echo "RUN_ID=$RUN_ID"

TRACE_DIR=$("$SQLPLUS" -S "/ as sysdba" <<'SQL' | trim_sqlplus | tail -1
SET HEADING OFF FEEDBACK OFF PAGESIZE 0 VERIFY OFF
SELECT value FROM v$diag_info WHERE name='Diag Trace';
EXIT;
SQL
)
if [ -z "$TRACE_DIR" ] || [ ! -d "$TRACE_DIR" ]; then
  echo "TRACE_DIR_NOT_FOUND=$TRACE_DIR"
  exit 6
fi
echo "TRACE_DIR=$TRACE_DIR"

DB_HOST=${INFRACTL_ORACLE_HOST:-}
if [ -z "$DB_HOST" ]; then
  DB_HOST=$(hostname -I 2>/dev/null | awk '{print $1}')
fi
if [ -z "$DB_HOST" ]; then
  DB_HOST=$(hostname -f 2>/dev/null || hostname)
fi
echo "DB_HOST=$DB_HOST"

PDB=$("$SQLPLUS" -S "/ as sysdba" <<'SQL' | trim_sqlplus | head -1
SET HEADING OFF FEEDBACK OFF PAGESIZE 0 VERIFY OFF
SELECT name FROM (
  SELECT name FROM v$pdbs
  WHERE name <> 'PDB$SEED' AND open_mode = 'READ WRITE'
  ORDER BY con_id
) WHERE ROWNUM = 1;
EXIT;
SQL
)
case "$PDB" in
  ""|*[!A-Za-z0-9_\$#]*)
    echo "NO_SAFE_OPEN_PDB=$PDB"
    exit 7
    ;;
esac
echo "SELECTED_PDB=$PDB"

DEFAULT_TBS=$("$SQLPLUS" -S "/ as sysdba" <<SQL | trim_sqlplus | head -1
SET HEADING OFF FEEDBACK OFF PAGESIZE 0 VERIFY OFF
ALTER SESSION SET CONTAINER = "$PDB";
SELECT tablespace_name FROM (
  SELECT tablespace_name
  FROM dba_tablespaces
  WHERE contents = 'PERMANENT' AND status = 'ONLINE'
  ORDER BY CASE
    WHEN tablespace_name = 'USERS' THEN 0
    WHEN tablespace_name NOT IN ('SYSTEM', 'SYSAUX') THEN 1
    ELSE 2
  END, tablespace_name
) WHERE ROWNUM = 1;
EXIT;
SQL
)
case "$DEFAULT_TBS" in
  ""|*[!A-Za-z0-9_\$#]*) DEFAULT_TBS=USERS ;;
esac
echo "DEFAULT_TABLESPACE=$DEFAULT_TBS"

cleanup() {
  set +e
  "$SQLPLUS" -S "/ as sysdba" <<SQL >/dev/null 2>&1
WHENEVER SQLERROR CONTINUE
ALTER SESSION SET CONTAINER = "$PDB";
DROP USER $LAB_USER CASCADE;
EXIT;
SQL
}
trap cleanup EXIT

echo "== ROOT/PDB DISCOVERY =="
"$SQLPLUS" -S "/ as sysdba" <<SQL
SET LINESIZE 220 PAGESIZE 100 FEEDBACK OFF VERIFY OFF
COLUMN name FORMAT A30
COLUMN open_mode FORMAT A20
SELECT name, cdb, open_mode, log_mode FROM v\$database;
SELECT con_id, name, open_mode, restricted FROM v\$pdbs ORDER BY con_id;
EXIT;
SQL

echo "== LAB SETUP =="
"$SQLPLUS" -S "/ as sysdba" <<SQL
WHENEVER SQLERROR EXIT SQL.SQLCODE
SET FEEDBACK ON
ALTER SESSION SET CONTAINER = "$PDB";
BEGIN
  EXECUTE IMMEDIATE 'DROP USER $LAB_USER CASCADE';
EXCEPTION
  WHEN OTHERS THEN
    IF SQLCODE != -1918 THEN RAISE; END IF;
END;
/
CREATE USER $LAB_USER IDENTIFIED BY "$LAB_PASS" DEFAULT TABLESPACE $DEFAULT_TBS QUOTA UNLIMITED ON $DEFAULT_TBS;
GRANT CREATE SESSION, CREATE TABLE, ALTER SESSION TO $LAB_USER;
EXIT;
SQL

LAB_CONN="${LAB_USER}/\"${LAB_PASS}\"@//${DB_HOST}:1521/${PDB}"

"$SQLPLUS" -S "$LAB_CONN" <<SQL
WHENEVER SQLERROR EXIT SQL.SQLCODE
SET TIMING ON
SET FEEDBACK ON
CREATE TABLE TUNE_LAB NOLOGGING AS
SELECT LEVEL AS id,
       MOD(LEVEL, 10000) AS lookup_key,
       RPAD('X', 200, 'X') AS pad
FROM dual
CONNECT BY LEVEL <= $ROWS;
BEGIN
  DBMS_STATS.GATHER_TABLE_STATS(USER, 'TUNE_LAB');
END;
/
SELECT COUNT(*) AS tune_lab_rows FROM TUNE_LAB;
EXIT;
SQL

run_trace() {
  label="$1"
  trace_id="INFRACTL_${label}_${RUN_ID}"
  sqlplus_out="$OUT_DIR/${label}.sqlplus.out"
  tkprof_file="$OUT_DIR/${label}.tkprof.txt"

  echo "== TRACE_${label}_SQLPLUS =="
  set +e
  "$SQLPLUS" -S "$LAB_CONN" <<SQL > "$sqlplus_out"
WHENEVER SQLERROR EXIT SQL.SQLCODE
SET LINESIZE 220 PAGESIZE 100 FEEDBACK ON VERIFY OFF
SET TIMING ON
ALTER SESSION SET tracefile_identifier='${trace_id}';
ALTER SESSION SET statistics_level=ALL;
ALTER SESSION SET timed_statistics=TRUE;
ALTER SESSION SET SQL_TRACE=TRUE;
SELECT /* ${trace_id} */ COUNT(*) AS miss_count
FROM TUNE_LAB
WHERE lookup_key = -1;
ALTER SESSION SET SQL_TRACE=FALSE;
EXIT;
SQL
  sql_rc=$?
  set -e
  cat "$sqlplus_out"
  if [ "$sql_rc" -ne 0 ]; then
    echo "TRACE_SQLPLUS_FAILED_${label}=$sql_rc"
    exit "$sql_rc"
  fi

  trace_file=$(find "$TRACE_DIR" -name "*${trace_id}*.trc" -type f -printf '%T@ %p\n' 2>/dev/null | sort -nr | awk 'NR==1 {print $2}')
  if [ -z "$trace_file" ] || [ ! -r "$trace_file" ]; then
    echo "TRACE_FILE_NOT_FOUND_${label}=$trace_file"
    exit 8
  fi
  "$TKPROF" "$trace_file" "$tkprof_file" sys=no aggregate=no sort=exeela,fchela,prsela >/dev/null
  echo "TRACE_FILE_${label}=$trace_file"
  echo "TKPROF_FILE_${label}=$tkprof_file"
  echo "== TKPROF_${label}_TARGET_SQL =="
  awk -v marker="$trace_id" '
    index($0, marker) {found=1}
    found {print; lines++}
    found && lines >= 110 {exit}
  ' "$tkprof_file"
}

run_trace BEFORE

echo "== TUNING =="
"$SQLPLUS" -S "$LAB_CONN" <<'SQL'
WHENEVER SQLERROR EXIT SQL.SQLCODE
SET TIMING ON
SET FEEDBACK ON
CREATE INDEX IX_TUNE_LAB_LOOKUP ON TUNE_LAB(lookup_key);
BEGIN
  DBMS_STATS.GATHER_TABLE_STATS(USER, 'TUNE_LAB', cascade=>TRUE);
END;
/
EXIT;
SQL

run_trace AFTER

echo "== DB_HEALTH_CHECK =="
"$SQLPLUS" -S "/ as sysdba" <<SQL
SET LINESIZE 220 PAGESIZE 100 FEEDBACK OFF VERIFY OFF
COLUMN instance_name FORMAT A20
COLUMN status FORMAT A12
COLUMN database_status FORMAT A18
COLUMN name FORMAT A30
COLUMN open_mode FORMAT A20
COLUMN tablespace_name FORMAT A30
PROMPT -- INSTANCE
SELECT instance_name, status, database_status, host_name FROM v\$instance;
PROMPT -- DATABASE
SELECT name, cdb, open_mode, database_role, log_mode FROM v\$database;
PROMPT -- PDBS
SELECT con_id, name, open_mode, restricted FROM v\$pdbs ORDER BY con_id;
ALTER SESSION SET CONTAINER = "$PDB";
PROMPT -- PDB TABLESPACE TOP
SELECT * FROM (
  SELECT tablespace_name, ROUND(used_percent, 1) AS used_pct
  FROM dba_tablespace_usage_metrics
  ORDER BY used_percent DESC
) WHERE ROWNUM <= 5;
PROMPT -- ACTIVE USER SESSIONS
SELECT COUNT(*) AS active_user_sessions FROM v\$session WHERE type='USER' AND status='ACTIVE';
PROMPT -- LOCK WAITS
SELECT COUNT(*) AS lock_waits FROM v\$lock WHERE request > 0;
EXIT;
SQL

echo "== CLEANUP =="
cleanup
trap - EXIT
echo "SCENARIO_STATUS=OK"
echo "ARTIFACT_DIR=$OUT_DIR"
`
}
