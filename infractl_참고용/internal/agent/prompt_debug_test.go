package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
)

func TestPromptInputCacheCachesAndInvalidates(t *testing.T) {
	cache := newPromptInputCache()
	ctx := context.Background()

	serverLoads := 0
	servers := func(context.Context) ([]store.Server, error) {
		serverLoads++
		return []store.Server{{Name: "srv-1"}}, nil
	}

	if got := cache.GetServers(ctx, servers); len(got) != 1 || got[0].Name != "srv-1" {
		t.Fatalf("unexpected servers on first load: %+v", got)
	}
	if got := cache.GetServers(ctx, servers); len(got) != 1 || got[0].Name != "srv-1" {
		t.Fatalf("unexpected servers on cache hit: %+v", got)
	}
	if serverLoads != 1 {
		t.Fatalf("expected one server load before invalidation, got %d", serverLoads)
	}

	ragLoads := 0
	statsLoader := func(context.Context) (*rag.KnowledgeStats, error) {
		ragLoads++
		return &rag.KnowledgeStats{
			TotalDocs:       10,
			TotalWithVec:    8,
			CountBySource:   map[string]int{"knowledge_add": 3},
			CountByCategory: map[string]int{"task_success": 2},
		}, nil
	}

	firstStats := cache.GetKnowledgeStats(ctx, statsLoader)
	secondStats := cache.GetKnowledgeStats(ctx, statsLoader)
	if firstStats == nil || secondStats == nil || secondStats.TotalDocs != 10 {
		t.Fatalf("unexpected knowledge stats: first=%+v second=%+v", firstStats, secondStats)
	}
	if ragLoads != 1 {
		t.Fatalf("expected one knowledge stats load before invalidation, got %d", ragLoads)
	}

	cache.InvalidateServers()
	cache.InvalidateRAG()

	_ = cache.GetServers(ctx, servers)
	_ = cache.GetKnowledgeStats(ctx, statsLoader)
	if serverLoads != 2 {
		t.Fatalf("expected server reload after invalidation, got %d", serverLoads)
	}
	if ragLoads != 2 {
		t.Fatalf("expected knowledge stats reload after invalidation, got %d", ragLoads)
	}

	stats := cache.Stats()
	if stats.ServersHits == 0 || stats.ServersMisses == 0 {
		t.Fatalf("expected server cache hits and misses, got %+v", stats)
	}
	if stats.KnowledgeStatsHits == 0 || stats.KnowledgeStatsMisses == 0 {
		t.Fatalf("expected knowledge stats cache hits and misses, got %+v", stats)
	}
}

func TestPromptInputCacheReloadsInfractlMDWhenFileChanges(t *testing.T) {
	cache := newPromptInputCache()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := filepath.Join(wd, "..", "..", "scratch", fmt.Sprintf("prompt-md-test-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "INFRACTL.md")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	loads := 0
	loader := func() (string, []infractlMDFileState) {
		loads++
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat file: %v", err)
		}
		return fmt.Sprintf("load-%d", loads), []infractlMDFileState{{
			Path:        path,
			Size:        info.Size(),
			ModUnixNano: info.ModTime().UnixNano(),
		}}
	}

	first := cache.GetInfractlMD(loader)
	second := cache.GetInfractlMD(loader)
	if first != "load-1" || second != "load-1" || loads != 1 {
		t.Fatalf("expected initial cache hit behavior, first=%q second=%q loads=%d", first, second, loads)
	}

	time.Sleep(5 * time.Millisecond)
	if err := os.WriteFile(path, []byte("v2-updated"), 0o644); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}

	third := cache.GetInfractlMD(loader)
	if third != "load-2" || loads != 2 {
		t.Fatalf("expected reload after file change, third=%q loads=%d", third, loads)
	}
}

func TestPromptDebugReportIncludesPromptAndCacheStats(t *testing.T) {
	ag := &Agent{
		promptCache:  newPromptCache(),
		promptInputs: newPromptInputCache(),
		lastPromptProfile: promptProfile{
			Mode:                 "contextual",
			Model:                "qwen3",
			NeedsTools:           true,
			PromptCacheHit:       true,
			TotalBytes:           1234,
			PrefixBytes:          200,
			TaskMemoryBytes:      30,
			BeforeKnowledgeBytes: 400,
			KnowledgeBytes:       50,
			AfterKnowledgeBytes:  500,
			ToolCount:            15,
			SectionCount:         7,
		},
	}
	ag.promptCache.PutContextual("ctx", ContextualPromptLayout{prefix: "x"})
	_, _ = ag.promptCache.GetContextual("ctx")
	_ = ag.promptInputs.GetInfractlMD(func() (string, []infractlMDFileState) { return "md", nil })
	_ = ag.promptInputs.GetInfractlMD(func() (string, []infractlMDFileState) { return "ignored", nil })

	report := ag.PromptDebugReport()
	for _, want := range []string{
		"Last prompt:",
		"mode: contextual",
		"total_bytes: 1234",
		"Prompt cache:",
		"contextual: entries=1 hits=1",
		"Prompt input cache:",
		"INFRACTL.md: hits=1 misses=1",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("expected %q in report:\n%s", want, report)
		}
	}
}
