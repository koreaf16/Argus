//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

type oracleStorePrompter struct {
	st store.ServerStore
}

func (p oracleStorePrompter) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	srv, err := p.st.Get(ctx, req.Target)
	if err != nil {
		return privilege.PromptResponse{}, err
	}
	if srv.AuthType != store.AuthTypePassword || strings.TrimSpace(srv.Credential) == "" {
		return privilege.PromptResponse{}, fmt.Errorf("no stored password for %s", req.Target)
	}
	return privilege.PromptResponse{Password: srv.Credential}, nil
}

func main() {
	ctx := context.Background()

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
		Timeout:      60 * time.Second,
	}
	if srv.AuthType == store.AuthTypeKey {
		cfg.KeyPath = srv.Credential
	} else {
		cfg.Password = srv.Credential
	}

	client := sshconn.NewClient(cfg)
	defer client.Close()

	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)
	tool := &tools.ShellExecTool{
		PrivilegeCache: privilege.NewCache(),
		PromptHandler:  oracleStorePrompter{st: st},
	}

	fmt.Printf("db=%s target=%s host=%s ssh_user=%s os=%s workspace=%s\n", dbPath, srv.Name, srv.Host, srv.User, srv.OS, srv.WorkspaceDir)

	runTool(ctx, tool, exec, "oracle-env-discovery", oracleEnvDiscoveryCmd(), 30*time.Second)
	runTool(ctx, tool, exec, "cdb-pdb-before", oracleCDBPDBCmd(), 45*time.Second)
	runTool(ctx, tool, exec, "sql-trace-tkprof", oracleTraceCmd(), 60*time.Second)
	runTool(ctx, tool, exec, "shutdown-startup-verify", oracleShutdownStartupCmd(), 180*time.Second)
	runTool(ctx, tool, exec, "cdb-pdb-after", oracleCDBPDBCmd(), 45*time.Second)
}

func runTool(ctx context.Context, tool *tools.ShellExecTool, exec executor.Executor, name, command string, timeout time.Duration) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	out, err := tool.Execute(cctx, map[string]interface{}{
		"command":     command,
		"target":      exec.Target(),
		"description": name,
	}, exec)

	fmt.Printf("\n=== %s (%s) ===\n", name, time.Since(start).Round(time.Millisecond))
	fmt.Printf("success=%v exit=%d err=%v\n", out.Success, out.ExitCode, err)
	if strings.TrimSpace(out.Content) != "" {
		fmt.Println(trimOutput(out.Content, 8000))
	}
	if !out.Success && strings.TrimSpace(out.ErrorMessage) != "" {
		fmt.Printf("[error] %s\n", out.ErrorMessage)
	}
}

func oracleEnvPrefix() string {
	return `set -u
SQLPLUS=$(command -v sqlplus || true)
if [ -z "$SQLPLUS" ]; then
  for p in /u01/app/oracle/product/*/dbhome*/bin/sqlplus /opt/oracle/product/*/dbhome*/bin/sqlplus /home/oracle/*/bin/sqlplus; do
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
`
}

func oracleEnvDiscoveryCmd() string {
	return oracleEnvPrefix() + `
echo "OS_USER=$(whoami)"
echo "ID=$(id)"
echo "SQLPLUS=$SQLPLUS"
echo "ORACLE_HOME=$ORACLE_HOME"
echo "ORACLE_SID=$ORACLE_SID"
echo "PMON=$(ps -ef | awk '/[o]ra_pmon_/ {print $8}' | paste -sd, -)"
echo "TKPROF=$(command -v tkprof || true)"
`
}

func oracleCDBPDBCmd() string {
	return oracleEnvPrefix() + `
sqlplus -S "/ as sysdba" <<'SQL'
SET LINESIZE 220
SET PAGESIZE 200
SET FEEDBACK OFF
SET TRIMSPOOL ON
COLUMN instance_name FORMAT A20
COLUMN status FORMAT A12
COLUMN database_status FORMAT A18
COLUMN host_name FORMAT A40
COLUMN name FORMAT A30
COLUMN open_mode FORMAT A20
COLUMN database_role FORMAT A24
COLUMN restricted FORMAT A12
PROMPT == INSTANCE ==
SELECT instance_name, status, database_status, host_name FROM v$instance;
PROMPT == DATABASE ==
SELECT name, cdb, open_mode, database_role, log_mode FROM v$database;
PROMPT == CONTAINERS ==
SELECT con_id, name, open_mode FROM v$containers ORDER BY con_id;
PROMPT == PDBS ==
SELECT con_id, name, open_mode, restricted FROM v$pdbs ORDER BY con_id;
EXIT;
SQL
`
}

