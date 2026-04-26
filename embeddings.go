package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// EmbeddingClient computes vector embeddings for batches of text.
// All implementations are OpenAI-compatible at the request level but may
// differ in batching and response shape; callers see the same texts-in /
// vectors-out contract.
type EmbeddingClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Model() string
	Dim() int
}

// OpenRouterEmbeddings calls OpenRouter's /v1/embeddings endpoint.
type OpenRouterEmbeddings struct {
	BaseURL   string // override for testing; empty uses default
	APIKey    string
	ModelName string
	dim       int // populated on first response, then trusted thereafter
}

const defaultOpenRouterEmbeddingsURL = "https://openrouter.ai/api/v1/embeddings"

func (e *OpenRouterEmbeddings) Model() string { return e.ModelName }
func (e *OpenRouterEmbeddings) Dim() int      { return e.dim }
func (e *OpenRouterEmbeddings) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	url := e.BaseURL
	if url == "" {
		url = defaultOpenRouterEmbeddingsURL
	}
	const batchSize = 100
	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := e.embedBatch(ctx, url, texts[i:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	if len(out) > 0 {
		e.dim = len(out[0])
	}
	return out, nil
}

func (e *OpenRouterEmbeddings) embedBatch(ctx context.Context, url string, batch []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]interface{}{
		"model": e.ModelName,
		"input": batch,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter embeddings (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(parsed.Data) != len(batch) {
		return nil, fmt.Errorf("openrouter embeddings: got %d vectors for %d inputs", len(parsed.Data), len(batch))
	}
	out := make([][]float32, len(parsed.Data))
	for i, d := range parsed.Data {
		out[i] = d.Embedding
	}
	return out, nil
}

// OllamaEmbeddings calls a local Ollama daemon's /api/embeddings.
type OllamaEmbeddings struct {
	BaseURL   string // e.g. "http://localhost:11434"
	ModelName string
	dim       int
}

func (e *OllamaEmbeddings) Model() string { return e.ModelName }
func (e *OllamaEmbeddings) Dim() int      { return e.dim }
func (e *OllamaEmbeddings) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("OllamaEmbeddings.Embed not implemented")
}

// newEmbeddingClient picks the implementation based on cfg.EmbeddingProvider.
// Returns an error if the provider is unknown or required config is missing.
func newEmbeddingClient(cfg Config) (EmbeddingClient, error) {
	switch cfg.EmbeddingProvider {
	case "ollama":
		return &OllamaEmbeddings{BaseURL: cfg.OllamaURL, ModelName: cfg.EmbeddingModel}, nil
	case "openrouter", "":
		keyCfg, err := loadPluginConfig("openrouter")
		if err != nil {
			return nil, fmt.Errorf("openrouter embeddings need plugin config: %w", err)
		}
		apiKey := keyCfg["api_key"]
		if apiKey == "" {
			return nil, fmt.Errorf("openrouter embeddings: api_key not set in plugin config")
		}
		return &OpenRouterEmbeddings{APIKey: apiKey, ModelName: cfg.EmbeddingModel}, nil
	default:
		return nil, fmt.Errorf("unknown embedding_provider: %q", cfg.EmbeddingProvider)
	}
}
