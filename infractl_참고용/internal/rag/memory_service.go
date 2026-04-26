// Package rag
// File: memory_service.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/store"
)

const (
	memoryFTSLimit    = 20
	memoryVectorLimit = 20
	messageChunkSize  = 800
	messageOverlap    = 120
)

type memoryCandidate struct {
	doc         store.MemoryDocument
	score       float64
	vectorScore float32
}

// BackgroundSubmitter is the subset of the background manager used by memory indexing.
type BackgroundSubmitter interface {
	Submit(ctx context.Context, description string, fn func(ctx context.Context) (string, error)) int
}

// MemoryService owns the unified internal memory index and hybrid search.
type MemoryService struct {
	memoryStore    store.MemoryStore
	knowledgeStore store.KnowledgeStore
	learnedStore   store.LearnedSystemStore
	sessionStore   store.SessionStore
	embedder       EmbeddingGenerator
	bgManager      BackgroundSubmitter

	pendingMu sync.Mutex
	pending   map[string]struct{}
}

type embedderStatusProvider interface {
	Status() string
}

// NewMemoryService creates the internal memory index/search service.
func NewMemoryService(
	mem store.MemoryStore,
	ks store.KnowledgeStore,
	ls store.LearnedSystemStore,
	ss store.SessionStore,
	embedder EmbeddingGenerator,
	bg BackgroundSubmitter,
) *MemoryService {
	return &MemoryService{
		memoryStore:    mem,
		knowledgeStore: ks,
		learnedStore:   ls,
		sessionStore:   ss,
		embedder:       embedder,
		bgManager:      bg,
		pending:        make(map[string]struct{}),
	}
}

// Stats returns current internal memory index counts.
func (m *MemoryService) Stats(ctx context.Context) (store.MemoryStats, error) {
	return m.memoryStore.CountMemoryDocuments(ctx)
}

// EmbedderStatus returns the current internal embedder runtime status.
func (m *MemoryService) EmbedderStatus() string {
	if provider, ok := m.embedder.(embedderStatusProvider); ok {
		return provider.Status()
	}
	if m.embedder == nil {
		return "embedder=unset state=disabled"
	}
	return fmt.Sprintf("embedder=%s state=ready", m.embedder.ModelName())
}

// SubmitBackfill schedules a full internal memory rebuild.
func (m *MemoryService) SubmitBackfill(ctx context.Context) int {
	if m.bgManager == nil {
		if _, err := m.BackfillAll(ctx); err != nil {
			slog.Warn("memory backfill failed", "err", err)
		}
		return 0
	}
	return m.bgManager.Submit(ctx, "rebuild internal memory index", func(jobCtx context.Context) (string, error) {
		return m.BackfillAll(jobCtx)
	})
}

// BackfillAll rebuilds the internal memory index from authoritative tables.
func (m *MemoryService) BackfillAll(ctx context.Context) (string, error) {
	knowledgeCount := 0
	learnedCount := 0
	messageCount := 0

	if m.knowledgeStore != nil {
		entries, err := m.knowledgeStore.ListKnowledge(ctx, "", 1_000_000)
		if err != nil {
			return "", fmt.Errorf("list knowledge for backfill: %w", err)
		}
		for _, entry := range entries {
			if err := m.indexKnowledgeEntry(ctx, entry, false); err != nil {
				slog.Warn("index knowledge entry", "id", entry.ID, "err", err)
				continue
			}
			knowledgeCount++
		}
	}

	if m.learnedStore != nil {
		systems, err := m.learnedStore.ListLearnedSystems(ctx)
		if err != nil {
			return "", fmt.Errorf("list learned systems for backfill: %w", err)
		}
		for _, sys := range systems {
			if err := m.indexLearnedSystemEntry(ctx, sys, false); err != nil {
				slog.Warn("index learned system", "id", sys.ID, "err", err)
				continue
			}
			learnedCount++
		}
	}

	if m.sessionStore != nil {
		convs, err := m.sessionStore.ListConversations(ctx, 1_000_000)
		if err != nil {
			return "", fmt.Errorf("list conversations for backfill: %w", err)
		}
		for _, conv := range convs {
			msgs, err := m.sessionStore.LoadMessages(ctx, conv.ID)
			if err != nil {
				slog.Warn("load conversation messages", "conversation_id", conv.ID, "err", err)
				continue
			}
			for _, msg := range msgs {
				if err := m.indexSessionMessageEntry(ctx, msg, false); err != nil {
					slog.Warn("index session message", "message_id", msg.ID, "err", err)
					continue
				}
				if shouldIndexSessionMessage(msg) {
					messageCount++
				}
			}
		}
	}

	stats, _ := m.Stats(ctx)
	return fmt.Sprintf(
		"knowledge=%d learned_systems=%d messages=%d docs=%d with_vectors=%d",
		knowledgeCount, learnedCount, messageCount, stats.TotalDocs, stats.TotalWithVector,
	), nil
}

