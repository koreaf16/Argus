// Package store
// File: sqlite.go
// Description: SQLite 기반 서버 저장소 구현
// Responsibility: servers 테이블 CRUD 및 크리덴셜 암호화/복호화 처리

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/yourorg/infractl/internal/crypto"
	"github.com/yourorg/infractl/internal/serverenv"
	_ "modernc.org/sqlite" // SQLite 드라이버 등록
)

const createTableSQL = `
CREATE TABLE IF NOT EXISTS servers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    host        TEXT    NOT NULL,
    port        INTEGER NOT NULL DEFAULT 22,
    user        TEXT    NOT NULL,
    auth_type   TEXT    NOT NULL,
    credential  TEXT    NOT NULL,
    os          TEXT    DEFAULT '',
    env_profile TEXT    DEFAULT '',
    server_dir TEXT  NOT NULL DEFAULT '~/.infractl/workspace',
    created_at  DATETIME NOT NULL
);`

// SQLiteStore는 SQLite를 사용하는 ServerStore 구현체이다.
type SQLiteStore struct {
	db        *sql.DB
	cryptoKey []byte
}

// NewSQLiteStore는 dbPath에 SQLite 데이터베이스를 열고 스키마를 초기화한다.
func NewSQLiteStore(ctx context.Context, dbPath string) (*SQLiteStore, error) {
	key, err := crypto.DeriveKey()
	if err != nil {
		return nil, fmt.Errorf("derive crypto key: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}

	// SQLite는 단일 파일 쓰기 기반이므로 MaxOpenConns를 1로 설정하여 경합을 방지한다.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if _, err := db.ExecContext(ctx, createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create servers table: %w", err)
	}

	// 단순 스키마 마이그레이션: os, env_profile 칼럼이 없을 수도 있으므로 무시 처리하며 추가 시도
	_, _ = db.ExecContext(ctx, "ALTER TABLE servers ADD COLUMN os TEXT DEFAULT ''")
	_, _ = db.ExecContext(ctx, "ALTER TABLE servers ADD COLUMN env_profile TEXT DEFAULT ''")
	_, _ = db.ExecContext(ctx, "ALTER TABLE servers ADD COLUMN server_dir TEXT NOT NULL DEFAULT '~/.infractl/workspace'")

	st := &SQLiteStore{db: db, cryptoKey: key}

	// Phase 4: 디스커버리 및 커넥터 테이블 초기화
	st.initDiscoverySchema(ctx)
	st.initConnectorSchema(ctx)

	// Phase 5: 세션, 히스토리, 실행이력, knowledge_base FTS5 초기화
	st.initSessionSchema(ctx)
	st.initHistorySchema(ctx)
	st.initExecLogSchema(ctx)
	st.initKnowledgeFTSSchema(ctx)

	// Phase 6: 적응형 학습 시스템 및 사용자 도구 테이블 초기화
	st.initLearnedSystemSchema(ctx)
	st.initUserToolSchema(ctx)
	st.initMemorySchema(ctx)

	// Phase 7: RAG 소스 테이블 초기화
	st.initRAGSourceSchema(ctx)

	// Phase 8: 비용/사용량, 체크포인트, 훅, 스케줄 테이블 초기화
	st.initCostSchema(ctx)
	st.initCheckpointSchema(ctx)
	st.initHookSchema(ctx)
	st.initScheduleSchema(ctx)

	// Audit: 감사(작업 이력) 레이어 초기화
	st.initAuditRunSchema(ctx)
	st.initAuditPlanSchema(ctx)
	st.initAuditStepSchema(ctx)
	st.initAuditCheckSchema(ctx)
	st.initAuditPrivilegeSchema(ctx)
	st.initAuditSyncOutboxSchema(ctx)

	slog.Debug("sqlite store initialized", "path", dbPath)
	return st, nil
}

// List는 등록된 모든 서버를 반환한다. 비밀번호 크리덴셜은 복호화하여 반환한다.
func (s *SQLiteStore) List(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, host, port, user, auth_type, credential, os, env_profile, server_dir, created_at FROM servers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		srv, err := s.scanServer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan server: %w", err)
		}
		servers = append(servers, srv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate servers: %w", err)
	}
	return servers, nil
}

