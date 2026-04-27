package connector

import (
	"github.com/koreaf16/argus/internal/services/mcp"
)

type Manager struct {
	Aggregator *Aggregator
	Installer  *Installer
}

func NewManager(mcpManager *mcp.Manager, cacheDir string) *Manager {
	cache := NewCache(cacheDir)
	return &Manager{
		Aggregator: NewAggregator(cache),
		Installer:  NewInstaller(mcpManager),
	}
}
