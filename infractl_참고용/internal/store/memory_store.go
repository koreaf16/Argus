// Package store
// File: memory_store.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const createMemoryDocumentsSQL = `
CREATE TABLE IF NOT EXISTS memory_documents (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    source_type     TEXT NOT NULL,
    source_row_id   INTEGER NOT NULL,
    chunk_no        INTEGER NOT NULL DEFAULT 0,
    server_name     TEXT NOT NULL DEFAULT '',
    conversation_id INTEGER,
    role            TEXT NOT NULL DEFAULT '',
    title           TEXT NOT NULL DEFAULT '',
    content         TEXT NOT NULL DEFAULT '',
    metadata_json   TEXT NOT NULL DEFAULT '{}',
    embedding       BLOB,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_type, source_row_id, chunk_no)
);`

const createMemoryFTSSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
    title, content, source_type, server_name, role,
    content='memory_documents', content_rowid='id'
);`

const createMemoryIdxSQL = `
CREATE INDEX IF NOT EXISTS idx_memory_source ON memory_documents(source_type, source_row_id);
CREATE INDEX IF NOT EXISTS idx_memory_conv ON memory_documents(conversation_id);
CREATE INDEX IF NOT EXISTS idx_memory_role ON memory_documents(role);`

const createMemoryInsertTriggerSQL = `
CREATE TRIGGER IF NOT EXISTS memory_documents_ai AFTER INSERT ON memory_documents BEGIN
    INSERT INTO memory_fts(rowid, title, content, source_type, server_name, role)
    VALUES (new.id, new.title, new.content, new.source_type, new.server_name, new.role);
END;`

const createMemoryDeleteTriggerSQL = `
CREATE TRIGGER IF NOT EXISTS memory_documents_ad AFTER DELETE ON memory_documents BEGIN
    INSERT INTO memory_fts(memory_fts, rowid, title, content, source_type, server_name, role)
    VALUES ('delete', old.id, old.title, old.content, old.source_type, old.server_name, old.role);
END;`

const createMemoryUpdateTriggerSQL = `
CREATE TRIGGER IF NOT EXISTS memory_documents_au AFTER UPDATE ON memory_documents BEGIN
    INSERT INTO memory_fts(memory_fts, rowid, title, content, source_type, server_name, role)
    VALUES ('delete', old.id, old.title, old.content, old.source_type, old.server_name, old.role);
    INSERT INTO memory_fts(rowid, title, content, source_type, server_name, role)
    VALUES (new.id, new.title, new.content, new.source_type, new.server_name, new.role);
END;`