// IndexKnowledge stores and asynchronously embeds a knowledge entry in the internal memory index.
func (m *MemoryService) IndexKnowledge(ctx context.Context, entry store.KnowledgeEntry) error {
	return m.indexKnowledgeEntry(ctx, entry, true)
}

// IndexLearnedSystem stores and asynchronously embeds a learned system in the internal memory index.
func (m *MemoryService) IndexLearnedSystem(ctx context.Context, sys store.LearnedSystem) error {
	return m.indexLearnedSystemEntry(ctx, sys, true)
}

// IndexSessionMessage stores and asynchronously embeds a conversation message in the internal memory index.
func (m *MemoryService) IndexSessionMessage(ctx context.Context, msg store.SessionMessage) error {
	return m.indexSessionMessageEntry(ctx, msg, true)
}

func (m *MemoryService) indexKnowledgeEntry(ctx context.Context, entry store.KnowledgeEntry, async bool) error {
	if entry.ID == 0 {
		return nil
	}
	metadata := encodeMetadata(map[string]string{
		"category":        entry.Category,
		"tool_name":       entry.ToolName,
		"error_pattern":   entry.ErrorPattern,
		"success_command": entry.SuccessCommand,
	})
	content := strings.TrimSpace(strings.Join([]string{
		"Category: " + entry.Category,
		"Situation: " + entry.Situation,
		"Resolution: " + entry.Resolution,
		"Error Pattern: " + entry.ErrorPattern,
		"Success Command: " + entry.SuccessCommand,
	}, "\n"))
	docs := []store.MemoryDocument{{
		SourceType:   "knowledge",
		SourceRowID:  entry.ID,
		ChunkNo:      0,
		Title:        defaultString(entry.Title, "Knowledge"),
		Content:      content,
		MetadataJSON: metadata,
		CreatedAt:    entry.CreatedAt,
		UpdatedAt:    entry.CreatedAt,
	}}
	return m.replaceDocuments(ctx, "knowledge", entry.ID, docs, async)
}

func (m *MemoryService) indexLearnedSystemEntry(ctx context.Context, sys store.LearnedSystem, async bool) error {
	if sys.ID == 0 {
		return nil
	}
	metadata := encodeMetadata(map[string]string{
		"service_type": sys.ServiceType,
		"server_name":  sys.ServerName,
		"cli_path":     sys.CLIPath,
		"config_path":  sys.ConfigPath,
		"log_path":     sys.LogPath,
		"commands":     sys.Commands,
	})
	content := strings.TrimSpace(strings.Join([]string{
		"Service: " + sys.ServiceType,
		"Server: " + sys.ServerName,
		"CLI Path: " + sys.CLIPath,
		"Config Path: " + sys.ConfigPath,
		"Log Path: " + sys.LogPath,
		"Commands: " + sys.Commands,
	}, "\n"))
	docs := []store.MemoryDocument{{
		SourceType:   "learned_system",
		SourceRowID:  sys.ID,
		ChunkNo:      0,
		ServerName:   sys.ServerName,
		Title:        fmt.Sprintf("%s @ %s", sys.ServiceType, sys.ServerName),
		Content:      content,
		MetadataJSON: metadata,
		CreatedAt:    sys.CreatedAt,
		UpdatedAt:    sys.CreatedAt,
	}}
	return m.replaceDocuments(ctx, "learned_system", sys.ID, docs, async)
}

func (m *MemoryService) indexSessionMessageEntry(ctx context.Context, msg store.SessionMessage, async bool) error {
	if msg.ID == 0 {
		return nil
	}
	if !shouldIndexSessionMessage(msg) {
		return m.replaceDocuments(ctx, "conversation", msg.ID, nil, false)
	}

	chunks := chunkText(msg.Content, messageChunkSize, messageOverlap)
	var convID *int64
	if msg.ConversationID > 0 {
		convID = &msg.ConversationID
	}
	docs := make([]store.MemoryDocument, 0, len(chunks))
	for i, chunk := range chunks {
		docs = append(docs, store.MemoryDocument{
			SourceType:     "conversation",
			SourceRowID:    msg.ID,
			ChunkNo:        i,
			ConversationID: convID,
			Role:           msg.Role,
			Title:          fmt.Sprintf("Conversation %d / %s", msg.ConversationID, msg.Role),
			Content:        chunk,
			MetadataJSON: encodeMetadata(map[string]string{
				"conversation_id": fmt.Sprintf("%d", msg.ConversationID),
				"role":            msg.Role,
			}),
			CreatedAt: msg.Timestamp,
			UpdatedAt: msg.Timestamp,
		})
	}
	return m.replaceDocuments(ctx, "conversation", msg.ID, docs, async)
}

