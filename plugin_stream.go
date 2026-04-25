package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func streamChatCompletion(ctx context.Context, url, apiKey, model, systemPrompt, userMessage string) (<-chan string, <-chan error) {
	chunks := make(chan string)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)

		body, err := json.Marshal(map[string]interface{}{
			"model":  model,
			"stream": true,
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": userMessage},
			},
		})
		if err != nil {
			errs <- fmt.Errorf("marshaling request: %w", err)
			return
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			errs <- fmt.Errorf("creating request: %w", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errs <- fmt.Errorf("request failed: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			errs <- fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
			return
		}

		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "text/event-stream") {
			emitBlocking(ctx, resp.Body, chunks, errs)
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				return
			}
			var frame struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err != nil {
				continue // tolerant of malformed frames
			}
			if len(frame.Choices) == 0 || frame.Choices[0].Delta.Content == "" {
				continue
			}
			select {
			case chunks <- frame.Choices[0].Delta.Content:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			errs <- fmt.Errorf("stream read error: %w", err)
		}
	}()

	return chunks, errs
}

// emitBlocking parses a non-SSE JSON response (the regular blocking
// chat-completion shape) and emits the full content as one chunk.
func emitBlocking(ctx context.Context, body io.Reader, chunks chan<- string, errs chan<- error) {
	respBody, err := io.ReadAll(body)
	if err != nil {
		errs <- fmt.Errorf("reading response: %w", err)
		return
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		errs <- fmt.Errorf("parsing response: %w", err)
		return
	}
	if len(result.Choices) == 0 {
		errs <- fmt.Errorf("no choices in response")
		return
	}
	select {
	case chunks <- result.Choices[0].Message.Content:
	case <-ctx.Done():
	}
}

// truncate cuts s to at most max runes, appending "..." if truncated.
// Used by error messages so a noisy provider response doesn't flood the UI.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
