package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type KnowledgeEntry struct {
	ID                int64
	Category          string
	Title             string
	Situation         string
	Resolution        string
	ToolName          string
	ErrorPattern      string
	SuccessCommand    string
	ServerName        string
	TaskKey           string
	CommandKey        string
	WorkflowJSON      string
	Embedding         []byte
	SourceExecutionID *int64
	Confidence        float64
	UseCount          int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastUsedAt        *time.Time
}

type EmbeddingRow struct {
	ID         int64
	Embedding  []byte
	Confidence float64
}

type KnowledgeStore interface {
	SaveKnowledge(ctx context.Context, entry KnowledgeEntry) (int64, error)
	SearchKnowledge(ctx context.Context, query string, limit int) ([]KnowledgeEntry, error)
	GetKnowledge(ctx context.Context, id int64) (KnowledgeEntry, error)
	ListKnowledge(ctx context.Context, category string, limit int) ([]KnowledgeEntry, error)
	DeleteKnowledge(ctx context.Context, id int64) error
	IncrementUseCount(ctx context.Context, id int64) error
	ListEmbeddings(ctx context.Context) ([]EmbeddingRow, error)
	CountKnowledge(ctx context.Context) (map[string]int, error)
	UpsertTaskMemory(ctx context.Context, entry KnowledgeEntry) (int64, error)
	SearchTaskMemories(ctx context.Context, category, serverName, taskKey, commandKey string, limit int) ([]KnowledgeEntry, error)
}

func (s *SQLiteStore) SaveKnowledge(ctx context.Context, entry KnowledgeEntry) (int64, error) {
	var srcID interface{}
	if entry.SourceExecutionID != nil {
		srcID = *entry.SourceExecutionID
	}
	var embedding interface{}
	if len(entry.Embedding) > 0 {
		embedding = entry.Embedding
	}
	if strings.TrimSpace(entry.WorkflowJSON) == "" {
		entry.WorkflowJSON = "[]"
	}
	now := time.Now().UTC()

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO knowledge_base
		 (category, title, situation, resolution, tool_name, error_pattern, success_command,
		  server_name, task_key, command_key, workflow_json,
		  embedding, source_execution_id, confidence, use_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		entry.Category, entry.Title, entry.Situation, entry.Resolution,
		entry.ToolName, entry.ErrorPattern, entry.SuccessCommand,
		entry.ServerName, entry.TaskKey, entry.CommandKey, entry.WorkflowJSON,
		embedding, srcID, entry.Confidence, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("save knowledge: %w", err)
	}
	id, _ := res.LastInsertId()

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO knowledge_fts(rowid, title, situation, resolution, error_pattern, category)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, entry.Title, entry.Situation, entry.Resolution, entry.ErrorPattern, entry.Category,
	); err != nil {
		slog.Warn("sync knowledge to fts5", "id", id, "err", err)
	}

	slog.Info("knowledge saved", "id", id, "category", entry.Category, "title", entry.Title)
	return id, nil
}

func (s *SQLiteStore) SearchKnowledge(ctx context.Context, query string, limit int) ([]KnowledgeEntry, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT kb.id, kb.category, kb.title, kb.situation, kb.resolution,
		        kb.tool_name, kb.error_pattern, kb.success_command,
		        kb.server_name, kb.task_key, kb.command_key, kb.workflow_json,
		        kb.source_execution_id, kb.confidence, kb.use_count,
		        kb.created_at, kb.updated_at, kb.last_used_at
		 FROM knowledge_fts
		 JOIN knowledge_base kb ON knowledge_fts.rowid = kb.id
		 WHERE knowledge_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search knowledge: %w", err)
	}
	defer rows.Close()
	return scanKnowledgeRows(rows)
}

