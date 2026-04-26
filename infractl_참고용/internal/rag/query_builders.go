// Package rag
// File: query_builders.go
// Description: DB별 벡터 검색 SQL 및 CLI 명령 빌더
// Responsibility: Oracle/MySQL/PostgreSQL 벡터 검색 쿼리를 DB CLI 실행 형태로 변환

package rag

import (
	"fmt"
	"slices"
	"strings"

	"github.com/yourorg/infractl/internal/store"
)

// buildVectorSearchCmd는 db_type에 따라 벡터 검색 CLI 명령을 반환한다.
// 벡터는 임시 파일(/tmp/rag_vec_query.sql)에 기록하여 셸 인자 길이 제한을 우회한다.
func buildVectorSearchCmd(source store.RAGSource, vecStr string, topK int) string {
	switch source.DBType {
	case "oracle":
		return buildOracleVectorCmd(source, vecStr, topK)
	case "mysql":
		return buildMySQLVectorCmd(source, vecStr, topK)
	case "postgresql":
		return buildPGVectorCmd(source, vecStr, topK)
	default:
		return ""
	}
}

// buildOracleVectorCmd는 Oracle VECTOR_DISTANCE를 사용하는 명령을 생성한다.
// 벡터 리터럴이 길어 임시 SQL 파일 방식을 사용한다.
func buildOracleVectorCmd(source store.RAGSource, vecStr string, topK int) string {
	host := source.DBHost
	if host == "" {
		host = source.ServerName
	}
	user := source.Credentials.Username
	pass := source.Credentials.Password
	role := source.Credentials.Role
	sid := source.DBName
	port := source.DBPort
	table := qualifyTable(source.SchemaName, source.TableName)
	selectCols := mergedSelectColumns(source.ResultColumns, source.MetadataColumns)
	fromClause := table + " t" + joinClause(source)

	vecExpr := prefixedColumn(source.VectorColumn, "t.")
	if port == 0 {
		port = 1521
	}
	connStr := fmt.Sprintf("%s/%s@%s:%d/%s", user, pass, host, port, sid)
	if role == "sysdba" {
		connStr += " as sysdba"
	}

	// Oracle VECTOR 리터럴 형식: VECTOR('[0.1,0.2,...]', dim, FLOAT32)
	sqlContent := fmt.Sprintf(
		"SELECT %s FROM %s ORDER BY VECTOR_DISTANCE(%s, VECTOR('%s', %d, FLOAT32), COSINE) FETCH FIRST %d ROWS ONLY;\nEXIT;\n",
		selectCols, fromClause, vecExpr, vecStr, len(strings.Split(vecStr, ",")), topK,
	)
	runCmd := fmt.Sprintf("sqlplus -L -S %s @\"$tmp_sql\"", shellQuote(connStr))
	return wrapSQLFileExec(runCmd, sqlContent)
}

// buildMySQLVectorCmd는 MySQL 9.0+ STRING_TO_VECTOR를 사용하는 명령을 생성한다.
func buildMySQLVectorCmd(source store.RAGSource, vecStr string, topK int) string {
	host := source.DBHost
	if host == "" {
		host = source.ServerName
	}
	port := source.DBPort
	user := source.Credentials.Username
	pass := source.Credentials.Password
	dbName := source.DBName
	table := qualifyTable(source.SchemaName, source.TableName)
	selectCols := mergedSelectColumns(source.ResultColumns, source.MetadataColumns)
	fromClause := table + " t" + joinClause(source)
	vecExpr := prefixedColumn(source.VectorColumn, "t.")
	if port == 0 {
		port = 3306
	}
	sqlContent := fmt.Sprintf(
		"SELECT %s, DISTANCE(%s, STRING_TO_VECTOR('%s'), 'cosine') AS _dist FROM %s ORDER BY _dist LIMIT %d;\n",
		selectCols, vecExpr, vecStr, fromClause, topK,
	)
	runCmd := fmt.Sprintf(
		"MYSQL_PWD=%s mysql -u %s -h %s -P %d -D %s --batch --silent < \"$tmp_sql\"",
		shellQuote(pass), shellQuote(user), shellQuote(host), port, shellQuote(dbName),
	)
	return wrapSQLFileExec(runCmd, sqlContent)
}

// buildPGVectorCmd는 pgvector <=> 연산자를 사용하는 명령을 생성한다.
func buildPGVectorCmd(source store.RAGSource, vecStr string, topK int) string {
	host := source.DBHost
	if host == "" {
		host = source.ServerName
	}
	port := source.DBPort
	user := source.Credentials.Username
	pass := source.Credentials.Password
	dbName := source.DBName
	table := qualifyTable(source.SchemaName, source.TableName)
	selectCols := mergedSelectColumns(source.ResultColumns, source.MetadataColumns)
	fromClause := table + " t" + joinClause(source)
	vecExpr := prefixedColumn(source.VectorColumn, "t.")
	if port == 0 {
		port = 5432
	}
	// pgvector 리터럴 형식: '[0.1,0.2,...]'::vector
	sqlContent := fmt.Sprintf(
		"SELECT %s FROM %s ORDER BY %s <=> '[%s]'::vector LIMIT %d;\n",
		selectCols, fromClause, vecExpr, vecStr, topK,
	)
	runCmd := fmt.Sprintf(
		"PGPASSWORD=%s psql -U %s -h %s -p %d -d %s -f \"$tmp_sql\"",
		shellQuote(pass), shellQuote(user), shellQuote(host), port, shellQuote(dbName),
	)
	return wrapSQLFileExec(runCmd, sqlContent)
}

// formatVectorStr은 []float32를 콤마 구분 문자열로 변환한다.
func formatVectorStr(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%.6f", f)
	}
	return strings.Join(parts, ",")
}

// shellQuote는 single-quote로 문자열을 감싼다. 내부 single-quote는 이스케이프한다.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func wrapSQLFileExec(runCmd, sqlContent string) string {
	return fmt.Sprintf(
		`tmp_sql="$(mktemp /tmp/rag_vec_q_XXXXXX.sql)" && trap 'rm -f "$tmp_sql"' EXIT && printf '%%s' %s > "$tmp_sql" && %s`,
		shellQuote(sqlContent), runCmd,
	)
}

func qualifyTable(schema, table string) string {
	if strings.TrimSpace(schema) == "" {
		return table
	}
	return schema + "." + table
}

func joinClause(source store.RAGSource) string {
	if strings.TrimSpace(source.MetadataTable) == "" ||
		strings.TrimSpace(source.TableJoinKey) == "" ||
		strings.TrimSpace(source.MetadataJoinKey) == "" {
		return ""
	}
	meta := qualifyTable(source.SchemaName, source.MetadataTable)
	return fmt.Sprintf(" LEFT JOIN %s m ON t.%s = m.%s", meta, source.TableJoinKey, source.MetadataJoinKey)
}

func mergedSelectColumns(resultCols, metadataCols string) string {
	base := splitCols(resultCols)
	extra := splitCols(metadataCols)
	for _, col := range extra {
		if !slices.Contains(base, col) {
			base = append(base, col)
		}
	}
	return strings.Join(base, ", ")
}

func prefixedColumn(col, prefix string) string {
	if strings.Contains(col, ".") {
		return col
	}
	return prefix + col
}
