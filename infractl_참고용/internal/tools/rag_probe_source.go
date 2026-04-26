package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

type RAGProbeSourceTool struct {
	ExecManager *executor.Manager
}

var _ Tool = (*RAGProbeSourceTool)(nil)

func (t *RAGProbeSourceTool) Name() string     { return "rag_probe_source" }
func (t *RAGProbeSourceTool) IsReadOnly() bool { return true }
func (t *RAGProbeSourceTool) IsEnabled() bool  { return true }

func (t *RAGProbeSourceTool) Description() string {
	return "Probe external DB vector source schema before rag_register. " +
		"Checks table/column presence and detects vector dimension."
}

func (t *RAGProbeSourceTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"server_name":       map[string]interface{}{"type": "string"},
			"db_type":           map[string]interface{}{"type": "string", "enum": []string{"oracle", "mysql", "postgresql"}},
			"db_host":           map[string]interface{}{"type": "string"},
			"db_port":           map[string]interface{}{"type": "integer"},
			"db_name":           map[string]interface{}{"type": "string"},
			"schema_name":       map[string]interface{}{"type": "string"},
			"table_name":        map[string]interface{}{"type": "string"},
			"text_column":       map[string]interface{}{"type": "string"},
			"vector_column":     map[string]interface{}{"type": "string"},
			"metadata_table":    map[string]interface{}{"type": "string"},
			"metadata_columns":  map[string]interface{}{"type": "string"},
			"table_join_key":    map[string]interface{}{"type": "string"},
			"metadata_join_key": map[string]interface{}{"type": "string"},
			"username":          map[string]interface{}{"type": "string"},
			"password":          map[string]interface{}{"type": "string"},
			"role":              map[string]interface{}{"type": "string"},
		},
		"required": []string{"server_name", "db_type", "db_name", "table_name", "vector_column", "username", "password"},
	}
}

func (t *RAGProbeSourceTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	req, err := parseRAGProbeRequest(args)
	if err != nil {
		msg := fmt.Sprintf("probe args invalid: %v", err)
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}
	result, err := probeRAGSource(ctx, t.ExecManager, req)
	if err != nil {
		msg := fmt.Sprintf("RAG source probe failed: %v", err)
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}
	body, _ := json.MarshalIndent(result, "", "  ")
	return ToolOutcome{Content: string(body), Success: true}, nil
}

type ragProbeRequest struct {
	ServerName      string
	DBType          string
	DBHost          string
	DBPort          int
	DBName          string
	SchemaName      string
	TableName       string
	TextColumn      string
	VectorColumn    string
	MetadataTable   string
	MetadataColumns string
	TableJoinKey    string
	MetadataJoinKey string
	Username        string
	Password        string
	Role            string
}