func oracleTraceCmd() string {
	return oracleEnvPrefix() + `
OUT_FILE=$(mktemp /tmp/infractl_sqltrace_XXXXXX.out)
TKPROF_FILE=/tmp/infractl_tkprof_verify.txt
sqlplus -S "/ as sysdba" <<'SQL' > "$OUT_FILE"
SET LINESIZE 220
SET PAGESIZE 100
SET FEEDBACK OFF
SET TRIMSPOOL ON
COLUMN trace_file FORMAT A180
ALTER SESSION SET tracefile_identifier='INFRACTL_VERIFY';
BEGIN DBMS_MONITOR.SESSION_TRACE_ENABLE(waits=>TRUE, binds=>TRUE); END;
/
SELECT COUNT(*) AS session_count FROM v$session;
SELECT COUNT(*) AS container_count FROM v$containers;
BEGIN DBMS_MONITOR.SESSION_TRACE_DISABLE; END;
/
SELECT value AS trace_file FROM v$diag_info WHERE name='Default Trace File';
EXIT;
SQL
cat "$OUT_FILE"
TRACE_FILE=$(awk '/\/.*\.trc/ {print $NF}' "$OUT_FILE" | tail -1)
echo "TRACE_FILE=$TRACE_FILE"
if [ -n "$TRACE_FILE" ] && [ -r "$TRACE_FILE" ]; then
  ls -l "$TRACE_FILE"
else
  echo "TRACE_FILE_NOT_READABLE"
fi
if command -v tkprof >/dev/null 2>&1 && [ -n "$TRACE_FILE" ] && [ -r "$TRACE_FILE" ]; then
  tkprof "$TRACE_FILE" "$TKPROF_FILE" sys=no sort=prsela,exeela,fchela >/dev/null 2>&1 || true
  echo "TKPROF_FILE=$TKPROF_FILE"
  if [ -r "$TKPROF_FILE" ]; then
    sed -n '1,80p' "$TKPROF_FILE"
  fi
else
  echo "TKPROF_UNAVAILABLE"
fi
`
}

func oracleShutdownStartupCmd() string {
	return oracleEnvPrefix() + `
echo "== BEFORE =="
sqlplus -S "/ as sysdba" <<'SQL'
SET PAGESIZE 0
SET FEEDBACK OFF
SET HEADING OFF
SELECT 'BEFORE_INSTANCE='||instance_name||':'||status||':'||database_status FROM v$instance;
SELECT 'BEFORE_DB='||name||':CDB='||cdb||':OPEN='||open_mode FROM v$database;
SELECT 'BEFORE_PDB='||name||':'||open_mode FROM v$pdbs WHERE name <> 'PDB$SEED' ORDER BY con_id;
EXIT;
SQL

echo "== SHUTDOWN_STARTUP =="
sqlplus -S "/ as sysdba" <<'SQL'
SET ECHO ON
SET TIMING ON
SHUTDOWN IMMEDIATE;
STARTUP;
SET ECHO OFF
SET FEEDBACK OFF
SET PAGESIZE 100
SET LINESIZE 220
SELECT 'AFTER_INSTANCE='||instance_name||':'||status||':'||database_status FROM v$instance;
SELECT 'AFTER_DB='||name||':CDB='||cdb||':OPEN='||open_mode FROM v$database;
ALTER PLUGGABLE DATABASE ALL OPEN;
SELECT 'AFTER_PDB='||name||':'||open_mode FROM v$pdbs WHERE name <> 'PDB$SEED' ORDER BY con_id;
EXIT;
SQL
RC=$?

echo "== FINAL_VERIFY =="
sqlplus -S "/ as sysdba" <<'SQL'
WHENEVER SQLERROR CONTINUE
STARTUP;
ALTER PLUGGABLE DATABASE ALL OPEN;
SET PAGESIZE 100
SET LINESIZE 220
SET FEEDBACK OFF
SELECT 'FINAL_INSTANCE='||instance_name||':'||status||':'||database_status FROM v$instance;
SELECT 'FINAL_DB='||name||':CDB='||cdb||':OPEN='||open_mode FROM v$database;
SELECT 'FINAL_PDB='||name||':'||open_mode FROM v$pdbs WHERE name <> 'PDB$SEED' ORDER BY con_id;
EXIT;
SQL
exit $RC
`
}

func trimOutput(s string, limit int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "\n...<truncated>..."
}
