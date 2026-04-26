// Package store
// File: history_store.go
// Description: 프롬프트 히스토리 저장소 인터페이스 및 SQLite 구현
// Responsibility: prompt_history 테이블 CRUD 및 민감 정보 마스킹 처리

package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const createPromptHistorySQL = `
CREATE TABLE IF NOT EXISTS prompt_history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    input      TEXT NOT NULL,
    pasted_contents_json TEXT NOT NULL DEFAULT '',
    session_id INTEGER DEFAULT 0,
    timestamp  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const createPromptHistoryIdxSQL = `
CREATE INDEX IF NOT EXISTS idx_prompt_history_ts ON prompt_history(timestamp DESC);`

// PromptHistoryEntry는 prompt_history 테이블의 단일 행이다.
type PromptHistoryEntry struct {
	ID             int64
	Input          string
	PastedContents map[int]PastedContent
	SessionID      int64
	Timestamp      time.Time
}

// HistoryStore는 프롬프트 히스토리 영속화를 담당하는 인터페이스이다.
type HistoryStore interface {
	// SavePromptHistory는 사용자 입력을 마스킹 후 저장한다.
	SavePromptHistory(ctx context.Context, input string, sessionID int64) error

	// SavePromptDraft는 표시 입력과 붙여넣기 원문을 함께 저장한다.
	SavePromptDraft(ctx context.Context, input string, pastedContents map[int]PastedContent, sessionID int64) error

	// ListPromptHistory는 최근 입력 목록을 반환한다.
	ListPromptHistory(ctx context.Context, limit int) ([]PromptHistoryEntry, error)

	// SearchPromptHistory는 키워드로 히스토리를 검색한다.
	SearchPromptHistory(ctx context.Context, query string, limit int) ([]PromptHistoryEntry, error)
}

// initHistorySchema는 prompt_history 테이블과 인덱스를 생성한다.
func (s *SQLiteStore) initHistorySchema(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, createPromptHistorySQL); err != nil {
		slog.Warn("create prompt_history table", "err", err)
	}
	s.db.ExecContext(ctx, "ALTER TABLE prompt_history ADD COLUMN pasted_contents_json TEXT NOT NULL DEFAULT ''")
	if _, err := s.db.ExecContext(ctx, createPromptHistoryIdxSQL); err != nil {
		slog.Warn("create prompt_history index", "err", err)
	}
}

// SavePromptHistory는 사용자 입력을 민감 정보 마스킹 후 저장한다.
func (s *SQLiteStore) SavePromptHistory(ctx context.Context, input string, sessionID int64) error {
	return s.SavePromptDraft(ctx, input, nil, sessionID)
}

// SavePromptDraft는 사용자 입력과 붙여넣기 원문을 민감 정보 마스킹 후 저장한다.
func (s *SQLiteStore) SavePromptDraft(
	ctx context.Context,
	input string,
	pastedContents map[int]PastedContent,
	sessionID int64,
) error {
	masked := MaskSensitive(input)
	maskedPasted := maskPastedContents(pastedContents)
	pastedJSON, err := marshalStoredPastedContents(maskedPasted)
	if err != nil {
		return fmt.Errorf("marshal prompt history pasted contents: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO prompt_history (input, pasted_contents_json, session_id, timestamp)
		 VALUES (?, ?, ?, ?)`,
		masked, pastedJSON, sessionID, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("save prompt history: %w", err)
	}
	return nil
}

// ListPromptHistory는 최근 입력 목록을 시간 역순으로 반환한다.
func (s *SQLiteStore) ListPromptHistory(ctx context.Context, limit int) ([]PromptHistoryEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, input, pasted_contents_json, session_id, timestamp
		 FROM prompt_history ORDER BY timestamp DESC LIMIT ?`,
		limit)
	if err != nil {
		return nil, fmt.Errorf("list prompt history: %w", err)
	}
	defer rows.Close()
	return s.scanHistoryRows(rows)
}

// SearchPromptHistory는 LIKE 검색으로 히스토리를 조회한다.
func (s *SQLiteStore) SearchPromptHistory(ctx context.Context, query string, limit int) ([]PromptHistoryEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, input, pasted_contents_json, session_id, timestamp FROM prompt_history
		 WHERE input LIKE ? ORDER BY timestamp DESC LIMIT ?`,
		"%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("search prompt history: %w", err)
	}
	defer rows.Close()
	return s.scanHistoryRows(rows)
}

func (s *SQLiteStore) scanHistoryRows(rows interface {
	Scan(...interface{}) error
	Next() bool
	Err() error
}) ([]PromptHistoryEntry, error) {
	var entries []PromptHistoryEntry
	for rows.Next() {
		var e PromptHistoryEntry
		var pastedJSON string
		if err := rows.Scan(&e.ID, &e.Input, &pastedJSON, &e.SessionID, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("scan prompt history: %w", err)
		}
		pastedContents, err := unmarshalStoredPastedContents(pastedJSON)
		if err != nil {
			return nil, fmt.Errorf("restore prompt history pasted contents: %w", err)
		}
		e.PastedContents = pastedContents
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompt history: %w", err)
	}
	return entries, nil
}