func (s *SQLiteStore) GetKnowledge(ctx context.Context, id int64) (KnowledgeEntry, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, category, title, situation, resolution,
		        tool_name, error_pattern, success_command,
		        server_name, task_key, command_key, workflow_json,
		        source_execution_id, confidence, use_count,
		        created_at, updated_at, last_used_at
		 FROM knowledge_base WHERE id=?`, id)
	return scanKnowledgeRow(row)
}

func (s *SQLiteStore) ListKnowledge(ctx context.Context, category string, limit int) ([]KnowledgeEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows *sql.Rows
	var err error
	if category == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, category, title, situation, resolution,
			        tool_name, error_pattern, success_command,
			        server_name, task_key, command_key, workflow_json,
			        source_execution_id, confidence, use_count,
			        created_at, updated_at, last_used_at
			 FROM knowledge_base ORDER BY created_at DESC LIMIT ?`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, category, title, situation, resolution,
			        tool_name, error_pattern, success_command,
			        server_name, task_key, command_key, workflow_json,
			        source_execution_id, confidence, use_count,
			        created_at, updated_at, last_used_at
			 FROM knowledge_base WHERE category=? ORDER BY created_at DESC LIMIT ?`,
			category, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list knowledge: %w", err)
	}
	defer rows.Close()
	return scanKnowledgeRows(rows)
}

func (s *SQLiteStore) DeleteKnowledge(ctx context.Context, id int64) error {
	s.db.ExecContext(ctx, //nolint: errcheck
		`INSERT INTO knowledge_fts(knowledge_fts, rowid, title, situation, resolution, error_pattern, category)
		 SELECT 'delete', id, title, situation, resolution, error_pattern, category
		 FROM knowledge_base WHERE id=?`, id)

	res, err := s.db.ExecContext(ctx, `DELETE FROM knowledge_base WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete knowledge %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("knowledge not found: %d", id)
	}
	return nil
}

func (s *SQLiteStore) IncrementUseCount(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE knowledge_base SET use_count=use_count+1, last_used_at=?, updated_at=? WHERE id=?`,
		time.Now().UTC(), time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("increment use count %d: %w", id, err)
	}
	return nil
}

func (s *SQLiteStore) ListEmbeddings(ctx context.Context) ([]EmbeddingRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, embedding, confidence FROM knowledge_base WHERE embedding IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("list embeddings: %w", err)
	}
	defer rows.Close()

	var result []EmbeddingRow
	for rows.Next() {
		var r EmbeddingRow
		if err := rows.Scan(&r.ID, &r.Embedding, &r.Confidence); err != nil {
			return nil, fmt.Errorf("scan embedding row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate embeddings: %w", err)
	}
	return result, nil
}

func (s *SQLiteStore) CountKnowledge(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT category, COUNT(*) FROM knowledge_base GROUP BY category`)
	if err != nil {
		return nil, fmt.Errorf("count knowledge: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var cat string
		var cnt int
		if err := rows.Scan(&cat, &cnt); err != nil {
			return nil, fmt.Errorf("scan count row: %w", err)
		}
		result[cat] = cnt
	}
	return result, nil
}