// MemoryDocument is a unified internal memory chunk used for hybrid retrieval.
type MemoryDocument struct {
	ID             int64
	SourceType     string
	SourceRowID    int64
	ChunkNo        int
	ServerName     string
	ConversationID *int64
	Role           string
	Title          string
	Content        string
	MetadataJSON   string
	Embedding      []byte
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// MemoryEmbeddingRow is the lightweight vector row used for in-process search.
type MemoryEmbeddingRow struct {
	ID        int64
	Embedding []byte
}

// MemoryQueryFilter scopes internal memory queries before vector scoring.
type MemoryQueryFilter struct {
	SourceTypes    []string
	ServerName     string
	ConversationID *int64
}

// MemoryFTSResult is an FTS hit together with the resolved document.
type MemoryFTSResult struct {
	Document MemoryDocument
	Score    float64
}

// MemoryStats exposes internal memory index status.
type MemoryStats struct {
	CountBySource   map[string]int
	TotalDocs       int
	TotalWithVector int
}

// MemoryStore provides persistence for the unified internal memory index.
type MemoryStore interface {
	ReplaceMemoryDocuments(ctx context.Context, sourceType string, sourceRowID int64, docs []MemoryDocument) ([]MemoryDocument, error)
	UpdateMemoryEmbedding(ctx context.Context, docID int64, embedding []byte) error
	GetMemoryDocument(ctx context.Context, id int64) (MemoryDocument, error)
	ListMemoryEmbeddings(ctx context.Context, filter MemoryQueryFilter) ([]MemoryEmbeddingRow, error)
	SearchMemoryFTS(ctx context.Context, query string, limit int, filter MemoryQueryFilter) ([]MemoryFTSResult, error)
	CountMemoryDocuments(ctx context.Context) (MemoryStats, error)
}

func (s *SQLiteStore) initMemorySchema(ctx context.Context) {
	stmts := []string{
		createMemoryDocumentsSQL,
		createMemoryFTSSQL,
		createMemoryIdxSQL,
		createMemoryInsertTriggerSQL,
		createMemoryDeleteTriggerSQL,
		createMemoryUpdateTriggerSQL,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			slog.Warn("init memory schema", "err", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO memory_fts(memory_fts) VALUES ('rebuild')`); err != nil {
		slog.Debug("rebuild memory_fts", "err", err)
	}
}

func (s *SQLiteStore) ReplaceMemoryDocuments(ctx context.Context, sourceType string, sourceRowID int64, docs []MemoryDocument) ([]MemoryDocument, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin replace memory docs: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx,
		`DELETE FROM memory_documents WHERE source_type=? AND source_row_id=?`,
		sourceType, sourceRowID,
	); err != nil {
		return nil, fmt.Errorf("delete old memory docs: %w", err)
	}

	now := time.Now().UTC()
	out := make([]MemoryDocument, 0, len(docs))
	for _, doc := range docs {
		if doc.MetadataJSON == "" {
			doc.MetadataJSON = "{}"
		}
		var convID interface{}
		if doc.ConversationID != nil {
			convID = *doc.ConversationID
		}
		var embedding interface{}
		if len(doc.Embedding) > 0 {
			embedding = doc.Embedding
		}
		createdAt := doc.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := doc.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		res, execErr := tx.ExecContext(ctx,
			`INSERT INTO memory_documents
			 (source_type, source_row_id, chunk_no, server_name, conversation_id, role, title, content, metadata_json, embedding, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sourceType, sourceRowID, doc.ChunkNo, doc.ServerName, convID, doc.Role,
			doc.Title, doc.Content, doc.MetadataJSON, embedding, createdAt, updatedAt,
		)
		if execErr != nil {
			err = fmt.Errorf("insert memory doc: %w", execErr)
			return nil, err
		}
		id, _ := res.LastInsertId()
		doc.ID = id
		doc.SourceType = sourceType
		doc.SourceRowID = sourceRowID
		doc.CreatedAt = createdAt
		doc.UpdatedAt = updatedAt
		out = append(out, doc)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit memory docs: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) UpdateMemoryEmbedding(ctx context.Context, docID int64, embedding []byte) error {
	var value interface{}
	if len(embedding) > 0 {
		value = embedding
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE memory_documents SET embedding=?, updated_at=? WHERE id=?`,
		value, time.Now().UTC(), docID,
	); err != nil {
		return fmt.Errorf("update memory embedding %d: %w", docID, err)
	}
	return nil
}

func (s *SQLiteStore) GetMemoryDocument(ctx context.Context, id int64) (MemoryDocument, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, source_type, source_row_id, chunk_no, server_name, conversation_id, role, title, content, metadata_json, embedding, created_at, updated_at
		 FROM memory_documents WHERE id=?`,
		id,
	)
	return scanMemoryDocument(row)
}

func (s *SQLiteStore) ListMemoryEmbeddings(ctx context.Context, filter MemoryQueryFilter) ([]MemoryEmbeddingRow, error) {
	query := `SELECT id, embedding FROM memory_documents WHERE embedding IS NOT NULL`
	args := make([]interface{}, 0)
	query, args = appendMemoryFilter(query, args, filter, "")

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list memory embeddings: %w", err)
	}
	defer rows.Close()

	var result []MemoryEmbeddingRow
	for rows.Next() {
		var row MemoryEmbeddingRow
		if err := rows.Scan(&row.ID, &row.Embedding); err != nil {
			return nil, fmt.Errorf("scan memory embedding row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memory embeddings: %w", err)
	}
	return result, nil
}

func (s *SQLiteStore) SearchMemoryFTS(ctx context.Context, query string, limit int, filter MemoryQueryFilter) ([]MemoryFTSResult, error) {
	if limit <= 0 {
		limit = 5
	}
	sqlText := `SELECT md.id, md.source_type, md.source_row_id, md.chunk_no, md.server_name, md.conversation_id, md.role, md.title, md.content, md.metadata_json, md.embedding, md.created_at, md.updated_at,
	        bm25(memory_fts) AS rank
	 FROM memory_fts
	 JOIN memory_documents md ON md.id = memory_fts.rowid
	 WHERE memory_fts MATCH ?`
	args := []interface{}{query}
	sqlText, args = appendMemoryFilter(sqlText, args, filter, "md.")
	sqlText += `
	 ORDER BY rank
	 LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("search memory fts: %w", err)
	}
	defer rows.Close()

	var result []MemoryFTSResult
	for rows.Next() {
		var hit MemoryFTSResult
		if err := scanMemoryDocumentWithScore(rows, &hit); err != nil {
			return nil, err
		}
		result = append(result, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memory fts rows: %w", err)
	}
	return result, nil
}

func (s *SQLiteStore) CountMemoryDocuments(ctx context.Context) (MemoryStats, error) {
	stats := MemoryStats{CountBySource: make(map[string]int)}

	rows, err := s.db.QueryContext(ctx,
		`SELECT source_type, COUNT(*) FROM memory_documents GROUP BY source_type`,
	)
	if err != nil {
		return stats, fmt.Errorf("count memory documents: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return stats, fmt.Errorf("scan memory count row: %w", err)
		}
		stats.CountBySource[source] = count
		stats.TotalDocs += count
	}
	if err := rows.Err(); err != nil {
		return stats, fmt.Errorf("iterate memory counts: %w", err)
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memory_documents WHERE embedding IS NOT NULL`,
	)
	if err := row.Scan(&stats.TotalWithVector); err != nil {
		return stats, fmt.Errorf("count memory embeddings: %w", err)
	}
	return stats, nil
}

type memoryScanner interface {
	Scan(dest ...interface{}) error
}

func scanMemoryDocument(row memoryScanner) (MemoryDocument, error) {
	var doc MemoryDocument
	var convID sql.NullInt64
	if err := row.Scan(
		&doc.ID, &doc.SourceType, &doc.SourceRowID, &doc.ChunkNo, &doc.ServerName,
		&convID, &doc.Role, &doc.Title, &doc.Content, &doc.MetadataJSON, &doc.Embedding,
		&doc.CreatedAt, &doc.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return MemoryDocument{}, fmt.Errorf("memory document not found")
		}
		return MemoryDocument{}, fmt.Errorf("scan memory document: %w", err)
	}
	if convID.Valid {
		doc.ConversationID = &convID.Int64
	}
	return doc, nil
}

func scanMemoryDocumentWithScore(row memoryScanner, hit *MemoryFTSResult) error {
	var convID sql.NullInt64
	if err := row.Scan(
		&hit.Document.ID, &hit.Document.SourceType, &hit.Document.SourceRowID, &hit.Document.ChunkNo, &hit.Document.ServerName,
		&convID, &hit.Document.Role, &hit.Document.Title, &hit.Document.Content, &hit.Document.MetadataJSON, &hit.Document.Embedding,
		&hit.Document.CreatedAt, &hit.Document.UpdatedAt, &hit.Score,
	); err != nil {
		return fmt.Errorf("scan memory fts row: %w", err)
	}
	if convID.Valid {
		hit.Document.ConversationID = &convID.Int64
	}
	return nil
}

func appendMemoryFilter(query string, args []interface{}, filter MemoryQueryFilter, prefix string) (string, []interface{}) {
	var clauses []string

	if len(filter.SourceTypes) > 0 {
		placeholders := make([]string, len(filter.SourceTypes))
		for i, sourceType := range filter.SourceTypes {
			placeholders[i] = "?"
			args = append(args, sourceType)
		}
		clauses = append(clauses, fmt.Sprintf("%ssource_type IN (%s)", prefix, strings.Join(placeholders, ",")))
	}

	if strings.TrimSpace(filter.ServerName) != "" {
		clauses = append(clauses, fmt.Sprintf("(%sserver_name = '' OR %sserver_name = ?)", prefix, prefix))
		args = append(args, filter.ServerName)
	}

	if filter.ConversationID != nil {
		clauses = append(clauses, fmt.Sprintf("(%ssource_type <> 'conversation' OR %sconversation_id = ?)", prefix, prefix))
		args = append(args, *filter.ConversationID)
	}

	if len(clauses) == 0 {
		return query, args
	}

	return query + " AND " + strings.Join(clauses, " AND "), args
}

