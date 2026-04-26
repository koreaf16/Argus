// Package store
// File: system_profile_store.go
// Description: capability/profile 기반 자동 tool 생성을 위한 profile 저장소
// Responsibility: external RAG/web/probe 결과를 구조화해 보관

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const createSystemProfilesSQL = `
CREATE TABLE IF NOT EXISTS system_profiles (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_ref     TEXT NOT NULL UNIQUE,
    server_name     TEXT NOT NULL,
    service_type    TEXT NOT NULL,
    service_name    TEXT NOT NULL DEFAULT '',
    host            TEXT NOT NULL DEFAULT '',
    port            INTEGER NOT NULL DEFAULT 0,
    fingerprint_json TEXT NOT NULL DEFAULT '{}',
    profile_json     TEXT NOT NULL DEFAULT '{}',
    commands_json    TEXT NOT NULL DEFAULT '{}',
    evidence_json    TEXT NOT NULL DEFAULT '[]',
    questions_json   TEXT NOT NULL DEFAULT '[]',
    status          TEXT NOT NULL DEFAULT 'prepared',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const createSystemProfilesIdxSQL = `
CREATE INDEX IF NOT EXISTS idx_system_profiles_server ON system_profiles(server_name, service_type);
CREATE INDEX IF NOT EXISTS idx_system_profiles_ref ON system_profiles(profile_ref);`

// SystemProfile은 capability synthesis 결과를 담는다.
type SystemProfile struct {
	ID              int64
	ProfileRef      string
	ServerName      string
	ServiceType     string
	ServiceName     string
	Host            string
	Port            int
	FingerprintJSON string
	ProfileJSON     string
	CommandsJSON    string
	EvidenceJSON    string
	QuestionsJSON   string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SystemProfileStore는 profile 저장/조회 기능을 제공한다.
type SystemProfileStore interface {
	SaveSystemProfile(ctx context.Context, profile SystemProfile) (int64, error)
	GetSystemProfile(ctx context.Context, profileRef string) (SystemProfile, error)
	ListSystemProfiles(ctx context.Context) ([]SystemProfile, error)
	DeleteSystemProfile(ctx context.Context, profileRef string) error
}

// initSystemProfileSchema는 system_profiles 테이블을 생성한다.
func (s *SQLiteStore) initSystemProfileSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createSystemProfilesSQL); err != nil {
		slog.Warn("create system_profiles table", "err", err)
	}
	if _, err := s.db.ExecContext(ctx, createSystemProfilesIdxSQL); err != nil {
		slog.Warn("create system_profiles index", "err", err)
	}
}

// SaveSystemProfile은 profile을 저장하고 ID를 반환한다.
func (s *SQLiteStore) SaveSystemProfile(ctx context.Context, profile SystemProfile) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO system_profiles
		 (profile_ref, server_name, service_type, service_name, host, port,
		  fingerprint_json, profile_json, commands_json, evidence_json, questions_json,
		  status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(profile_ref) DO UPDATE SET
		   server_name=excluded.server_name,
		   service_type=excluded.service_type,
		   service_name=excluded.service_name,
		   host=excluded.host,
		   port=excluded.port,
		   fingerprint_json=excluded.fingerprint_json,
		   profile_json=excluded.profile_json,
		   commands_json=excluded.commands_json,
		   evidence_json=excluded.evidence_json,
		   questions_json=excluded.questions_json,
		   status=excluded.status,
		   updated_at=excluded.updated_at`,
		profile.ProfileRef, profile.ServerName, profile.ServiceType, profile.ServiceName,
		profile.Host, profile.Port, profile.FingerprintJSON, profile.ProfileJSON,
		profile.CommandsJSON, profile.EvidenceJSON, profile.QuestionsJSON,
		profile.Status, time.Now().UTC(), time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("save system profile %s: %w", profile.ProfileRef, err)
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		row := s.db.QueryRowContext(ctx, `SELECT id FROM system_profiles WHERE profile_ref=?`, profile.ProfileRef)
		if scanErr := row.Scan(&id); scanErr != nil {
			return 0, fmt.Errorf("lookup system profile id: %w", scanErr)
		}
	}
	slog.Debug("system profile saved", "ref", profile.ProfileRef, "server", profile.ServerName, "type", profile.ServiceType)
	return id, nil
}

// GetSystemProfile은 profile_ref로 단일 profile을 조회한다.
func (s *SQLiteStore) GetSystemProfile(ctx context.Context, profileRef string) (SystemProfile, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, profile_ref, server_name, service_type, service_name, host, port,
		        fingerprint_json, profile_json, commands_json, evidence_json, questions_json,
		        status, created_at, updated_at
		 FROM system_profiles WHERE profile_ref=?`, profileRef)
	profile, err := scanSystemProfile(row)
	if err == sql.ErrNoRows {
		return SystemProfile{}, fmt.Errorf("system profile not found: %s", profileRef)
	}
	if err != nil {
		return SystemProfile{}, fmt.Errorf("get system profile: %w", err)
	}
	return profile, nil
}

// ListSystemProfiles는 저장된 profile 목록을 반환한다.
func (s *SQLiteStore) ListSystemProfiles(ctx context.Context) ([]SystemProfile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, profile_ref, server_name, service_type, service_name, host, port,
		        fingerprint_json, profile_json, commands_json, evidence_json, questions_json,
		        status, created_at, updated_at
		 FROM system_profiles ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list system profiles: %w", err)
	}
	defer rows.Close()

	var result []SystemProfile
	for rows.Next() {
		profile, err := scanSystemProfile(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate system profiles: %w", err)
	}
	return result, nil
}

// DeleteSystemProfile은 profile_ref로 profile을 삭제한다.
func (s *SQLiteStore) DeleteSystemProfile(ctx context.Context, profileRef string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM system_profiles WHERE profile_ref=?`, profileRef)
	if err != nil {
		return fmt.Errorf("delete system profile %s: %w", profileRef, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("system profile not found: %s", profileRef)
	}
	return nil
}

type systemProfileScanner interface {
	Scan(dest ...interface{}) error
}

func scanSystemProfile(row systemProfileScanner) (SystemProfile, error) {
	var p SystemProfile
	if err := row.Scan(
		&p.ID, &p.ProfileRef, &p.ServerName, &p.ServiceType, &p.ServiceName, &p.Host, &p.Port,
		&p.FingerprintJSON, &p.ProfileJSON, &p.CommandsJSON, &p.EvidenceJSON, &p.QuestionsJSON,
		&p.Status, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return SystemProfile{}, err
	}
	return p, nil
}
