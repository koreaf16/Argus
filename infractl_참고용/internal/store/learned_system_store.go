// Package store
// File: learned_system_store.go
// Description: 적응형 학습으로 발견한 시스템 정보 저장소
// Responsibility: learned_systems 테이블 CRUD 및 LearnedSystemStore 인터페이스 정의

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const createLearnedSystemsSQL = `
CREATE TABLE IF NOT EXISTS learned_systems (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    service_type TEXT NOT NULL,
    cli_path     TEXT NOT NULL DEFAULT '',
    config_path  TEXT NOT NULL DEFAULT '',
    log_path     TEXT NOT NULL DEFAULT '',
    commands     TEXT NOT NULL DEFAULT '{}',
    server_name  TEXT NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

// LearnedSystem은 웹 검색 + 서버 탐색으로 학습한 시스템 정보이다.
type LearnedSystem struct {
	ID          int64
	ServiceType string    // "kafka", "redis", "nginx" 등
	CLIPath     string    // "/opt/kafka/bin/"
	ConfigPath  string    // "/opt/kafka/config/"
	LogPath     string    // "/opt/kafka/logs/"
	Commands    string    // JSON: 학습된 명령 목록
	ServerName  string    // 어느 서버에서 학습했는지
	CreatedAt   time.Time
}

// LearnedSystemStore는 학습된 시스템 정보 영속화를 담당하는 인터페이스이다.
type LearnedSystemStore interface {
	// SaveLearnedSystem은 학습된 시스템 정보를 저장하고 ID를 반환한다.
	SaveLearnedSystem(ctx context.Context, sys LearnedSystem) (int64, error)

	// GetLearnedSystem은 서비스 타입과 서버 이름으로 학습 정보를 조회한다.
	GetLearnedSystem(ctx context.Context, serviceType, serverName string) (LearnedSystem, error)

	// ListLearnedSystems는 저장된 학습 시스템 목록을 반환한다.
	ListLearnedSystems(ctx context.Context) ([]LearnedSystem, error)

	// DeleteLearnedSystem은 ID로 학습 정보를 삭제한다.
	DeleteLearnedSystem(ctx context.Context, id int64) error
}

// initLearnedSystemSchema는 learned_systems 테이블을 생성한다.
func (s *SQLiteStore) initLearnedSystemSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createLearnedSystemsSQL); err != nil {
		slog.Warn("create learned_systems table", "err", err)
	}
}

// SaveLearnedSystem은 학습된 시스템 정보를 저장하고 ID를 반환한다.
func (s *SQLiteStore) SaveLearnedSystem(ctx context.Context, sys LearnedSystem) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO learned_systems (service_type, cli_path, config_path, log_path, commands, server_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sys.ServiceType, sys.CLIPath, sys.ConfigPath, sys.LogPath,
		sys.Commands, sys.ServerName, time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("save learned system: %w", err)
	}
	id, _ := res.LastInsertId()
	slog.Info("learned system saved", "type", sys.ServiceType, "server", sys.ServerName)
	return id, nil
}

// GetLearnedSystem은 서비스 타입과 서버 이름으로 가장 최근 학습 정보를 반환한다.
func (s *SQLiteStore) GetLearnedSystem(ctx context.Context, serviceType, serverName string) (LearnedSystem, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, service_type, cli_path, config_path, log_path, commands, server_name, created_at
		 FROM learned_systems WHERE service_type=? AND server_name=? ORDER BY created_at DESC LIMIT 1`,
		serviceType, serverName,
	)
	return scanLearnedSystem(row)
}

// ListLearnedSystems는 저장된 모든 학습 시스템을 최신순으로 반환한다.
func (s *SQLiteStore) ListLearnedSystems(ctx context.Context) ([]LearnedSystem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, service_type, cli_path, config_path, log_path, commands, server_name, created_at
		 FROM learned_systems ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list learned systems: %w", err)
	}
	defer rows.Close()

	var result []LearnedSystem
	for rows.Next() {
		sys, err := scanLearnedSystem(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sys)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate learned systems: %w", err)
	}
	return result, nil
}

// DeleteLearnedSystem은 ID로 학습 정보를 삭제한다.
func (s *SQLiteStore) DeleteLearnedSystem(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM learned_systems WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete learned system %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("learned system not found: %d", id)
	}
	return nil
}

type learnedSystemScanner interface {
	Scan(dest ...interface{}) error
}

func scanLearnedSystem(row learnedSystemScanner) (LearnedSystem, error) {
	var sys LearnedSystem
	err := row.Scan(
		&sys.ID, &sys.ServiceType, &sys.CLIPath, &sys.ConfigPath,
		&sys.LogPath, &sys.Commands, &sys.ServerName, &sys.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return LearnedSystem{}, fmt.Errorf("learned system not found")
	}
	if err != nil {
		return LearnedSystem{}, fmt.Errorf("scan learned system: %w", err)
	}
	return sys, nil
}
