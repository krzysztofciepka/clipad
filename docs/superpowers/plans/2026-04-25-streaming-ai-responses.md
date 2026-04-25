# Streaming AI Responses Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the blocking POST in `OpenRouterPlugin.Run`, `BlackboxPlugin.Run`, and `runShortcutCmd` with Server-Sent Events streaming so the diff view opens immediately on plugin/shortcut launch and fills progressively as tokens arrive. Esc cancels mid-stream and discards partial output.

**Architecture:** Build a shared `streamChatCompletion(ctx, ...)` helper that handles SSE parsing and a Content-Type-based blocking fallback. Both `Plugin.Run` (now returning `<-chan string, <-chan error`) and `runShortcutCmd` are thin wrappers around it. Bubble Tea consumes chunks via a recursive `tea.Cmd` that emits `pluginChunkMsg` / `pluginDoneMsg` / `pluginErrMsg`, and a stored `context.CancelFunc` on the model lets Esc tear down the in-flight HTTP request.

**Tech Stack:** Go, Bubble Tea, `net/http`, `bufio.Scanner`, `httptest` for testing.

**Spec:** [`docs/superpowers/specs/2026-04-25-streaming-ai-responses-design.md`](../specs/2026-04-25-streaming-ai-responses-design.md)

---

## File Map

| File | Status | Responsibility |
|---|---|---|
| `plugin_stream.go` | **NEW** | `streamChatCompletion` — SSE parser, blocking fallback, ctx cancellation. Owns `truncate` (moved from `plugin_blackbox.go`). |
| `plugin_stream_test.go` | **NEW** | Direct SSE-parser tests via `httptest.NewServer`. |
| `plugin.go` | MODIFY | New `Plugin.Run` signature; new `pluginChunkMsg` / `pluginDoneMsg` / `pluginErrMsg`; `streamPluginCmd` + `readNextChunk`; remove `pluginResultMsg` and `runPluginCmd`. |
| `plugin_openrouter.go` | MODIFY | `Run` becomes thin wrapper around `streamChatCompletion`; delete `callOpenRouter`. |
| `plugin_blackbox.go` | MODIFY | `Run` becomes thin wrapper around `streamChatCompletion`; delete `callBlackbox`; `truncate` moved to `plugin_stream.go`. |
| `shortcuts.go` | MODIFY | `runShortcutCmd` returns `(<-chan string, <-chan error)`; remove `shortcutResultMsg`. |
| `plugin_input.go` | MODIFY | At stream start in `handlePluginPrompt` and `handlePluginConfig`: create ctx, store cancel, build empty diff viewports, set `inputMode = inputPluginDiff`, store `activeChunks`, return `streamPluginCmd`. |
| `shortcuts_input.go` | MODIFY | Same stream-start treatment in `handleShortcutSelect`. |
| `model.go` | MODIFY | Add `pluginCancel` and `activeChunks` fields; rewrite key gating in `tea.KeyMsg` branch to allow Esc/Ctrl+Q during streaming; replace `pluginResultMsg`/`shortcutResultMsg` handlers with `pluginChunkMsg`/`pluginDoneMsg`/`pluginErrMsg`. |
| `plugin_openrouter_test.go` | MODIFY | Rewrite tests against streaming `Run`; one happy-path smoke test. |
| `plugin_blackbox_test.go` | MODIFY | Rewrite tests against streaming `Run`; one happy-path smoke test. |
| `plugin_selection_test.go` | MODIFY | Update `fakePlugin.Run` to the new channel signature so the file still compiles against the new `Plugin` interface. |

---

## Phase 1 — SSE Primitive (TDD, isolated)

The SSE primitive lives in a new file with no callers yet. Phase 1 builds it test-first, one feature at a time. Nothing else in the codebase compiles against it until Phase 2.

### Task 1: Skeleton — empty function and first test compiling

**Files:**
- Create: `plugin_stream.go`
- Create: `plugin_stream_test.go`

- [ ] **Step 1: Create `plugin_stream.go` with a stub that compiles**

```go
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
```

- [ ] **Step 2: Create `plugin_stream_test.go` with a drainer helper and the happy-path test (failing)**

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// drainStream collects all chunks until both channels close or a timeout
// elapses. Returns the concatenated content and any error produced.
func drainStream(t *testing.T, chunks <-chan string, errs <-chan error) (string, error) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	var sb strings.Builder
	var streamErr error
	chunksOpen, errsOpen := true, true
	for chunksOpen || errsOpen {
		select {
		case d, ok := <-chunks:
			if !ok {
				chunksOpen = false
				chunks = nil
				continue
			}
			sb.WriteString(d)
		case e, ok := <-errs:
			if !ok {
				errsOpen = false
				errs = nil
				continue
			}
			if e != nil {
				streamErr = e
			}
		case <-deadline:
			t.Fatal("drainStream: timed out")
		}
	}
	return sb.String(), streamErr
}

