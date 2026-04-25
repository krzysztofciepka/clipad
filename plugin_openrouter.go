package main

import (
	"context"
	"fmt"
)

const defaultOpenRouterURL = "https://openrouter.ai/api/v1/chat/completions"

type OpenRouterPlugin struct {
	BaseURL string // override for testing; empty uses default
}

func (p *OpenRouterPlugin) Name() string        { return "openrouter" }
func (p *OpenRouterPlugin) Description() string { return "LLM-powered note transformation via OpenRouter" }

func (p *OpenRouterPlugin) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "api_key", Label: "API Key", Placeholder: "sk-or-...", Secret: true},
		{Key: "model", Label: "Model", Placeholder: "openai/gpt-4o", Secret: false},
	}
}

func (p *OpenRouterPlugin) Run(ctx context.Context, content, prompt string, cfg map[string]string) (<-chan string, <-chan error) {
	url := p.BaseURL
	if url == "" {
		url = defaultOpenRouterURL
	}
	systemPrompt := "You are a note editor. Apply the following transformation to the note provided by the user. Return only the transformed note content, no explanations."
	userMessage := fmt.Sprintf("Instruction: %s\n\nNote:\n%s", prompt, content)
	return streamChatCompletion(ctx, url, cfg["api_key"], cfg["model"], systemPrompt, userMessage)
}
