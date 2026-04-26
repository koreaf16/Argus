// Package llm — provider model discovery.
//
// 파일 역할: 등록된 서버에서 모델 목록을 조회해 init 메뉴와 관리 UI에 공급한다.
//
//	OpenAI-compatible, Anthropic, Gemini 각 provider의 REST 응답 차이를 흡수한다.
//
// 포함 모듈:
//   - DiscoveredModel: discovery 결과의 provider-neutral 표현.
//   - DiscoverServerModels/DiscoverModels: 서버 기준 모델 조회 진입점.
//   - discoverOpenAIModels/discoverAnthropicModels/discoverGeminiModels: provider 구현.
//
// 호출/사용 방식:
//   - cmd/argus/init*.go가 서버 등록 직후와 기존 서버 import 시 호출한다.
//
// 호출/사용 방식:
//   - external entrypoints: (*Registry).DiscoverServerModels, (*Registry).DiscoverModels.
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): 없음.
//   - 이 파일을 import 하는 주요 패키지: cmd/argus/init*.go.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
)

type DiscoveredModel struct {
	ModelID    string `json:"model_id"`
	Display    string `json:"display"`
	ContextWin int    `json:"context_win,omitempty"`
	Caps       Caps   `json:"caps"`
}

func (r *Registry) DiscoverServerModels(ctx context.Context, alias string) ([]DiscoveredModel, error) {
	server, ok := r.GetServer(alias)
	if !ok {
		return nil, fmt.Errorf("unknown server alias: %s", alias)
	}
	return r.DiscoverModels(ctx, server)
}

func (r *Registry) DiscoverModels(ctx context.Context, server ServerEntry) ([]DiscoveredModel, error) {
	switch server.Provider {
	case ProviderOpenAICompat:
		return r.discoverOpenAIModels(ctx, server)
	case ProviderAnthropic:
		return r.discoverAnthropicModels(ctx, server)
	case ProviderGemini:
		return r.discoverGeminiModels(ctx, server)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", server.Provider)
	}
}

func (r *Registry) discoverOpenAIModels(ctx context.Context, server ServerEntry) ([]DiscoveredModel, error) {
	probedServer, models, err := r.ProbeOpenAICompatServer(ctx, server)
	if err != nil {
		return nil, err
	}
	_ = probedServer
	return models, nil
}