func (m *MemoryService) replaceDocuments(ctx context.Context, sourceType string, sourceRowID int64, docs []store.MemoryDocument, async bool) error {
	inserted, err := m.memoryStore.ReplaceMemoryDocuments(ctx, sourceType, sourceRowID, docs)
	if err != nil {
		return err
	}
	if len(inserted) == 0 {
		return nil
	}
	if async {
		m.queueEmbeddings(ctx, sourceType, sourceRowID, inserted)
		return nil
	}
	return m.embedDocuments(ctx, inserted)
}

func (m *MemoryService) queueEmbeddings(ctx context.Context, sourceType string, sourceRowID int64, docs []store.MemoryDocument) {
	if m.bgManager == nil {
		if err := m.embedDocuments(ctx, docs); err != nil {
			slog.Warn("embed internal memory docs", "source_type", sourceType, "source_row_id", sourceRowID, "err", err)
		}
		return
	}

	key := fmt.Sprintf("%s:%d", sourceType, sourceRowID)
	m.pendingMu.Lock()
	if _, exists := m.pending[key]; exists {
		m.pendingMu.Unlock()
		return
	}
	m.pending[key] = struct{}{}
	m.pendingMu.Unlock()

	m.bgManager.Submit(ctx, fmt.Sprintf("embed %s #%d", sourceType, sourceRowID), func(jobCtx context.Context) (string, error) {
		defer func() {
			m.pendingMu.Lock()
			delete(m.pending, key)
			m.pendingMu.Unlock()
		}()
		if err := m.embedDocuments(jobCtx, docs); err != nil {
			return "", err
		}
		return fmt.Sprintf("embedded %d docs", len(docs)), nil
	})
}

func (m *MemoryService) embedDocuments(ctx context.Context, docs []store.MemoryDocument) error {
	if m.embedder == nil {
		return nil
	}
	for _, doc := range docs {
		vec, err := m.embedder.Generate(ctx, memoryPassageText(doc))
		if err != nil {
			return fmt.Errorf("generate local embedding for doc %d: %w", doc.ID, err)
		}
		if err := m.memoryStore.UpdateMemoryEmbedding(ctx, doc.ID, SerializeVector(vec)); err != nil {
			return err
		}
	}
	return nil
}

func (m *MemoryService) Search(ctx context.Context, query string, topK int, minScore float32) ([]LocalResult, error) {
	return m.SearchWithOptions(ctx, query, SearchOptions{
		TopK:     topK,
		MinScore: minScore,
	})
}

