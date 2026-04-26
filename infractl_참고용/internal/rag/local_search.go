// Package rag
// File: local_search.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package rag

import (
	"context"

	"github.com/yourorg/infractl/internal/store"
)

// LocalResult is a unified internal memory search hit.
type LocalResult struct {
	Document    store.MemoryDocument
	Score       float64
	VectorScore float32
}

// SearchOptions scopes internal retrieval for prompt injection and hybrid search.
type SearchOptions struct {
	TopK           int
	MinScore       float32
	LocalOnly      bool
	SourceTypes    []string
	ServerName     string
	ConversationID *int64
}

// LocalSearcher searches internal CPU-only memory.
type LocalSearcher interface {
	Search(ctx context.Context, query string, topK int, minScore float32) ([]LocalResult, error)
}

