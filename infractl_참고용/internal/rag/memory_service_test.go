// Package rag
// File: memory_service_test.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package rag

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/store"
)

type memoryStoreFilterStub struct {
	lastEmbeddingFilter store.MemoryQueryFilter
	lastFTSFilter       store.MemoryQueryFilter
}

type testEmbedder struct{}

func (testEmbedder) Generate(context.Context, string) ([]float32, error) {
	return []float32{1, 0}, nil
}

func (testEmbedder) ModelName() string { return "test" }

func (s *memoryStoreFilterStub) ReplaceMemoryDocuments(context.Context, string, int64, []store.MemoryDocument) ([]store.MemoryDocument, error) {
	return nil, nil
}

func (s *memoryStoreFilterStub) UpdateMemoryEmbedding(context.Context, int64, []byte) error {
	return nil
}

func (s *memoryStoreFilterStub) GetMemoryDocument(context.Context, int64) (store.MemoryDocument, error) {
	return store.MemoryDocument{}, nil
}

func (s *memoryStoreFilterStub) ListMemoryEmbeddings(_ context.Context, filter store.MemoryQueryFilter) ([]store.MemoryEmbeddingRow, error) {
	s.lastEmbeddingFilter = filter
	return nil, nil
}

func (s *memoryStoreFilterStub) SearchMemoryFTS(_ context.Context, _ string, _ int, filter store.MemoryQueryFilter) ([]store.MemoryFTSResult, error) {
	s.lastFTSFilter = filter
	return nil, nil
}

func (s *memoryStoreFilterStub) CountMemoryDocuments(context.Context) (store.MemoryStats, error) {
	return store.MemoryStats{}, nil
}

func TestMemoryServiceSearchWithOptionsPassesScopedFilters(t *testing.T) {
	memStore := &memoryStoreFilterStub{}
	service := NewMemoryService(memStore, nil, nil, nil, testEmbedder{}, nil)
	conversationID := int64(42)

	_, err := service.SearchWithOptions(context.Background(), "rollback failed", SearchOptions{
		TopK:           3,
		ServerName:     "db-prod",
		ConversationID: &conversationID,
		SourceTypes:    []string{"knowledge", "conversation"},
	})
	if err != nil {
		t.Fatalf("SearchWithOptions() error = %v", err)
	}

	if memStore.lastFTSFilter.ServerName != "db-prod" {
		t.Fatalf("expected FTS filter server_name db-prod, got %q", memStore.lastFTSFilter.ServerName)
	}
	if memStore.lastFTSFilter.ConversationID == nil || *memStore.lastFTSFilter.ConversationID != 42 {
		t.Fatalf("expected FTS filter conversation_id 42, got %#v", memStore.lastFTSFilter.ConversationID)
	}
	if len(memStore.lastFTSFilter.SourceTypes) != 2 {
		t.Fatalf("expected source type filters to be forwarded")
	}
	if memStore.lastEmbeddingFilter.ServerName != "db-prod" {
		t.Fatalf("expected vector filter server_name db-prod, got %q", memStore.lastEmbeddingFilter.ServerName)
	}
}