// sseServer returns an httptest.Server that writes the given frames as
// text/event-stream and flushes after each. If the test wants to assert on
// request shape (auth, model, etc.) it should use an inline handler instead.
func sseServer(t *testing.T, frames []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("sseServer: ResponseWriter is not a Flusher")
		}
		for _, f := range frames {
			fmt.Fprint(w, f)
			flusher.Flush()
		}
	}))
}

func TestStreamChatCompletion_MultiChunk(t *testing.T) {
	server := sseServer(t, []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n",
		"data: [DONE]\n\n",
	})
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "k", "m", "sys", "user")
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "Hello world" {
		t.Errorf("got %q, want %q", got, "Hello world")
	}
}
```

- [ ] **Step 3: Run the test — it should fail because the stub never produces chunks**

Run: `go test ./... -run TestStreamChatCompletion_MultiChunk -v`
Expected: FAIL — `got "", want "Hello world"`.

- [ ] **Step 4: Commit**

```bash
git add plugin_stream.go plugin_stream_test.go
git commit -m "$(cat <<'EOF'
test(stream): scaffold streamChatCompletion with failing happy-path test

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Implement happy-path SSE parsing

**Files:**
- Modify: `plugin_stream.go`

- [ ] **Step 1: Replace the stub body with the real SSE implementation**

Replace the entire body of `streamChatCompletion` with:

```go
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
```

- [ ] **Step 2: Run the test**

Run: `go test ./... -run TestStreamChatCompletion_MultiChunk -v`
Expected: PASS.

- [ ] **Step 3: Run all tests to confirm nothing else broke**

Run: `go build ./... && go test ./...`

Expected: build succeeds. Old provider tests **may now fail to compile** because we have not yet moved `truncate` deletion in `plugin_blackbox.go`. Verify the failure (if any) is *only* in `plugin_blackbox.go` due to a duplicate `truncate` definition. If so, **delete the duplicate `truncate` function from `plugin_blackbox.go` now** (we're going to delete `callBlackbox` in Phase 2 anyway, but `truncate` removal is required for the build to pass).

Re-run `go build ./... && go test ./...` and confirm green.

- [ ] **Step 4: Commit**

```bash
git add plugin_stream.go plugin_blackbox.go
git commit -m "$(cat <<'EOF'
feat(stream): SSE chat-completion parser with multi-chunk support

Move truncate() helper to plugin_stream.go; the SSE primitive owns it now.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Malformed-frame tolerance

**Files:**
- Modify: `plugin_stream_test.go`

- [ ] **Step 1: Add the failing test**

Append to `plugin_stream_test.go`:

```go
func TestStreamChatCompletion_MalformedFrameSkipped(t *testing.T) {
	server := sseServer(t, []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n",
		"data: {not valid json}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"second\"}}]}\n\n",
		"data: [DONE]\n\n",
	})
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "k", "m", "sys", "user")
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "firstsecond" {
		t.Errorf("got %q, want %q", got, "firstsecond")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./... -run TestStreamChatCompletion_MalformedFrameSkipped -v`
Expected: PASS — the implementation already calls `continue` on `json.Unmarshal` errors. This test guards regression.

- [ ] **Step 3: Commit**

```bash
git add plugin_stream_test.go
git commit -m "$(cat <<'EOF'
test(stream): malformed SSE frames are skipped silently

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Comment / keep-alive line skipping

**Files:**
- Modify: `plugin_stream_test.go`

- [ ] **Step 1: Add the test**

Append to `plugin_stream_test.go`:

```go
func TestStreamChatCompletion_KeepAliveSkipped(t *testing.T) {
	server := sseServer(t, []string{
		": keep-alive\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n",
		": ping\n\n",
		"\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\n",
		"data: [DONE]\n\n",
	})
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "k", "m", "sys", "user")
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "ab" {
		t.Errorf("got %q, want %q", got, "ab")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./... -run TestStreamChatCompletion_KeepAliveSkipped -v`
Expected: PASS — implementation already skips empty lines and lines starting with `:`.

- [ ] **Step 3: Commit**

```bash
git add plugin_stream_test.go
git commit -m "$(cat <<'EOF'
test(stream): comments and blank lines between SSE frames are skipped

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Non-streaming fallback

**Files:**
- Modify: `plugin_stream_test.go`

- [ ] **Step 1: Add the test**

Append to `plugin_stream_test.go`:

```go
func TestStreamChatCompletion_FallbackToBlocking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"full response"}}]}`)
	}))
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "k", "m", "sys", "user")
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "full response" {
		t.Errorf("got %q, want %q", got, "full response")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./... -run TestStreamChatCompletion_FallbackToBlocking -v`