// Get은 이름으로 서버를 조회한다.
func (s *SQLiteStore) Get(ctx context.Context, name string) (Server, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, host, port, user, auth_type, credential, os, env_profile, server_dir, created_at FROM servers WHERE name = ?`, name)
	srv, err := s.scanServer(row)
	if err == sql.ErrNoRows {
		return Server{}, fmt.Errorf("server not found: %s", name)
	}
	if err != nil {
		return Server{}, fmt.Errorf("get server %s: %w", name, err)
	}
	return srv, nil
}

// Add는 새 서버를 등록한다. 비밀번호 인증의 경우 크리덴셜을 암호화한다.
func (s *SQLiteStore) Add(ctx context.Context, server Server) error {
	cred := server.Credential
	if server.AuthType == AuthTypePassword && cred != "" {
		encrypted, err := crypto.Encrypt(s.cryptoKey, cred)
		if err != nil {
			return fmt.Errorf("encrypt credential: %w", err)
		}
		cred = encrypted
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO servers (name, host, port, user, auth_type, credential, os, env_profile, server_dir, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		server.Name, server.Host, server.Port, server.User,
		string(server.AuthType), cred, server.OS, server.EnvProfile,
		serverenv.RemoteDirOrDefault(server.WorkspaceDir), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("add server %s: %w", server.Name, err)
	}
	slog.Info("server registered", "name", server.Name, "host", server.Host)
	return nil
}

// Update는 기존 서버 정보를 수정한다. (주로 OS, EnvProfile 업데이트에 사용)
func (s *SQLiteStore) Update(ctx context.Context, server Server) error {
	cred := server.Credential
	if server.AuthType == AuthTypePassword && cred != "" {
		encrypted, err := crypto.Encrypt(s.cryptoKey, cred)
		if err != nil {
			return fmt.Errorf("encrypt credential: %w", err)
		}
		cred = encrypted
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE servers SET host = ?, port = ?, user = ?, auth_type = ?, credential = ?, os = ?, env_profile = ?, server_dir = ? WHERE name = ?`,
		server.Host, server.Port, server.User, string(server.AuthType), cred, server.OS, server.EnvProfile,
		serverenv.RemoteDirOrDefault(server.WorkspaceDir), server.Name,
	)
	if err != nil {
		return fmt.Errorf("update server %s: %w", server.Name, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("server not found: %s", server.Name)
	}
	slog.Info("server updated", "name", server.Name)
	return nil
}

// Remove는 이름으로 서버를 삭제한다.
func (s *SQLiteStore) Remove(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM servers WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("remove server %s: %w", name, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("server not found: %s", name)
	}
	slog.Info("server removed", "name", name)
	return nil
}

// Close는 데이터베이스 연결을 닫는다.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// scanServer는 sql.Row 또는 sql.Rows에서 Server를 읽는다.
// 비밀번호 크리덴셜은 복호화하여 반환한다.
type scanner interface {
	Scan(dest ...interface{}) error
}

func (s *SQLiteStore) scanServer(row scanner) (Server, error) {
	var srv Server
	var authType string
	var createdAt time.Time

	if err := row.Scan(&srv.ID, &srv.Name, &srv.Host, &srv.Port, &srv.User,
		&authType, &srv.Credential, &srv.OS, &srv.EnvProfile, &srv.WorkspaceDir, &createdAt); err != nil {
		return Server{}, err
	}

	srv.AuthType = AuthType(authType)
	srv.WorkspaceDir = serverenv.RemoteDirOrDefault(srv.WorkspaceDir)
	srv.CreatedAt = createdAt

	if srv.AuthType == AuthTypePassword && srv.Credential != "" {
		decrypted, err := crypto.Decrypt(s.cryptoKey, srv.Credential)
		if err != nil {
			return Server{}, fmt.Errorf("decrypt credential for %s: %w", srv.Name, err)
		}
		srv.Credential = decrypted
	}

	return srv, nil
}