func (s *SQLiteStore) UpsertTaskMemory(ctx context.Context, entry KnowledgeEntry) (int64, error) {
	if strings.TrimSpace(entry.WorkflowJSON) == "" {
		entry.WorkflowJSON = "[]"
	}
	now := time.Now().UTC()
	row := s.db.QueryRowContext(ctx,
		`SELECT id FROM knowledge_base
		 WHERE category=? AND server_name=? AND task_key=? AND command_key=? LIMIT 1`,
		entry.Category, entry.ServerName, entry.TaskKey, entry.CommandKey,
	)
	var id int64
	if err := row.Scan(&id); err == nil {
		_, err := s.db.ExecContext(ctx,
			`UPDATE knowledge_base
			 SET title=?, situation=?, resolution=?, tool_name=?, error_pattern=?, success_command=?,
			     workflow_json=?, confidence=?, updated_at=?
			 WHERE id=?`,
			entry.Title, entry.Situation, entry.Resolution, entry.ToolName, entry.ErrorPattern, entry.SuccessCommand,
			entry.WorkflowJSON, entry.Confidence, now, id,
		)
		if err != nil {
			return 0, fmt.Errorf("update task memory: %w", err)
		}
		s.db.ExecContext(ctx, //nolint: errcheck
			`INSERT INTO knowledge_fts(knowledge_fts, rowid, title, situation, resolution, error_pattern, category)
			 VALUES ('delete', ?, ?, ?, ?, ?, ?)`,
			id, entry.Title, entry.Situation, entry.Resolution, entry.ErrorPattern, entry.Category,
		)
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO knowledge_fts(rowid, title, situation, resolution, error_pattern, category)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			id, entry.Title, entry.Situation, entry.Resolution, entry.ErrorPattern, entry.Category,
		); err != nil {
			slog.Debug("sync updated task memory to fts5", "id", id, "err", err)
		}
		return id, nil
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO knowledge_base
		 (category, title, situation, resolution, tool_name, error_pattern, success_command,
		  server_name, task_key, command_key, workflow_json,
		  confidence, use_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		entry.Category, entry.Title, entry.Situation, entry.Resolution,
		entry.ToolName, entry.ErrorPattern, entry.SuccessCommand,
		entry.ServerName, entry.TaskKey, entry.CommandKey, entry.WorkflowJSON,
		entry.Confidence, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert task memory: %w", err)
	}
	id, _ = res.LastInsertId()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO knowledge_fts(rowid, title, situation, resolution, error_pattern, category)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, entry.Title, entry.Situation, entry.Resolution, entry.ErrorPattern, entry.Category,
	); err != nil {
		slog.Debug("sync inserted task memory to fts5", "id", id, "err", err)
	}
	return id, nil
}

func (s *SQLiteStore) SearchTaskMemories(ctx context.Context, category, serverName, taskKey, commandKey string, limit int) ([]KnowledgeEntry, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, category, title, situation, resolution,
		        tool_name, error_pattern, success_command,
		        server_name, task_key, command_key, workflow_json,
		        source_execution_id, confidence, use_count,
		        created_at, updated_at, last_used_at
		 FROM knowledge_base
		 WHERE (? = '' OR category = ?)
		   AND (? = '' OR server_name = ?)
		   AND (? = '' OR task_key = ?)
		   AND (? = '' OR command_key = ?)
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		category, category,
		serverName, serverName,
		taskKey, taskKey,
		commandKey, commandKey,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search task memories: %w", err)
	}
	defer rows.Close()
	return scanKnowledgeRows(rows)
}

func scanKnowledgeRows(rows *sql.Rows) ([]KnowledgeEntry, error) {
	var result []KnowledgeEntry
	for rows.Next() {
		entry, err := scanKnowledgeRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge: %w", err)
	}
	return result, nil
}

type knowledgeScanner interface {
	Scan(dest ...interface{}) error
}

func scanKnowledgeRow(row knowledgeScanner) (KnowledgeEntry, error) {
	var e KnowledgeEntry
	var srcID sql.NullInt64
	var lastUsed sql.NullTime
	err := row.Scan(
		&e.ID, &e.Category, &e.Title, &e.Situation, &e.Resolution,
		&e.ToolName, &e.ErrorPattern, &e.SuccessCommand,
		&e.ServerName, &e.TaskKey, &e.CommandKey, &e.WorkflowJSON,
		&srcID, &e.Confidence, &e.UseCount,
		&e.CreatedAt, &e.UpdatedAt, &lastUsed,
	)
	if err == sql.ErrNoRows {
		return KnowledgeEntry{}, fmt.Errorf("knowledge not found")
	}
	if err != nil {
		return KnowledgeEntry{}, fmt.Errorf("scan knowledge: %w", err)
	}
	if srcID.Valid {
		e.SourceExecutionID = &srcID.Int64
	}
	if lastUsed.Valid {
		e.LastUsedAt = &lastUsed.Time
	}
	return e, nil
}