Expected: PASS — `emitBlocking` is already wired.

- [ ] **Step 3: Commit**

```bash
git add plugin_stream_test.go
git commit -m "$(cat <<'EOF'
test(stream): non-SSE Content-Type falls back to blocking parse

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Non-2xx error surfacing

**Files:**
- Modify: `plugin_stream_test.go`

- [ ] **Step 1: Add the test**

Append to `plugin_stream_test.go`:

```go
func TestStreamChatCompletion_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid api key"}`)
	}))
	defer server.Close()

	chunks, errs := streamChatCompletion(context.Background(), server.URL, "bad-key", "m", "sys", "user")
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
```

- [ ] **Step 2: Run the test**

Run: `go test ./... -run TestStreamChatCompletion_AuthError -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add plugin_stream_test.go
git commit -m "$(cat <<'EOF'
test(stream): non-2xx response surfaces as error with HTTP code

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Cancel mid-stream

**Files:**
- Modify: `plugin_stream_test.go`

- [ ] **Step 1: Add the test**

Append to `plugin_stream_test.go`:

```go
func TestStreamChatCompletion_CancelMidStream(t *testing.T) {
	// Server flushes one chunk, then blocks until the client disconnects.
	requestDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(requestDone)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		flusher.Flush()
		// Block until client cancels (r.Context() fires)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	chunks, errs := streamChatCompletion(ctx, server.URL, "k", "m", "sys", "user")

	// Read the first chunk to confirm streaming started
	select {
	case d := <-chunks:
		if d != "first" {
			t.Fatalf("first chunk = %q, want %q", d, "first")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive first chunk")
	}

	// Cancel and assert both channels close (or yield ctx.Canceled) promptly
	cancel()

	deadline := time.After(2 * time.Second)
	chunksClosed, errsClosed := false, false
	for !chunksClosed || !errsClosed {
		select {
		case _, ok := <-chunks:
			if !ok {
				chunks = nil
				chunksClosed = true
			}
		case _, ok := <-errs:
			if !ok {
				errs = nil
				errsClosed = true
			}
		case <-deadline:
			t.Fatal("channels did not close after cancel")
		}
	}

	// Server-side handler must observe the disconnect
	select {
	case <-requestDone:
	case <-time.After(2 * time.Second):
		t.Fatal("server-side request did not terminate after client cancel")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./... -run TestStreamChatCompletion_CancelMidStream -v`
Expected: PASS — `http.NewRequestWithContext` + `select { case chunks <- ... : case <-ctx.Done(): return }` already handle this.

- [ ] **Step 3: Run the full test file**

Run: `go test ./... -run TestStreamChatCompletion -v`
Expected: All six `TestStreamChatCompletion_*` tests pass.

- [ ] **Step 4: Commit**

