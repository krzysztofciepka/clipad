package main

import (
	"encoding/json"
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