// SearchWithOptions runs a scoped hybrid search over internal memory.
func (m *MemoryService) SearchWithOptions(ctx context.Context, query string, opts SearchOptions) ([]LocalResult, error) {
	topK := opts.TopK
	if topK <= 0 {
		topK = 5
	}
	if opts.MinScore == 0 {
		opts.MinScore = 0.7
	}

	filter := store.MemoryQueryFilter{
		SourceTypes:    opts.SourceTypes,
		ServerName:     opts.ServerName,
		ConversationID: opts.ConversationID,
	}

	ftsHits, ftsErr := m.searchFTS(ctx, query, maxInt(memoryFTSLimit, topK*4), filter)
	vectorHits, vectorErr := m.searchVector(ctx, query, maxInt(memoryVectorLimit, topK*4), opts.MinScore, filter)

	if ftsErr != nil && vectorErr != nil {
		return nil, fmt.Errorf("fts: %v; vector: %v", ftsErr, vectorErr)
	}
	if ftsErr != nil {
		slog.Warn("memory fts search failed", "err", ftsErr)
	}
	if vectorErr != nil {
		slog.Warn("memory vector search failed, using fts-only fallback", "err", errString(vectorErr))
	}

	candidates := make(map[int64]*memoryCandidate)
	for i, hit := range ftsHits {
		entry := ensureMemoryCandidate(candidates, hit.Document)
		entry.score += reciprocalRank(i)
		entry.score += sourceBoost(hit.Document.SourceType)
	}
	for i, hit := range vectorHits {
		entry := ensureMemoryCandidate(candidates, hit.Document)
		entry.score += reciprocalRank(i)
		entry.score += sourceBoost(hit.Document.SourceType)
		entry.vectorScore = maxFloat32(entry.vectorScore, hit.VectorScore)
	}

	out := make([]LocalResult, 0, len(candidates))
	for _, cand := range candidates {
		out = append(out, LocalResult{
			Document:    cand.doc,
			Score:       cand.score,
			VectorScore: cand.vectorScore,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].VectorScore > out[j].VectorScore
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func (m *MemoryService) searchFTS(ctx context.Context, query string, limit int, filter store.MemoryQueryFilter) ([]store.MemoryFTSResult, error) {
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}
	return m.memoryStore.SearchMemoryFTS(ctx, ftsQuery, limit, filter)
}

func (m *MemoryService) searchVector(ctx context.Context, query string, limit int, minScore float32, filter store.MemoryQueryFilter) ([]LocalResult, error) {
	if m.embedder == nil {
		return nil, nil
	}
	queryVec, err := m.embedder.Generate(ctx, memoryQueryText(query))
	if err != nil {
		return nil, fmt.Errorf("generate local query embedding: %w", err)
	}

	rows, err := m.memoryStore.ListMemoryEmbeddings(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list local memory embeddings: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	var candidates []ScoredID
	for _, row := range rows {
		entryVec, err := DeserializeVector(row.Embedding)
		if err != nil {
			slog.Debug("deserialize local vector", "id", row.ID, "err", err)
			continue
		}
		sim := CosineSimilarity(queryVec, entryVec)
		candidates = append(candidates, ScoredID{ID: row.ID, Score: sim})
	}

	top := TopK(candidates, limit, minScore)
	results := make([]LocalResult, 0, len(top))
	for _, scored := range top {
		doc, err := m.memoryStore.GetMemoryDocument(ctx, scored.ID)
		if err != nil {
			slog.Debug("get local memory doc", "id", scored.ID, "err", err)
			continue
		}
		results = append(results, LocalResult{
			Document:    doc,
			Score:       float64(scored.Score),
			VectorScore: scored.Score,
		})
	}
	return results, nil
}

func memoryQueryText(query string) string {
	return "query: " + strings.TrimSpace(query)
}

func memoryPassageText(doc store.MemoryDocument) string {
	body := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(doc.Title),
		strings.TrimSpace(doc.Content),
	}, "\n\n"))
	return "passage: " + body
}

func shouldIndexSessionMessage(msg store.SessionMessage) bool {
	if strings.TrimSpace(msg.Content) == "" {
		return false
	}
	role := llm.Role(msg.Role)
	return role == llm.RoleUser || role == llm.RoleAssistant
}

func chunkText(text string, size int, overlap int) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	if len(runes) <= size {
		return []string{string(runes)}
	}

	var chunks []string
	step := size - overlap
	if step <= 0 {
		step = size
	}
	for start := 0; start < len(runes); start += step {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
	}
	return chunks
}

func encodeMetadata(values map[string]string) string {
	data, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func buildFTSQuery(query string) string {
	var terms []string
	var token []rune
	flush := func() {
		if len(token) == 0 {
			return
		}
		terms = append(terms, string(token))
		token = token[:0]
	}
	for _, r := range strings.ToLower(query) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '/' || r == '.' {
			token = append(token, r)
			continue
		}
		flush()
	}
	flush()

	if len(terms) == 0 {
		return ""
	}

	unique := make(map[string]struct{}, len(terms))
	var parts []string
	for _, term := range terms {
		if len([]rune(term)) < 2 {
			continue
		}
		if _, exists := unique[term]; exists {
			continue
		}
		unique[term] = struct{}{}
		parts = append(parts, fmt.Sprintf(`"%s"*`, term))
	}
	return strings.Join(parts, " OR ")
}

func ensureMemoryCandidate(items map[int64]*memoryCandidate, doc store.MemoryDocument) *memoryCandidate {
	if existing, ok := items[doc.ID]; ok {
		return existing
	}
	entry := &memoryCandidate{doc: doc}
	items[doc.ID] = entry
	return entry
}

func reciprocalRank(rank int) float64 {
	return 1.0 / float64(rank+61)
}

func sourceBoost(sourceType string) float64 {
	switch sourceType {
	case "knowledge":
		return 0.30
	case "learned_system":
		return 0.20
	default:
		return 0
	}
}

func defaultString(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