```bash
git add plugin_stream_test.go
git commit -m "$(cat <<'EOF'
test(stream): cancel via ctx tears down HTTP request and closes channels

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 2 — Migration to Streaming Everywhere

Phase 2 changes the `Plugin.Run` signature, which cascades through providers, both runners, all messages, and all four call sites. Go won't compile partial states, so this phase is **one commit** at the end. Sub-steps walk through it in dependency order; verify with `go build` and `go test` at the end before committing.

### Task 8: Big-bang migration

**Files:**
- Modify: `plugin.go`
- Modify: `plugin_openrouter.go`
- Modify: `plugin_blackbox.go`
- Modify: `shortcuts.go`
- Modify: `plugin_input.go`
- Modify: `shortcuts_input.go`
- Modify: `model.go`
- Modify: `plugin_openrouter_test.go`
- Modify: `plugin_blackbox_test.go`

- [ ] **Step 1: Update `plugin.go`**

Replace the entire file with:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	toml "github.com/pelletier/go-toml/v2"
)

type ConfigField struct {
	Key         string
	Label       string
	Placeholder string
	Secret      bool
}

type Plugin interface {
	Name() string
	Description() string
	ConfigFields() []ConfigField
	Run(ctx context.Context, content, prompt string, cfg map[string]string) (<-chan string, <-chan error)
}

// pluginChunkMsg carries one streamed delta plus the channels needed to
// re-queue the read. The chunks field also serves as the identity token used
// to discard messages from a superseded stream.
type pluginChunkMsg struct {
	chunks <-chan string
	errs   <-chan error
	delta  string
}

// pluginDoneMsg fires when the chunks channel closes cleanly.
type pluginDoneMsg struct {
	chunks <-chan string
}

// pluginErrMsg fires when the errs channel yields a non-nil error.
type pluginErrMsg struct {
	chunks <-chan string
	err    error
}

// streamPluginCmd is the entry point: it returns the first read.
func streamPluginCmd(chunks <-chan string, errs <-chan error) tea.Cmd {
	return readNextChunk(chunks, errs)
}

// readNextChunk reads one value from chunks (or an error from errs) and
// returns a tea.Msg. The model handler re-invokes readNextChunk to keep
// draining the stream.
func readNextChunk(chunks <-chan string, errs <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case d, ok := <-chunks:
			if !ok {
				return pluginDoneMsg{chunks: chunks}
			}
			return pluginChunkMsg{chunks: chunks, errs: errs, delta: d}
		case err := <-errs:
			if err != nil {
				return pluginErrMsg{chunks: chunks, err: err}
			}
			return pluginDoneMsg{chunks: chunks}
		}
	}
}

func pluginConfigPath(name string) string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "plugins", name+".toml")
}

func loadPluginConfig(name string) (map[string]string, error) {
	data, err := os.ReadFile(pluginConfigPath(name))
	if err != nil {
		return nil, fmt.Errorf("reading plugin config: %w", err)
	}
	var values map[string]string
	if err := toml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parsing plugin config: %w", err)
	}
	return values, nil
}

func savePluginConfig(name string, values map[string]string) error {
	path := pluginConfigPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating plugin config dir: %w", err)
	}
	data, err := toml.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshaling plugin config: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

func pluginConfigComplete(fields []ConfigField, values map[string]string) bool {
	for _, f := range fields {
		if values[f.Key] == "" {
			return false
		}
	}
	return true
}
```

Note: `pluginResultMsg` and `runPluginCmd` are gone.

- [ ] **Step 2: Update `plugin_openrouter.go`**

Replace the entire file with:

```go
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
```

`callOpenRouter` is deleted.

- [ ] **Step 3: Update `plugin_blackbox.go`**

Replace the entire file with:

```go
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
```

`callBlackbox` and (if not already removed in Task 2 Step 3) `truncate` are deleted.

- [ ] **Step 4: Update `shortcuts.go`**

Replace the `runShortcutCmd` function and remove `shortcutResultMsg`. The full file becomes:

```go
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

//go:embed defaults/ai_shortcuts.toml
var defaultShortcutsTOML []byte

type AIShortcut struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Prompt      string `toml:"prompt"`
}

type aiShortcutsConfig struct {
	Shortcuts []AIShortcut `toml:"shortcuts"`
}

func shortcutsPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "ai_shortcuts.toml")
}

func loadShortcuts() ([]AIShortcut, error) {
	path := shortcutsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("creating shortcuts dir: %w", err)
			}
			if err := os.WriteFile(path, defaultShortcutsTOML, 0o644); err != nil {
				return nil, fmt.Errorf("seeding shortcuts: %w", err)
			}
			data = defaultShortcutsTOML
		} else {
			return nil, fmt.Errorf("reading shortcuts: %w", err)
		}
	}
	var cfg aiShortcutsConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing shortcuts: %w", err)
	}
	return cfg.Shortcuts, nil
}

func saveShortcuts(shortcuts []AIShortcut) error {
	path := shortcutsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating shortcuts dir: %w", err)
	}
	cfg := aiShortcutsConfig{Shortcuts: shortcuts}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling shortcuts: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// runShortcutStream issues a streaming chat-completion using the shortcut's
// prompt as the user instruction. Returns channels that mirror Plugin.Run.
func runShortcutStream(ctx context.Context, shortcut AIShortcut, content, provider string, config map[string]string) (<-chan string, <-chan error) {
	systemPrompt := "You are a text processing assistant. Apply the following instruction to the provided text. Return ONLY the processed text, nothing else."
	userMessage := fmt.Sprintf("Instruction: %s\n\nText:\n%s", shortcut.Prompt, content)
	var url string
	switch provider {
	case "openrouter":
		url = defaultOpenRouterURL
	default:
		url = defaultBlackboxURL
	}
	return streamChatCompletion(ctx, url, config["api_key"], config["model"], systemPrompt, userMessage)
}
```

