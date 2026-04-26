package main

import (
	"context"
	"fmt"
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
	return nil, fmt.Errorf("OpenRouterEmbeddings.Embed not implemented")
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
