package main

import (
	"context"
)

// streamChatCompletion issues a chat-completion request with stream=true
// against an OpenAI-compatible endpoint and returns channels that yield
// content deltas. Cancellation: caller's ctx tears down the HTTP request.
// Fallback: if the response is 200 but Content-Type is not text/event-stream,
// the body is parsed as a regular blocking chat-completion response and the
// full content is emitted as a single chunk.
func streamChatCompletion(ctx context.Context, url, apiKey, model, systemPrompt, userMessage string) (<-chan string, <-chan error) {
	chunks := make(chan string)
	errs := make(chan error, 1)
	go func() {
		defer close(chunks)
		defer close(errs)
		// implementation arrives in subsequent tasks
		_ = ctx
		_ = url
		_ = apiKey
		_ = model
		_ = systemPrompt
		_ = userMessage
	}()
	return chunks, errs
}