The function is renamed `runShortcutCmd` → `runShortcutStream` to reflect that it no longer returns a `tea.Cmd`. `shortcutResultMsg` is gone.

- [ ] **Step 5: Update `model.go` — add fields, key gating, message handlers**

Apply three edits to `model.go`.

**5a. Add the new fields** to the `model` struct (around the existing plugin block at line ~125):

Before:
```go
	pluginDiffViewL    viewport.Model
	pluginDiffViewR    viewport.Model
	pluginProcessing   bool
```

After:
```go
	pluginDiffViewL    viewport.Model
	pluginDiffViewR    viewport.Model
	pluginProcessing   bool
	pluginCancel       context.CancelFunc
	activeChunks       <-chan string
```

Also add `"context"` to the imports at the top of the file (it isn't there yet).

**5b. Replace the key gating** at lines 385–388. Before:
```go
	case tea.KeyMsg:
		if m.pluginProcessing {
			return m, nil
		}
```

After:
```go
	case tea.KeyMsg:
		if m.pluginProcessing {
			switch msg.String() {
			case "esc":
				if m.pluginCancel != nil {
					m.pluginCancel()
					m.pluginCancel = nil
				}
				m.pluginProcessing = false
				m.activeChunks = nil
				m.inputMode = inputNone
				m.pluginActive = nil
				m.pluginDiffOriginal = ""
				m.pluginDiffResult = ""
				return m, nil
			case "ctrl+q":
				if m.isDirty() {
					m.inputMode = inputUnsavedGuard
					m.pendingAction = pendingQuit
					return m, nil
				}
				return m, tea.Quit
			}
			return m, nil
		}
```

**5c. Replace the `pluginResultMsg` and `shortcutResultMsg` cases** (lines 331–371). Delete both `case` blocks and replace with:

```go
	case pluginChunkMsg:
		if msg.chunks != m.activeChunks {
			return m, nil // stale: superseded or cancelled stream
		}
		m.pluginDiffResult += msg.delta
		// Right-pane width matches the calculation in newDiffViewports.
		halfWidth := m.editorWidth / 2
		rightWidth := m.editorWidth - halfWidth - 3
		if rightWidth < 1 {
			rightWidth = 1
		}
		m.pluginDiffViewR.SetContent(wordWrap(m.pluginDiffResult, rightWidth))
		m.pluginDiffViewR.GotoBottom()
		return m, readNextChunk(msg.chunks, msg.errs)

	case pluginDoneMsg:
		if msg.chunks != m.activeChunks {
			return m, nil // stale
		}
		m.pluginProcessing = false
		m.pluginCancel = nil
		m.activeChunks = nil
		if m.pluginDiffResult == m.pluginDiffOriginal || m.pluginDiffResult == "" {
			m.errMsg = "No changes"
			m.inputMode = inputNone
			m.pluginActive = nil
			m.pluginDiffOriginal = ""
			m.pluginDiffResult = ""
		}
		return m, nil

	case pluginErrMsg:
		if msg.chunks != m.activeChunks {
			return m, nil // stale
		}
		m.pluginProcessing = false
		m.pluginCancel = nil
		m.activeChunks = nil
		m.errMsg = "Plugin error: " + msg.err.Error()
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		return m, nil
```

- [ ] **Step 6: Update `plugin_input.go` — handlePluginPrompt and handlePluginConfig**

**6a. `handlePluginPrompt` (around line 121).** Before:
```go
		content, onSelection := m.aiInputContent()
		m.aiRunOnSelection = onSelection
		m.pluginDiffOriginal = content
		m.pluginProcessing = true
		m.inputMode = inputNone
		return m, runPluginCmd(m.pluginActive, content, prompt, cfg)
```

After:
```go
		content, onSelection := m.aiInputContent()
		m.aiRunOnSelection = onSelection
		m.pluginDiffOriginal = content
		m.pluginDiffResult = ""
		m.pluginProcessing = true
		ctx, cancel := context.WithCancel(context.Background())
		m.pluginCancel = cancel
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(content, "", m.editorWidth, m.editorHeight)
		m.inputMode = inputPluginDiff
		chunks, errs := m.pluginActive.Run(ctx, content, prompt, cfg)
		m.activeChunks = chunks
		return m, streamPluginCmd(chunks, errs)
```

**6b. `handlePluginConfig` shortcut-pending branch (around line 78).** Before:
```go
			if m.shortcutPending {
				m.shortcutPending = false
				shortcut := m.shortcuts[m.shortcutCursor]
				provider := m.pluginActive.Name()
				cfg, _ := loadPluginConfig(provider)
				content, onSelection := m.aiInputContent()
				m.aiRunOnSelection = onSelection
				m.pluginDiffOriginal = content
				m.pluginProcessing = true
				m.inputMode = inputNone
				return m, runShortcutCmd(shortcut, content, provider, cfg)
			}
```

After:
```go
			if m.shortcutPending {
				m.shortcutPending = false
				shortcut := m.shortcuts[m.shortcutCursor]
				provider := m.pluginActive.Name()
				cfg, _ := loadPluginConfig(provider)
				content, onSelection := m.aiInputContent()
				m.aiRunOnSelection = onSelection
				m.pluginDiffOriginal = content
				m.pluginDiffResult = ""
				m.pluginProcessing = true
				ctx, cancel := context.WithCancel(context.Background())
				m.pluginCancel = cancel
				m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(content, "", m.editorWidth, m.editorHeight)
				m.inputMode = inputPluginDiff
				chunks, errs := runShortcutStream(ctx, shortcut, content, provider, cfg)
				m.activeChunks = chunks
				return m, streamPluginCmd(chunks, errs)
			}
```

Add `"context"` to the imports of `plugin_input.go` (it isn't there yet).

- [ ] **Step 7: Update `shortcuts_input.go` — handleShortcutSelect**

Around line 66. Before:
```go
		content, onSelection := m.aiInputContent()
		m.aiRunOnSelection = onSelection
		m.pluginDiffOriginal = content
		m.pluginProcessing = true
		m.inputMode = inputNone
		return m, runShortcutCmd(shortcut, content, provider, cfg)
```

After:
```go
		content, onSelection := m.aiInputContent()
		m.aiRunOnSelection = onSelection
		m.pluginDiffOriginal = content
		m.pluginDiffResult = ""
		m.pluginProcessing = true
		ctx, cancel := context.WithCancel(context.Background())
		m.pluginCancel = cancel
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(content, "", m.editorWidth, m.editorHeight)
		m.inputMode = inputPluginDiff
		chunks, errs := runShortcutStream(ctx, shortcut, content, provider, cfg)
		m.activeChunks = chunks
		return m, streamPluginCmd(chunks, errs)
```

Add `"context"` to the imports of `shortcuts_input.go`.

- [ ] **Step 8: Rewrite `plugin_openrouter_test.go`**

Replace the entire file with:

```go
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

func TestOpenRouterPlugin_Name(t *testing.T) {
	p := &OpenRouterPlugin{}
	if p.Name() != "openrouter" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openrouter")
	}
}

func TestOpenRouterPlugin_ConfigFields(t *testing.T) {
	p := &OpenRouterPlugin{}
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

func TestOpenRouterPlugin_Run_StreamingSmoke(t *testing.T) {
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
		if req["model"] != "openai/gpt-4o" {
			t.Errorf("model = %v, want openai/gpt-4o", req["model"])
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

	p := &OpenRouterPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "test-key", "model": "openai/gpt-4o"}
	chunks, errs := p.Run(context.Background(), "Original", "Translate", cfg)
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "Translated" {
		t.Errorf("got %q, want %q", got, "Translated")
	}
}

func TestOpenRouterPlugin_Run_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid api key"}`)
	}))
	defer server.Close()

	p := &OpenRouterPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "bad", "model": "openai/gpt-4o"}
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
```

- [ ] **Step 9: Rewrite `plugin_blackbox_test.go`**

Replace the entire file with:

```go
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

