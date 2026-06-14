package main

import (
	"context"
	"fmt"
)

const defaultOpenCodeURL = "https://opencode.ai/zen/go/v1/chat/completions"

type OpenCodePlugin struct {
	BaseURL string // override for testing; empty uses default
}

func (p *OpenCodePlugin) Name() string        { return "opencode" }
func (p *OpenCodePlugin) Description() string { return "LLM-powered note transformation via OpenCode Go (Zen)" }

func (p *OpenCodePlugin) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "api_key", Label: "API Key", Placeholder: "sk-...", Secret: true},
		{Key: "model", Label: "Model", Placeholder: "minimax-m3", Secret: false},
	}
}

func (p *OpenCodePlugin) Run(ctx context.Context, content, prompt string, cfg map[string]string) (<-chan string, <-chan error) {
	url := p.BaseURL
	if url == "" {
		url = defaultOpenCodeURL
	}
	systemPrompt := "You are a note editor. Apply the following transformation to the note provided by the user. Return only the transformed note content, no explanations."
	userMessage := fmt.Sprintf("Instruction: %s\n\nNote:\n%s", prompt, content)
	return streamChatCompletion(ctx, url, cfg["api_key"], cfg["model"], systemPrompt, userMessage)
}
