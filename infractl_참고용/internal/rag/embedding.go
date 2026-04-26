// Package rag
// File: embedding.go
// Description: 임베딩 생성 클라이언트 — Ollama / OpenAI 호환 API
// Responsibility: 텍스트를 float32 벡터로 변환하는 EmbeddingGenerator 인터페이스 및 구현체 제공

package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yourorg/infractl/internal/config"
)

// EmbeddingGenerator는 텍스트를 벡터로 변환하는 인터페이스이다.
type EmbeddingGenerator interface {
	// Generate는 텍스트를 임베딩 벡터로 변환한다.
	Generate(ctx context.Context, text string) ([]float32, error)
	// ModelName은 임베딩 모델명을 반환한다.
	ModelName() string
}

// NewEmbeddingGenerator는 설정에 따라 적절한 EmbeddingGenerator를 반환한다.
// Endpoint 또는 Model이 비어 있으면 nil을 반환한다 (임베딩 비활성화).
func NewEmbeddingGenerator(cfg config.EmbeddingConfig) EmbeddingGenerator {
	if cfg.Endpoint == "" || cfg.Model == "" {
		return nil
	}
	switch cfg.Provider {
	case "openai", "custom":
		return newOpenAIEmbedder(cfg.Endpoint, cfg.Model, cfg.APIKey)
	default: // "ollama"
		return newOllamaEmbedder(cfg.Endpoint, cfg.Model)
	}
}

// --- Ollama 임베딩 ---

type ollamaEmbedder struct {
	endpoint string
	model    string
	client   *http.Client
}

func newOllamaEmbedder(endpoint, model string) *ollamaEmbedder {
	return &ollamaEmbedder{
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *ollamaEmbedder) ModelName() string { return e.model }

func (e *ollamaEmbedder) Generate(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]string{
		"model":  e.model,
		"prompt": text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.endpoint+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embeddings request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embeddings status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding")
	}
	return result.Embedding, nil
}

// --- OpenAI 호환 임베딩 ---

type openAIEmbedder struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

func newOpenAIEmbedder(endpoint, model, apiKey string) *openAIEmbedder {
	return &openAIEmbedder{
		endpoint: endpoint,
		model:    model,
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *openAIEmbedder) ModelName() string { return e.model }

func (e *openAIEmbedder) Generate(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]string{
		"model": e.model,
		"input": text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.endpoint+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embeddings request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embeddings status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("openai returned empty embedding")
	}
	return result.Data[0].Embedding, nil
}