func TestBlackboxPlugin_Name(t *testing.T) {
	p := &BlackboxPlugin{}
	if p.Name() != "blackbox" {
		t.Errorf("Name() = %q, want %q", p.Name(), "blackbox")
	}
}

func TestBlackboxPlugin_ConfigFields(t *testing.T) {
	p := &BlackboxPlugin{}
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

func TestBlackboxPlugin_Run_StreamingSmoke(t *testing.T) {
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
		if req["model"] != "blackboxai/minimax/minimax-m2.5" {
			t.Errorf("model = %v, want blackboxai/minimax/minimax-m2.5", req["model"])
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

	p := &BlackboxPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "test-key", "model": "blackboxai/minimax/minimax-m2.5"}
	chunks, errs := p.Run(context.Background(), "Original", "Translate", cfg)
	got, err := drainStream(t, chunks, errs)
	if err != nil {
		t.Fatalf("drainStream error: %v", err)
	}
	if got != "Translated" {
		t.Errorf("got %q, want %q", got, "Translated")
	}
}

func TestBlackboxPlugin_Run_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid api key"}`)
	}))
	defer server.Close()

	p := &BlackboxPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "bad", "model": "blackboxai/minimax/minimax-m2.5"}
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
```

- [ ] **Step 10: Update `plugin_selection_test.go` — fakePlugin signature**

`plugin_selection_test.go` defines a `fakePlugin` that implements the old `Run(content, prompt, config) (string, error)` signature. After the interface change it no longer satisfies `Plugin` and the test file won't compile.

**10a.** Add `"context"` to the imports of `plugin_selection_test.go`. The current import block is:

```go
import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)
```

Make it:

```go
import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)
```

**10b.** Replace the `Run` method on `fakePlugin` (lines 52–54). Before:

```go
func (f *fakePlugin) Run(content, prompt string, config map[string]string) (string, error) {
	return "result", nil
}
```

After:

```go
func (f *fakePlugin) Run(ctx context.Context, content, prompt string, config map[string]string) (<-chan string, <-chan error) {
	chunks := make(chan string)
	errs := make(chan error)
	close(chunks)
	close(errs)
	return chunks, errs
}
```

The fake returns immediately-closed channels: tests that don't drain them are unaffected, and any drainer sees a clean `pluginDoneMsg`. `ctx` is unused — that's fine for a fake.

**10c.** Verify the existing tests still hold under the new flow. The tests `TestShortcutSelect_WithSelection_SendsOnlySelection`, `TestPluginPrompt_NoSelection_SendsWholeContent`, and `TestPluginPrompt_WithSelection_SendsOnlySelection` only assert on `nm.pluginDiffOriginal` and `nm.aiRunOnSelection`, both of which are still set at stream start in the new code. They should pass without further changes. If `newDiffViewports` fails on a zero-sized model, set `m.editorWidth = 80; m.editorHeight = 10` immediately after `newTestModel(t)` in any test that drives the stream-start path. (`setEditorSize` covers the editor but not these stream-start fields.)

- [ ] **Step 11: Build and run the full test suite**

Run: `go build ./...`
Expected: build succeeds, no compile errors.

Run: `go test ./...`
Expected: all tests pass.

If any test fails or build breaks, fix in place before committing. Most likely culprits:
- Missing `"context"` import in `model.go`, `plugin_input.go`, `shortcuts_input.go`, or `plugin_selection_test.go`.
- Stray reference to `runShortcutCmd`, `runPluginCmd`, `pluginResultMsg`, `shortcutResultMsg`, `callOpenRouter`, or `callBlackbox` that I missed. Run `grep -rn "runPluginCmd\|runShortcutCmd\|pluginResultMsg\|shortcutResultMsg\|callOpenRouter\|callBlackbox" --include="*.go" .` and fix any hits.
- A test that drove `handlePluginPrompt` / `handleShortcutSelect` panics on `newDiffViewports` if `editorWidth` is 0; set the field explicitly in the test setup as noted in 10c.

- [ ] **Step 12: Commit**

```bash
git add plugin.go plugin_openrouter.go plugin_blackbox.go shortcuts.go plugin_input.go shortcuts_input.go model.go plugin_openrouter_test.go plugin_blackbox_test.go plugin_selection_test.go
git commit -m "$(cat <<'EOF'
feat: stream AI responses into the diff view

