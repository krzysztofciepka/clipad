package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterPlugin_Name(t *testing.T) {
	p := &OpenRouterPlugin{}
	if p.Name() != "openrouter" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openrouter")
	}
}

func TestOpenRouterPlugin_ConfigFields(t *testing.T) {
	p := &OpenRouterPlugin{}
	fields := p.ConfigFields()
	if len(fields) != 2 {
		t.Fatalf("ConfigFields() returned %d fields, want 2", len(fields))
	}
	if fields[0].Key != "api_key" || !fields[0].Secret {
		t.Errorf("first field: key=%q secret=%v, want api_key/true", fields[0].Key, fields[0].Secret)
	}
	if fields[1].Key != "model" || fields[1].Secret {
		t.Errorf("second field: key=%q secret=%v, want model/false", fields[1].Key, fields[1].Secret)
	}
}

func TestOpenRouterPlugin_Run_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "openai/gpt-4o" {
			t.Errorf("model = %v, want openai/gpt-4o", req["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "Translated content"}},
			},
		})
	}))
	defer server.Close()

	p := &OpenRouterPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "test-key", "model": "openai/gpt-4o"}
	result, err := p.Run("Original content", "Translate to Polish", cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result != "Translated content" {
		t.Errorf("Run() = %q, want %q", result, "Translated content")
	}
}

func TestOpenRouterPlugin_Run_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	p := &OpenRouterPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "bad-key", "model": "openai/gpt-4o"}
	_, err := p.Run("content", "prompt", cfg)
	if err == nil {
		t.Error("expected error for 401 response, got nil")
	}
}

func TestOpenRouterPlugin_Run_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{},
		})
	}))
	defer server.Close()

	p := &OpenRouterPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "key", "model": "m"}
	_, err := p.Run("content", "prompt", cfg)
	if err == nil {
		t.Error("expected error for empty choices, got nil")
	}
}
