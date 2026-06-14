package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenCodePlugin_Name(t *testing.T) {
	p := &OpenCodePlugin{}
	if p.Name() != "opencode" {
		t.Errorf("Name() = %q, want %q", p.Name(), "opencode")
	}
}

func TestOpenCodePlugin_ConfigFields(t *testing.T) {
	p := &OpenCodePlugin{}
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

func TestOpenCodePlugin_Run_StreamingSmoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if req["model"] != "minimax-m3-free" {
			t.Errorf("model = %v, want minimax-m3-free", req["model"])
		}
		if req["stream"] != true {
			t.Errorf("stream = %v, want true", req["stream"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Trans\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lated\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := &OpenCodePlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "test-key", "model": "minimax-m3-free"}
	chunks, errs := p.Run(context.Background(), "Original", "Translate", cfg)
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "Translated" {
		t.Errorf("got %q, want %q", got, "Translated")
	}
}

func TestOpenCodePlugin_Run_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid api key"}`)
	}))
	defer server.Close()

	p := &OpenCodePlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "bad", "model": "minimax-m3-free"}
	chunks, errs := p.Run(context.Background(), "c", "p", cfg)
	got, err := drainStream(t, chunks, errs)
	if got != "" {
		t.Errorf("got chunks %q, want none", got)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v, want substring 401", err)
	}
}
