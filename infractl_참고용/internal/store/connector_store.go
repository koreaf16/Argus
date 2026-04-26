// Package store
// File: connector_store.go
// Description: 커넥터 접속 정보 저장소 인터페이스 및 SQLite 구현
// Responsibility: connectors 테이블 CRUD, 크리덴셜 암호화, 스키마 마이그레이션

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/yourorg/infractl/internal/crypto"
)

const createConnectorsSQL = `
CREATE TABLE IF NOT EXISTS connectors (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    server_name  TEXT NOT NULL,
    service_type TEXT NOT NULL,
    service_name TEXT NOT NULL,
    config       TEXT DEFAULT '{}',
    credentials  TEXT DEFAULT '',
    tools        TEXT DEFAULT '[]',
    save_mode    TEXT NOT NULL DEFAULT 'session',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(server_name, service_type, service_name)
);`

// ConnectorEntry는 connectors 테이블의 단일 행이다.
type ConnectorEntry struct {
	ID          int
	ServerName  string
	ServiceType string
	ServiceName string
	Config      string    // JSON 형식의 서비스별 설정
	Credentials string    // AES-256-GCM 암호화된 자격증명 JSON
	Tools       string    // JSON 배열 — 생성된 도구 이름 목록
	SaveMode    string    // "permanent", "session", "none"
	CreatedAt   time.Time
}

// ConnectorStore는 커넥터 접속 정보의 영속화를 담당하는 인터페이스이다.
type ConnectorStore interface {
	// SaveConnector는 커넥터를 저장한다. 이미 존재하면 업데이트한다.
	SaveConnector(ctx context.Context, entry ConnectorEntry) error

	// ListConnectors는 저장된 모든 커넥터를 반환한다.
	ListConnectors(ctx context.Context) ([]ConnectorEntry, error)

	// ListPermanentConnectors는 save_mode='permanent'인 커넥터만 반환한다.
	ListPermanentConnectors(ctx context.Context) ([]ConnectorEntry, error)

	// GetConnector는 복합 키로 커넥터를 조회한다.
	GetConnector(ctx context.Context, serverName, serviceType, serviceName string) (ConnectorEntry, error)

	// RemoveConnector는 복합 키로 커넥터를 삭제한다.
	RemoveConnector(ctx context.Context, serverName, serviceType, serviceName string) error

	// EncryptConnectorCreds는 자격증명 문자열을 암호화한다.
	EncryptConnectorCreds(plaintext string) (string, error)

	// DecryptConnectorCreds는 암호화된 자격증명을 복호화한다.
	DecryptConnectorCreds(ciphertext string) (string, error)
}

// initConnectorSchema는 connectors 테이블을 생성한다.
func (s *SQLiteStore) initConnectorSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createConnectorsSQL); err != nil {
		slog.Warn("create connectors table", "err", err)
	}
}

// SaveConnector는 커넥터를 저장한다. 이미 존재하면 업데이트(UPSERT)한다.
func (s *SQLiteStore) SaveConnector(ctx context.Context, entry ConnectorEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO connectors (server_name, service_type, service_name, config, credentials, tools, save_mode, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(server_name, service_type, service_name)
		 DO UPDATE SET config=excluded.config, credentials=excluded.credentials,
		               tools=excluded.tools, save_mode=excluded.save_mode`,
		entry.ServerName, entry.ServiceType, entry.ServiceName,
		entry.Config, entry.Credentials, entry.Tools, entry.SaveMode, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("save connector %s/%s/%s: %w", entry.ServerName, entry.ServiceType, entry.ServiceName, err)
	}
	slog.Debug("connector saved", "server", entry.ServerName, "type", entry.ServiceType, "name", entry.ServiceName)
	return nil
}

// ListConnectors는 저장된 모든 커넥터를 반환한다.
func (s *SQLiteStore) ListConnectors(ctx context.Context) ([]ConnectorEntry, error) {
	return s.queryConnectors(ctx, `SELECT id, server_name, service_type, service_name, config, credentials, tools, save_mode, created_at FROM connectors ORDER BY created_at`)
}

// ListPermanentConnectors는 save_mode='permanent'인 커넥터만 반환한다.
func (s *SQLiteStore) ListPermanentConnectors(ctx context.Context) ([]ConnectorEntry, error) {
	return s.queryConnectors(ctx, `SELECT id, server_name, service_type, service_name, config, credentials, tools, save_mode, created_at FROM connectors WHERE save_mode='permanent' ORDER BY created_at`)
}

// GetConnector는 복합 키로 커넥터를 조회한다.
func (s *SQLiteStore) GetConnector(ctx context.Context, serverName, serviceType, serviceName string) (ConnectorEntry, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, server_name, service_type, service_name, config, credentials, tools, save_mode, created_at
		 FROM connectors WHERE server_name=? AND service_type=? AND service_name=?`,
		serverName, serviceType, serviceName)
	entries, err := s.scanConnectorRow(row)
	if err == sql.ErrNoRows {
		return ConnectorEntry{}, fmt.Errorf("connector not found: %s/%s/%s", serverName, serviceType, serviceName)
	}
	if err != nil {
		return ConnectorEntry{}, fmt.Errorf("get connector: %w", err)
	}
	return entries, nil
}

// RemoveConnector는 복합 키로 커넥터를 삭제한다.
func (s *SQLiteStore) RemoveConnector(ctx context.Context, serverName, serviceType, serviceName string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM connectors WHERE server_name=? AND service_type=? AND service_name=?`,
		serverName, serviceType, serviceName)
	if err != nil {
		return fmt.Errorf("remove connector %s/%s/%s: %w", serverName, serviceType, serviceName, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("connector not found: %s/%s/%s", serverName, serviceType, serviceName)
	}
	return nil
}

// EncryptConnectorCreds는 자격증명 평문을 암호화한다.
func (s *SQLiteStore) EncryptConnectorCreds(plaintext string) (string, error) {
	return crypto.Encrypt(s.cryptoKey, plaintext)
}

// DecryptConnectorCreds는 암호화된 자격증명을 복호화한다.
func (s *SQLiteStore) DecryptConnectorCreds(ciphertext string) (string, error) {
	return crypto.Decrypt(s.cryptoKey, ciphertext)
}

// queryConnectors는 SQL 쿼리로 커넥터 목록을 조회한다.
func (s *SQLiteStore) queryConnectors(ctx context.Context, query string) ([]ConnectorEntry, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query connectors: %w", err)
	}
	defer rows.Close()

	var entries []ConnectorEntry
	for rows.Next() {
		e, err := s.scanConnectorRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan connector: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connectors: %w", err)
	}
	return entries, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanConnectorRow는 sql.Row 또는 sql.Rows에서 ConnectorEntry를 읽는다.
func (s *SQLiteStore) scanConnectorRow(row rowScanner) (ConnectorEntry, error) {
	var e ConnectorEntry
	if err := row.Scan(&e.ID, &e.ServerName, &e.ServiceType, &e.ServiceName,
		&e.Config, &e.Credentials, &e.Tools, &e.SaveMode, &e.CreatedAt); err != nil {
		return ConnectorEntry{}, err
	}
	return e, nil
}
