// Package rag
// File: search.go
// Description: 외부 DB 벡터 검색 — SSH를 통해 원격 Oracle/MySQL/PostgreSQL에 벡터 쿼리 실행
// Responsibility: RAGSource별 벡터 검색 쿼리를 SSH executor로 실행하고 결과 파싱

package rag

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

// ExternalResult는 외부 RAG 소스 검색의 단일 결과이다.
type ExternalResult struct {
	SourceName string
	// Columns는 result_columns에 지정된 컬럼명 → 값 매핑이다.
	Columns map[string]string
}

// ExternalSearcher는 외부 DB 벡터 검색 인터페이스이다.
type ExternalSearcher interface {
	Search(ctx context.Context, source store.RAGSource, queryVec []float32, topK int) ([]ExternalResult, error)
}

// SSHExternalSearcher는 SSH executor를 통해 원격 DB에 벡터 검색을 수행한다.
type SSHExternalSearcher struct {
	execManager *executor.Manager
}

// NewExternalSearcher는 SSHExternalSearcher를 생성한다.
func NewExternalSearcher(mgr *executor.Manager) *SSHExternalSearcher {
	return &SSHExternalSearcher{execManager: mgr}
}

// Search는 source 설정에 따라 SSH로 원격 DB에 벡터 검색 쿼리를 실행한다.
func (s *SSHExternalSearcher) Search(ctx context.Context, source store.RAGSource, queryVec []float32, topK int) ([]ExternalResult, error) {
	exec, err := s.execManager.Get(source.ServerName)
	if err != nil {
		return nil, fmt.Errorf("executor not found for server %s: %w", source.ServerName, err)
	}

	vecStr := formatVectorStr(queryVec)

	cmd := buildVectorSearchCmd(source, vecStr, topK)
	if cmd == "" {
		return nil, fmt.Errorf("unsupported db type: %s", source.DBType)
	}

	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("execute vector search on %s: %w", source.ServerName, err)
	}
	if result.ExitCode != 0 {
		slog.Warn("vector search non-zero exit", "source", source.Name, "stderr", result.Stderr)
	}

	parsed := parseQueryOutput(result.Stdout, mergedSelectColumns(source.ResultColumns, source.MetadataColumns))
	slog.Debug("external rag search", "source", source.Name, "rows", len(parsed))
	return parsed, nil
}

// parseQueryOutput은 DB CLI의 탭 구분 출력을 ExternalResult 슬라이스로 파싱한다.
func parseQueryOutput(output, resultCols string) []ExternalResult {
	cols := splitCols(resultCols)
	if len(cols) == 0 {
		return nil
	}

	var results []ExternalResult
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 탭 구분 또는 공백 구분 파싱 시도
		parts := strings.Split(line, "\t")
		if len(parts) < len(cols) {
			// 탭이 없으면 다중 공백으로 분리 시도 (sqlplus -S 출력)
			parts = splitByMultiSpace(line)
		}

		row := make(map[string]string, len(cols))
		for i, col := range cols {
			if i < len(parts) {
				row[strings.TrimSpace(col)] = strings.TrimSpace(parts[i])
			}
		}
		results = append(results, ExternalResult{Columns: row})
	}
	return results
}

// splitCols는 콤마 구분 컬럼 목록을 분리한다.
func splitCols(s string) []string {
	var cols []string
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			cols = append(cols, c)
		}
	}
	return cols
}

// splitByMultiSpace는 2개 이상의 공백으로 컬럼을 분리한다 (sqlplus -S 출력용).
func splitByMultiSpace(s string) []string {
	var parts []string
	for _, p := range strings.Split(s, "  ") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
