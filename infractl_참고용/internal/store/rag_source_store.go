// Package store
// File: rag_source_store.go
// Description: RAG 소스 저장소 인터페이스 및 SQLite CRUD 구현
// Responsibility: rag_sources 테이블 CRUD, 우선순위 관리, credentials 암호화

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/yourorg/infractl/internal/crypto"
)

const createRAGSourcesSQL = `
CREATE TABLE IF NOT EXISTS rag_sources (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    name             TEXT NOT NULL,
    server_name      TEXT NOT NULL,
    db_type          TEXT NOT NULL,
    db_host          TEXT NOT NULL DEFAULT '',
    db_port          INTEGER NOT NULL DEFAULT 0,
    db_name          TEXT NOT NULL,
    schema_name      TEXT NOT NULL DEFAULT '',
    table_name       TEXT NOT NULL,
    metadata_table   TEXT NOT NULL DEFAULT '',
    table_join_key   TEXT NOT NULL DEFAULT '',
    metadata_join_key TEXT NOT NULL DEFAULT '',
    metadata_columns TEXT NOT NULL DEFAULT '',
    text_column      TEXT NOT NULL,
    vector_column    TEXT NOT NULL,
    result_columns   TEXT NOT NULL,
    embedding_model  TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    priority         INTEGER NOT NULL DEFAULT 1,
    credentials      TEXT NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(server_name, db_type, db_name, table_name)
);`

// RAGSourceCredentials는 RAG 소스 접속 정보를 나타낸다.
type RAGSourceCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

// RAGSource는 rag_sources 테이블의 단일 행이다.
type RAGSource struct {
	ID              int64
	Name            string
	ServerName      string
	DBType          string // "oracle" | "mysql" | "postgresql"
	DBHost          string
	DBPort          int
	DBName          string // Oracle SID, MySQL DB명 등
	SchemaName      string
	TableName       string // 벡터 테이블명
	MetadataTable   string
	TableJoinKey    string
	MetadataJoinKey string
	MetadataColumns string
	TextColumn      string // 검색 텍스트 컬럼
	VectorColumn    string // 벡터 컬럼
	ResultColumns   string // 결과에 포함할 컬럼 (콤마 구분)
	EmbeddingModel  string // 벡터 생성에 사용된 임베딩 모델
	Description     string // 용도 설명
	Priority        int    // 검색 우선순위 (낮을수록 먼저)
	Credentials     RAGSourceCredentials
	CreatedAt       time.Time
}

// RAGSourceStore는 RAG 소스 영속화를 담당하는 인터페이스이다.
type RAGSourceStore interface {
	// SaveRAGSource는 RAG 소스를 저장하고 ID를 반환한다.
	SaveRAGSource(ctx context.Context, source RAGSource) (int64, error)

	// ListRAGSources는 우선순위 오름차순으로 전체 RAG 소스를 반환한다.
	ListRAGSources(ctx context.Context) ([]RAGSource, error)

	// GetRAGSource는 ID로 RAG 소스를 조회한다.
	GetRAGSource(ctx context.Context, id int64) (RAGSource, error)

	// DeleteRAGSource는 ID로 RAG 소스를 삭제한다.
	DeleteRAGSource(ctx context.Context, id int64) error

	// UpdateRAGSourcePriority는 RAG 소스의 우선순위를 변경한다.
	UpdateRAGSourcePriority(ctx context.Context, id int64, priority int) error
}

// initRAGSourceSchema는 rag_sources 테이블을 생성한다.
func (s *SQLiteStore) initRAGSourceSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createRAGSourcesSQL); err != nil {
		slog.Warn("create rag_sources table", "err", err)
	}
	migrations := []string{
		`ALTER TABLE rag_sources ADD COLUMN db_host TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE rag_sources ADD COLUMN db_port INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE rag_sources ADD COLUMN schema_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE rag_sources ADD COLUMN metadata_table TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE rag_sources ADD COLUMN table_join_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE rag_sources ADD COLUMN metadata_join_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE rag_sources ADD COLUMN metadata_columns TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range migrations {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			// duplicate-column errors are expected on existing DB files.
		}
	}
}

// SaveRAGSource는 RAG 소스를 저장하고 ID를 반환한다. credentials는 암호화하여 저장한다.
func (s *SQLiteStore) SaveRAGSource(ctx context.Context, source RAGSource) (int64, error) {
	encCreds, err := s.encryptRAGCreds(source.Credentials)
	if err != nil {
		return 0, fmt.Errorf("encrypt rag credentials: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO rag_sources
		 (name, server_name, db_type, db_host, db_port, db_name, schema_name, table_name,
		  metadata_table, table_join_key, metadata_join_key, metadata_columns,
		  text_column, vector_column, result_columns, embedding_model, description, priority, credentials, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(server_name, db_type, db_name, table_name) DO UPDATE SET
		   name=excluded.name, db_host=excluded.db_host, db_port=excluded.db_port,
		   schema_name=excluded.schema_name, metadata_table=excluded.metadata_table,
		   table_join_key=excluded.table_join_key, metadata_join_key=excluded.metadata_join_key,
		   metadata_columns=excluded.metadata_columns, text_column=excluded.text_column,
		   vector_column=excluded.vector_column, result_columns=excluded.result_columns,
		   embedding_model=excluded.embedding_model, description=excluded.description,
		   priority=excluded.priority, credentials=excluded.credentials`,
		source.Name, source.ServerName, source.DBType, source.DBHost, source.DBPort, source.DBName, source.SchemaName,
		source.TableName, source.MetadataTable, source.TableJoinKey, source.MetadataJoinKey, source.MetadataColumns,
		source.TextColumn, source.VectorColumn,
		source.ResultColumns, source.EmbeddingModel, source.Description,
		source.Priority, encCreds, time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("save rag source: %w", err)
	}
	id, _ := res.LastInsertId()
	slog.Info("rag source saved", "id", id, "name", source.Name, "server", source.ServerName)
	return id, nil
}

