package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentTools_HasBashAndSearchVault(t *testing.T) {
	tools := agentTools()
	names := map[string]bool{}
	for _, tl := range tools {
		if tl.Type != "function" {
			t.Errorf("tool type = %q, want function", tl.Type)
		}
		names[tl.Function.Name] = true
	}
	for _, want := range []string{"bash", "search_vault"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

func TestAgentMessage_MarshalsToolCall(t *testing.T) {
	m := agentMessage{
		Role: "assistant",
		ToolCalls: []agentToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: agentToolFunction{Name: "bash", Arguments: `{"cmd":"ls"}`},
		}},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var round map[string]any
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatal(err)
	}
	if round["role"] != "assistant" {
		t.Errorf("role = %v", round["role"])
	}
	if _, ok := round["tool_calls"]; !ok {
		t.Errorf("tool_calls missing in %s", b)
	}
	if _, ok := round["content"]; ok {
		t.Errorf("content should be omitted for a tool-call-only message, got %s", b)
	}
}

func TestAgentMessage_KeepsContentForPlainMessage(t *testing.T) {
	b, err := json.Marshal(agentMessage{Role: "user", Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	var round map[string]any
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatal(err)
	}
	if round["content"] != "hi" {
		t.Errorf("content = %v, want hi", round["content"])
	}
}

func TestRunAgentTurn_ParsesToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"ls\"}"}}]}}]}`)
	}))
	defer server.Close()

	msg, err := runAgentTurn(context.Background(), server.URL, "k", "m",
		[]agentMessage{{Role: "user", Content: "list files"}}, agentTools())
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "bash" {
		t.Fatalf("got %+v, want one bash tool call", msg)
	}
}

func TestRunAgentTurn_ParsesContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"all done"}}]}`)
	}))
	defer server.Close()

	msg, err := runAgentTurn(context.Background(), server.URL, "k", "m",
		[]agentMessage{{Role: "user", Content: "hi"}}, agentTools())
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "all done" || len(msg.ToolCalls) != 0 {
		t.Fatalf("got %+v, want plain content", msg)
	}
}

func TestRunAgentTurn_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"bad key"}`)
	}))
	defer server.Close()

	_, err := runAgentTurn(context.Background(), server.URL, "bad", "m",
		[]agentMessage{{Role: "user", Content: "hi"}}, agentTools())
	if err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestRunAgentTurn_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{not valid json`)
	}))
	defer server.Close()

	_, err := runAgentTurn(context.Background(), server.URL, "k", "m",
		[]agentMessage{{Role: "user", Content: "hi"}}, agentTools())
	if err == nil {
		t.Fatal("expected error on malformed JSON response")
	}
}

func TestRunAgentTurn_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer server.Close()

	_, err := runAgentTurn(context.Background(), server.URL, "k", "m",
		[]agentMessage{{Role: "user", Content: "hi"}}, agentTools())
	if err == nil {
		t.Fatal("expected error when response has zero choices")
	}
}
