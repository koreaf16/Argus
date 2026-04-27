package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type RegistrySource struct {
	cache *Cache
}

func NewRegistrySource(cache *Cache) *RegistrySource {
	return &RegistrySource{cache: cache}
}

func (s *RegistrySource) Search(ctx context.Context, query string) ([]ConnectorSpec, error) {
	// Fetch from registry
	specs, err := s.fetchRegistry(ctx)
	if err != nil {
		return nil, err
	}

	// Filter
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return specs, nil
	}

	var results []ConnectorSpec
	for _, spec := range specs {
		if strings.Contains(strings.ToLower(spec.Name), query) ||
			strings.Contains(strings.ToLower(spec.Description), query) {
			results = append(results, spec)
		}
	}
	return results, nil
}

func (s *RegistrySource) Info(ctx context.Context, name string) (*ConnectorSpec, error) {
	specs, err := s.fetchRegistry(ctx)
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		if spec.Name == name {
			return &spec, nil
		}
	}
	return nil, fmt.Errorf("connector %q not found in registry", name)
}

func (s *RegistrySource) fetchRegistry(ctx context.Context) ([]ConnectorSpec, error) {
	const cacheKey = "mcp-registry"
	const cacheTTL = 1 * time.Hour
	const registryURL = "https://registry.modelcontextprotocol.io/v0.1/servers"

	if cached, ok := s.cache.Get(cacheKey, cacheTTL); ok {
		var specs []ConnectorSpec
		if err := json.Unmarshal(cached, &specs); err == nil {
			return specs, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", registryURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Servers []struct {
			Server struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Remotes     []struct {
					Type string `json:"type"`
					Url  string `json:"url"`
				} `json:"remotes"`
				Packages []struct {
					Identifier   string `json:"identifier"`
					RegistryType string `json:"registryType"`
					Transport    struct {
						Type string `json:"type"`
					} `json:"transport"`
					EnvironmentVariables []struct {
						Name        string `json:"name"`
						Description string `json:"description"`
						Required    bool   `json:"required"`
					} `json:"environmentVariables"`
				} `json:"packages"`
			} `json:"server"`
		} `json:"servers"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	var specs []ConnectorSpec
	for _, item := range payload.Servers {
		srv := item.Server
		if len(srv.Packages) == 0 {
			// We only support package-based stdio servers for auto-install right now
			continue
		}
		pkg := srv.Packages[0]
		if pkg.Transport.Type != "stdio" {
			continue
		}

		spec := ConnectorSpec{
			Name:        srv.Name,
			Description: srv.Description,
			Source:      "registry",
		}

		switch pkg.RegistryType {
		case "npm":
			spec.Runtime = RuntimeNPX
			spec.Command = "npx"
			spec.Args = []string{"-y", pkg.Identifier}
		case "pypi":
			spec.Runtime = RuntimeUVX
			spec.Command = "uvx"
			spec.Args = []string{pkg.Identifier}
		default:
			// Unsupported registry type
			continue
		}

		for _, ev := range pkg.EnvironmentVariables {
			spec.EnvPrompts = append(spec.EnvPrompts, EnvPrompt{
				Key:         ev.Name,
				Description: ev.Description,
				Required:    ev.Required,
				Secret:      strings.Contains(strings.ToLower(ev.Name), "token") || strings.Contains(strings.ToLower(ev.Name), "key") || strings.Contains(strings.ToLower(ev.Name), "pass"),
			})
		}

		specs = append(specs, spec)
	}

	if b, err := json.Marshal(specs); err == nil {
		_ = s.cache.Set(cacheKey, b)
	}

	return specs, nil
}