// ListRAGSources는 우선순위 오름차순으로 전체 RAG 소스를 반환한다.
func (s *SQLiteStore) ListRAGSources(ctx context.Context) ([]RAGSource, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, server_name, db_type, db_host, db_port, db_name, schema_name,
		        table_name, metadata_table, table_join_key, metadata_join_key, metadata_columns,
		        text_column, vector_column, result_columns, embedding_model, description,
		        priority, credentials, created_at
		 FROM rag_sources ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list rag sources: %w", err)
	}
	defer rows.Close()

	var result []RAGSource
	for rows.Next() {
		src, err := s.scanRAGSource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, src)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rag sources: %w", err)
	}
	return result, nil
}

// GetRAGSource는 ID로 RAG 소스를 조회한다.
func (s *SQLiteStore) GetRAGSource(ctx context.Context, id int64) (RAGSource, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, server_name, db_type, db_host, db_port, db_name, schema_name,
		        table_name, metadata_table, table_join_key, metadata_join_key, metadata_columns,
		        text_column, vector_column, result_columns, embedding_model, description,
		        priority, credentials, created_at
		 FROM rag_sources WHERE id=?`, id)
	src, err := s.scanRAGSource(row)
	if err == sql.ErrNoRows {
		return RAGSource{}, fmt.Errorf("rag source not found: %d", id)
	}
	return src, err
}

// DeleteRAGSource는 ID로 RAG 소스를 삭제한다.
func (s *SQLiteStore) DeleteRAGSource(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM rag_sources WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete rag source %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rag source not found: %d", id)
	}
	return nil
}

// UpdateRAGSourcePriority는 RAG 소스의 우선순위를 변경한다.
func (s *SQLiteStore) UpdateRAGSourcePriority(ctx context.Context, id int64, priority int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE rag_sources SET priority=? WHERE id=?`, priority, id)
	if err != nil {
		return fmt.Errorf("update rag source priority %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rag source not found: %d", id)
	}
	return nil
}

type ragSourceScanner interface {
	Scan(dest ...interface{}) error
}

func (s *SQLiteStore) scanRAGSource(row ragSourceScanner) (RAGSource, error) {
	var src RAGSource
	var encCreds string
	err := row.Scan(
		&src.ID, &src.Name, &src.ServerName, &src.DBType,
		&src.DBHost, &src.DBPort, &src.DBName, &src.SchemaName,
		&src.TableName, &src.MetadataTable, &src.TableJoinKey, &src.MetadataJoinKey, &src.MetadataColumns,
		&src.TextColumn, &src.VectorColumn,
		&src.ResultColumns, &src.EmbeddingModel, &src.Description,
		&src.Priority, &encCreds, &src.CreatedAt,
	)
	if err != nil {
		return RAGSource{}, fmt.Errorf("scan rag source: %w", err)
	}
	creds, err := s.decryptRAGCreds(encCreds)
	if err != nil {
		slog.Warn("decrypt rag credentials", "id", src.ID, "err", err)
	} else {
		src.Credentials = creds
	}
	return src, nil
}

func (s *SQLiteStore) encryptRAGCreds(creds RAGSourceCredentials) (string, error) {
	if creds.Username == "" && creds.Password == "" {
		return "", nil
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("marshal rag credentials: %w", err)
	}
	return crypto.Encrypt(s.cryptoKey, string(data))
}

func (s *SQLiteStore) decryptRAGCreds(encrypted string) (RAGSourceCredentials, error) {
	if encrypted == "" {
		return RAGSourceCredentials{}, nil
	}
	plain, err := crypto.Decrypt(s.cryptoKey, encrypted)
	if err != nil {
		return RAGSourceCredentials{}, fmt.Errorf("decrypt rag credentials: %w", err)
	}
	var creds RAGSourceCredentials
	if err := json.Unmarshal([]byte(plain), &creds); err != nil {
		return RAGSourceCredentials{}, fmt.Errorf("unmarshal rag credentials: %w", err)
	}
	return creds, nil
}