Plugin.Run now returns (<-chan string, <-chan error). Both providers and
runShortcutStream are thin wrappers around the shared streamChatCompletion
SSE primitive. The diff view opens immediately on launch and the right pane
fills as deltas arrive. Esc cancels mid-stream via context.CancelFunc.

Stale messages from cancelled streams are discarded by comparing the
chunks-channel identity against model.activeChunks.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 3 — Manual Verification

Streaming behavior is hard to unit-test through Bubble Tea. After Phase 2 lands, run the binary and verify the full happy path and the cancel path against a real provider.

### Task 9: Manual smoke test

**Files:** none (binary check)

- [ ] **Step 1: Build and run**

```bash
go build -o clipad . && ./clipad
```

- [ ] **Step 2: Verify happy path**

1. Open any note in your vault (or create one with `Ctrl+N`).
2. Press `Ctrl+G` to open the AI shortcut selector. Pick any shortcut.
3. **Expected:** the diff view opens immediately. Original content on the left. Right pane starts empty and fills with tokens as they arrive. The view auto-scrolls so the latest text is always visible.
4. When the stream completes, the status bar shows `Accept changes? (y/n)`.
5. Press `n` to discard.

- [ ] **Step 3: Verify cancellation**

1. Press `Ctrl+G` again, pick a shortcut.
2. As soon as the diff opens and tokens start arriving, press `Esc`.
3. **Expected:** the diff view closes immediately, no error in the status bar, the editor is back in focus, no partial content was applied.

