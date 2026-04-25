package main

import (
	"context"
	"fmt"
)

const defaultBlackboxURL = "https://api.blackbox.ai/v1/chat/completions"

type BlackboxPlugin struct {
	BaseURL string // override for testing; empty uses default
}

func (p *BlackboxPlugin) Name() string        { return "blackbox" }
func (p *BlackboxPlugin) Description() string { return "LLM-powered note transformation via blackbox.ai" }

func (p *BlackboxPlugin) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "api_key", Label: "API Key", Placeholder: "sk-...", Secret: true},
		{Key: "model", Label: "Model", Placeholder: "blackboxai/minimax/minimax-m2.5", Secret: false},
	}
}

func (p *BlackboxPlugin) Run(ctx context.Context, content, prompt string, cfg map[string]string) (<-chan string, <-chan error) {
	url := p.BaseURL
	if url == "" {
		url = defaultBlackboxURL
	}
	systemPrompt := "You are a note editor. Apply the following transformation to the note provided by the user. Return only the transformed note content, no explanations."
	userMessage := fmt.Sprintf("Instruction: %s\n\nNote:\n%s", prompt, content)
	return streamChatCompletion(ctx, url, cfg["api_key"], cfg["model"], systemPrompt, userMessage)
}