func (r *Registry) discoverOpenAIModelsAtBaseURL(ctx context.Context, server ServerEntry) ([]DiscoveredModel, error) {
	endpoint := strings.TrimRight(server.EffectiveBaseURL(), "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	if apiKey := resolveOptionalAPIKey(server); apiKey != "" {
		req.Header.Set("authorization", "Bearer "+apiKey)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := doJSON(r.currentHTTPClient(), req, &payload); err != nil {
		return nil, err
	}

	models := make([]DiscoveredModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		modelID := strings.TrimSpace(item.ID)
		if modelID == "" {
			continue
		}
		models = append(models, DiscoveredModel{
			ModelID: modelID,
			Display: modelID,
			Caps:    DefaultCapsForProvider(ProviderOpenAICompat),
		})
	}
	sortDiscovered(models)
	return models, nil
}

func (r *Registry) discoverAnthropicModels(ctx context.Context, server ServerEntry) ([]DiscoveredModel, error) {
	apiKey, err := resolveRequiredAPIKey(server)
	if err != nil {
		return nil, err
	}

	base := strings.TrimRight(server.EffectiveBaseURL(), "/")
	cursor := ""
	models := make([]DiscoveredModel, 0)
	for {
		endpoint := base + "/models?limit=1000"
		if cursor != "" {
			endpoint += "&after_id=" + url.QueryEscape(cursor)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("accept", "application/json")
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		var payload struct {
			Data []struct {
				ID             string `json:"id"`
				DisplayName    string `json:"display_name"`
				MaxInputTokens int    `json:"max_input_tokens"`
				Capabilities   struct {
					Vision struct {
						Supported bool `json:"supported"`
					} `json:"vision"`
					Thinking struct {
						Enabled struct {
							Supported bool `json:"supported"`
						} `json:"enabled"`
					} `json:"thinking"`
				} `json:"capabilities"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := doJSON(r.currentHTTPClient(), req, &payload); err != nil {
			return nil, err
		}

		for _, item := range payload.Data {
			modelID := strings.TrimSpace(item.ID)
			if modelID == "" {
				continue
			}
			caps := DefaultCapsForProvider(ProviderAnthropic)
			if item.Capabilities.Thinking.Enabled.Supported {
				caps.Thinking = true
			}
			if item.Capabilities.Vision.Supported {
				caps.Vision = true
			}
			models = append(models, DiscoveredModel{
				ModelID:    modelID,
				Display:    firstNonEmpty(item.DisplayName, modelID),
				ContextWin: item.MaxInputTokens,
				Caps:       caps,
			})
		}
		if !payload.HasMore || strings.TrimSpace(payload.LastID) == "" {
			break
		}
		cursor = payload.LastID
	}

	sortDiscovered(models)
	return dedupeDiscovered(models), nil
}

func (r *Registry) discoverGeminiModels(ctx context.Context, server ServerEntry) ([]DiscoveredModel, error) {
	apiKey, err := resolveRequiredAPIKey(server)
	if err != nil {
		return nil, err
	}

	base := strings.TrimRight(server.EffectiveBaseURL(), "/")
	pageToken := ""
	models := make([]DiscoveredModel, 0)
	for {
		endpoint := base + "/models?pageSize=1000&key=" + url.QueryEscape(apiKey)
		if pageToken != "" {
			endpoint += "&pageToken=" + url.QueryEscape(pageToken)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("accept", "application/json")

		var payload struct {
			Models []struct {
				Name                       string   `json:"name"`
				BaseModelID                string   `json:"baseModelId"`
				DisplayName                string   `json:"displayName"`
				InputTokenLimit            int      `json:"inputTokenLimit"`
				SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
				Thinking                   bool     `json:"thinking"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := doJSON(r.currentHTTPClient(), req, &payload); err != nil {
			return nil, err
		}

		for _, item := range payload.Models {
			if !supportsMethod(item.SupportedGenerationMethods, "generateContent") {
				continue
			}
			modelID := strings.TrimSpace(item.BaseModelID)
			if modelID == "" {
				modelID = strings.TrimPrefix(strings.TrimSpace(item.Name), "models/")
			}
			if modelID == "" {
				continue
			}
			caps := DefaultCapsForProvider(ProviderGemini)
			caps.Thinking = item.Thinking
			models = append(models, DiscoveredModel{
				ModelID:    modelID,
				Display:    firstNonEmpty(item.DisplayName, modelID),
				ContextWin: item.InputTokenLimit,
				Caps:       caps,
			})
		}

		if strings.TrimSpace(payload.NextPageToken) == "" {
			break
		}
		pageToken = payload.NextPageToken
	}

	sortDiscovered(models)
	return dedupeDiscovered(models), nil
}

func resolveOptionalAPIKey(server ServerEntry) string {
	envName := strings.TrimSpace(server.APIKeyEnv)
	if envName == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(envName))
}

func resolveRequiredAPIKey(server ServerEntry) (string, error) {
	envName := strings.TrimSpace(server.APIKeyEnv)
	if envName == "" {
		return "", fmt.Errorf("server %s requires api_key_env", server.Alias)
	}
	apiKey := strings.TrimSpace(os.Getenv(envName))
	if apiKey == "" {
		return "", fmt.Errorf("missing api key env: %s", envName)
	}
	return apiKey, nil
}

func doJSON(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return fmt.Errorf("request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func supportsMethod(methods []string, target string) bool {
	for _, method := range methods {
		if strings.EqualFold(strings.TrimSpace(method), target) {
			return true
		}
	}
	return false
}

func sortDiscovered(models []DiscoveredModel) {
	sort.Slice(models, func(i, j int) bool {
		if models[i].Display == models[j].Display {
			return models[i].ModelID < models[j].ModelID
		}
		return models[i].Display < models[j].Display
	})
}

func dedupeDiscovered(models []DiscoveredModel) []DiscoveredModel {
	seen := make(map[string]struct{}, len(models))
	out := make([]DiscoveredModel, 0, len(models))
	for _, model := range models {
		if _, ok := seen[model.ModelID]; ok {
			continue
		}
		seen[model.ModelID] = struct{}{}
		out = append(out, model)
	}
	return out
}
