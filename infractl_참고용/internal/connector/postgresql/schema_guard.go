// Package postgresql
// File: schema_guard.go
// Description: PostgreSQL 쿼리 실행 후 테이블·컬럼 존재 검증 및 자동 스키마 조회
// Responsibility: 쿼리 실패 시 실제 테이블 존재 여부와 컬럼 목록을 자동으로 조회하여 오류 응답을 보강

package postgresql

import (
	"context"
	"fmt"
	"strings"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
)

// validateAndEnrich는 쿼리 결과에 오류가 있으면 테이블/컬럼 스키마를 자동 조회하여
// 원본 오류 메시지에 실제 스키마 정보를 추가한 문자열을 반환한다.
func validateAndEnrich(ctx context.Context, user, pass, host string, port int, dbName, sql, output string, exec executor.Executor) string {
	if !conn.HasQueryError(output, 0) {
		return output
	}

	tables := conn.ExtractTableNames(sql)
	if len(tables) == 0 {
		return output
	}

	var schemaInfo strings.Builder
	schemaInfo.WriteString("\n\n[자동 스키마 조회 결과]\n")

	for _, table := range tables {
		info := fetchPGTableSchema(ctx, user, pass, host, port, dbName, table, exec)
		schemaInfo.WriteString(info)
	}

	return output + schemaInfo.String()
}

// fetchPGTableSchema는 information_schema에서 테이블 존재 여부와 컬럼 목록을 조회한다.
// pg_catalog 시스템 테이블은 pg_attribute를 통해 조회한다.
func fetchPGTableSchema(ctx context.Context, user, pass, host string, port int, dbName, tableName string, exec executor.Executor) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "── %s:\n", tableName)

	// 테이블 존재 여부 확인
	existSQL := fmt.Sprintf(
		`SELECT table_schema, table_type FROM information_schema.tables WHERE table_name='%s' LIMIT 5;`,
		strings.ToLower(tableName),
	)
	res, err := exec.Execute(ctx, buildCmd(user, pass, host, port, dbName, existSQL))
	if err != nil || strings.TrimSpace(res.Stdout) == "" {
		// pg_catalog 시스템 카탈로그 시도
		return sb.String() + fetchPGCatalogSchema(ctx, user, pass, host, port, dbName, tableName, exec)
	}

	// 컬럼 목록 조회
	colSQL := fmt.Sprintf(
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_name='%s' ORDER BY ordinal_position;`,
		strings.ToLower(tableName),
	)
	colRes, err := exec.Execute(ctx, buildCmd(user, pass, host, port, dbName, colSQL))
	if err != nil || strings.TrimSpace(colRes.Stdout) == "" {
		sb.WriteString("  → 컬럼 정보를 가져올 수 없습니다.\n")
		return sb.String()
	}

	sb.WriteString("  사용 가능한 컬럼:\n")
	for _, line := range strings.Split(strings.TrimSpace(colRes.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			sb.WriteString("    " + line + "\n")
		}
	}
	return sb.String()
}

// fetchPGCatalogSchema는 pg_class/pg_attribute를 통해 시스템 카탈로그 테이블의 컬럼을 조회한다.
func fetchPGCatalogSchema(ctx context.Context, user, pass, host string, port int, dbName, tableName string, exec executor.Executor) string {
	var sb strings.Builder

	catSQL := fmt.Sprintf(
		`SELECT a.attname, pg_catalog.format_type(a.atttypid, a.atttypmod)
		 FROM pg_catalog.pg_attribute a
		 JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
		 WHERE c.relname = '%s' AND a.attnum > 0 AND NOT a.attisdropped
		 ORDER BY a.attnum;`,
		strings.ToLower(tableName),
	)
	catRes, err := exec.Execute(ctx, buildCmd(user, pass, host, port, dbName, catSQL))
	if err != nil || strings.TrimSpace(catRes.Stdout) == "" {
		sb.WriteString("  → 테이블/뷰를 찾을 수 없습니다.\n")
		sb.WriteString(fmt.Sprintf(
			"  힌트: SELECT relname, relkind FROM pg_catalog.pg_class WHERE relname LIKE '%%%s%%' LIMIT 10;\n",
			strings.ToLower(tableName)[:min(len(tableName), 6)],
		))
		return sb.String()
	}

	sb.WriteString("  사용 가능한 컬럼 (pg_catalog):\n")
	for _, line := range strings.Split(strings.TrimSpace(catRes.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			sb.WriteString("    " + line + "\n")
		}
	}
	return sb.String()
}
