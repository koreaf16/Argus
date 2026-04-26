package agent

import (
	"context"
	"os"
	"sync"

	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
)

type promptInputCache struct {
	mu sync.RWMutex

	infractlMD       string
	infractlMDFiles  []infractlMDFileState
	infractlMDLoaded bool
	infractlMDHits   int
	infractlMDMisses int

	servers       []store.Server
	serversLoaded bool
	serversHits   int
	serversMisses int

	learnedSystems       []store.LearnedSystem
	learnedSystemsLoaded bool
	learnedSystemsHits   int
	learnedSystemsMisses int

	ragSources       []store.RAGSource
	ragSourcesLoaded bool
	ragSourcesHits   int
	ragSourcesMisses int

	knowledgeStats       *rag.KnowledgeStats
	knowledgeStatsLoaded bool
	knowledgeStatsHits   int
	knowledgeStatsMisses int
}

type promptInputCacheStats struct {
	InfractlMDHits       int
	InfractlMDMisses     int
	ServersHits          int
	ServersMisses        int
	LearnedSystemsHits   int
	LearnedSystemsMisses int
	RAGSourcesHits       int
	RAGSourcesMisses     int
	KnowledgeStatsHits   int
	KnowledgeStatsMisses int
}

func newPromptInputCache() *promptInputCache {
	return &promptInputCache{}
}

func (c *promptInputCache) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.infractlMD = ""
	c.infractlMDFiles = nil
	c.infractlMDLoaded = false
	c.servers = nil
	c.serversLoaded = false
	c.learnedSystems = nil
	c.learnedSystemsLoaded = false
	c.ragSources = nil
	c.ragSourcesLoaded = false
	c.knowledgeStats = nil
	c.knowledgeStatsLoaded = false
}

func (c *promptInputCache) InvalidateServers() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.servers = nil
	c.serversLoaded = false
}

func (c *promptInputCache) InvalidateLearnedSystems() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.learnedSystems = nil
	c.learnedSystemsLoaded = false
}

func (c *promptInputCache) InvalidateRAG() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ragSources = nil
	c.ragSourcesLoaded = false
	c.knowledgeStats = nil
	c.knowledgeStatsLoaded = false
}

func (c *promptInputCache) GetInfractlMD(load func() (string, []infractlMDFileState)) string {
	if c == nil {
		value, _ := load()
		return value
	}
	c.mu.RLock()
	if c.infractlMDLoaded {
		value := c.infractlMD
		files := cloneInfractlMDFileStates(c.infractlMDFiles)
		c.mu.RUnlock()
		if !infractlMDStatesChanged(files) {
			c.mu.Lock()
			c.infractlMDHits++
			c.mu.Unlock()
			return value
		}
		c.mu.Lock()
		c.infractlMD = ""
		c.infractlMDFiles = nil
		c.infractlMDLoaded = false
		c.mu.Unlock()
	} else {
		c.mu.RUnlock()
	}

	value, files := load()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.infractlMDLoaded {
		c.infractlMDHits++
		return c.infractlMD
	}
	c.infractlMD = value
	c.infractlMDFiles = cloneInfractlMDFileStates(files)
	c.infractlMDLoaded = true
	c.infractlMDMisses++
	return c.infractlMD
}

func (c *promptInputCache) GetServers(ctx context.Context, load func(context.Context) ([]store.Server, error)) []store.Server {
	if c == nil {
		servers, _ := load(ctx)
		return cloneServers(servers)
	}
	c.mu.RLock()
	if c.serversLoaded {
		servers := cloneServers(c.servers)
		c.mu.RUnlock()
		c.mu.Lock()
		c.serversHits++
		c.mu.Unlock()
		return servers
	}
	c.mu.RUnlock()

	servers, err := load(ctx)
	if err != nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.serversLoaded {
		c.serversHits++
		return cloneServers(c.servers)
	}
	c.servers = cloneServers(servers)
	c.serversLoaded = true
	c.serversMisses++
	return cloneServers(c.servers)
}

