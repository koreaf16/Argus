// Package oracle
// File: advanced_tools.go
// Description: oracle 모듈의 기능 수행.
// Responsibility: oracle 관련 로직 처리 및 관리.

package oracle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

type tuningProposal struct {
	ID        string
	SQL       string
	Summary   string
	Evidence  string
	SQLHash   string
	CreatedAt time.Time
}

func (c *OracleConnector) makeTraceProfileTool(prefix, sid, connStr, pdb string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".trace_profile",
		fmt.Sprintf("Oracle(%s) runs trace + tkprof in one session.", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sql":         map[string]interface{}{"type": "string", "description": "workload SQL to trace"},
				"trace_id":    map[string]interface{}{"type": "string", "description": "tracefile identifier"},
				"tkprof_sort": map[string]interface{}{"type": "string", "description": "tkprof sort keys"},
				"target":      targetParam(),
			},
			"required": []string{"sql"},
		},
		false, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			sql, _ := args["sql"].(string)
			if strings.TrimSpace(sql) == "" {
				return "sql parameter is required", nil
			}
			traceID, _ := args["trace_id"].(string)
			tkSort, _ := args["tkprof_sort"].(string)

			cmd := buildTraceProfileShellCmd(sid, connStr, pdb, sql, traceID, tkSort)
			res, err := execWithStream(ctx, exec, cmd)
			if err != nil {
				return fmt.Sprintf("execution error: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *OracleConnector) makeSQLProfileTool(prefix, sid, connStr, pdb string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".sql_profile",
		fmt.Sprintf("Oracle(%s) fetches SQL stats and DISPLAY_CURSOR plan.", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sql_id":       map[string]interface{}{"type": "string", "description": "SQL_ID"},
				"child_number": map[string]interface{}{"type": "integer", "description": "optional child number"},
				"include_text": map[string]interface{}{"type": "boolean", "description": "include sql_text in summary"},
				"target":       targetParam(),
			},
			"required": []string{"sql_id"},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			sqlID, _ := args["sql_id"].(string)
			if strings.TrimSpace(sqlID) == "" {
				return "sql_id parameter is required", nil
			}
			child := -1
			if v, ok := args["child_number"].(float64); ok {
				child = int(v)
			}
			includeText := true
			if v, ok := args["include_text"].(bool); ok {
				includeText = v
			}

			query := "PROMPT -- SQL SUMMARY\n" +
				buildSQLProfileSummaryQuery(sqlID, child, includeText) +
				"\nPROMPT -- SQL PLAN\n" +
				buildSQLProfilePlanQuery(sqlID, child)
			cmd := buildSQLPlusCmd(sid, connStr, query, pdb)
			res, err := execWithStream(ctx, exec, cmd)
			if err != nil {
				return fmt.Sprintf("execution error: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *OracleConnector) makeHealthCheckTool(prefix, sid, connStr, pdb, oracleHome string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".health_check",
		fmt.Sprintf("Oracle(%s) instance/database/pdb/tablespace/session/lock health summary.", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"alert_lines": map[string]interface{}{"type": "integer", "description": "alert log lines to scan (default 200)"},
				"target":      targetParam(),
			},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			sqlCmd := buildSQLPlusCmd(sid, connStr, buildHealthCheckQuery(), pdb)
			sqlRes, err := execWithStream(ctx, exec, sqlCmd)
			if err != nil {
				return fmt.Sprintf("execution error: %s", err), nil
			}
			output := formatOutput(sqlRes.Stdout, sqlRes.Stderr, sqlRes.ExitCode)

			lines := 200
			if v, ok := args["alert_lines"].(float64); ok {
				lines = int(v)
			}
			home := oracleHome
			if strings.TrimSpace(home) == "" {
				home = "/u01/app/oracle/product"
			}
			alertSummaryCmd := fmt.Sprintf(
				`alert_file=$(find %s/diag/rdbms -name 'alert_%s.log' 2>/dev/null | head -1); if [ -n "$alert_file" ]; then echo "-- ALERT ORA SUMMARY"; tail -n %d "$alert_file" | grep -Eo 'ORA-[0-9]+' | sort | uniq -c || true; else echo "-- ALERT ORA SUMMARY"; echo "(alert log not found)"; fi`,
				home, sid, lines,
			)
			alertRes, alertErr := execWithStream(ctx, exec, alertSummaryCmd)
			if alertErr != nil {
				return output + "\n\n-- ALERT ORA SUMMARY\n(error reading alert log: " + alertErr.Error() + ")", nil
			}
			return output + "\n\n" + strings.TrimSpace(alertRes.Stdout), nil
		},
	)
}

func (c *OracleConnector) makeAWRReportTool(prefix, sid, connStr, pdb string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".awr_report",
		fmt.Sprintf("Oracle(%s) AWR report (requires Diagnostics Pack confirmation).", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"begin_snap":                 map[string]interface{}{"type": "integer", "description": "begin snap id"},
				"end_snap":                   map[string]interface{}{"type": "integer", "description": "end snap id"},
				"report_type":                map[string]interface{}{"type": "string", "description": "text|html"},
				"diagnostics_pack_confirmed": map[string]interface{}{"type": "boolean", "description": "must be true to run"},
				"target":                     targetParam(),
			},
			"required": []string{"begin_snap", "end_snap", "diagnostics_pack_confirmed"},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			confirmed, _ := args["diagnostics_pack_confirmed"].(bool)
			if !confirmed {
				return "blocked: diagnostics_pack_confirmed=true is required for AWR/ASH features", nil
			}
			beginSnap, bok := args["begin_snap"].(float64)
			endSnap, eok := args["end_snap"].(float64)
			if !bok || !eok {
				return "begin_snap and end_snap are required", nil
			}
			beginID := int(beginSnap)
			endID := int(endSnap)
			if beginID <= 0 || endID <= 0 || beginID >= endID {
				return "invalid snap range: begin_snap and end_snap must be positive and begin < end", nil
			}
			reportType, _ := args["report_type"].(string)
			cmd := buildSQLPlusCmd(sid, connStr, buildAWRReportQuery(beginID, endID, reportType), pdb)
			res, err := execWithStream(ctx, exec, cmd)
			if err != nil {
				return fmt.Sprintf("execution error: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *OracleConnector) makeASHReportTool(prefix, sid, connStr, pdb string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".ash_report",
		fmt.Sprintf("Oracle(%s) ASH summary (requires Diagnostics Pack confirmation).", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"minutes":                    map[string]interface{}{"type": "integer", "description": "lookback minutes (default 30)"},
				"diagnostics_pack_confirmed": map[string]interface{}{"type": "boolean", "description": "must be true to run"},
				"target":                     targetParam(),
			},
			"required": []string{"diagnostics_pack_confirmed"},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			confirmed, _ := args["diagnostics_pack_confirmed"].(bool)
			if !confirmed {
				return "blocked: diagnostics_pack_confirmed=true is required for AWR/ASH features", nil
			}
			minutes := 30
			if v, ok := args["minutes"].(float64); ok {
				minutes = int(v)
			}
			cmd := buildSQLPlusCmd(sid, connStr, buildASHSummaryQuery(minutes), pdb)
			res, err := execWithStream(ctx, exec, cmd)
			if err != nil {
				return fmt.Sprintf("execution error: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *OracleConnector) makeTuningRecommendTool(prefix, sid, connStr, pdb string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".tuning_recommend",
		fmt.Sprintf("Oracle(%s) generates tuning recommendation and proposal_id.", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sql_id":       map[string]interface{}{"type": "string", "description": "SQL_ID source"},
				"child_number": map[string]interface{}{"type": "integer", "description": "optional child number"},
				"tkprof_text":  map[string]interface{}{"type": "string", "description": "raw tkprof text"},
				"tkprof_file":  map[string]interface{}{"type": "string", "description": "remote tkprof file path"},
				"strategy":     map[string]interface{}{"type": "string", "description": "index|stats|rewrite|all"},
				"target":       targetParam(),
			},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			sqlID, _ := args["sql_id"].(string)
			tkprofText, _ := args["tkprof_text"].(string)
			tkprofFile, _ := args["tkprof_file"].(string)
			strategy, _ := args["strategy"].(string)
			if strings.TrimSpace(strategy) == "" {
				strategy = "all"
			}
			child := -1
			if v, ok := args["child_number"].(float64); ok {
				child = int(v)
			}
			if strings.TrimSpace(sqlID) == "" && strings.TrimSpace(tkprofText) == "" && strings.TrimSpace(tkprofFile) == "" {
				return "one of sql_id, tkprof_text, or tkprof_file is required", nil
			}

			var diagParts []string
			var fullScanObjects []string
			if strings.TrimSpace(sqlID) != "" {
				profileQuery := "PROMPT -- SQL SUMMARY\n" +
					buildSQLProfileSummaryQuery(sqlID, child, true) +
					"\nPROMPT -- SQL PLAN\n" +
					buildSQLProfilePlanQuery(sqlID, child)
				profileCmd := buildSQLPlusCmd(sid, connStr, profileQuery, pdb)
				profileRes, err := execWithStream(ctx, exec, profileCmd)
				if err == nil {
					profileOut := formatOutput(profileRes.Stdout, profileRes.Stderr, profileRes.ExitCode)
					diagParts = append(diagParts, profileOut)
					fullScanObjects = append(fullScanObjects, extractFullScanObjectsFromText(profileOut)...)
				}

				objQuery := "SET HEADING OFF\n" + buildFullScanObjectsQuery(sqlID, child)
				objCmd := buildSQLPlusCmd(sid, connStr, objQuery, pdb)
				objRes, err := execWithStream(ctx, exec, objCmd)
				if err == nil {
					fullScanObjects = append(fullScanObjects, parseObjectRows(objRes.Stdout)...)
				}
			}

			if strings.TrimSpace(tkprofText) == "" && strings.TrimSpace(tkprofFile) != "" {
				readCmd := fmt.Sprintf("if [ -r %s ]; then cat %s; else echo 'FILE_NOT_READABLE'; fi", shellQuotePOSIX(tkprofFile), shellQuotePOSIX(tkprofFile))
				readRes, err := execWithStream(ctx, exec, readCmd)
				if err == nil {
					tkprofText = readRes.Stdout
				}
			}
			if strings.TrimSpace(tkprofText) != "" {
				diagParts = append(diagParts, tkprofText)
				fullScanObjects = append(fullScanObjects, extractFullScanObjectsFromText(tkprofText)...)
			}
			fullScanObjects = uniqueStrings(fullScanObjects)

			var recommendations []string
			var proposalSQL []string
			useIndex := strategy == "all" || strategy == "index"
			useStats := strategy == "all" || strategy == "stats"
			useRewrite := strategy == "all" || strategy == "rewrite"

			for _, obj := range fullScanObjects {
				owner, table := splitOwnerTable(obj)
				if owner == "" || table == "" {
					continue
				}
				if useIndex {
					col, err := c.pickCandidateColumn(ctx, sid, connStr, pdb, owner, table, exec)
					if err == nil && col != "" {
						idx := buildIndexName(table, col)
						sql := fmt.Sprintf("CREATE INDEX %s ON %s.%s(%s);", idx, owner, table, col)
						proposalSQL = append(proposalSQL, sql)
						recommendations = append(recommendations, fmt.Sprintf("index candidate: %s.%s(%s) to reduce FULL SCAN", owner, table, col))
					}
				}
				if useStats {
					sql := fmt.Sprintf("BEGIN DBMS_STATS.GATHER_TABLE_STATS('%s','%s', cascade=>TRUE); END;", owner, table)
					proposalSQL = append(proposalSQL, sql)
					recommendations = append(recommendations, fmt.Sprintf("stats refresh: DBMS_STATS for %s.%s", owner, table))
				}
			}

			if useRewrite {
				recommendations = append(recommendations, "rewrite check: review predicates and bind selectivity for the SQL text")
			}
			if len(recommendations) == 0 {
				recommendations = append(recommendations, "no concrete FULL SCAN object found; capture additional trace/tkprof with target workload")
			}

			var sb strings.Builder
			sb.WriteString("-- TUNING RECOMMENDATIONS\n")
			for i, rec := range recommendations {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
			}
			if len(diagParts) > 0 {
				sb.WriteString("\n-- DIAGNOSTIC EXCERPT\n")
				sb.WriteString(trimLarge(strings.Join(diagParts, "\n\n"), 3000))
			}

			if len(proposalSQL) == 0 {
				sb.WriteString("\n\n(no executable proposal generated)")
				return sb.String(), nil
			}
			sql := strings.Join(uniqueStrings(proposalSQL), "\n")
			proposal := c.saveProposal(sql, strings.Join(recommendations, "; "), trimLarge(strings.Join(diagParts, "\n\n"), 1200))
			sb.WriteString("\n\n-- PROPOSAL_ID\n")
			sb.WriteString(proposal.ID)
			sb.WriteString("\n\n-- APPLY_SQL\n")
			sb.WriteString(sql)
			sb.WriteString("\n\nUse tuning_apply with confirmed=true and the same SQL text.")
			return sb.String(), nil
		},
	)
}

func (c *OracleConnector) makeTuningApplyTool(prefix, sid, connStr, pdb string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".tuning_apply",
		fmt.Sprintf("Oracle(%s) applies approved tuning SQL by proposal_id.", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"proposal_id": map[string]interface{}{"type": "string", "description": "proposal id from tuning_recommend"},
				"sql":         map[string]interface{}{"type": "string", "description": "exact SQL from recommendation"},
				"confirmed":   map[string]interface{}{"type": "boolean", "description": "must be true to execute"},
				"target":      targetParam(),
			},
			"required": []string{"proposal_id", "sql", "confirmed"},
		},
		false, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			proposalID, _ := args["proposal_id"].(string)
			sql, _ := args["sql"].(string)
			confirmed, _ := args["confirmed"].(bool)
			if !confirmed {
				return "blocked: confirmed=true is required", nil
			}
			if strings.TrimSpace(proposalID) == "" || strings.TrimSpace(sql) == "" {
				return "proposal_id and sql are required", nil
			}

			proposal, ok := c.getProposal(proposalID)
			if !ok {
				return "unknown proposal_id: run tuning_recommend again", nil
			}
			if normalizeSQL(sql) != normalizeSQL(proposal.SQL) {
				return "sql mismatch: the provided SQL must exactly match the recommended proposal SQL", nil
			}

			cmd := buildSQLPlusCmd(sid, connStr, sql, pdb)
			res, err := execWithStream(ctx, exec, cmd)
			if err != nil {
				return fmt.Sprintf("execution error: %s", err), nil
			}
			c.deleteProposal(proposalID)
			return "applied proposal " + proposalID + "\n" + formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *OracleConnector) saveProposal(sql, summary, evidence string) tuningProposal {
	normalized := normalizeSQL(sql)
	sum := sha256.Sum256([]byte(normalized + "|" + summary + "|" + time.Now().UTC().Format(time.RFC3339Nano)))
	id := "tp_" + hex.EncodeToString(sum[:8])
	sqlHash := sha256.Sum256([]byte(normalized))
	p := tuningProposal{
		ID:        id,
		SQL:       strings.TrimSpace(sql),
		Summary:   summary,
		Evidence:  evidence,
		SQLHash:   hex.EncodeToString(sqlHash[:]),
		CreatedAt: time.Now().UTC(),
	}
	c.proposalMu.Lock()
	if c.proposals == nil {
		c.proposals = map[string]tuningProposal{}
	}
	c.proposals[id] = p
	c.proposalMu.Unlock()
	return p
}

func (c *OracleConnector) getProposal(id string) (tuningProposal, bool) {
	c.proposalMu.Lock()
	defer c.proposalMu.Unlock()
	p, ok := c.proposals[id]
	return p, ok
}

func (c *OracleConnector) deleteProposal(id string) {
	c.proposalMu.Lock()
	delete(c.proposals, id)
	c.proposalMu.Unlock()
}

func (c *OracleConnector) pickCandidateColumn(
	ctx context.Context,
	sid, connStr, pdb, owner, table string,
	exec executor.Executor,
) (string, error) {
	q := "SET HEADING OFF\n" + buildCandidateColumnQuery(owner, table)
	cmd := buildSQLPlusCmd(sid, connStr, q, pdb)
	res, err := execWithStream(ctx, exec, cmd)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_#$]+$`).MatchString(line) {
			continue
		}
		return line, nil
	}
	return "", nil
}

