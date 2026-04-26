// Package rag
// File: manager.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package rag

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/yourorg/infractl/internal/store"
)

// SearchResult is a merged internal/external RAG hit.
type SearchResult struct {
	Source          string
	Priority        int
	Title           string
	Content         string
	Score           float32
	LocalSourceType string
	ServerName      string
}

// KnowledgeStats summarizes internal memory state for prompting.
type KnowledgeStats struct {
	CountByCategory map[string]int
	CountBySource   map[string]int
	TotalDocs       int
	TotalWithVec    int
}

// Manager combines internal memory retrieval with external RAG sources.
type Manager struct {
	localSearcher    LocalSearcher
	externalSearcher ExternalSearcher
	ragSourceStore   store.RAGSourceStore
	knowledgeStore   store.KnowledgeStore
	memoryStore      store.MemoryStore
	externalEmbedder EmbeddingGenerator
	reranker         Reranker
}

// NewManager creates a new RAG manager.
func NewManager(
	local LocalSearcher,
	external ExternalSearcher,
	ragStore store.RAGSourceStore,
	ks store.KnowledgeStore,
	mem store.MemoryStore,
	externalEmbedder EmbeddingGenerator,
	reranker Reranker,
) *Manager {
	return &Manager{
		localSearcher:    local,
		externalSearcher: external,
		ragSourceStore:   ragStore,
		knowledgeStore:   ks,
		memoryStore:      mem,
		externalEmbedder: externalEmbedder,
		reranker:         reranker,
	}
}

// Search returns internal results first, followed by external RAG results.
func (m *Manager) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	return m.SearchWithOptions(ctx, query, SearchOptions{TopK: topK, MinScore: 0.05})
}

