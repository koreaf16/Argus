// Package infrainit
// File: model_fetcher.go
// Description: LLM/임베딩/리랭커 서버 자동 탐색 및 모델 목록 조회
// Responsibility: host:port 입력 시 서버 유형을 자동 감지하고 모델 ID 목록을 반환

package infrainit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ServerType은 감지된 서버 종류를 나타낸다.
type ServerType string

const (
	ServerOpenAI  ServerType = "OpenAI 호환 (vLLM / SGLang / LiteLLM)"
	ServerOllama  ServerType = "Ollama"
	ServerTEI     ServerType = "Text Embeddings Inference (TEI)"
	ServerUnknown ServerType = "알 수 없음"
)

// DiscoveryResult는 서버 탐색 결과를 담는다.
type DiscoveryResult struct {
	Models   []string   // 탐색된 모델 ID 목록
	Endpoint string     // config에 저장할 최종 엔드포인트 (예: http://host:port/v1)
	Type     ServerType // 감지된 서버 유형
}

// DiscoverServer는 baseURL의 서버 유형을 자동으로 탐지하고 모델 목록을 반환한다.
// baseURL은 http://host:port 형태이며, 경로가 포함된 경우 자동으로 제거한다.
// 탐지 순서: /v1/models (OpenAI 호환) → /api/tags (Ollama) → /info (TEI)
func DiscoverServer(ctx context.Context, rawURL, apiKey string) (DiscoveryResult, error) {
	base, err := normalizeBaseURL(rawURL)
	if err != nil {
		return DiscoveryResult{}, fmt.Errorf("invalid endpoint url: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 1) OpenAI 호환: GET /v1/models
	if models, ok := probeOpenAI(probeCtx, base, apiKey); ok {
		return DiscoveryResult{
			Models:   models,
			Endpoint: base + "/v1",
			Type:     ServerOpenAI,
		}, nil
	}

	// 2) Ollama 네이티브: GET /api/tags
	if models, ok := probeOllamaTags(probeCtx, base, apiKey); ok {
		return DiscoveryResult{
			Models:   models,
			Endpoint: base + "/v1", // Ollama도 /v1 OpenAI 호환 API를 제공한다
			Type:     ServerOllama,
		}, nil
	}

	// 3) TEI: GET /info (단일 모델 반환)
	if model, ok := probeTEIInfo(probeCtx, base, apiKey); ok {
		return DiscoveryResult{
			Models:   []string{model},
			Endpoint: base,
			Type:     ServerTEI,
		}, nil
	}

	return DiscoveryResult{
		Models:   nil,
		Endpoint: base + "/v1",
		Type:     ServerUnknown,
	}, fmt.Errorf("서버 탐지 실패: %s 에서 알려진 API를 찾지 못했습니다", base)
}

// normalizeBaseURL은 URL에서 경로를 제거하고 http://host:port 형태로 반환한다.
func normalizeBaseURL(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	return u.Scheme + "://" + u.Host, nil
}

// probeOpenAI는 GET /v1/models 를 시도하고 성공하면 모델 ID 목록을 반환한다.
func probeOpenAI(ctx context.Context, base, apiKey string) ([]string, bool) {
	type openAIModels struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	body, ok := doGet(ctx, base+"/v1/models", apiKey)
	if !ok {
		return nil, false
	}

	var resp openAIModels
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Data) == 0 {
		return nil, false
	}

	ids := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, len(ids) > 0
}

// probeOllamaTags는 GET /api/tags 를 시도하고 성공하면 모델 이름 목록을 반환한다.
func probeOllamaTags(ctx context.Context, base, apiKey string) ([]string, bool) {
	type ollamaTags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	body, ok := doGet(ctx, base+"/api/tags", apiKey)
	if !ok {
		return nil, false
	}

	var resp ollamaTags
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Models) == 0 {
		return nil, false
	}

	names := make([]string, 0, len(resp.Models))
	for _, m := range resp.Models {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	return names, len(names) > 0
}

// probeTEIInfo는 GET /info 를 시도하고 성공하면 단일 모델 ID를 반환한다.
func probeTEIInfo(ctx context.Context, base, apiKey string) (string, bool) {
	type teiInfo struct {
		ModelID string `json:"model_id"`
	}

	body, ok := doGet(ctx, base+"/info", apiKey)
	if !ok {
		return "", false
	}

	var resp teiInfo
	if err := json.Unmarshal(body, &resp); err != nil || resp.ModelID == "" {
		return "", false
	}
	return resp.ModelID, true
}

// doGet은 HTTP GET 요청을 전송하고 응답 body를 반환한다.
// 실패 또는 비정상 상태코드이면 (nil, false)를 반환한다.
func doGet(ctx context.Context, url, apiKey string) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, false
	}
	return body, true
}
