// Package connector
// File: activation_resolver.go
// Description: 외부 RAG/web/probe를 이용한 activation profile 해석기
// Responsibility: capability syntheses의 입력인 system profile과 command specs 생성

package connector

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
	"github.com/yourorg/infractl/internal/web"
)

// ActivationRequest는 profile synthesis 입력이다.
type ActivationRequest struct {
	ServerName  string
	ServiceType string
	ServiceName string
	Host        string
	Port        int
	Target      string
	OS          string
	EnvProfile  string
	Details     map[string]string
}

// ActivationEvidence는 profile synthesis에 사용된 근거다.
type ActivationEvidence struct {
	Source     string  `json:"source"`
	Title      string  `json:"title,omitempty"`
	URL        string  `json:"url,omitempty"`
	Snippet    string  `json:"snippet,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Category   string  `json:"category,omitempty"`
	ObservedAt string  `json:"observed_at,omitempty"`
}

// ResolvedActivation은 profile, command spec, 질문을 함께 반환한다.
type ResolvedActivation struct {
	Profile     store.SystemProfile
	Commands    map[string]LearnedCommand
	Evidence    []ActivationEvidence
	Questions   []tools.InputQuestion
	Notes       []string
	NeedsInput  bool
	Summary     string
	ServiceKind string
}

// ActivationResolver는 web/rag/probe를 묶어서 profile을 만든다.
type ActivationResolver struct {
	ServerStore  store.ServerStore
	LearnedStore store.LearnedSystemStore
	RAGManager   *rag.Manager
	Fetcher      *web.Fetcher
}

// Resolve는 system profile과 command specs를 생성한다.
func (r *ActivationResolver) Resolve(ctx context.Context, exec executor.Executor, req ActivationRequest) (ResolvedActivation, error) {
	result := ResolvedActivation{}

	server := req.ServerName
	host := req.Host
	port := req.Port
	serviceType := strings.ToLower(strings.TrimSpace(req.ServiceType))
	serviceName := strings.TrimSpace(req.ServiceName)

	if server == "" && exec != nil {
		server = exec.Target()
	}

	if req.Details == nil {
		req.Details = make(map[string]string)
	}

	if host == "" {
		host = req.Details["host"]
	}
	if port == 0 {
		if v := req.Details["port"]; v != "" {
			if p, err := strconv.Atoi(v); err == nil {
				port = p
			}
		}
	}

	if serviceName == "" {
		serviceName = defaultServiceName(serviceType, server)
		result.NeedsInput = true
		result.Questions = append(result.Questions, tools.InputQuestion{
			ID:       "service_name",
			Header:   "Service name",
			Question: fmt.Sprintf("%s 대상의 서비스 이름(service_name)이나 PDB/service alias를 알려주세요. 없으면 기본값으로 진행할 수 있습니다.", strings.Title(serviceType)),
			Default:  serviceName,
		})
	}

	knownServer, _ := r.lookupServer(ctx, server)
	if host == "" && knownServer != nil {
		host = knownServer.Host
	}
	if port == 0 && knownServer != nil {
		port = knownServer.Port
	}
	if req.OS == "" && knownServer != nil {
		req.OS = knownServer.OS
	}
	if req.EnvProfile == "" && knownServer != nil {
		req.EnvProfile = knownServer.EnvProfile
	}

	evidence := r.collectEvidence(ctx, req, exec)
	evidence = append(evidence, r.collectLearnedEvidence(ctx, req)...)

	caps := r.probeCapabilities(ctx, exec, serviceType, req, evidence)
	commands, serviceKind := synthesizeCommands(req, caps, evidence)

	fingerprint := map[string]any{
		"server_name":  server,
		"service_type": serviceType,
		"service_name": serviceName,
		"host":         host,
		"port":         port,
		"os":           req.OS,
		"env_profile":  req.EnvProfile,
		"capabilities": caps,
	}
	fingerprintJSON, _ := json.Marshal(fingerprint)

	profileJSON, _ := json.Marshal(map[string]any{
		"service_kind": serviceKind,
		"service_type": serviceType,
		"service_name": serviceName,
		"host":         host,
		"port":         port,
		"os":           req.OS,
		"env_profile":  req.EnvProfile,
		"capabilities": caps,
		"notes":        result.Notes,
		"evidence":     evidence,
	})
	commandsJSON, _ := json.Marshal(commands)
	evidenceJSON, _ := json.Marshal(evidence)
	questionsJSON, _ := json.Marshal(result.Questions)

	ref := profileRef(server, serviceType, serviceName, host, port, caps, evidence)
	result.Profile = store.SystemProfile{
		ProfileRef:      ref,
		ServerName:      server,
		ServiceType:     serviceType,
		ServiceName:     serviceName,
		Host:            host,
		Port:            port,
		FingerprintJSON: string(fingerprintJSON),
		ProfileJSON:     string(profileJSON),
		CommandsJSON:    string(commandsJSON),
		EvidenceJSON:    string(evidenceJSON),
		QuestionsJSON:   string(questionsJSON),
		Status:          profileStatus(result.NeedsInput),
	}
	result.Commands = commands
	result.Evidence = evidence
	result.ServiceKind = serviceKind
	result.Summary = buildSummary(serviceType, serviceName, serviceKind, caps, result.NeedsInput)
	return result, nil
}

func (r *ActivationResolver) lookupServer(ctx context.Context, name string) (*store.Server, error) {
	if r.ServerStore == nil || strings.TrimSpace(name) == "" {
		return nil, nil
	}
	srv, err := r.ServerStore.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return &srv, nil
}

func (r *ActivationResolver) collectLearnedEvidence(ctx context.Context, req ActivationRequest) []ActivationEvidence {
	if r.LearnedStore == nil || req.ServerName == "" || req.ServiceType == "" {
		return nil
	}
	sys, err := r.LearnedStore.GetLearnedSystem(ctx, req.ServiceType, req.ServerName)
	if err != nil {
		return nil
	}
	return []ActivationEvidence{
		{
			Source:     "learned_systems",
			Title:      fmt.Sprintf("%s @ %s", sys.ServiceType, sys.ServerName),
			Snippet:    summarizeText(sys.Commands, 240),
			Confidence: 0.85,
			Category:   "learned",
			ObservedAt: sys.CreatedAt.Format(time.RFC3339),
		},
	}
}

func (r *ActivationResolver) collectEvidence(ctx context.Context, req ActivationRequest, exec executor.Executor) []ActivationEvidence {
	var evidence []ActivationEvidence

	queryParts := []string{req.ServiceType, req.ServiceName, req.OS, req.EnvProfile}
	query := strings.TrimSpace(strings.Join(filterNonEmpty(queryParts), " "))
	if query == "" {
		query = req.ServiceType
	}
	if query != "" && r.RAGManager != nil {
		if results, err := r.RAGManager.Search(ctx, query+" command line documentation trace tkprof", 5); err == nil {
			for _, res := range results {
				evidence = append(evidence, ActivationEvidence{
					Source:     "rag:" + res.Source,
					Title:      res.Title,
					Snippet:    summarizeText(res.Content, 320),
					Confidence: scoreFromRAG(res),
					Category:   "rag",
				})
			}
		}
	}

	if query != "" {
		webResults, _ := web.Search(ctx, query+" official documentation command line trace tkprof", 5)
		for _, res := range webResults {
			evidence = append(evidence, ActivationEvidence{
				Source:     "web",
				Title:      res.Title,
				URL:        res.URL,
				Snippet:    summarizeText(res.Snippet, 320),
				Confidence: 0.45,
				Category:   "web",
			})
		}
	}

	// Fetch the top web result if it looks promising.
	if r.Fetcher != nil {
		for _, ev := range evidence {
			if ev.Source != "web" || ev.URL == "" {
				continue
			}
			md, err := r.Fetcher.Fetch(ctx, ev.URL)
			if err != nil {
				continue
			}
			ev.Snippet = summarizeText(md, 400)
			ev.Confidence = 0.55
			evidence = append(evidence, ActivationEvidence{
				Source:     "web_fetch",
				Title:      ev.Title,
				URL:        ev.URL,
				Snippet:    summarizeText(md, 600),
				Confidence: 0.6,
				Category:   "web",
				ObservedAt: time.Now().UTC().Format(time.RFC3339),
			})
			break
		}
	}

	// Probe-derived evidence.
	if exec != nil {
		evidence = append(evidence, r.probeEvidence(ctx, exec, req)...)
	}

	return evidence
}

func (r *ActivationResolver) probeEvidence(ctx context.Context, exec executor.Executor, req ActivationRequest) []ActivationEvidence {
	var evidence []ActivationEvidence
	probes := probePlanFor(req.ServiceType)
	for _, p := range probes {
		res, err := exec.Execute(ctx, p.command)
		if err != nil {
			continue
		}
		if p.matcher != nil && !p.matcher(res.Stdout, res.Stderr, res.ExitCode) {
			continue
		}
		evidence = append(evidence, ActivationEvidence{
			Source:     "probe",
			Title:      p.title,
			Snippet:    summarizeText(formatOutput(res.Stdout, res.Stderr, res.ExitCode), 220),
			Confidence: p.confidence,
			Category:   "probe",
			ObservedAt: time.Now().UTC().Format(time.RFC3339),
		})
	}
	return evidence
}

type probeSpec struct {
	title      string
	command    string
	confidence float64
	matcher    func(stdout, stderr string, exitCode int) bool
}

func probePlanFor(serviceType string) []probeSpec {
	switch strings.ToLower(serviceType) {
	case "oracle":
		return []probeSpec{
			{title: "sqlplus available", command: "which sqlplus", confidence: 0.8, matcher: nonEmptyStdout},
			{title: "tkprof available", command: "which tkprof", confidence: 0.7, matcher: nonEmptyStdout},
			{title: "oracle home", command: "echo ${ORACLE_HOME:-}", confidence: 0.5, matcher: nonEmptyStdout},
			{title: "oracle sid", command: "echo ${ORACLE_SID:-}", confidence: 0.4, matcher: nonEmptyStdout},
		}
	case "mysql":
		return []probeSpec{
			{title: "mysql client available", command: "which mysql", confidence: 0.8, matcher: nonEmptyStdout},
			{title: "mysql version", command: "mysql --version", confidence: 0.6, matcher: nonEmptyStdout},
		}
	case "postgresql", "postgres":
		return []probeSpec{
			{title: "psql available", command: "which psql", confidence: 0.8, matcher: nonEmptyStdout},
			{title: "psql version", command: "psql --version", confidence: 0.6, matcher: nonEmptyStdout},
		}
	case "tomcat":
		return []probeSpec{
			{title: "catalina script", command: "which catalina.sh", confidence: 0.5, matcher: nonEmptyStdout},
			{title: "tomcat process", command: "ps -ef | grep -i '[t]omcat\\|[c]atalina' | head -20", confidence: 0.4, matcher: nonEmptyStdout},
		}
	case "weblogic":
		return []probeSpec{
			{title: "weblogic process", command: "ps -ef | grep -i '[w]eblogic' | head -20", confidence: 0.4, matcher: nonEmptyStdout},
		}
	default:
		return []probeSpec{
			{title: "process inventory", command: "ps -ef | head -40", confidence: 0.3, matcher: nonEmptyStdout},
			{title: "os release", command: "cat /etc/os-release 2>/dev/null | head -20", confidence: 0.2, matcher: nonEmptyStdout},
		}
	}
}

func nonEmptyStdout(stdout, _ string, _ int) bool {
	return strings.TrimSpace(stdout) != ""
}

func synthesizeCommands(req ActivationRequest, _ map[string]bool, evidence []ActivationEvidence) (map[string]LearnedCommand, string) {
	serviceType := strings.ToLower(strings.TrimSpace(req.ServiceType))
	switch serviceType {
	case "oracle":
		return synthesizeOracleCommands(req, evidence), "oracle"
	case "mysql":
		return synthesizeMySQLCommands(req, evidence), "mysql"
	case "postgresql", "postgres":
		return synthesizePostgreSQLCommands(req, evidence), "postgresql"
	case "tomcat":
		return synthesizeTomcatCommands(req, evidence), "tomcat"
	case "weblogic":
		return synthesizeWebLogicCommands(req, evidence), "weblogic"
	case "mssql", "sqlserver":
		return synthesizeMSSQLCommands(req, evidence), "generic"
	case "db2", "ibmdb2":
		return synthesizeDB2Commands(req, evidence), "generic"
	case "hana", "saphana":
		return synthesizeHANACommands(req, evidence), "generic"
	case "sybase", "ase":
		return synthesizeSybaseCommands(req, evidence), "generic"
	default:
		return synthesizeGenericCommands(req, evidence), "generic"
	}
}

func synthesizeOracleCommands(req ActivationRequest, evidence []ActivationEvidence) map[string]LearnedCommand {
	serviceName := defaultServiceName("oracle", req.ServerName)
	if req.ServiceName != "" {
		serviceName = req.ServiceName
	}

	host := defaultIfEmpty(req.Host, "{{host}}")
	port := req.Port
	if port == 0 {
		port = 1521
	}

	connExpr := fmt.Sprintf("{{username}}/{{password}}@%s:%d/%s", host, port, serviceName)
	baseQuery := fmt.Sprintf(`tmp_sql=$(mktemp /tmp/infractl_sql_XXXXXX.sql)
printf '%%s\n' {{q:sql}} > "$tmp_sql"
sqlplus -S '%s' @"$tmp_sql"
rm -f "$tmp_sql"`, connExpr)

	statusCmd := fmt.Sprintf(`sqlplus -S '%s' <<'SQLEOF'
SET FEEDBACK OFF
SET PAGESIZE 0
SELECT 'OK' FROM dual;
EXIT;
SQLEOF`, connExpr)

	traceCmd := buildActivationTraceProfileShellCmd("", connExpr, "")

	cmds := map[string]LearnedCommand{
		"status": {
			Name:        "status",
			Description: "Oracle instance health and version",
			Command:     statusCmd,
			ReadOnly:    true,
			Parameters: map[string]interface{}{
				"target": targetParam(),
			},
		},
		"query": {
			Name:        "query",
			Description: "Run Oracle SQL",
			Command:     baseQuery,
			ReadOnly:    false,
			Parameters: map[string]interface{}{
				"sql": map[string]interface{}{
					"type":        "string",
					"description": "SQL to execute inside the Oracle session",
				},
				"target": targetParam(),
			},
		},
		"sessions": {
			Name:        "sessions",
			Description: "List Oracle sessions",
			Command:     fmt.Sprintf("sqlplus -S '%s' <<'SQLEOF'\nSET FEEDBACK OFF\nSET PAGESIZE 100\nSELECT sid, serial#, username, status, machine, program, sql_id, event FROM v$session WHERE type = 'USER' ORDER BY status, username;\nEXIT;\nSQLEOF", connExpr),
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
		"locks": {
			Name:        "locks",
			Description: "List Oracle locks",
			Command:     fmt.Sprintf("sqlplus -S '%s' <<'SQLEOF'\nSET FEEDBACK OFF\nSET PAGESIZE 100\nSELECT s.sid, s.serial#, s.username, l.type, l.lmode, l.request, l.block, s.status, s.machine FROM v$lock l JOIN v$session s ON l.sid = s.sid WHERE l.block = 1 OR l.request > 0 ORDER BY l.block DESC;\nEXIT;\nSQLEOF", connExpr),
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
		"alert_log": {
			Name:        "alert_log",
			Description: "Tail Oracle alert log",
			Command: `oracle_home="{{host}}"
if [ -n "{{oracle_home}}" ]; then oracle_home="{{oracle_home}}"; fi
find "$oracle_home/diag/rdbms" -name 'alert_*.log' 2>/dev/null | head -1 | xargs -I{} tail -n {{lines}} {}`,
			ReadOnly: true,
			Parameters: map[string]interface{}{
				"lines":  map[string]interface{}{"type": "integer", "description": "Number of lines to return", "default": 50},
				"target": targetParam(),
			},
		},
		"top_sql": {
			Name:        "top_sql",
			Description: "Show Oracle top SQL",
			Command:     fmt.Sprintf("sqlplus -S '%s' <<'SQLEOF'\nSET FEEDBACK OFF\nSET PAGESIZE 100\nSELECT * FROM (SELECT sql_id, executions, ROUND(elapsed_time/1000000,2) AS elapsed_sec, ROUND(elapsed_time/GREATEST(executions,1)/1000000,4) AS avg_elapsed, SUBSTR(sql_text,1,100) AS sql_text FROM v$sql WHERE executions > 0 ORDER BY elapsed_time DESC) WHERE ROWNUM <= 20;\nEXIT;\nSQLEOF", connExpr),
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
	}

	if hasCapability(evidence, "tkprof") || hasCapability(evidence, "trace") {
		cmds["trace_profile"] = LearnedCommand{
			Name:        "trace_profile",
			Description: "Run Oracle workload with trace enabled and return TKPROF artifacts",
			Command:     traceCmd,
			ReadOnly:    false,
			Parameters: map[string]interface{}{
				"sql": map[string]interface{}{
					"type":        "string",
					"description": "Workload SQL or SQL*Plus script to trace",
				},
				"trace_id": map[string]interface{}{
					"type":        "string",
					"description": "Trace file identifier",
				},
				"target": targetParam(),
			},
			Required: []string{"sql"},
		}
	}

	return cmds
}

func buildActivationTraceProfileShellCmd(sid, connStr, pdb string) string {
	var sb strings.Builder
	sb.WriteString("set -euo pipefail\n")
	sb.WriteString("tmp_dir=$(mktemp -d /tmp/infractl_trace_XXXXXX)\n")
	sb.WriteString("sql_file=\"$tmp_dir/work.sql\"\n")
	sb.WriteString("driver_file=\"$tmp_dir/driver.sql\"\n")
	sb.WriteString("out_file=\"$tmp_dir/sqlplus.out\"\n")
	sb.WriteString("tkprof_file=\"$tmp_dir/tkprof.txt\"\n")
	sb.WriteString("trace_id=\"{{q:trace_id}}\"\n")
	sb.WriteString("if [ -z \"$trace_id\" ]; then trace_id=\"INFRACTL_TRACE_$(date +%s)\"; fi\n")
	sb.WriteString("cat > \"$sql_file\" <<'SQLPAYLOAD'\n")
	sb.WriteString("{{q:sql}}\n")
	sb.WriteString("SQLPAYLOAD\n")
	sb.WriteString("{\n")
	sb.WriteString("  printf 'SET FEEDBACK OFF\\n'\n")
	sb.WriteString("  printf 'SET PAGESIZE 0\\n'\n")
	sb.WriteString("  printf 'SET LINESIZE 220\\n'\n")
	if strings.TrimSpace(pdb) != "" {
		sb.WriteString("  printf 'ALTER SESSION SET CONTAINER = " + quoteOracleIdentifier(pdb) + ";\\n'\n")
	}
	sb.WriteString("  printf 'ALTER SESSION SET tracefile_identifier=%s;\\n' \"'$trace_id'\"\n")
	sb.WriteString("  printf 'BEGIN DBMS_MONITOR.SESSION_TRACE_ENABLE(waits=>TRUE, binds=>TRUE); END;\\n'\n")
	sb.WriteString("  printf '/\\n'\n")
	sb.WriteString("  printf '@\"%s\"\\n' \"$sql_file\"\n")
	sb.WriteString("  printf 'BEGIN DBMS_MONITOR.SESSION_TRACE_DISABLE; END;\\n'\n")
	sb.WriteString("  printf '/\\n'\n")
	sb.WriteString("  printf \"SELECT 'TRACE_FILE=' || value FROM v\\$diag_info WHERE name='Default Trace File';\\n\"\n")
	sb.WriteString("  printf 'EXIT;\\n'\n")
	sb.WriteString("} > \"$driver_file\"\n")
	sqlplusPrefix := ""
	if strings.TrimSpace(sid) != "" {
		sqlplusPrefix = "ORACLE_SID=" + quoteShellValue(sid) + " "
	}
	sb.WriteString(sqlplusPrefix + "sqlplus -S " + quoteShellValue(connStr) + " @\"$driver_file\" > \"$out_file\"\n")
	sb.WriteString("cat \"$out_file\"\n")
	sb.WriteString("trace_path=$(awk -F= '/^TRACE_FILE=/{print $2}' \"$out_file\" | tail -1)\n")
	sb.WriteString("if [ -z \"$trace_path\" ] || [ ! -r \"$trace_path\" ]; then echo \"TRACE_FILE_NOT_READABLE=$trace_path\"; exit 2; fi\n")
	sb.WriteString("tkprof \"$trace_path\" \"$tkprof_file\" sys=no aggregate=no sort=exeela,fchela,prsela >/dev/null\n")
	sb.WriteString("echo \"TRACE_FILE=$trace_path\"\n")
	sb.WriteString("echo \"TKPROF_FILE=$tkprof_file\"\n")
	sb.WriteString("sed -n '1,220p' \"$tkprof_file\"\n")
	return sb.String()
}

func quoteShellValue(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func quoteOracleIdentifier(id string) string {
	return `"` + strings.ReplaceAll(id, `"`, `""`) + `"`
}

