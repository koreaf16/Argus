package connector

import (
	"context"
	"fmt"

	"github.com/koreaf16/argus/internal/services/mcp"
)

type Installer struct {
	manager *mcp.Manager
}

func NewInstaller(manager *mcp.Manager) *Installer {
	return &Installer{manager: manager}
}

func (i *Installer) Install(ctx context.Context, spec ConnectorSpec, envAnswers map[string]string) error {
	if err := CheckRuntime(spec.Runtime); err != nil {
		return fmt.Errorf("runtime check failed: %w", err)
	}
	
	cmd, args, err := ResolveCommand(spec)
	if err != nil {
		return err
	}

	cfg := mcp.ServerConfig{
		Name:    spec.Name,
		Command: cmd,
		Args:    args,
		Env:     make(map[string]string),
	}

	for k, v := range spec.Env {
		cfg.Env[k] = v
	}
	for k, v := range envAnswers {
		cfg.Env[k] = v
	}

	// Discover tools
	tools, err := i.manager.DiscoverTools(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to discover tools for %q: %w", spec.Name, err)
	}
	cfg.Tools = tools

	return i.manager.AddServer(cfg)
}

func (i *Installer) Remove(name string) error {
	return i.manager.RemoveServer(name)
}

func (i *Installer) ListInstalled() ([]InstalledConnector, error) {
	var installed []InstalledConnector
	for _, name := range i.manager.ServerNames() {
		cfg, ok := i.manager.GetServer(name)
		if !ok {
			continue
		}
		
		spec := ConnectorSpec{
			Name:    cfg.Name,
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     cfg.Env,
		}
		
		installed = append(installed, InstalledConnector{
			Spec:   spec,
			Status: "active",
		})
	}
	return installed, nil
}
