// Package llm — OpenAI-compatible URL normalization and probing.
//
// 파일 역할: OpenAI-호환 서버 주소를 사용자가 간단히 입력했을 때
//
//	실제 discovery/runtime에 쓸 base URL을 자동 추론하고 검증한다.
//
// 포함 모듈:
//   - ProbeOpenAICompatServer: 후보 endpoint를 자동 시도해 canonical base URL을 확정.
//   - normalizeOpenAICompatURL: scheme 보정과 URL 정규화.
//   - openAICompatCandidateBaseURLs: path 유무에 따라 probe 후보를 생성.
//
// 호출/사용 방식:
//   - cmd/argus/init_menu.go 의 addServerFlow 가 신규 서버 등록 시 호출한다.
//   - internal/services/llm/registry_discovery.go 가 기존 서버 discovery 시 재사용한다.
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): 없음.
//   - 이 파일을 import 하는 주요 패키지: internal/services/llm/registry_discovery.go, cmd/argus/init_menu.go.
package llm

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

func (r *Registry) ProbeOpenAICompatServer(ctx context.Context, server ServerEntry) (ServerEntry, []DiscoveredModel, error) {
	rawBaseURL := strings.TrimSpace(server.BaseURL)
	if rawBaseURL == "" {
		rawBaseURL = strings.TrimSpace(server.EffectiveBaseURL())
	}
	normalizedBaseURL, err := normalizeOpenAICompatURL(rawBaseURL)
	if err != nil {
		return ServerEntry{}, nil, err
	}

	candidates, err := openAICompatCandidateBaseURLs(normalizedBaseURL)
	if err != nil {
		return ServerEntry{}, nil, err
	}

	tried := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		tried = append(tried, strings.TrimRight(candidate, "/")+"/models")

		candidateServer := server
		candidateServer.BaseURL = candidate
		candidateServer.Scheme = ""
		candidateServer.Host = ""
		candidateServer.Port = 0
		candidateServer.PathPrefix = ""

		models, err := r.discoverOpenAIModelsAtBaseURL(ctx, candidateServer)
		if err != nil {
			continue
		}
		if len(models) == 0 {
			continue
		}
		if strings.TrimSpace(candidateServer.Display) == "" {
			candidateServer.Display = candidateServer.Alias
		}
		return candidateServer, models, nil
	}

	sort.Strings(tried)
	return ServerEntry{}, nil, fmt.Errorf("openai-compatible discovery failed; tried: %s", strings.Join(tried, ", "))
}

func normalizeOpenAICompatURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("server url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid server url: %w", err)
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("server url must include host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.EscapedPath(), "/")
	if parsed.Path == "/" {
		parsed.Path = ""
	}
	return parsed.String(), nil
}

func openAICompatCandidateBaseURLs(rawBaseURL string) ([]string, error) {
	parsed, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server url: %w", err)
	}

	root := (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
	path := strings.Trim(parsed.Path, "/")
	candidates := make([]string, 0, 2)
	if path == "" {
		candidates = append(candidates, root+"/v1", root)
	} else {
		candidates = append(candidates, root+"/"+path)
	}
	return dedupeBaseURLs(candidates), nil
}

func dedupeBaseURLs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimRight(strings.TrimSpace(value), "/")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