func synthesizeMySQLCommands(req ActivationRequest, evidence []ActivationEvidence) map[string]LearnedCommand {
	host := defaultIfEmpty(req.Host, "{{host}}")
	port := req.Port
	if port == 0 {
		port = 3306
	}
	connExpr := fmt.Sprintf("MYSQL_PWD={{q:password}} mysql -u {{q:username}} -h %s -P %d --batch --silent", host, port)

	return map[string]LearnedCommand{
		"query": {
			Name:        "query",
			Description: "Run MySQL SQL",
			Command: fmt.Sprintf(`tmp_sql=$(mktemp /tmp/infractl_sql_XXXXXX.sql)
printf '%%s\n' {{q:sql}} > "$tmp_sql"
%s -e "source $tmp_sql"
rm -f "$tmp_sql"`, connExpr),
			ReadOnly: true,
			Parameters: map[string]interface{}{
				"sql":    map[string]interface{}{"type": "string", "description": "SQL to execute"},
				"target": targetParam(),
			},
			Required: []string{"sql"},
		},
		"status": {
			Name:        "status",
			Description: "Show MySQL status",
			Command:     fmt.Sprintf(`%s -e "SHOW GLOBAL STATUS LIKE 'Threads_%%'; SHOW GLOBAL STATUS LIKE 'Uptime';"`, connExpr),
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
		"processlist": {
			Name:        "processlist",
			Description: "Show MySQL processlist",
			Command:     fmt.Sprintf(`%s -e "SHOW FULL PROCESSLIST;"`, connExpr),
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
		"databases": {
			Name:        "databases",
			Description: "List MySQL databases",
			Command:     fmt.Sprintf(`%s -e "SELECT table_schema AS DatabaseName, ROUND(SUM(data_length+index_length)/1024/1024,1) AS MB FROM information_schema.tables GROUP BY table_schema ORDER BY MB DESC;"`, connExpr),
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
	}
}

func synthesizePostgreSQLCommands(req ActivationRequest, evidence []ActivationEvidence) map[string]LearnedCommand {
	serviceName := defaultServiceName("postgresql", req.ServerName)
	if req.ServiceName != "" {
		serviceName = req.ServiceName
	}
	host := defaultIfEmpty(req.Host, "{{host}}")
	port := req.Port
	if port == 0 {
		port = 5432
	}
	connExpr := fmt.Sprintf("PGPASSWORD={{q:password}} psql -U {{q:username}} -h %s -p %d -d {{q:database}} -At -F ' | '", host, port)

	return map[string]LearnedCommand{
		"query": {
			Name:        "query",
			Description: "Run PostgreSQL SQL",
			Command: fmt.Sprintf(`tmp_sql=$(mktemp /tmp/infractl_sql_XXXXXX.sql)
printf '%%s\n' {{q:sql}} > "$tmp_sql"
%s -f "$tmp_sql"
rm -f "$tmp_sql"`, connExpr),
			ReadOnly: true,
			Parameters: map[string]interface{}{
				"sql":      map[string]interface{}{"type": "string", "description": "SQL to execute"},
				"database": map[string]interface{}{"type": "string", "description": "Database name", "default": serviceName},
				"target":   targetParam(),
			},
			Required: []string{"sql"},
		},
		"connections": {
			Name:        "connections",
			Description: "Show PostgreSQL connections",
			Command:     fmt.Sprintf(`%s -d postgres -c "SELECT pid, usename, datname, state, wait_event_type, wait_event, LEFT(query,80) AS query FROM pg_stat_activity WHERE state IS NOT NULL ORDER BY state;"`, connExpr),
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
		"locks": {
			Name:        "locks",
			Description: "Show PostgreSQL locks",
			Command:     fmt.Sprintf(`%s -d postgres -c "SELECT l.pid, l.mode, l.granted, a.usename, a.state, LEFT(a.query,80) FROM pg_locks l JOIN pg_stat_activity a ON l.pid = a.pid WHERE NOT l.granted OR l.pid IN (SELECT blocking_pid FROM pg_stat_activity WHERE cardinality(pg_blocking_pids(pid)) > 0) ORDER BY l.granted;"`, connExpr),
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
		"databases": {
			Name:        "databases",
			Description: "List PostgreSQL databases",
			Command:     fmt.Sprintf(`%s -d postgres -c "SELECT datname, pg_size_pretty(pg_database_size(datname)) AS size FROM pg_database ORDER BY pg_database_size(datname) DESC;"`, connExpr),
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
	}
}

func synthesizeTomcatCommands(req ActivationRequest, evidence []ActivationEvidence) map[string]LearnedCommand {
	return map[string]LearnedCommand{
		"status": {
			Name:        "status",
			Description: "Show Tomcat status",
			Command:     `ps -ef | grep -i '[t]omcat\|[c]atalina' | head -20`,
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
		"logs": {
			Name:        "logs",
			Description: "Tail Tomcat logs",
			Command:     `if [ -n "{{log_path}}" ] && [ -d "{{log_path}}" ]; then find "{{log_path}}" -name '*.log' -o -name 'catalina*.out' 2>/dev/null | head -3 | xargs -I{} tail -n {{lines}} {}; else ps -ef | grep -i '[t]omcat\|[c]atalina' | head -20; fi`,
			ReadOnly:    true,
			Parameters: map[string]interface{}{
				"lines":  map[string]interface{}{"type": "integer", "default": 80},
				"target": targetParam(),
			},
		},
		"restart": {
			Name:        "restart",
			Description: "Restart Tomcat",
			Command:     `systemctl restart {{service_name}} || ({{catalina_home}}/bin/shutdown.sh && {{catalina_home}}/bin/startup.sh)`,
			ReadOnly:    false,
			Parameters: map[string]interface{}{
				"target": targetParam(),
			},
		},
	}
}

func synthesizeWebLogicCommands(req ActivationRequest, evidence []ActivationEvidence) map[string]LearnedCommand {
	return map[string]LearnedCommand{
		"status": {
			Name:        "status",
			Description: "Show WebLogic status",
			Command:     `ps -ef | grep -i '[w]eblogic' | head -20`,
			ReadOnly:    true,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
		"logs": {
			Name:        "logs",
			Description: "Tail WebLogic logs",
			Command:     `if [ -n "{{log_path}}" ] && [ -d "{{log_path}}" ]; then find "{{log_path}}" -name '*.log' 2>/dev/null | head -3 | xargs -I{} tail -n {{lines}} {}; else ps -ef | grep -i '[w]eblogic' | head -20; fi`,
			ReadOnly:    true,
			Parameters: map[string]interface{}{
				"lines":  map[string]interface{}{"type": "integer", "default": 80},
				"target": targetParam(),
			},
		},
		"restart": {
			Name:        "restart",
			Description: "Restart WebLogic",
			Command:     `systemctl restart {{service_name}}`,
			ReadOnly:    false,
			Parameters:  map[string]interface{}{"target": targetParam()},
		},
	}
}

func synthesizeGenericCommands(req ActivationRequest, evidence []ActivationEvidence) map[string]LearnedCommand {
	return map[string]LearnedCommand{
		"status": {
			Name:        "status",
			Description: "Inspect the target system",
			Command:     `uname -a && ps -ef | head -20`,
			ReadOnly:    true,
			Parameters:  map[string]any{"target": targetParam()},
		},
		"logs": {
			Name:        "logs",
			Description: "Inspect likely log files",
			Command:     `if [ -n "{{log_path}}" ] && [ -d "{{log_path}}" ]; then find "{{log_path}}" -maxdepth 1 -type f | head -5 | xargs -r tail -n {{lines}}; else ps -ef | head -20; fi`,
			ReadOnly:    true,
			Parameters: map[string]any{
				"lines":  map[string]any{"type": "integer", "default": 50},
				"target": targetParam(),
			},
		},
	}
}

func synthesizeMSSQLCommands(req ActivationRequest, _ []ActivationEvidence) map[string]LearnedCommand {
	host := defaultIfEmpty(req.Host, "{{host}}")
	port := req.Port
	if port == 0 {
		port = 1433
	}
	portStr := fmt.Sprintf("%d", port)
	schemaCheck := fmt.Sprintf(
		`sqlcmd -S %s,%s -U {{username}} -P {{password}} -Q "SELECT COLUMN_NAME, DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME='{{table_name}}' ORDER BY ORDINAL_POSITION" -W 2>&1`,
		host, portStr,
	)
	return map[string]LearnedCommand{
		"query": {
			Name:               "query",
			Description:        "MSSQL SQL 실행 (sqlcmd). SELECT/DML 모두 가능.",
			Command:            fmt.Sprintf(`sqlcmd -S %s,%s -U {{username}} -P {{password}} -d {{database}} -Q "{{raw:sql}}" -W -s "|" 2>&1`, host, portStr),
			ReadOnly:           false,
			IsSQLQuery:         true,
			SchemaCheckCommand: schemaCheck,
			Parameters: map[string]any{
				"sql":      map[string]any{"type": "string", "description": "실행할 SQL 문"},
				"database": map[string]any{"type": "string", "description": "대상 데이터베이스 이름"},
				"target":   targetParam(),
			},
			Required: []string{"sql"},
		},
		"status": {
			Name:        "status",
			Description: "MSSQL 서버 버전 및 상태 확인.",
			Command:     fmt.Sprintf(`sqlcmd -S %s,%s -U {{username}} -P {{password}} -Q "SELECT @@VERSION; SELECT name, state_desc FROM sys.databases ORDER BY name;" -W 2>&1`, host, portStr),
			ReadOnly:    true,
			Parameters:  map[string]any{"target": targetParam()},
		},
		"processlist": {
			Name:        "processlist",
			Description: "MSSQL 활성 세션 목록 조회 (sys.dm_exec_sessions).",
			Command:     fmt.Sprintf(`sqlcmd -S %s,%s -U {{username}} -P {{password}} -Q "SELECT session_id, login_name, status, host_name, program_name, LEFT(text,80) AS query FROM sys.dm_exec_sessions s OUTER APPLY sys.dm_exec_sql_text(s.most_recent_sql_handle) WHERE is_user_process=1 ORDER BY session_id;" -W 2>&1`, host, portStr),
			ReadOnly:    true,
			Parameters:  map[string]any{"target": targetParam()},
		},
	}
}

func synthesizeDB2Commands(req ActivationRequest, _ []ActivationEvidence) map[string]LearnedCommand {
	dbName := defaultIfEmpty(req.ServiceName, "SAMPLE")
	schemaCheck := fmt.Sprintf(
		`db2 "CONNECT TO %s USER {{username}} USING '{{raw:password}}'" && db2 "SELECT COLNAME, TYPENAME FROM SYSCAT.COLUMNS WHERE TABNAME=UPPER('{{table_name}}') ORDER BY COLNO" && db2 TERMINATE 2>&1`,
		dbName,
	)
	return map[string]LearnedCommand{
		"query": {
			Name:               "query",
			Description:        "DB2 SQL 실행. SELECT/DML 모두 가능.",
			Command:            fmt.Sprintf(`db2 "CONNECT TO %s USER {{username}} USING '{{raw:password}}'" && db2 "{{raw:sql}}" && db2 TERMINATE 2>&1`, dbName),
			ReadOnly:           false,
			IsSQLQuery:         true,
			SchemaCheckCommand: schemaCheck,
			Parameters: map[string]any{
				"sql":    map[string]any{"type": "string", "description": "실행할 SQL 문"},
				"target": targetParam(),
			},
			Required: []string{"sql"},
		},
		"status": {
			Name:        "status",
			Description: "DB2 인스턴스 상태 및 버전 확인.",
			Command:     `db2 get dbm cfg | grep -E "^(DB2|Instance)" | head -10 && db2level 2>&1`,
			ReadOnly:    true,
			Parameters:  map[string]any{"target": targetParam()},
		},
		"tablespaces": {
			Name:        "tablespaces",
			Description: "DB2 테이블스페이스 사용량 조회.",
			Command:     fmt.Sprintf(`db2 "CONNECT TO %s USER {{username}} USING '{{raw:password}}'" && db2 "SELECT TBSPACE, TBSPACETYPE, DATASIZE/1024 AS SIZE_MB, USEDSIZE/1024 AS USED_MB FROM SYSCAT.TABLESPACES ORDER BY SIZE_MB DESC" && db2 TERMINATE 2>&1`, dbName),
			ReadOnly:    true,
			Parameters:  map[string]any{"target": targetParam()},
		},
	}
}

func synthesizeHANACommands(req ActivationRequest, _ []ActivationEvidence) map[string]LearnedCommand {
	host := defaultIfEmpty(req.Host, "{{host}}")
	port := req.Port
	if port == 0 {
		port = 30015
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	schemaCheck := fmt.Sprintf(
		`hdbsql -n %s -u {{username}} -p {{password}} "SELECT COLUMN_NAME, DATA_TYPE_NAME FROM SYS.TABLE_COLUMNS WHERE TABLE_NAME='{{table_name}}' ORDER BY POSITION" 2>&1`,
		addr,
	)
	return map[string]LearnedCommand{
		"query": {
			Name:               "query",
			Description:        "SAP HANA SQL 실행 (hdbsql). SELECT/DML 모두 가능.",
			Command:            fmt.Sprintf(`hdbsql -n %s -u {{username}} -p {{password}} "{{raw:sql}}" 2>&1`, addr),
			ReadOnly:           false,
			IsSQLQuery:         true,
			SchemaCheckCommand: schemaCheck,
			Parameters: map[string]any{
				"sql":    map[string]any{"type": "string", "description": "실행할 SQL 문"},
				"target": targetParam(),
			},
			Required: []string{"sql"},
		},
		"status": {
			Name:        "status",
			Description: "SAP HANA 서버 버전 및 서비스 상태 확인.",
			Command:     fmt.Sprintf(`hdbsql -n %s -u {{username}} -p {{password}} "SELECT VERSION FROM SYS.M_DATABASE; SELECT HOST, SERVICE_NAME, PORT, ACTIVE_STATUS FROM SYS.M_SERVICES ORDER BY SERVICE_NAME;" 2>&1`, addr),
			ReadOnly:    true,
			Parameters:  map[string]any{"target": targetParam()},
		},
		"services": {
			Name:        "services",
			Description: "SAP HANA 메모리 및 CPU 사용량 조회.",
			Command:     fmt.Sprintf(`hdbsql -n %s -u {{username}} -p {{password}} "SELECT SERVICE_NAME, ROUND(PROCESS_MEMORY_USED/1024/1024,0) AS MEM_MB, ROUND(PROCESS_CPU/100.0,1) AS CPU_PCT FROM SYS.M_SERVICE_RESOURCE_UTILIZATION ORDER BY MEM_MB DESC;" 2>&1`, addr),
			ReadOnly:    true,
			Parameters:  map[string]any{"target": targetParam()},
		},
	}
}

func synthesizeSybaseCommands(req ActivationRequest, _ []ActivationEvidence) map[string]LearnedCommand {
	server := defaultIfEmpty(req.Host, req.ServerName)
	schemaCheck := fmt.Sprintf(
		"isql -S %s -U {{username}} -P {{password}} -w 512 -e <<'EOSQL'\nSELECT c.name, t.name FROM syscolumns c JOIN systypes t ON c.usertype=t.usertype JOIN sysobjects o ON c.id=o.id WHERE o.name='{{table_name}}' ORDER BY c.colid\ngo\nEOSQL",
		server,
	)
	return map[string]LearnedCommand{
		"query": {
			Name:               "query",
			Description:        "Sybase ASE SQL 실행 (isql). SELECT/DML 모두 가능.",
			Command:            fmt.Sprintf("isql -S %s -U {{username}} -P {{password}} -w 512 -e <<'EOSQL'\n{{raw:sql}}\ngo\nEOSQL", server),
			ReadOnly:           false,
			IsSQLQuery:         true,
			SchemaCheckCommand: schemaCheck,
			Parameters: map[string]any{
				"sql":    map[string]any{"type": "string", "description": "실행할 SQL 문"},
				"target": targetParam(),
			},
			Required: []string{"sql"},
		},
		"status": {
			Name:        "status",
			Description: "Sybase ASE 버전 및 서버 상태 확인.",
			Command:     fmt.Sprintf("isql -S %s -U {{username}} -P {{password}} -w 512 -e <<'EOSQL'\nselect @@version\ngo\nselect name, dbid from sysdatabases order by name\ngo\nEOSQL", server),
			ReadOnly:    true,
			Parameters:  map[string]any{"target": targetParam()},
		},
		"processlist": {
			Name:        "processlist",
			Description: "Sybase ASE 활성 프로세스 목록 조회 (sp_who).",
			Command:     fmt.Sprintf("isql -S %s -U {{username}} -P {{password}} -w 512 -e <<'EOSQL'\nexec sp_who\ngo\nEOSQL", server),
			ReadOnly:    true,
			Parameters:  map[string]any{"target": targetParam()},
		},
	}
}

func buildSummary(serviceType, serviceName, serviceKind string, caps map[string]bool, needsInput bool) string {
	var capNames []string
	for name, ok := range caps {
		if ok {
			capNames = append(capNames, name)
		}
	}
	sort.Strings(capNames)
	summary := fmt.Sprintf("%s/%s synthesized as %s", serviceType, serviceName, serviceKind)
	if len(capNames) > 0 {
		summary += fmt.Sprintf(" with capabilities: %s", strings.Join(capNames, ", "))
	}
	if needsInput {
		summary += " (needs input)"
	}
	return summary
}

func profileRef(server, serviceType, serviceName, host string, port int, caps map[string]bool, evidence []ActivationEvidence) string {
	h := sha256.New()
	_, _ = h.Write([]byte(strings.ToLower(server)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.ToLower(serviceType)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.ToLower(serviceName)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.ToLower(host)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strconv.Itoa(port)))

	var keys []string
	for k, ok := range caps {
		if ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = h.Write([]byte("|" + k))
	}
	for _, ev := range evidence {
		_, _ = h.Write([]byte("|" + ev.Source + ":" + ev.Title))
	}

	sum := h.Sum(nil)
	return fmt.Sprintf("%s_%s", sanitizeToolName(serviceType+"_"+serviceName), fmt.Sprintf("%x", sum[:8]))
}

func profileStatus(needsInput bool) string {
	if needsInput {
		return "needs_input"
	}
	return "prepared"
}

func defaultServiceName(serviceType, server string) string {
	base := strings.TrimSpace(serviceType)
	if base == "" {
		base = "service"
	}
	if server == "" {
		return sanitizeToolName(base)
	}
	return sanitizeToolName(base + "_" + server)
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func summarizeText(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

func filterNonEmpty(values []string) []string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

func scoreFromRAG(res rag.SearchResult) float64 {
	score := float64(res.Score)
	if score <= 0 {
		score = 0.5
	}
	if res.Priority > 0 {
		score += float64(res.Priority) * 0.05
	}
	return score
}

func hasCapability(evidence []ActivationEvidence, keyword string) bool {
	key := strings.ToLower(strings.TrimSpace(keyword))
	for _, ev := range evidence {
		text := strings.ToLower(ev.Title + " " + ev.Snippet + " " + ev.Source + " " + ev.Category)
		if strings.Contains(text, key) {
			return true
		}
	}
	return false
}
