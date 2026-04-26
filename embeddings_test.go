package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func openrouterTestServer(t *testing.T, vectors [][]float32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}{}
		for _, v := range vectors {
			out.Data = append(out.Data, struct {
				Embedding []float32 `json:"embedding"`
			}{Embedding: v})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
}

func TestOpenRouterEmbeddings_HappyPath(t *testing.T) {
	srv := openrouterTestServer(t, [][]float32{
		{1, 0, 0},
		{0, 1, 0},
	})
	defer srv.Close()

	e := &OpenRouterEmbeddings{BaseURL: srv.URL, APIKey: "k", ModelName: "qwen/qwen3-embedding-8b"}
	got, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d vectors, want 2", len(got))
	}
	if got[0][0] != 1 || got[1][1] != 1 {
		t.Errorf("vectors: %v", got)
	}
	if e.Dim() != 3 {
		t.Errorf("Dim() = %d, want 3", e.Dim())
	}
}

func TestOpenRouterEmbeddings_Batches(t *testing.T) {
	calls := 0
	totalInputs := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		totalInputs += len(req.Input)
		out := struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}{}
		for range req.Input {
			out.Data = append(out.Data, struct {
				Embedding []float32 `json:"embedding"`
			}{Embedding: []float32{0, 0, 1}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	e := &OpenRouterEmbeddings{BaseURL: srv.URL, APIKey: "k", ModelName: "m"}
	inputs := make([]string, 101)
	for i := range inputs {
		inputs[i] = "x"
	}
	got, err := e.Embed(context.Background(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("HTTP calls = %d, want 2", calls)
	}
	if totalInputs != 101 {
		t.Errorf("total inputs = %d, want 101", totalInputs)
	}
	if len(got) != 101 {
		t.Errorf("vectors = %d, want 101", len(got))
	}
}

func TestOllamaEmbeddings_HappyPath(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var v []float32
		if req.Prompt == "alpha" {
			v = []float32{1, 0, 0}
		} else {
			v = []float32{0, 1, 0}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string][]float32{"embedding": v})
	}))
	defer srv.Close()

	e := &OllamaEmbeddings{BaseURL: srv.URL, ModelName: "nomic-embed-text"}
	got, err := e.Embed(context.Background(), []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("HTTP calls = %d, want 2 (one per text)", calls)
	}
	if len(got) != 2 || got[0][0] != 1 || got[1][1] != 1 {
		t.Errorf("vectors = %v", got)
	}
	if e.Dim() != 3 {
		t.Errorf("Dim() = %d, want 3", e.Dim())
	}
}

func TestNewEmbeddingClient_Ollama(t *testing.T) {
	cfg := Config{EmbeddingProvider: "ollama", EmbeddingModel: "nomic-embed-text", OllamaURL: "http://localhost:11434"}
	c, err := newEmbeddingClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*OllamaEmbeddings); !ok {
		t.Errorf("got %T, want *OllamaEmbeddings", c)
	}
	if c.Model() != "nomic-embed-text" {
		t.Errorf("Model() = %q", c.Model())
	}
}

func TestNewEmbeddingClient_UnknownProvider(t *testing.T) {
	cfg := Config{EmbeddingProvider: "bogus"}
	_, err := newEmbeddingClient(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown embedding_provider") {
		t.Errorf("error = %v", err)
	}
}

func TestOpenRouterEmbeddings_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	e := &OpenRouterEmbeddings{BaseURL: srv.URL, APIKey: "k", ModelName: "m"}
	_, err := e.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %v, want HTTP 401 mention", err)
	}
}