// SearchWithOptions returns scoped results (local + external) reranked by a cross-encoder if available.
func (m *Manager) SearchWithOptions(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	topK := opts.TopK
	if topK <= 0 {
		topK = 5
	}
	if opts.MinScore == 0 {
		opts.MinScore = 0.7
	}

	// 1. 후보군 수집 (리랭커가 있다면 더 넉넉하게 수집)
	candidateK := topK
	if m.reranker != nil {
		candidateK = 30 // 리랭커를 위한 충분한 후보 확보
	}

	var candidates []SearchResult

	// 로컬 검색 수행
	localOpts := opts
	localOpts.TopK = candidateK
	localResults, err := m.searchLocal(ctx, query, localOpts)
	if err != nil {
		slog.Warn("local memory search failed", "err", err)
	}
	for _, lr := range localResults {
		candidates = append(candidates, SearchResult{
			Source:          "local",
			Priority:        0,
			Title:           lr.Document.Title,
			Content:         lr.Document.Content,
			Score:           float32(lr.Score),
			LocalSourceType: lr.Document.SourceType,
			ServerName:      lr.Document.ServerName,
		})
	}

	// 외부 검색 수행 (LocalOnly가 아닌 경우)
	if !opts.LocalOnly {
		sources, err := m.ragSourceStore.ListRAGSources(ctx)
		if err == nil && len(sources) > 0 && m.externalEmbedder != nil {
			queryVec, err := m.externalEmbedder.Generate(ctx, query)
			if err == nil {
				for _, src := range sources {
					extResults, err := m.externalSearcher.Search(ctx, src, queryVec, candidateK)
					if err != nil {
						slog.Warn("external rag search failed", "source", src.Name, "err", err)
						continue
					}
					for _, er := range extResults {
						candidates = append(candidates, SearchResult{
							Source:     src.Name,
							Priority:   src.Priority,
							Title:      fmt.Sprintf("[%s] %s", src.Name, src.Description),
							Content:    formatExternalResult(er, src.ResultColumns),
							Score:      0, // 외부 검색은 베이스 점수 생략 (리랭커가 채점함)
							ServerName: src.ServerName,
						})
					}
				}
			} else {
				slog.Warn("generate query embedding for external search", "err", err)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// 2. 리랭킹 적용 (통합 후보군 대상)
	if m.reranker != nil && len(candidates) > 1 {
		slog.Debug("applying unified reranking", "candidates", len(candidates), "topK", topK)
		reranked := m.rerankResults(ctx, query, candidates, topK)
		if len(reranked) > 0 {
			return reranked, nil
		}
		// 리랭커 실패 시 폴백: 기존 순서(Local -> External) 유지하되 점수 기반 정렬만 수행
	}

	// 3. 리랭킹 미적용 또는 실패 시: 점수 및 우선순위 기반 정렬 후 top-K 반환
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return candidates, nil
}

func (m *Manager) searchLocal(ctx context.Context, query string, opts SearchOptions) ([]LocalResult, error) {
	if svc, ok := m.localSearcher.(*MemoryService); ok {
		return svc.SearchWithOptions(ctx, query, opts)
	}
	return m.localSearcher.Search(ctx, query, opts.TopK, opts.MinScore)
}

func (m *Manager) rerankResults(ctx context.Context, query string, results []SearchResult, topK int) []SearchResult {
	documents := make([]string, len(results))
	for i, r := range results {
		documents[i] = r.Title + "\n" + r.Content
	}

	ranked, err := m.reranker.Rerank(ctx, query, documents, topK)
	if err != nil {
		slog.Warn("reranking failed, using original order", "err", err)
		return results
	}

	reranked := make([]SearchResult, 0, len(ranked))
	for _, rr := range ranked {
		if rr.Index < len(results) {
			entry := results[rr.Index]
			entry.Score = float32(rr.Score)
			reranked = append(reranked, entry)
		}
	}
	return reranked
}

func (m *Manager) internalMemory() *MemoryService {
	service, _ := m.localSearcher.(*MemoryService)
	return service
}

// ListSources lists registered external RAG sources.
func (m *Manager) ListSources(ctx context.Context) ([]store.RAGSource, error) {
	return m.ragSourceStore.ListRAGSources(ctx)
}

// DeleteSource deletes a registered external RAG source.
func (m *Manager) DeleteSource(ctx context.Context, id int64) error {
	return m.ragSourceStore.DeleteRAGSource(ctx, id)
}

// UpdatePriority changes external RAG source priority.
func (m *Manager) UpdatePriority(ctx context.Context, id int64, priority int) error {
	return m.ragSourceStore.UpdateRAGSourcePriority(ctx, id, priority)
}

// Stats returns knowledge and internal memory counts.
func (m *Manager) Stats(ctx context.Context) (*KnowledgeStats, error) {
	counts, err := m.knowledgeStore.CountKnowledge(ctx)
	if err != nil {
		return nil, fmt.Errorf("count knowledge: %w", err)
	}
	stats, err := m.memoryStore.CountMemoryDocuments(ctx)
	if err != nil {
		return nil, fmt.Errorf("count memory docs: %w", err)
	}
	return &KnowledgeStats{
		CountByCategory: counts,
		CountBySource:   stats.CountBySource,
		TotalDocs:       stats.TotalDocs,
		TotalWithVec:    stats.TotalWithVector,
	}, nil
}

// IndexKnowledge forwards a knowledge entry into the internal memory index.
func (m *Manager) IndexKnowledge(ctx context.Context, entry store.KnowledgeEntry) error {
	if svc := m.internalMemory(); svc != nil {
		return svc.IndexKnowledge(ctx, entry)
	}
	return nil
}

// IndexLearnedSystem forwards a learned system entry into the internal memory index.
func (m *Manager) IndexLearnedSystem(ctx context.Context, sys store.LearnedSystem) error {
	if svc := m.internalMemory(); svc != nil {
		return svc.IndexLearnedSystem(ctx, sys)
	}
	return nil
}

// IndexSessionMessage forwards a session message into the internal memory index.
func (m *Manager) IndexSessionMessage(ctx context.Context, msg store.SessionMessage) error {
	if svc := m.internalMemory(); svc != nil {
		return svc.IndexSessionMessage(ctx, msg)
	}
	return nil
}

// SubmitMemoryBackfill schedules a full internal memory rebuild.
func (m *Manager) SubmitMemoryBackfill(ctx context.Context) int {
	if svc := m.internalMemory(); svc != nil {
		return svc.SubmitBackfill(ctx)
	}
	return 0
}

// EmbedderName returns the external embedder model name.
func (m *Manager) EmbedderName() string {
	if m.externalEmbedder == nil {
		return ""
	}
	return m.externalEmbedder.ModelName()
}

func formatExternalResult(r ExternalResult, resultCols string) string {
	var parts []string
	for _, col := range splitCols(resultCols) {
		if v, ok := r.Columns[col]; ok && v != "" {
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return parts[0] + func() string {
		if len(parts) > 1 {
			s := ""
			for _, p := range parts[1:] {
				s += " | " + p
			}
			return s
		}
		return ""
	}()
}

