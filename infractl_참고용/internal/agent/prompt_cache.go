package agent

import (
	"hash"
	"hash/fnv"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

const promptCacheLimit = 8

type promptCache struct {
	contextual       map[string]ContextualPromptLayout
	contextualOrder  []string
	minimal          map[string]string
	minimalOrder     []string
	contextualHits   int
	contextualMisses int
	minimalHits      int
	minimalMisses    int
}

type promptCacheStats struct {
	ContextualEntries int
	ContextualHits    int
	ContextualMisses  int
	MinimalEntries    int
	MinimalHits       int
	MinimalMisses     int
}

func newPromptCache() *promptCache {
	return &promptCache{
		contextual: make(map[string]ContextualPromptLayout),
		minimal:    make(map[string]string),
	}
}

func (c *promptCache) Clear() {
	if c == nil {
		return
	}
	c.contextual = make(map[string]ContextualPromptLayout)
	c.contextualOrder = c.contextualOrder[:0]
	c.minimal = make(map[string]string)
	c.minimalOrder = c.minimalOrder[:0]
}

func (c *promptCache) GetContextual(key string) (ContextualPromptLayout, bool) {
	if c == nil {
		return ContextualPromptLayout{}, false
	}
	layout, ok := c.contextual[key]
	if ok {
		c.contextualHits++
	} else {
		c.contextualMisses++
	}
	return layout, ok
}

func (c *promptCache) PutContextual(key string, layout ContextualPromptLayout) {
	if c == nil || key == "" {
		return
	}
	if _, exists := c.contextual[key]; exists {
		c.contextual[key] = layout
		return
	}
	c.contextual[key] = layout
	c.contextualOrder = append(c.contextualOrder, key)
	for len(c.contextualOrder) > promptCacheLimit {
		evict := c.contextualOrder[0]
		c.contextualOrder = c.contextualOrder[1:]
		delete(c.contextual, evict)
	}
}

func (c *promptCache) GetMinimal(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	value, ok := c.minimal[key]
	if ok {
		c.minimalHits++
	} else {
		c.minimalMisses++
	}
	return value, ok
}

func (c *promptCache) PutMinimal(key, value string) {
	if c == nil || key == "" {
		return
	}
	if _, exists := c.minimal[key]; exists {
		c.minimal[key] = value
		return
	}
	c.minimal[key] = value
	c.minimalOrder = append(c.minimalOrder, key)
	for len(c.minimalOrder) > promptCacheLimit {
		evict := c.minimalOrder[0]
		c.minimalOrder = c.minimalOrder[1:]
		delete(c.minimal, evict)
	}
}

func (c *promptCache) Stats() promptCacheStats {
	if c == nil {
		return promptCacheStats{}
	}
	return promptCacheStats{
		ContextualEntries: len(c.contextual),
		ContextualHits:    c.contextualHits,
		ContextualMisses:  c.contextualMisses,
		MinimalEntries:    len(c.minimal),
		MinimalHits:       c.minimalHits,
		MinimalMisses:     c.minimalMisses,
	}
}

func contextualPromptCacheKey(
	sections SectionSet,
	toolList []tools.Tool,
	infractlMD string,
	servers []store.Server,
	activeServer *store.Server,
	connectorStates []connector.ConnectorState,
	learnedSystems []store.LearnedSystem,
	ragSources []store.RAGSource,
	knowledgeStats *rag.KnowledgeStats,
	modelName string,
	now time.Time,
) string {
	h := fnv.New64a()
	writePromptHash(h, "date", now.Format("2006-01-02|-07:00"))
	writePromptHash(h, "model", strings.ToLower(strings.TrimSpace(modelName)))
	writePromptHash(h, "sections", fingerprintSections(sections))
	writePromptHash(h, "tools", fingerprintTools(sections, modelName, toolList))
	writePromptHash(h, "infractl_md", infractlMD)
	writePromptHash(h, "servers", fingerprintServers(servers))
	writePromptHash(h, "active_server", fingerprintActiveServer(activeServer))
	writePromptHash(h, "connectors", fingerprintConnectorStates(connectorStates))
	writePromptHash(h, "learned_systems", fingerprintLearnedSystems(learnedSystems))
	writePromptHash(h, "rag_sources", fingerprintRAGSources(ragSources))
	writePromptHash(h, "knowledge_stats", fingerprintKnowledgeStats(knowledgeStats))
	writePromptHash(h, "local_env", fingerprintLocalEnvironment())
	return strconv.FormatUint(h.Sum64(), 16)
}

func minimalPromptCacheKey(infractlMD string, now time.Time) string {
	h := fnv.New64a()
	writePromptHash(h, "date", now.Format("2006-01-02|-07:00"))
	writePromptHash(h, "infractl_md", infractlMD)
	return strconv.FormatUint(h.Sum64(), 16)
}

func writePromptHash(h hash.Hash64, parts ...string) {
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
}

func fingerprintSections(sections SectionSet) string {
	names := make([]string, 0, len(sections))
	for section, enabled := range sections {
		if enabled {
			names = append(names, string(section))
		}
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}

func fingerprintTools(sections SectionSet, modelName string, toolList []tools.Tool) string {
	if !sections.Has(SectionTools) || !strings.Contains(strings.ToLower(modelName), "qwen") {
		return ""
	}
	var sb strings.Builder
	for _, t := range toolList {
		description := t.Description()
		if mp, ok := t.(tools.ModelPromptDescriber); ok {
			if prompt := strings.TrimSpace(mp.ModelPrompt()); prompt != "" {
				description = prompt
			}
		}
		sb.WriteString(t.Name())
		sb.WriteByte('|')
		sb.WriteString(tools.CompactToolSignature(t))
		sb.WriteByte('|')
		sb.WriteString(tools.CompactToolDescription(description))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func fingerprintServers(servers []store.Server) string {
	var sb strings.Builder
	for _, srv := range servers {
		sb.WriteString(srv.Name)
		sb.WriteByte('|')
		sb.WriteString(srv.User)
		sb.WriteByte('|')
		sb.WriteString(srv.Host)
		sb.WriteByte('|')
		sb.WriteString(strconv.Itoa(srv.Port))
		sb.WriteByte('|')
		sb.WriteString(srv.OS)
		sb.WriteByte('|')
		sb.WriteString(srv.EnvProfile)
		sb.WriteByte('|')
		sb.WriteString(srv.WorkspaceDir)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func fingerprintActiveServer(activeServer *store.Server) string {
	if activeServer == nil {
		return ""
	}
	return strings.Join([]string{
		activeServer.Name,
		activeServer.User,
		activeServer.Host,
		strconv.Itoa(activeServer.Port),
		activeServer.OS,
		activeServer.EnvProfile,
		activeServer.WorkspaceDir,
	}, "|")
}

func fingerprintConnectorStates(states []connector.ConnectorState) string {
	var sb strings.Builder
	for _, cs := range states {
		sb.WriteString(cs.ServerName)
		sb.WriteByte('|')
		sb.WriteString(cs.Type)
		sb.WriteByte('|')
		sb.WriteString(cs.ServiceName)
		sb.WriteByte('|')
		sb.WriteString(string(cs.Status))
		sb.WriteByte('|')
		sb.WriteString(strings.Join(cs.Tools, ","))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func fingerprintLearnedSystems(systems []store.LearnedSystem) string {
	var sb strings.Builder
	for _, sys := range systems {
		sb.WriteString(sys.ServiceType)
		sb.WriteByte('|')
		sb.WriteString(sys.ServerName)
		sb.WriteByte('|')
		sb.WriteString(sys.CLIPath)
		sb.WriteByte('|')
		sb.WriteString(sys.ConfigPath)
		sb.WriteByte('|')
		sb.WriteString(sys.LogPath)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func fingerprintRAGSources(sources []store.RAGSource) string {
	var sb strings.Builder
	for _, src := range sources {
		sb.WriteString(src.Name)
		sb.WriteByte('|')
		sb.WriteString(src.Description)
		sb.WriteByte('|')
		sb.WriteString(src.ServerName)
		sb.WriteByte('|')
		sb.WriteString(src.DBType)
		sb.WriteByte('|')
		sb.WriteString(strconv.Itoa(src.Priority))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func fingerprintKnowledgeStats(stats *rag.KnowledgeStats) string {
	if stats == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(strconv.Itoa(stats.TotalDocs))
	sb.WriteByte('|')
	sb.WriteString(strconv.Itoa(stats.TotalWithVec))
	sb.WriteByte('\n')

	sourceKeys := make([]string, 0, len(stats.CountBySource))
	for key := range stats.CountBySource {
		sourceKeys = append(sourceKeys, key)
	}
	sort.Strings(sourceKeys)
	for _, key := range sourceKeys {
		sb.WriteString("src:")
		sb.WriteString(key)
		sb.WriteByte('=')
		sb.WriteString(strconv.Itoa(stats.CountBySource[key]))
		sb.WriteByte('\n')
	}

	categoryKeys := make([]string, 0, len(stats.CountByCategory))
	for key := range stats.CountByCategory {
		categoryKeys = append(categoryKeys, key)
	}
	sort.Strings(categoryKeys)
	for _, key := range categoryKeys {
		sb.WriteString("cat:")
		sb.WriteString(key)
		sb.WriteByte('=')
		sb.WriteString(strconv.Itoa(stats.CountByCategory[key]))
		sb.WriteByte('\n')
	}

	return sb.String()
}

func fingerprintLocalEnvironment() string {
	hostname, _ := os.Hostname()
	cwd, _ := os.Getwd()
	return strings.Join([]string{runtime.GOOS, runtime.GOARCH, hostname, cwd}, "|")
}
