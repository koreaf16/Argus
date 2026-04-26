package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

const createConversationsSQL = `
CREATE TABLE IF NOT EXISTS conversations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    title      TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const createMessagesSQL = `
CREATE TABLE IF NOT EXISTS messages (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL DEFAULT '',
    tool_calls      TEXT NOT NULL DEFAULT '[]',
    tool_call_id    TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL DEFAULT 'message',
    metadata_json   TEXT NOT NULL DEFAULT '{}',
    archived_at     DATETIME NULL,
    timestamp       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const createMessagesIdxSQL = `
CREATE INDEX IF NOT EXISTS idx_messages_conv ON messages(conversation_id);`

type Conversation struct {
	ID        int64
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SessionMessage struct {
	ID             int64
	ConversationID int64
	Role           string
	Content        string
	ToolCalls      string // JSON array
	ToolCallID     string
	Name           string
	Kind           string
	MetadataJSON   string
	ArchivedAt     *time.Time
	Timestamp      time.Time
}

type SessionStore interface {
	CreateConversation(ctx context.Context, title string) (int64, error)
	UpdateConversationTitle(ctx context.Context, id int64, title string) error
	GetConversation(ctx context.Context, id int64) (Conversation, error)
	ListConversations(ctx context.Context, limit int) ([]Conversation, error)
	SaveMessage(ctx context.Context, msg SessionMessage) (int64, error)
	LoadMessages(ctx context.Context, conversationID int64) ([]SessionMessage, error)
}

func (s *SQLiteStore) initSessionSchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createConversationsSQL); err != nil {
		slog.Warn("create conversations table", "err", err)
	}
	if _, err := s.db.ExecContext(ctx, createMessagesSQL); err != nil {
		slog.Warn("create messages table", "err", err)
	}
	if _, err := s.db.ExecContext(ctx, createMessagesIdxSQL); err != nil {
		slog.Warn("create messages index", "err", err)
	}
	alterStmts := []string{
		`ALTER TABLE messages ADD COLUMN kind TEXT NOT NULL DEFAULT 'message'`,
		`ALTER TABLE messages ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE messages ADD COLUMN archived_at DATETIME NULL`,
	}
	for _, stmt := range alterStmts {
		s.db.ExecContext(ctx, stmt) //nolint: errcheck
	}
}

func (s *SQLiteStore) CreateConversation(ctx context.Context, title string) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO conversations (title, created_at, updated_at) VALUES (?, ?, ?)`,
		title, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create conversation: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get conversation id: %w", err)
	}
	slog.Debug("conversation created", "id", id, "title", title)
	return id, nil
}

func (s *SQLiteStore) UpdateConversationTitle(ctx context.Context, id int64, title string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET title=?, updated_at=? WHERE id=?`,
		title, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update conversation title %d: %w", id, err)
	}
	return nil
}

func (s *SQLiteStore) GetConversation(ctx context.Context, id int64) (Conversation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, title, created_at, updated_at FROM conversations WHERE id=?`, id)
	var c Conversation
	if err := row.Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return Conversation{}, fmt.Errorf("conversation not found: %d", id)
		}
		return Conversation{}, fmt.Errorf("get conversation %d: %w", id, err)
	}
	return c, nil
}

func (s *SQLiteStore) ListConversations(ctx context.Context, limit int) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, created_at, updated_at FROM conversations ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		convs = append(convs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}
	return convs, nil
}

func (s *SQLiteStore) SaveMessage(ctx context.Context, msg SessionMessage) (int64, error) {
	toolCallsJSON := msg.ToolCalls
	if toolCallsJSON == "" {
		toolCallsJSON = "[]"
	}
	kind := msg.Kind
	if kind == "" {
		kind = "message"
	}
	metadataJSON := msg.MetadataJSON
	if metadataJSON == "" {
		metadataJSON = "{}"
	}
	var archivedAt interface{}
	if msg.ArchivedAt != nil {
		archivedAt = msg.ArchivedAt.UTC()
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (conversation_id, role, content, tool_calls, tool_call_id, name, kind, metadata_json, archived_at, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ConversationID, msg.Role, msg.Content,
		toolCallsJSON, msg.ToolCallID, msg.Name, kind, metadataJSON, archivedAt, time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("save message: %w", err)
	}

	_, _ = s.db.ExecContext(ctx,
		`UPDATE conversations SET updated_at=? WHERE id=?`,
		time.Now().UTC(), msg.ConversationID,
	)

	id, _ := res.LastInsertId()
	return id, nil
}

func (s *SQLiteStore) LoadMessages(ctx context.Context, conversationID int64) ([]SessionMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, tool_calls, tool_call_id, name, kind, metadata_json, archived_at, timestamp
		 FROM messages WHERE conversation_id=? AND archived_at IS NULL ORDER BY timestamp ASC`,
		conversationID)
	if err != nil {
		return nil, fmt.Errorf("load messages for conv %d: %w", conversationID, err)
	}
	defer rows.Close()

	var msgs []SessionMessage
	for rows.Next() {
		var m SessionMessage
		var archivedAt sql.NullTime
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.ToolCalls, &m.ToolCallID, &m.Name, &m.Kind, &m.MetadataJSON, &archivedAt, &m.Timestamp); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		if archivedAt.Valid {
			m.ArchivedAt = &archivedAt.Time
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return msgs, nil
}

func ToolCallsToJSON(v interface{}) string {
	if v == nil {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