- [ ] **Step 4: Verify plugin-prompt path**

1. Press `Ctrl+@` (or whatever opens the plugin selector — see `model.go:492`) to open the plugin selector. Pick a plugin.
2. Enter a prompt and press Enter.
3. **Expected:** same streaming and cancellation behavior as the shortcut path.

- [ ] **Step 5: Verify error surfacing**

1. Edit `~/.config/clipad/plugins/<provider>.toml` and set the api_key to an obviously invalid value.
2. Restart `clipad`, run a shortcut.
3. **Expected:** within a second or two, the diff closes (or never opens past the empty state) and the status bar shows `Plugin error: API error (HTTP 401): ...`.
4. Restore the real api_key.

If any check fails, file an issue against the implementation; do not mark this task complete.

- [ ] **Step 6: Commit nothing**

This task produces no code. If verification surfaced bugs, fix them in follow-up commits.

---

## Self-Review

**Spec coverage:**
- SSE streaming for both providers — Tasks 2, 8 (Steps 2–3).
- `stream: true` flag — Task 2 (Step 1) and asserted in Task 8 Steps 8–9.
- Diff view opens immediately — Task 8 Steps 6, 7.
- Esc cancels via `context.CancelFunc` — Task 8 Step 5b.
- HTTP teardown on cancel — Task 7 (test) and Task 2 (`http.NewRequestWithContext`).
- Partial output discarded on cancel — Task 8 Step 5b clears `pluginDiffResult`.
- Both providers stream — Tasks 8 Steps 2–3, 8–9.
- Non-streaming fallback — Tasks 2 (impl) and 5 (test).
- SSE parsing rules: line-by-line, `data: {json}` frames, skip comments and blanks, stop on `[DONE]` — Task 2 implementation; Tasks 3, 4 tests.
- Malformed frame tolerance — Task 3.
- Plugin.Run channel signature — Task 8 Step 1.
- Bubble Tea pattern: goroutine + channel + recursive `tea.Cmd` + chunk/done/err messages — Task 8 Step 1.
- `http.NewRequestWithContext` — Task 2.
- Test coverage: multi-chunk happy path (Task 2), cancel mid-stream (Task 7), non-streaming fallback (Task 5), malformed frame tolerance (Task 3), comment/keep-alive skipping (Task 4), non-2xx error (Task 6), provider smoke tests (Task 8 Steps 8–9).
- Stale-message guard via channel identity — Task 8 Steps 5a, 5c.
- Manual verification of the full UX — Task 9.

**Placeholder scan:** no "TBD" / "TODO" / vague-handling / "similar to Task N" / referenced-but-undefined symbols remain.

**Type consistency:**
- `Plugin.Run` signature: `(ctx context.Context, content, prompt string, cfg map[string]string) (<-chan string, <-chan error)` — used identically in `plugin.go`, `plugin_openrouter.go`, `plugin_blackbox.go`, and the provider tests.
- `runShortcutStream` signature matches across `shortcuts.go`, `plugin_input.go`, `shortcuts_input.go`.
- `pluginChunkMsg`/`pluginDoneMsg`/`pluginErrMsg` field names (`chunks`, `errs`, `delta`, `err`) match between `plugin.go` definitions and `model.go` handlers.
- `m.activeChunks` referenced consistently in stream-start sites and message handlers.
- `m.pluginCancel` referenced consistently in stream-start sites, Esc handler, and done/err handlers.

---

## Execution Notes

- Phase 1 has 7 tasks, each producing a small commit. Phase 2 is one large task with 12 sub-steps that all land in one commit. Phase 3 is manual verification.
- If any step in Phase 2 reveals an unexpected dependency, fix it as part of Step 11 (build/test) before the Step 12 commit.
- After Task 9 manual verification passes, this plan is done.