type ragProbeResult struct {
	MainColumns      []string `json:"main_columns,omitempty"`
	MetadataColumns  []string `json:"metadata_columns,omitempty"`
	VectorDimension  int      `json:"vector_dimension,omitempty"`
	TableExists      bool     `json:"table_exists"`
	MetadataExists   bool     `json:"metadata_exists"`
	MissingColumns   []string `json:"missing_columns,omitempty"`
	MissingMetaCols  []string `json:"missing_meta_columns,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	ProbeDBType      string   `json:"probe_db_type"`
	ProbeServer      string   `json:"probe_server"`
	ProbeTargetTable string   `json:"probe_target_table"`
}

func parseRAGProbeRequest(args map[string]interface{}) (ragProbeRequest, error) {
	req := ragProbeRequest{
		DBPort: argInt(args, "db_port", 0),
	}
	var err error
	req.ServerName, err = argString(args, "server_name", true)
	if err != nil {
		return req, err
	}
	req.DBType, err = argString(args, "db_type", true)
	if err != nil {
		return req, err
	}
	req.DBHost, _ = argString(args, "db_host", false)
	req.DBName, err = argString(args, "db_name", true)
	if err != nil {
		return req, err
	}
	req.SchemaName, _ = argString(args, "schema_name", false)
	req.TableName, err = argString(args, "table_name", true)
	if err != nil {
		return req, err
	}
	req.TextColumn, _ = argString(args, "text_column", false)
	req.VectorColumn, err = argString(args, "vector_column", true)
	if err != nil {
		return req, err
	}
	req.MetadataTable, _ = argString(args, "metadata_table", false)
	req.MetadataColumns, _ = argString(args, "metadata_columns", false)
	req.TableJoinKey, _ = argString(args, "table_join_key", false)
	req.MetadataJoinKey, _ = argString(args, "metadata_join_key", false)
	req.Username, err = argString(args, "username", true)
	if err != nil {
		return req, err
	}
	req.Password, err = argString(args, "password", true)
	if err != nil {
		return req, err
	}
	req.Role, _ = argString(args, "role", false)
	req.ServerName = strings.TrimSpace(req.ServerName)
	req.DBType = strings.ToLower(strings.TrimSpace(req.DBType))
	req.DBHost = strings.TrimSpace(req.DBHost)
	req.DBName = strings.TrimSpace(req.DBName)
	req.SchemaName = strings.TrimSpace(req.SchemaName)
	req.TableName = strings.TrimSpace(req.TableName)
	req.TextColumn = strings.TrimSpace(req.TextColumn)
	req.VectorColumn = strings.TrimSpace(req.VectorColumn)
	req.MetadataTable = strings.TrimSpace(req.MetadataTable)
	req.MetadataColumns = strings.TrimSpace(req.MetadataColumns)
	req.TableJoinKey = strings.TrimSpace(req.TableJoinKey)
	req.MetadataJoinKey = strings.TrimSpace(req.MetadataJoinKey)
	req.Username = strings.TrimSpace(req.Username)
	req.Role = strings.TrimSpace(req.Role)
	if req.ServerName == "" || req.DBType == "" || req.DBName == "" || req.TableName == "" || req.VectorColumn == "" || req.Username == "" || req.Password == "" {
		return req, fmt.Errorf("required: server_name, db_type, db_name, table_name, vector_column, username, password")
	}
	return req, nil
}

func probeRAGSource(ctx context.Context, mgr *executor.Manager, req ragProbeRequest) (ragProbeResult, error) {
	if mgr == nil {
		return ragProbeResult{}, fmt.Errorf("executor manager is nil")
	}
	exec, err := mgr.Get(req.ServerName)
	if err != nil {
		return ragProbeResult{}, fmt.Errorf("executor for %s: %w", req.ServerName, err)
	}

	var (
		mainCols  []string
		metaCols  []string
		vectorDim int
	)

	switch req.DBType {
	case "mysql":
		mainCols, err = mysqlColumns(ctx, exec, req, req.TableName)
		if err != nil {
			return ragProbeResult{}, err
		}
		vectorDim, _ = mysqlVectorDimension(ctx, exec, req)
		if req.MetadataTable != "" {
			metaCols, err = mysqlColumns(ctx, exec, req, req.MetadataTable)
			if err != nil {
				return ragProbeResult{}, err
			}
		}
	case "postgresql":
		mainCols, err = pgColumns(ctx, exec, req, req.TableName)
		if err != nil {
			return ragProbeResult{}, err
		}
		vectorDim, _ = pgVectorDimension(ctx, exec, req)
		if req.MetadataTable != "" {
			metaCols, err = pgColumns(ctx, exec, req, req.MetadataTable)
			if err != nil {
				return ragProbeResult{}, err
			}
		}
	case "oracle":
		mainCols, err = oracleColumns(ctx, exec, req, req.TableName)
		if err != nil {
			return ragProbeResult{}, err
		}
		vectorDim, _ = oracleVectorDimension(ctx, exec, req)
		if req.MetadataTable != "" {
			metaCols, err = oracleColumns(ctx, exec, req, req.MetadataTable)
			if err != nil {
				return ragProbeResult{}, err
			}
		}
	default:
		return ragProbeResult{}, fmt.Errorf("unsupported db_type: %s", req.DBType)
	}

	missing := missingCols(mainCols, []string{req.TextColumn, req.VectorColumn, req.TableJoinKey})
	missingMeta := []string{}
	if req.MetadataTable != "" {
		metaExpected := append(splitCSV(req.MetadataColumns), req.MetadataJoinKey)
		missingMeta = missingCols(metaCols, metaExpected)
	}
	res := ragProbeResult{
		MainColumns:      mainCols,
		MetadataColumns:  metaCols,
		VectorDimension:  vectorDim,
		TableExists:      len(mainCols) > 0,
		MetadataExists:   req.MetadataTable == "" || len(metaCols) > 0,
		MissingColumns:   missing,
		MissingMetaCols:  missingMeta,
		ProbeDBType:      req.DBType,
		ProbeServer:      req.ServerName,
		ProbeTargetTable: req.TableName,
	}
	if vectorDim == 0 {
		res.Warnings = append(res.Warnings, "vector dimension auto-detection failed")
	}
	return res, nil
}

func mysqlColumns(ctx context.Context, exec executor.Executor, req ragProbeRequest, table string) ([]string, error) {
	host := req.DBHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := req.DBPort
	if port == 0 {
		port = 3306
	}
	sql := fmt.Sprintf(
		"SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=%s AND TABLE_NAME=%s ORDER BY ORDINAL_POSITION;",
		sqlQuote(req.DBName), sqlQuote(table),
	)
	cmd := fmt.Sprintf(
		"MYSQL_PWD=%s mysql -u %s -h %s -P %d -D %s --batch --silent -N -e %s",
		shQuote(req.Password), shQuote(req.Username), shQuote(host), port, shQuote(req.DBName), shQuote(sql),
	)
	return runSingleColumnQuery(ctx, exec, cmd, "mysql columns")
}

func mysqlVectorDimension(ctx context.Context, exec executor.Executor, req ragProbeRequest) (int, error) {
	host := req.DBHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := req.DBPort
	if port == 0 {
		port = 3306
	}
	sql := fmt.Sprintf(
		"SELECT COLUMN_TYPE FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=%s AND TABLE_NAME=%s AND COLUMN_NAME=%s LIMIT 1;",
		sqlQuote(req.DBName), sqlQuote(req.TableName), sqlQuote(req.VectorColumn),
	)
	cmd := fmt.Sprintf(
		"MYSQL_PWD=%s mysql -u %s -h %s -P %d -D %s --batch --silent -N -e %s",
		shQuote(req.Password), shQuote(req.Username), shQuote(host), port, shQuote(req.DBName), shQuote(sql),
	)
	rows, err := runSingleColumnQuery(ctx, exec, cmd, "mysql vector dimension")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return parseDimension(rows[0]), nil
}

func pgColumns(ctx context.Context, exec executor.Executor, req ragProbeRequest, table string) ([]string, error) {
	host := req.DBHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := req.DBPort
	if port == 0 {
		port = 5432
	}
	schema := req.SchemaName
	if schema == "" {
		schema = "public"
	}
	sql := fmt.Sprintf(
		"SELECT column_name FROM information_schema.columns WHERE table_schema=%s AND table_name=%s ORDER BY ordinal_position;",
		sqlQuote(schema), sqlQuote(table),
	)
	cmd := fmt.Sprintf(
		"PGPASSWORD=%s psql -U %s -h %s -p %d -d %s -At -F $'\\t' -c %s",
		shQuote(req.Password), shQuote(req.Username), shQuote(host), port, shQuote(req.DBName), shQuote(sql),
	)
	return runSingleColumnQuery(ctx, exec, cmd, "postgres columns")
}

func pgVectorDimension(ctx context.Context, exec executor.Executor, req ragProbeRequest) (int, error) {
	host := req.DBHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := req.DBPort
	if port == 0 {
		port = 5432
	}
	schema := req.SchemaName
	if schema == "" {
		schema = "public"
	}
	sql := fmt.Sprintf(
		"SELECT CASE WHEN a.atttypmod > 0 THEN a.atttypmod - 4 ELSE 0 END FROM pg_attribute a JOIN pg_class c ON a.attrelid=c.oid JOIN pg_namespace n ON c.relnamespace=n.oid WHERE n.nspname=%s AND c.relname=%s AND a.attname=%s AND a.attnum>0 AND NOT a.attisdropped LIMIT 1;",
		sqlQuote(schema), sqlQuote(req.TableName), sqlQuote(req.VectorColumn),
	)
	cmd := fmt.Sprintf(
		"PGPASSWORD=%s psql -U %s -h %s -p %d -d %s -At -F $'\\t' -c %s",
		shQuote(req.Password), shQuote(req.Username), shQuote(host), port, shQuote(req.DBName), shQuote(sql),
	)
	rows, err := runSingleColumnQuery(ctx, exec, cmd, "postgres vector dimension")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(rows[0]))
	return n, nil
}

func oracleColumns(ctx context.Context, exec executor.Executor, req ragProbeRequest, table string) ([]string, error) {
	port := req.DBPort
	if port == 0 {
		port = 1521
	}
	host := req.DBHost
	if host == "" {
		host = req.ServerName
	}
	sql := ""
	if req.SchemaName == "" {
		sql = fmt.Sprintf("SELECT COLUMN_NAME FROM USER_TAB_COLUMNS WHERE TABLE_NAME=UPPER(%s) ORDER BY COLUMN_ID;\n", sqlQuote(table))
	} else {
		sql = fmt.Sprintf("SELECT COLUMN_NAME FROM ALL_TAB_COLUMNS WHERE OWNER=UPPER(%s) AND TABLE_NAME=UPPER(%s) ORDER BY COLUMN_ID;\n", sqlQuote(req.SchemaName), sqlQuote(table))
	}
	rows, err := runOracleQuery(ctx, exec, req, host, port, sql)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func oracleVectorDimension(ctx context.Context, exec executor.Executor, req ragProbeRequest) (int, error) {
	port := req.DBPort
	if port == 0 {
		port = 1521
	}
	host := req.DBHost
	if host == "" {
		host = req.ServerName
	}
	var sql string
	if req.SchemaName == "" {
		sql = fmt.Sprintf("SELECT DATA_TYPE FROM USER_TAB_COLUMNS WHERE TABLE_NAME=UPPER(%s) AND COLUMN_NAME=UPPER(%s);\n", sqlQuote(req.TableName), sqlQuote(req.VectorColumn))
	} else {
		sql = fmt.Sprintf("SELECT DATA_TYPE FROM ALL_TAB_COLUMNS WHERE OWNER=UPPER(%s) AND TABLE_NAME=UPPER(%s) AND COLUMN_NAME=UPPER(%s);\n", sqlQuote(req.SchemaName), sqlQuote(req.TableName), sqlQuote(req.VectorColumn))
	}
	rows, err := runOracleQuery(ctx, exec, req, host, port, sql)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return parseDimension(rows[0]), nil
}

func runOracleQuery(ctx context.Context, exec executor.Executor, req ragProbeRequest, host string, port int, sql string) ([]string, error) {
	connStr := fmt.Sprintf("%s/%s@%s:%d/%s", req.Username, req.Password, host, port, req.DBName)
	if strings.EqualFold(req.Role, "sysdba") {
		connStr += " as sysdba"
	}
	sqlplus := "SET FEEDBACK OFF\nSET HEADING OFF\nSET PAGESIZE 0\nSET VERIFY OFF\nSET ECHO OFF\nSET TRIMSPOOL ON\n" + sql + "EXIT;\n"
	cmd := fmt.Sprintf(
		`tmp_sql="$(mktemp /tmp/rag_probe_q_XXXXXX.sql)" && trap 'rm -f "$tmp_sql"' EXIT && printf '%%s' %s > "$tmp_sql" && sqlplus -L -S %s @"$tmp_sql"`,
		shQuote(sqlplus), shQuote(connStr),
	)
	return runSingleColumnQuery(ctx, exec, cmd, "oracle query")
}

func runSingleColumnQuery(ctx context.Context, exec executor.Executor, cmd, what string) ([]string, error) {
	res, err := exec.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("%s execute: %w", what, err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("%s exit=%d stderr=%s", what, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		parts := strings.Split(l, "\t")
		out = append(out, strings.TrimSpace(parts[0]))
	}
	return out, nil
}

func missingCols(actual []string, expected []string) []string {
	if len(expected) == 0 {
		return nil
	}
	lowerActual := make([]string, 0, len(actual))
	for _, c := range actual {
		lowerActual = append(lowerActual, strings.ToLower(strings.TrimSpace(c)))
	}
	var missing []string
	for _, c := range expected {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !slices.Contains(lowerActual, strings.ToLower(c)) {
			missing = append(missing, c)
		}
	}
	return missing
}

func parseDimension(s string) int {
	re := regexp.MustCompile(`\((\d+)\)`)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