func buildTraceProfileShellCmd(sid, connStr, pdb, sql, traceID, tkSort string) string {
	if strings.TrimSpace(traceID) == "" {
		traceID = "INFRACTL_TRACE"
	}
	if strings.TrimSpace(tkSort) == "" {
		tkSort = "exeela,fchela,prsela"
	}
	var sb strings.Builder
	sb.WriteString("set -euo pipefail\n")
	sb.WriteString("tmp_dir=$(mktemp -d /tmp/infractl_trace_XXXXXX)\n")
	sb.WriteString("sql_file=\"$tmp_dir/work.sql\"\n")
	sb.WriteString("driver_file=\"$tmp_dir/driver.sql\"\n")
	sb.WriteString("out_file=\"$tmp_dir/sqlplus.out\"\n")
	sb.WriteString("tkprof_file=\"$tmp_dir/tkprof.txt\"\n")
	sb.WriteString("trace_id=" + shellQuotePOSIX(traceID) + "\n")
	sb.WriteString("if [ -z \"$trace_id\" ] || [ \"$trace_id\" = \"INFRACTL_TRACE\" ]; then trace_id=\"INFRACTL_TRACE_$(date +%s)\"; fi\n")
	sb.WriteString("cat > \"$sql_file\" <<'SQLPAYLOAD'\n")
	sb.WriteString(strings.TrimSpace(sql) + "\n")
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
		sqlplusPrefix = "ORACLE_SID=" + shellQuotePOSIX(sid) + " "
	}
	sb.WriteString(sqlplusPrefix + "sqlplus -S " + shellQuotePOSIX(connStr) + " @\"$driver_file\" > \"$out_file\"\n")
	sb.WriteString("cat \"$out_file\"\n")
	sb.WriteString("trace_path=$(awk -F= '/^TRACE_FILE=/{print $2}' \"$out_file\" | tail -1)\n")
	sb.WriteString("if [ -z \"$trace_path\" ] || [ ! -r \"$trace_path\" ]; then echo \"TRACE_FILE_NOT_READABLE=$trace_path\"; exit 2; fi\n")
	sb.WriteString("tkprof \"$trace_path\" \"$tkprof_file\" sys=no aggregate=no sort=" + shellQuotePOSIX(tkSort) + " >/dev/null\n")
	sb.WriteString("echo \"TRACE_FILE=$trace_path\"\n")
	sb.WriteString("echo \"TKPROF_FILE=$tkprof_file\"\n")
	sb.WriteString("sed -n '1,220p' \"$tkprof_file\"\n")
	return sb.String()
}

func extractFullScanObjectsFromText(text string) []string {
	re := regexp.MustCompile(`(?im)TABLE ACCESS FULL\s+([A-Z0-9_$#\.]+)`)
	matches := re.FindAllStringSubmatch(strings.ToUpper(text), -1)
	var out []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

func parseObjectRows(output string) []string {
	var out []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "FULL_SCAN_OBJECT") {
			continue
		}
		if strings.Count(line, "|") != 1 {
			continue
		}
		out = append(out, line)
	}
	return out
}

func splitOwnerTable(obj string) (string, string) {
	s := strings.ToUpper(strings.TrimSpace(obj))
	if strings.Contains(s, "|") {
		parts := strings.SplitN(s, "|", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
	}
	if strings.Contains(s, ".") {
		parts := strings.SplitN(s, ".", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
	}
	return "", strings.TrimSpace(s)
}

func buildIndexName(table, column string) string {
	name := "IX_" + sanitizeIdent(table) + "_" + sanitizeIdent(column) + "_I1"
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}

func sanitizeIdent(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "X"
	}
	return b.String()
}

func normalizeSQL(sql string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(sql)), " ")
}

func shellQuotePOSIX(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func trimLarge(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n...<truncated>..."
}
