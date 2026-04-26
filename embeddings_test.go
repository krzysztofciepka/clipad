package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
