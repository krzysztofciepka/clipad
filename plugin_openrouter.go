package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultOpenRouterURL = "https://openrouter.ai/api/v1/chat/completions"

type OpenRouterPlugin struct {
	BaseURL string // override for testing; empty uses default
}

func (p *OpenRouterPlugin) Name() string       { return "openrouter" }
func (p *OpenRouterPlugin) Description() string { return "LLM-powered note transformation via OpenRouter" }

func (p *OpenRouterPlugin) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "api_key", Label: "API Key", Placeholder: "sk-or-...", Secret: true},
		{Key: "model", Label: "Model", Placeholder: "openai/gpt-4o", Secret: false},
	}
}

func (p *OpenRouterPlugin) Run(content string, prompt string, config map[string]string) (string, error) {
	url := p.BaseURL
	if url == "" {
		url = defaultOpenRouterURL
	}

	reqBody := map[string]interface{}{
		"model": config["model"],
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a note editor. Apply the following transformation to the note provided by the user. Return only the transformed note content, no explanations.",
			},
			{
				"role":    "user",
				"content": fmt.Sprintf("Instruction: %s\n\nNote:\n%s", prompt, content),
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config["api_key"])

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
