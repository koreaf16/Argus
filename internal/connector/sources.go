package connector

import (
	"context"
	"fmt"
	"sort"
)

type Source interface {
	Search(ctx context.Context, query string) ([]ConnectorSpec, error)
	Info(ctx context.Context, name string) (*ConnectorSpec, error)
}

type Aggregator struct {
	sources []Source
	cache   *Cache
}

func NewAggregator(cache *Cache) *Aggregator {
	return &Aggregator{
		sources: []Source{
			NewRegistrySource(cache),
			NewGitHubSource(cache),
		},
		cache: cache,
	}
}

func (a *Aggregator) Search(ctx context.Context, query string) ([]ConnectorSpec, error) {
	var results []ConnectorSpec
	seen := make(map[string]bool)

	for _, src := range a.sources {
		specs, err := src.Search(ctx, query)
		if err != nil {
			// Log error but continue with other sources
			continue
		}
		for _, spec := range specs {
			if !seen[spec.Name] {
				seen[spec.Name] = true
				results = append(results, spec)
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return results, nil
}

func (a *Aggregator) Info(ctx context.Context, name string) (*ConnectorSpec, error) {
	for _, src := range a.sources {
		spec, err := src.Info(ctx, name)
		if err == nil && spec != nil {
			return spec, nil
		}
	}
	return nil, fmt.Errorf("connector %q not found", name)
}
