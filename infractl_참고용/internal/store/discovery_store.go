// Package store
// File: discovery_store.go
// Description: 디스커버리 결과 저장소 인터페이스 및 SQLite 구현
// Responsibility: discovered_services 테이블 CRUD 및 스키마 마이그레이션

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

const createDiscoveredServicesSQL = `
CREATE TABLE IF NOT EXISTS discovered_services (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    server_name   TEXT NOT NULL,
    service_type  TEXT NOT NULL,
    service_name  TEXT DEFAULT '',
    port          INTEGER NOT NULL DEFAULT 0,
    details       TEXT DEFAULT '{}',
    confidence    REAL NOT NULL DEFAULT 0.0,
    discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const createDiscoveredServicesIdxSQL = `
CREATE INDEX IF NOT EXISTS idx_discovered_server ON discovered_services(server_name);`

// DiscoveredServiceEntry는 discovered_services 테이블의 단일 행이다.
type DiscoveredServiceEntry struct {
	ID           int
	ServerName   string
	ServiceType  string
	ServiceName  string
	Port         int
	Details      map[string]string
	Confidence   float64
	DiscoveredAt time.Time
}

// DiscoveryStore는 서비스 탐색 결과의 영속화를 담당하는 인터페이스이다.
type DiscoveryStore interface {
	// SaveDiscoveredServices는 탐색 결과를 저장한다. 기존 데이터를 덮어쓴다.
	SaveDiscoveredServices(ctx context.Context, serverName string, entries []DiscoveredServiceEntry) error

	// ListDiscoveredServices는 서버 이름으로 탐색된 서비스 목록을 반환한다.
	ListDiscoveredServices(ctx context.Context, serverName string) ([]DiscoveredServiceEntry, error)

	// ClearDiscoveredServices는 서버의 탐색 결과를 전부 삭제한다.
	ClearDiscoveredServices(ctx context.Context, serverName string) error
}

// initDiscoverySchema는 discovered_services 테이블과 인덱스를 생성한다.
func (s *SQLiteStore) initDiscoverySchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createDiscoveredServicesSQL); err != nil {
		slog.Warn("create discovered_services table", "err", err)
	}
	// additive migration: existing DBs without "port" keep working.
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE discovered_services ADD COLUMN port INTEGER NOT NULL DEFAULT 0`); err != nil {
		slog.Debug("add discovered_services.port skipped", "err", err)
	}
	if _, err := s.db.ExecContext(ctx, createDiscoveredServicesIdxSQL); err != nil {
		slog.Warn("create discovered_services index", "err", err)
	}
}

// SaveDiscoveredServices는 서버의 탐색 결과를 저장한다.
// 기존 데이터를 삭제 후 새 데이터를 삽입한다 (upsert 대신 clear-insert 방식).
func (s *SQLiteStore) SaveDiscoveredServices(ctx context.Context, serverName string, entries []DiscoveredServiceEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM discovered_services WHERE server_name = ?`, serverName); err != nil {
		return fmt.Errorf("clear discovered_services for %s: %w", serverName, err)
	}

	for _, e := range entries {
		detailsJSON, err := json.Marshal(e.Details)
		if err != nil {
			return fmt.Errorf("marshal details: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO discovered_services (server_name, service_type, service_name, port, details, confidence, discovered_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			serverName, e.ServiceType, e.ServiceName, e.Port, string(detailsJSON), e.Confidence, time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("insert discovered service: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	slog.Debug("saved discovered services", "server", serverName, "count", len(entries))
	return nil
}

// ListDiscoveredServices는 서버 이름으로 탐색된 서비스 목록을 반환한다.
func (s *SQLiteStore) ListDiscoveredServices(ctx context.Context, serverName string) ([]DiscoveredServiceEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, server_name, service_type, service_name, port, details, confidence, discovered_at
		 FROM discovered_services WHERE server_name = ? ORDER BY confidence DESC`,
		serverName)
	if err != nil {
		return nil, fmt.Errorf("list discovered services: %w", err)
	}
	defer rows.Close()

	var entries []DiscoveredServiceEntry
	for rows.Next() {
		var e DiscoveredServiceEntry
		var detailsJSON string
		if err := rows.Scan(&e.ID, &e.ServerName, &e.ServiceType, &e.ServiceName, &e.Port,
			&detailsJSON, &e.Confidence, &e.DiscoveredAt); err != nil {
			return nil, fmt.Errorf("scan discovered service: %w", err)
		}
		if err := json.Unmarshal([]byte(detailsJSON), &e.Details); err != nil {
			e.Details = map[string]string{}
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate discovered services: %w", err)
	}
	return entries, nil
}

// ClearDiscoveredServices는 서버의 탐색 결과를 전부 삭제한다.
func (s *SQLiteStore) ClearDiscoveredServices(ctx context.Context, serverName string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM discovered_services WHERE server_name = ?`, serverName); err != nil {
		return fmt.Errorf("clear discovered services for %s: %w", serverName, err)
	}
	return nil
}