func (c *promptInputCache) GetLearnedSystems(ctx context.Context, load func(context.Context) []store.LearnedSystem) []store.LearnedSystem {
	if c == nil {
		return cloneLearnedSystems(load(ctx))
	}
	c.mu.RLock()
	if c.learnedSystemsLoaded {
		systems := cloneLearnedSystems(c.learnedSystems)
		c.mu.RUnlock()
		c.mu.Lock()
		c.learnedSystemsHits++
		c.mu.Unlock()
		return systems
	}
	c.mu.RUnlock()

	systems := load(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.learnedSystemsLoaded {
		c.learnedSystemsHits++
		return cloneLearnedSystems(c.learnedSystems)
	}
	c.learnedSystems = cloneLearnedSystems(systems)
	c.learnedSystemsLoaded = true
	c.learnedSystemsMisses++
	return cloneLearnedSystems(c.learnedSystems)
}

func (c *promptInputCache) GetRAGSources(ctx context.Context, load func(context.Context) ([]store.RAGSource, error)) []store.RAGSource {
	if c == nil {
		sources, _ := load(ctx)
		return cloneRAGSources(sources)
	}
	c.mu.RLock()
	if c.ragSourcesLoaded {
		sources := cloneRAGSources(c.ragSources)
		c.mu.RUnlock()
		c.mu.Lock()
		c.ragSourcesHits++
		c.mu.Unlock()
		return sources
	}
	c.mu.RUnlock()

	sources, err := load(ctx)
	if err != nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ragSourcesLoaded {
		c.ragSourcesHits++
		return cloneRAGSources(c.ragSources)
	}
	c.ragSources = cloneRAGSources(sources)
	c.ragSourcesLoaded = true
	c.ragSourcesMisses++
	return cloneRAGSources(c.ragSources)
}

func (c *promptInputCache) GetKnowledgeStats(ctx context.Context, load func(context.Context) (*rag.KnowledgeStats, error)) *rag.KnowledgeStats {
	if c == nil {
		stats, _ := load(ctx)
		return cloneKnowledgeStats(stats)
	}
	c.mu.RLock()
	if c.knowledgeStatsLoaded {
		stats := cloneKnowledgeStats(c.knowledgeStats)
		c.mu.RUnlock()
		c.mu.Lock()
		c.knowledgeStatsHits++
		c.mu.Unlock()
		return stats
	}
	c.mu.RUnlock()

	stats, err := load(ctx)
	if err != nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.knowledgeStatsLoaded {
		c.knowledgeStatsHits++
		return cloneKnowledgeStats(c.knowledgeStats)
	}
	c.knowledgeStats = cloneKnowledgeStats(stats)
	c.knowledgeStatsLoaded = true
	c.knowledgeStatsMisses++
	return cloneKnowledgeStats(c.knowledgeStats)
}

func (c *promptInputCache) Stats() promptInputCacheStats {
	if c == nil {
		return promptInputCacheStats{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return promptInputCacheStats{
		InfractlMDHits:       c.infractlMDHits,
		InfractlMDMisses:     c.infractlMDMisses,
		ServersHits:          c.serversHits,
		ServersMisses:        c.serversMisses,
		LearnedSystemsHits:   c.learnedSystemsHits,
		LearnedSystemsMisses: c.learnedSystemsMisses,
		RAGSourcesHits:       c.ragSourcesHits,
		RAGSourcesMisses:     c.ragSourcesMisses,
		KnowledgeStatsHits:   c.knowledgeStatsHits,
		KnowledgeStatsMisses: c.knowledgeStatsMisses,
	}
}

func cloneServers(in []store.Server) []store.Server {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.Server, len(in))
	copy(out, in)
	return out
}

func cloneLearnedSystems(in []store.LearnedSystem) []store.LearnedSystem {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.LearnedSystem, len(in))
	copy(out, in)
	return out
}

func cloneRAGSources(in []store.RAGSource) []store.RAGSource {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.RAGSource, len(in))
	copy(out, in)
	return out
}

func cloneKnowledgeStats(in *rag.KnowledgeStats) *rag.KnowledgeStats {
	if in == nil {
		return nil
	}
	out := &rag.KnowledgeStats{
		TotalDocs:       in.TotalDocs,
		TotalWithVec:    in.TotalWithVec,
		CountBySource:   make(map[string]int, len(in.CountBySource)),
		CountByCategory: make(map[string]int, len(in.CountByCategory)),
	}
	for k, v := range in.CountBySource {
		out.CountBySource[k] = v
	}
	for k, v := range in.CountByCategory {
		out.CountByCategory[k] = v
	}
	return out
}

func cloneInfractlMDFileStates(in []infractlMDFileState) []infractlMDFileState {
	if len(in) == 0 {
		return nil
	}
	out := make([]infractlMDFileState, len(in))
	copy(out, in)
	return out
}

func infractlMDStatesChanged(files []infractlMDFileState) bool {
	if len(files) == 0 {
		return false
	}
	for _, file := range files {
		info, err := os.Stat(file.Path)
		if err != nil {
			return true
		}
		if info.Size() != file.Size || info.ModTime().UnixNano() != file.ModUnixNano {
			return true
		}
	}
	return false
}
