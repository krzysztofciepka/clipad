package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type agentMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []agentToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// MarshalJSON omits "content" when the message is a pure tool-call (empty
// content with tool_calls present), matching the OpenAI spec where content is
// optional in that case. Keeping Content as a plain string keeps construction
// sites ergonomic; this isolates the wire-format detail.
func (m agentMessage) MarshalJSON() ([]byte, error) {
	if m.Content == "" && len(m.ToolCalls) > 0 {
		return json.Marshal(struct {
			Role       string          `json:"role"`
			ToolCalls  []agentToolCall `json:"tool_calls,omitempty"`
			ToolCallID string          `json:"tool_call_id,omitempty"`
		}{Role: m.Role, ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID})
	}
	type alias agentMessage
	return json.Marshal(alias(m))
}

type agentToolCall struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function agentToolFunction `json:"function"`
}

type agentToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type agentTool struct {
	Type     string        `json:"type"`
	Function agentToolSpec `json:"function"`
}

type agentToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// agentTools returns the tool schema advertised to the model.
func agentTools() []agentTool {
	return []agentTool{
		{Type: "function", Function: agentToolSpec{
			Name:        "bash",
			Description: "Run a bash command in the notes vault (cwd is the vault root). Use for managing files: cd, ls, mv, cp, cat, sed, awk, etc. Confined to the vault.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cmd": map[string]any{
						"type":        "string",
						"description": "The bash command to run.",
					},
				},
				"required": []string{"cmd"},
			},
		}},
		{Type: "function", Function: agentToolSpec{
			Name:        "search_vault",
			Description: "Semantic search over the user's notes. Use to answer questions about note content. Returns numbered excerpts with file paths and line ranges to cite.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query.",
					},
					"k": map[string]any{
						"type":        "integer",
						"description": "Number of excerpts to return (default 8).",
					},
				},
				"required": []string{"query"},
			},
		}},
	}
}

// runAgentTurn performs one non-streaming chat completion with tools enabled
// and returns the assistant message (which may carry content, tool_calls, or
// both).
func runAgentTurn(ctx context.Context, url, apiKey, model string, messages []agentMessage, tools []agentTool) (agentMessage, error) {
	body, err := json.Marshal(map[string]any{
		"model":       model,
		"messages":    messages,
		"tools":       tools,
		"tool_choice": "auto",
		"stream":      false,
	})
	if err != nil {
		return agentMessage{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return agentMessage{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return agentMessage{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return agentMessage{}, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message agentMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return agentMessage{}, fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return agentMessage{}, fmt.Errorf("no choices in response")
	}
	return result.Choices[0].Message, nil
}
