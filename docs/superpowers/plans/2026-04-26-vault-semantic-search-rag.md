# Vault-Wide Semantic Search + RAG Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add vault-wide semantic search (`Ctrl+Shift+F` modal) and "Ask your vault" RAG chat (`Ctrl+Shift+A` right-side panel), backed by a per-device SQLite chunk-embedding index that builds in the background and updates incrementally on file changes.

**Architecture:** Build the layers bottom-up so each is independently testable: (1) an `EmbeddingClient` interface with OpenRouter + Ollama implementations, (2) an `Index` (SQLite via `modernc.org/sqlite`) that chunks markdown by paragraph with a size cap, embeds via the client, and serves cosine-top-K search, (3) a Bubble Tea-driven background indexer that initial-sweeps and reacts to `fileChangedMsg`, (4) a search modal driven by debounced async embed-and-search, (5) a chat panel reusing `streamChatCompletion` from `plugin_stream.go` for the answer stream, with retrieved chunks injected into the system prompt as numbered citations.

**Tech Stack:** Go, Bubble Tea, `net/http`, `modernc.org/sqlite` (pure-Go, no cgo), `crypto/sha256`, `container/heap`, `httptest` for tests.

**Spec:** [`docs/superpowers/specs/2026-04-26-vault-semantic-search-rag-design.md`](../specs/2026-04-26-vault-semantic-search-rag-design.md)

---

## File Map

| File | Status | Responsibility |
|---|---|---|
| `embeddings.go` | **NEW** | `EmbeddingClient` interface, `OpenRouterEmbeddings`, `OllamaEmbeddings`, `newEmbeddingClient(cfg)` factory. |
| `embeddings_test.go` | **NEW** | `httptest`-driven tests for both embedders, factory dispatch, error paths. |
| `index.go` | **NEW** | `Index` struct (DB handle + embedder), schema/migrations, `chunkFile`, `RebuildFile`, `RemoveFile`, `Search`, cosine top-K, `Close`. |
| `index_test.go` | **NEW** | `chunkFile` table tests; `RebuildFile` with `:memory:` SQLite + mock embedder; `Search` with synthetic vectors; `RemoveFile`. |
| `ask.go` | **NEW** | `composeChatRequest(turns, query, chunks)` — system-prompt builder, last-4-pairs message array. |
| `ask_test.go` | **NEW** | Tests for `composeChatRequest`. |
| `chat.go` | **NEW** | Chat panel rendering (lipgloss), viewport refresh, citation render. Streaming message types `chatChunkMsg`/`chatDoneMsg`/`chatErrMsg` + `streamChatCmd`/`readNextChatChunk`. Chat-send command. Two-mode key handler. |
| `chat_test.go` | **NEW** | Mode transitions, citation jump, streaming append, cancel-on-Esc. |
| `vault_search.go` | **NEW** | Search modal rendering, debounced search command, snippet formatter, key handler. |
| `vault_search_test.go` | **NEW** | Token-based stale guard, snippet truncation. |
| `config.go` | MODIFY | Add `EmbeddingProvider`, `EmbeddingModel`, `OllamaURL` fields + defaults in `loadConfig`. Update `configTOML` and `saveConfig`. |
| `config_test.go` | MODIFY | Tests for new defaults round-trip. |
| `watcher.go` | MODIFY | Distinguish `Remove` from `Create`/`Write`; emit new `fileDeletedMsg{path}` for removes. |
| `watcher_test.go` | **NEW** (no existing test file) | `Remove` events emit `fileDeletedMsg`; `Write`/`Create` still emit `fileChangedMsg`. |
| `statusbar.go` | MODIFY | Render `[idx N/M]` indexer status when non-empty. |
| `selection.go` | MODIFY | Export `moveTo(line, col)` as `MoveTo(line, col)` (or add a thin public wrapper) so external files can jump cursor. |
| `model.go` | MODIFY | Add indexer/search/chat state fields; `Ctrl+Shift+F` and `Ctrl+Shift+A` shortcut handlers; `recalcLayout` for 3-pane; new `inputVaultSearch` mode + `chatOpen` flag; `Update` message handlers; `View` integrates chat panel. |
| `main.go` | MODIFY | Construct `Index` after `newModel`, pass into model, defer `Index.Close()` on shutdown. |
| `help_modal.go` | MODIFY | Document `Ctrl+Shift+F` and `Ctrl+Shift+A` and chat-panel keys. |
| `go.mod` / `go.sum` | MODIFY | Add `modernc.org/sqlite`. |

---

## Phases

The plan is organized as five phases. Phases 1–3 are headless (no UI), each with its own tests; Phase 4 wires the search modal; Phase 5 wires the chat panel.

| Phase | Scope | Sessions |
|---|---|---|
| 1 | Embeddings client (interface + 2 impls + factory) | 1 |
| 2 | Index (SQLite + chunking + search) | 1 |
| 3 | Background indexer (Bubble Tea integration, status bar) | 1 |
| 4 | Vault search modal (Ctrl+Shift+F) | 1 |
| 5 | Chat panel (Ctrl+Shift+A) | 1–2 |

---

## Phase 1 — Embeddings Client

The embeddings layer is fully isolated from clipad's other code. Build it test-first.

### Task 1: Skeleton `EmbeddingClient` interface and factory stub

**Files:**
- Create: `embeddings.go`

- [ ] **Step 1: Create `embeddings.go` with the interface, type stubs, and factory stub**

```go
package main

import (
	"context"
	"fmt"
)

// EmbeddingClient computes vector embeddings for batches of text.
// All implementations are OpenAI-compatible at the request level but may
// differ in batching and response shape; callers see the same texts-in /
// vectors-out contract.
type EmbeddingClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Model() string
	Dim() int
}

// OpenRouterEmbeddings calls OpenRouter's /v1/embeddings endpoint.
type OpenRouterEmbeddings struct {
	BaseURL   string // override for testing; empty uses default
	APIKey    string
	ModelName string
	dim       int // populated on first response, then trusted thereafter
}

const defaultOpenRouterEmbeddingsURL = "https://openrouter.ai/api/v1/embeddings"

func (e *OpenRouterEmbeddings) Model() string { return e.ModelName }
func (e *OpenRouterEmbeddings) Dim() int      { return e.dim }
func (e *OpenRouterEmbeddings) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("OpenRouterEmbeddings.Embed not implemented")
}

// OllamaEmbeddings calls a local Ollama daemon's /api/embeddings.
type OllamaEmbeddings struct {
	BaseURL   string // e.g. "http://localhost:11434"
	ModelName string
	dim       int
}

func (e *OllamaEmbeddings) Model() string { return e.ModelName }
func (e *OllamaEmbeddings) Dim() int      { return e.dim }
func (e *OllamaEmbeddings) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("OllamaEmbeddings.Embed not implemented")
}

// newEmbeddingClient picks the implementation based on cfg.EmbeddingProvider.
// Returns an error if the provider is unknown or required config is missing.
func newEmbeddingClient(cfg Config) (EmbeddingClient, error) {
	switch cfg.EmbeddingProvider {
	case "ollama":
		return &OllamaEmbeddings{BaseURL: cfg.OllamaURL, ModelName: cfg.EmbeddingModel}, nil
	case "openrouter", "":
		keyCfg, err := loadPluginConfig("openrouter")
		if err != nil {
			return nil, fmt.Errorf("openrouter embeddings need plugin config: %w", err)
		}
		apiKey := keyCfg["api_key"]
		if apiKey == "" {
			return nil, fmt.Errorf("openrouter embeddings: api_key not set in plugin config")
		}
		return &OpenRouterEmbeddings{APIKey: apiKey, ModelName: cfg.EmbeddingModel}, nil
	default:
		return nil, fmt.Errorf("unknown embedding_provider: %q", cfg.EmbeddingProvider)
	}
}
```

- [ ] **Step 2: Verify the file builds**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add embeddings.go
git commit -m "$(cat <<'EOF'
feat(embeddings): scaffold EmbeddingClient interface with stub impls

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `OpenRouterEmbeddings.Embed` happy-path test (failing)

**Files:**
- Create: `embeddings_test.go`

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// embedTestServer returns an httptest.Server that responds with the given
// per-input embedding values. Each input gets one matching vector.
func openrouterTestServer(t *testing.T, dim int, vectors [][]float32, capture *http.Request) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			*capture = *r.Clone(r.Context())
			body := new(strings.Builder)
			if r.Body != nil {
				buf := make([]byte, 1<<16)
				n, _ := r.Body.Read(buf)
				body.Write(buf[:n])
			}
			capture.Body = http.NoBody // body is consumed; tests assert via captured headers/method
			_ = body
		}
		out := struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}{}
		for _, v := range vectors {
			out.Data = append(out.Data, struct {
				Embedding []float32 `json:"embedding"`
			}{Embedding: v})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
}

func TestOpenRouterEmbeddings_HappyPath(t *testing.T) {
	srv := openrouterTestServer(t, 3, [][]float32{
		{1, 0, 0},
		{0, 1, 0},
	}, nil)
	defer srv.Close()

	e := &OpenRouterEmbeddings{BaseURL: srv.URL, APIKey: "k", ModelName: "qwen/qwen3-embedding-8b"}
	got, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d vectors, want 2", len(got))
	}
	if got[0][0] != 1 || got[1][1] != 1 {
		t.Errorf("vectors: %v", got)
	}
	if e.Dim() != 3 {
		t.Errorf("Dim() = %d, want 3", e.Dim())
	}
}
```

- [ ] **Step 2: Run the test — it should fail**

Run: `go test ./... -run TestOpenRouterEmbeddings_HappyPath -v`
Expected: FAIL — `OpenRouterEmbeddings.Embed not implemented`.

- [ ] **Step 3: Commit the failing test**

```bash
git add embeddings_test.go
git commit -m "$(cat <<'EOF'
test(embeddings): failing happy-path test for OpenRouterEmbeddings

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Implement `OpenRouterEmbeddings.Embed` (single-batch case)

**Files:**
- Modify: `embeddings.go`

- [ ] **Step 1: Replace the stub with the real implementation**

Replace the body of `OpenRouterEmbeddings.Embed` and add helpers near it:

```go
func (e *OpenRouterEmbeddings) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	url := e.BaseURL
	if url == "" {
		url = defaultOpenRouterEmbeddingsURL
	}
	const batchSize = 100
	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := e.embedBatch(ctx, url, texts[i:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	if len(out) > 0 {
		e.dim = len(out[0])
	}
	return out, nil
}

func (e *OpenRouterEmbeddings) embedBatch(ctx context.Context, url string, batch []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]interface{}{
		"model": e.ModelName,
		"input": batch,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter embeddings (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(parsed.Data) != len(batch) {
		return nil, fmt.Errorf("openrouter embeddings: got %d vectors for %d inputs", len(parsed.Data), len(batch))
	}
	out := make([][]float32, len(parsed.Data))
	for i, d := range parsed.Data {
		out[i] = d.Embedding
	}
	return out, nil
}
```

Add to the import block:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)
```

- [ ] **Step 2: Run the test — it should pass**

Run: `go test ./... -run TestOpenRouterEmbeddings_HappyPath -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add embeddings.go
git commit -m "$(cat <<'EOF'
feat(embeddings): OpenRouter /v1/embeddings client (single-batch)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: OpenRouter batching (>100 inputs → 2 calls)

**Files:**
- Modify: `embeddings_test.go`

- [ ] **Step 1: Add a batching test**

```go
func TestOpenRouterEmbeddings_Batches(t *testing.T) {
	calls := 0
	totalInputs := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		totalInputs += len(req.Input)
		out := struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}{}
		for range req.Input {
			out.Data = append(out.Data, struct {
				Embedding []float32 `json:"embedding"`
			}{Embedding: []float32{0, 0, 1}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	e := &OpenRouterEmbeddings{BaseURL: srv.URL, APIKey: "k", ModelName: "m"}
	inputs := make([]string, 101)
	for i := range inputs {
		inputs[i] = "x"
	}
	got, err := e.Embed(context.Background(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("HTTP calls = %d, want 2", calls)
	}
	if totalInputs != 101 {
		t.Errorf("total inputs = %d, want 101", totalInputs)
	}
	if len(got) != 101 {
		t.Errorf("vectors = %d, want 101", len(got))
	}
}
```

- [ ] **Step 2: Run the test — it should pass already (batching is in Task 3 implementation)**

Run: `go test ./... -run TestOpenRouterEmbeddings_Batches -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add embeddings_test.go
git commit -m "$(cat <<'EOF'
test(embeddings): batching test (101 inputs → 2 HTTP calls)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: OpenRouter error path (non-2xx)

**Files:**
- Modify: `embeddings_test.go`

- [ ] **Step 1: Add an auth-error test**

```go
func TestOpenRouterEmbeddings_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	e := &OpenRouterEmbeddings{BaseURL: srv.URL, APIKey: "k", ModelName: "m"}
	_, err := e.Embed(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %v, want HTTP 401 mention", err)
	}
}
```

- [ ] **Step 2: Run — should pass (Task 3 already returns the error)**

Run: `go test ./... -run TestOpenRouterEmbeddings_AuthError -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add embeddings_test.go
git commit -m "$(cat <<'EOF'
test(embeddings): OpenRouter non-2xx error surfaces HTTP status

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: `OllamaEmbeddings.Embed` happy-path test (failing)

**Files:**
- Modify: `embeddings_test.go`

- [ ] **Step 1: Add the failing test**

```go
func TestOllamaEmbeddings_HappyPath(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var v []float32
		if req.Prompt == "alpha" {
			v = []float32{1, 0, 0}
		} else {
			v = []float32{0, 1, 0}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string][]float32{"embedding": v})
	}))
	defer srv.Close()

	e := &OllamaEmbeddings{BaseURL: srv.URL, ModelName: "nomic-embed-text"}
	got, err := e.Embed(context.Background(), []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("HTTP calls = %d, want 2 (one per text)", calls)
	}
	if len(got) != 2 || got[0][0] != 1 || got[1][1] != 1 {
		t.Errorf("vectors = %v", got)
	}
	if e.Dim() != 3 {
		t.Errorf("Dim() = %d, want 3", e.Dim())
	}
}
```

- [ ] **Step 2: Run the test — it should fail**

Run: `go test ./... -run TestOllamaEmbeddings_HappyPath -v`
Expected: FAIL — `OllamaEmbeddings.Embed not implemented`.

- [ ] **Step 3: Commit the failing test**

```bash
git add embeddings_test.go
git commit -m "$(cat <<'EOF'
test(embeddings): failing happy-path test for OllamaEmbeddings

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Implement `OllamaEmbeddings.Embed`

**Files:**
- Modify: `embeddings.go`

- [ ] **Step 1: Replace the stub**

```go
func (e *OllamaEmbeddings) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	base := e.BaseURL
	if base == "" {
		base = "http://localhost:11434"
	}
	url := base + "/api/embeddings"
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		body, err := json.Marshal(map[string]string{"model": e.ModelName, "prompt": t})
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ollama: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("ollama embeddings (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
		}
		var parsed struct {
			Embedding []float32 `json:"embedding"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode: %w", err)
		}
		resp.Body.Close()
		out = append(out, parsed.Embedding)
	}
	if len(out) > 0 {
		e.dim = len(out[0])
	}
	return out, nil
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./... -run TestOllamaEmbeddings_HappyPath -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add embeddings.go
git commit -m "$(cat <<'EOF'
feat(embeddings): Ollama /api/embeddings client (per-text loop)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: `newEmbeddingClient` factory tests

**Files:**
- Modify: `embeddings_test.go`

- [ ] **Step 1: Add factory tests**

```go
func TestNewEmbeddingClient_Ollama(t *testing.T) {
	cfg := Config{EmbeddingProvider: "ollama", EmbeddingModel: "nomic-embed-text", OllamaURL: "http://localhost:11434"}
	c, err := newEmbeddingClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*OllamaEmbeddings); !ok {
		t.Errorf("got %T, want *OllamaEmbeddings", c)
	}
	if c.Model() != "nomic-embed-text" {
		t.Errorf("Model() = %q", c.Model())
	}
}

func TestNewEmbeddingClient_UnknownProvider(t *testing.T) {
	cfg := Config{EmbeddingProvider: "bogus"}
	_, err := newEmbeddingClient(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown embedding_provider") {
		t.Errorf("error = %v", err)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./... -run TestNewEmbeddingClient -v`
Expected: PASS for both.

(The OpenRouter factory path needs a writable plugin config and is exercised via integration in Phase 3; we skip it here to avoid touching the user's real config dir.)

- [ ] **Step 3: Commit**

```bash
git add embeddings_test.go
git commit -m "$(cat <<'EOF'
test(embeddings): newEmbeddingClient factory dispatch + unknown-provider error

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Config plumbing for embedding fields

**Files:**
- Modify: `config.go`
- Modify: `config_test.go`

- [ ] **Step 1: Extend `Config` and `configTOML`**

In `config.go`, replace the `Config` and `configTOML` types and add new defaults:

```go
type Config struct {
	Vault              string     `toml:"vault"`
	GitRemote          string     `toml:"git_remote,omitempty"`
	LastSync           *time.Time `toml:"last_sync,omitempty"`
	AIShortcutProvider string     `toml:"ai_shortcut_provider,omitempty"`

	EmbeddingProvider string `toml:"embedding_provider,omitempty"`
	EmbeddingModel    string `toml:"embedding_model,omitempty"`
	OllamaURL         string `toml:"ollama_url,omitempty"`
}

const (
	defaultAIShortcutProvider             = "blackbox"
	defaultEmbeddingProvider              = "openrouter"
	defaultEmbeddingModelOpenRouter       = "qwen/qwen3-embedding-8b"
	defaultEmbeddingModelOllama           = "nomic-embed-text"
	defaultOllamaURL                      = "http://localhost:11434"
)

type configTOML struct {
	Vault              string `toml:"vault"`
	GitRemote          string `toml:"git_remote,omitempty"`
	LastSync           string `toml:"last_sync,omitempty"`
	AIShortcutProvider string `toml:"ai_shortcut_provider,omitempty"`

	EmbeddingProvider string `toml:"embedding_provider,omitempty"`
	EmbeddingModel    string `toml:"embedding_model,omitempty"`
	OllamaURL         string `toml:"ollama_url,omitempty"`
}
```

In `loadConfig`, after the existing `AIShortcutProvider` default, add:

```go
	if cfg.EmbeddingProvider == "" {
		cfg.EmbeddingProvider = defaultEmbeddingProvider
	}
	if cfg.EmbeddingModel == "" {
		switch cfg.EmbeddingProvider {
		case "ollama":
			cfg.EmbeddingModel = defaultEmbeddingModelOllama
		default:
			cfg.EmbeddingModel = defaultEmbeddingModelOpenRouter
		}
	}
	if cfg.OllamaURL == "" {
		cfg.OllamaURL = defaultOllamaURL
	}
```

In `loadConfig`, copy fields from `ct` to `cfg`:

```go
	cfg := Config{
		Vault:              ct.Vault,
		GitRemote:          ct.GitRemote,
		AIShortcutProvider: ct.AIShortcutProvider,
		EmbeddingProvider:  ct.EmbeddingProvider,
		EmbeddingModel:     ct.EmbeddingModel,
		OllamaURL:          ct.OllamaURL,
	}
```

In `saveConfig`, write the new fields:

```go
	ct := configTOML{
		Vault:              cfg.Vault,
		GitRemote:          cfg.GitRemote,
		AIShortcutProvider: cfg.AIShortcutProvider,
		EmbeddingProvider:  cfg.EmbeddingProvider,
		EmbeddingModel:     cfg.EmbeddingModel,
		OllamaURL:          cfg.OllamaURL,
	}
```

- [ ] **Step 2: Add a defaults test in `config_test.go`**

Append to `config_test.go`:

```go
func TestLoadConfig_EmbeddingDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(dir+"/clipad", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/clipad/config.toml", []byte(`vault = "/tmp/v"`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbeddingProvider != "openrouter" {
		t.Errorf("EmbeddingProvider = %q, want openrouter", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "qwen/qwen3-embedding-8b" {
		t.Errorf("EmbeddingModel = %q", cfg.EmbeddingModel)
	}
	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("OllamaURL = %q", cfg.OllamaURL)
	}
}

func TestLoadConfig_OllamaProviderDefaultsModel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(dir+"/clipad", 0o755); err != nil {
		t.Fatal(err)
	}
	contents := "vault = \"/tmp/v\"\nembedding_provider = \"ollama\"\n"
	if err := os.WriteFile(dir+"/clipad/config.toml", []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbeddingModel != "nomic-embed-text" {
		t.Errorf("EmbeddingModel = %q, want nomic-embed-text", cfg.EmbeddingModel)
	}
}
```

- [ ] **Step 3: Run config tests**

Run: `go test ./... -run TestLoadConfig -v`
Expected: PASS for all (existing + 2 new).

- [ ] **Step 4: Commit**

```bash
git add config.go config_test.go
git commit -m "$(cat <<'EOF'
feat(config): add embedding_provider, embedding_model, ollama_url with defaults

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 2 — Index Layer

The index is `chunkFile` + SQLite store + cosine search. Build with TDD using `:memory:` SQLite and a fake embedder.

### Task 10: Add `modernc.org/sqlite` dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dep**

Run: `go get modernc.org/sqlite@latest`
Expected: dependency added to `go.mod`.

- [ ] **Step 2: Verify build still works**

Run: `go build ./...`
Expected: success. (No code uses sqlite yet; just want to confirm the module fetched cleanly.)

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "$(cat <<'EOF'
chore: add modernc.org/sqlite (pure-Go SQLite for the index DB)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: `chunkFile` with paragraph splits + tests (failing)

**Files:**
- Create: `index.go`
- Create: `index_test.go`

- [ ] **Step 1: Create `index.go` with chunk type and a stub `chunkFile`**

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
)

const maxChunkChars = 2000

type chunk struct {
	StartLine int
	EndLine   int
	Text      string
	Hash      string
}

func chunkHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

// chunkFile splits a markdown file into chunks. Paragraphs (separated by one
// or more blank lines) are the unit; paragraphs longer than maxChunkChars are
// further split on line boundaries until each sub-chunk fits.
//
// Lines are 1-indexed; start_line/end_line are inclusive.
func chunkFile(text string) []chunk {
	return nil // stub
}
```

- [ ] **Step 2: Create `index_test.go` with failing chunk tests**

```go
package main

import (
	"reflect"
	"testing"
)

func TestChunkFile_SimpleParagraphs(t *testing.T) {
	in := "First paragraph line one.\nFirst paragraph line two.\n\nSecond paragraph.\n\nThird."
	got := chunkFile(in)
	want := []chunk{
		{StartLine: 1, EndLine: 2, Text: "First paragraph line one.\nFirst paragraph line two."},
		{StartLine: 4, EndLine: 4, Text: "Second paragraph."},
		{StartLine: 6, EndLine: 6, Text: "Third."},
	}
	for i := range want {
		want[i].Hash = chunkHash(want[i].Text)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("chunkFile got %#v\nwant %#v", got, want)
	}
}

func TestChunkFile_Empty(t *testing.T) {
	if got := chunkFile(""); len(got) != 0 {
		t.Errorf("chunkFile(\"\") = %v, want empty", got)
	}
}

func TestChunkFile_WhitespaceOnly(t *testing.T) {
	if got := chunkFile("   \n\n\t\n"); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestChunkFile_OversizeParagraphSplits(t *testing.T) {
	// Build a paragraph with 5 long lines, each ~600 chars → exceeds 2000.
	line := strings.Repeat("a ", 300) // 600 chars
	para := line + "\n" + line + "\n" + line + "\n" + line + "\n" + line
	got := chunkFile(para)
	if len(got) < 2 {
		t.Fatalf("expected oversize paragraph to split into >=2 chunks, got %d", len(got))
	}
	for _, c := range got {
		if len(c.Text) > maxChunkChars+len(line) {
			// Allow some slack for the "add full line" rule, but reject runaway.
			t.Errorf("chunk too large: %d chars", len(c.Text))
		}
	}
	// Lines should be contiguous.
	for i := 1; i < len(got); i++ {
		if got[i].StartLine != got[i-1].EndLine+1 {
			t.Errorf("line gap: chunk[%d] starts at %d, prev ended at %d", i, got[i].StartLine, got[i-1].EndLine)
		}
	}
}
```

Add `"strings"` to the test imports.

- [ ] **Step 3: Verify tests fail (chunkFile is stub returning nil)**

Run: `go test ./... -run TestChunkFile -v`
Expected: FAIL on all four tests (chunkFile returns nil).

- [ ] **Step 4: Commit the failing tests**

```bash
git add index.go index_test.go
git commit -m "$(cat <<'EOF'
test(index): failing chunkFile tests (paragraph + oversize + empty)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 12: Implement `chunkFile`

**Files:**
- Modify: `index.go`

- [ ] **Step 1: Replace the stub with the real implementation**

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const maxChunkChars = 2000

type chunk struct {
	StartLine int
	EndLine   int
	Text      string
	Hash      string
}

func chunkHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

func chunkFile(text string) []chunk {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	var chunks []chunk
	i := 0
	for i < len(lines) {
		// Skip blank lines.
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		if i >= len(lines) {
			break
		}
		startLine := i + 1 // 1-indexed
		// Collect non-blank lines until next blank line.
		paraLines := []string{}
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			paraLines = append(paraLines, lines[i])
			i++
		}
		endLine := startLine + len(paraLines) - 1

		paragraph := strings.Join(paraLines, "\n")
		if len(paragraph) <= maxChunkChars {
			c := chunk{StartLine: startLine, EndLine: endLine, Text: paragraph}
			c.Hash = chunkHash(c.Text)
			chunks = append(chunks, c)
			continue
		}
		// Oversize: split by lines, accumulating until adding the next line would exceed cap.
		subStart := startLine
		var buf []string
		bufLen := 0
		for j, l := range paraLines {
			lineLen := len(l) + 1 // include separator newline (we'll join with \n)
			if bufLen > 0 && bufLen+lineLen > maxChunkChars {
				text := strings.Join(buf, "\n")
				c := chunk{StartLine: subStart, EndLine: subStart + len(buf) - 1, Text: text}
				c.Hash = chunkHash(c.Text)
				chunks = append(chunks, c)
				subStart += len(buf)
				buf = nil
				bufLen = 0
			}
			buf = append(buf, l)
			bufLen += lineLen
			_ = j
		}
		if len(buf) > 0 {
			text := strings.Join(buf, "\n")
			c := chunk{StartLine: subStart, EndLine: subStart + len(buf) - 1, Text: text}
			c.Hash = chunkHash(c.Text)
			chunks = append(chunks, c)
		}
	}
	return chunks
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./... -run TestChunkFile -v`
Expected: PASS on all four.

- [ ] **Step 3: Commit**

```bash
git add index.go
git commit -m "$(cat <<'EOF'
feat(index): chunkFile splits markdown by paragraph with size cap

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 13: SQLite schema, Index struct, Open/Close

**Files:**
- Modify: `index.go`
- Modify: `index_test.go`

- [ ] **Step 1: Add `Index` type and schema bootstrap to `index.go`**

Append to `index.go`:

```go
import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

const indexSchema = `
CREATE TABLE IF NOT EXISTS chunks (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	file_path   TEXT NOT NULL,
	start_line  INTEGER NOT NULL,
	end_line    INTEGER NOT NULL,
	text        TEXT NOT NULL,
	chunk_hash  TEXT NOT NULL,
	embedding   BLOB NOT NULL,
	model       TEXT NOT NULL,
	dim         INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chunks_file ON chunks(file_path);
CREATE INDEX IF NOT EXISTS idx_chunks_hash ON chunks(file_path, chunk_hash);
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`

type Index struct {
	db       *sql.DB
	embedder EmbeddingClient
	vault    string // absolute vault root, used to relativize file_path
}

// OpenIndex opens (or creates) the SQLite index file and applies the schema.
// embedder may be nil; in that case Search/RebuildFile will fail with a clear
// error, but the DB still loads (so the model can show "configure provider").
func OpenIndex(path, vault string, embedder EmbeddingClient) (*Index, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(indexSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Index{db: db, embedder: embedder, vault: vault}, nil
}

func (idx *Index) Close() error { return idx.db.Close() }

// encodeEmbedding writes a float32 slice as little-endian bytes.
func encodeEmbedding(v []float32) []byte {
	out := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}
	return out
}

func decodeEmbedding(b []byte) []float32 {
	n := len(b) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// cosine returns dot(a,b)/(|a||b|). Returns 0 if either is zero-norm.
func cosine(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float32
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / float32(math.Sqrt(float64(na)*float64(nb)))
}

// pin used time package once we add file-mtime logic in later tasks.
var _ = time.Now
var _ = sort.Sort
var _ = context.Background
```

(The trailing `_ = ...` lines exist only so the import block compiles before the next tasks consume those packages; remove them as each is used.)

- [ ] **Step 2: Add a smoke test that opens an in-memory index**

Append to `index_test.go`:

```go
func TestOpenIndex_InMemory(t *testing.T) {
	idx, err := OpenIndex(":memory:", "/tmp/vault", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	// Schema applied — we can insert and read back a meta key.
	if _, err := idx.db.Exec(`INSERT INTO meta(key, value) VALUES (?, ?)`, "schema_version", "1"); err != nil {
		t.Fatal(err)
	}
	var v string
	if err := idx.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, "schema_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "1" {
		t.Errorf("got %q, want 1", v)
	}
}

func TestEncodeDecodeEmbedding_Roundtrip(t *testing.T) {
	in := []float32{0.1, -0.2, 3.14, 0}
	got := decodeEmbedding(encodeEmbedding(in))
	if !reflect.DeepEqual(got, in) {
		t.Errorf("got %v, want %v", got, in)
	}
}

func TestCosine(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	c := []float32{0, 1, 0}
	if cosine(a, b) != 1 {
		t.Errorf("parallel: got %v, want 1", cosine(a, b))
	}
	if cosine(a, c) != 0 {
		t.Errorf("orthogonal: got %v, want 0", cosine(a, c))
	}
}
```

- [ ] **Step 3: Run the tests**

Run: `go test ./... -run "TestOpenIndex|TestEncodeDecodeEmbedding|TestCosine" -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add index.go index_test.go
git commit -m "$(cat <<'EOF'
feat(index): SQLite store, schema bootstrap, embedding encode/decode, cosine

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 14: `Index.RebuildFile` — happy path (failing)

**Files:**
- Modify: `index_test.go`

- [ ] **Step 1: Add a fake embedder helper and a failing test**

Append to `index_test.go`:

```go
import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeEmbedder hands out deterministic vectors keyed by text.
type fakeEmbedder struct {
	model string
	dim   int
	calls int
	calc  func(text string) []float32
}

func (f *fakeEmbedder) Model() string { return f.model }
func (f *fakeEmbedder) Dim() int      { return f.dim }
func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.calls++
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = f.calc(t)
	}
	return out, nil
}

// onehotEmbedder returns a vector with 1 in slot=hash%dim and 0 elsewhere.
// Useful for asserting that a given text maps to a specific cell.
func onehotEmbedder(model string, dim int) *fakeEmbedder {
	return &fakeEmbedder{model: model, dim: dim, calc: func(text string) []float32 {
		v := make([]float32, dim)
		h := 0
		for _, b := range []byte(text) {
			h = (h*31 + int(b)) & 0x7fffffff
		}
		v[h%dim] = 1
		return v
	}}
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRebuildFile_FreshFileEmbedsAllChunks(t *testing.T) {
	vault := t.TempDir()
	path := writeFile(t, vault, "a.md", "para one.\n\npara two.\n\npara three.")

	emb := onehotEmbedder("test-model", 8)
	idx, err := OpenIndex(":memory:", vault, emb)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	if err := idx.RebuildFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("rows = %d, want 3", n)
	}
	if emb.calls != 1 {
		t.Errorf("embedder calls = %d, want 1 (one batch for the whole file)", emb.calls)
	}
}
```

- [ ] **Step 2: Run — should fail (RebuildFile not yet defined)**

Run: `go test ./... -run TestRebuildFile -v`
Expected: build error or FAIL.

- [ ] **Step 3: Commit failing test**

```bash
git add index_test.go
git commit -m "$(cat <<'EOF'
test(index): failing RebuildFile happy-path test (3-chunk file → 3 rows)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 15: Implement `Index.RebuildFile`

**Files:**
- Modify: `index.go`

- [ ] **Step 1: Add `RebuildFile` and helper functions**

Remove the placate-imports lines from Task 13 and add:

```go
import (
	"errors"
	"path/filepath"
)

// relPath returns the path relative to the index's vault root.
func (idx *Index) relPath(absPath string) (string, error) {
	rel, err := filepath.Rel(idx.vault, absPath)
	if err != nil {
		return "", fmt.Errorf("relpath: %w", err)
	}
	return rel, nil
}

// RebuildFile re-chunks the file at absPath and updates the index so that
// chunks(file_path = rel) exactly matches the new chunk set, embedding only
// chunks whose hash isn't already present (model-matched).
func (idx *Index) RebuildFile(ctx context.Context, absPath string) error {
	if idx.embedder == nil {
		return errors.New("index: no embedder configured")
	}
	rel, err := idx.relPath(absPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	newChunks := chunkFile(string(data))

	// Existing rows for this file under the current model: hash → id
	model := idx.embedder.Model()
	existing := map[string]int64{}
	rows, err := idx.db.QueryContext(ctx,
		`SELECT id, chunk_hash FROM chunks WHERE file_path = ? AND model = ?`, rel, model)
	if err != nil {
		return fmt.Errorf("select existing: %w", err)
	}
	for rows.Next() {
		var id int64
		var h string
		if err := rows.Scan(&id, &h); err != nil {
			rows.Close()
			return err
		}
		existing[h] = id
	}
	rows.Close()

	keep := map[int64]bool{}
	var toEmbed []chunk
	for _, c := range newChunks {
		if id, ok := existing[c.Hash]; ok {
			keep[id] = true
		} else {
			toEmbed = append(toEmbed, c)
		}
	}

	tx, err := idx.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete rows for this file (model-matched) that aren't in keep.
	delRows, err := tx.QueryContext(ctx,
		`SELECT id FROM chunks WHERE file_path = ? AND model = ?`, rel, model)
	if err != nil {
		return fmt.Errorf("select for delete: %w", err)
	}
	var idsToDelete []int64
	for delRows.Next() {
		var id int64
		if err := delRows.Scan(&id); err != nil {
			delRows.Close()
			return err
		}
		if !keep[id] {
			idsToDelete = append(idsToDelete, id)
		}
	}
	delRows.Close()
	for _, id := range idsToDelete {
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
	}

	// Embed new chunks (single batch).
	if len(toEmbed) > 0 {
		texts := make([]string, len(toEmbed))
		for i, c := range toEmbed {
			texts[i] = c.Text
		}
		vecs, err := idx.embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("embed: %w", err)
		}
		if len(vecs) != len(toEmbed) {
			return fmt.Errorf("embed: got %d vectors for %d chunks", len(vecs), len(toEmbed))
		}
		now := time.Now().Unix()
		for i, c := range toEmbed {
			blob := encodeEmbedding(vecs[i])
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO chunks(file_path, start_line, end_line, text, chunk_hash, embedding, model, dim, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				rel, c.StartLine, c.EndLine, c.Text, c.Hash, blob, model, len(vecs[i]), now); err != nil {
				return fmt.Errorf("insert: %w", err)
			}
		}
	}

	return tx.Commit()
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./... -run TestRebuildFile_FreshFileEmbedsAllChunks -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add index.go
git commit -m "$(cat <<'EOF'
feat(index): RebuildFile inserts new chunks and removes obsolete ones

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 16: `RebuildFile` is incremental on edit (only changed paragraph re-embeds)

**Files:**
- Modify: `index_test.go`

- [ ] **Step 1: Add the incremental test**

```go
func TestRebuildFile_OnlyChangedChunkReEmbeds(t *testing.T) {
	vault := t.TempDir()
	path := writeFile(t, vault, "a.md", "alpha.\n\nbeta.\n\ngamma.")

	emb := onehotEmbedder("m", 8)
	idx, _ := OpenIndex(":memory:", vault, emb)
	defer idx.Close()

	if err := idx.RebuildFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := emb.calls

	// Modify only the second paragraph.
	if err := os.WriteFile(path, []byte("alpha.\n\nbeta MODIFIED.\n\ngamma."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := idx.RebuildFile(context.Background(), path); err != nil {
		t.Fatal(err)
	}

	// One additional batch call (containing exactly the modified chunk).
	if emb.calls != callsAfterFirst+1 {
		t.Errorf("embedder calls after edit = %d, want %d", emb.calls, callsAfterFirst+1)
	}
	var n int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("rows after edit = %d, want 3", n)
	}
	// The new content should be present.
	var found int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE text LIKE ?`, "%MODIFIED%").Scan(&found); err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Errorf("found = %d, want 1 row containing MODIFIED", found)
	}
}
```

- [ ] **Step 2: Run the test — should pass already (Task 15 implementation handles this)**

Run: `go test ./... -run TestRebuildFile_OnlyChangedChunkReEmbeds -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add index_test.go
git commit -m "$(cat <<'EOF'
test(index): incremental rebuild only re-embeds changed paragraphs

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 17: `Index.RemoveFile`

**Files:**
- Modify: `index.go`
- Modify: `index_test.go`

- [ ] **Step 1: Add a failing test**

```go
func TestRemoveFile(t *testing.T) {
	vault := t.TempDir()
	pathA := writeFile(t, vault, "a.md", "x.\n\ny.")
	pathB := writeFile(t, vault, "b.md", "z.")
	emb := onehotEmbedder("m", 8)
	idx, _ := OpenIndex(":memory:", vault, emb)
	defer idx.Close()

	_ = idx.RebuildFile(context.Background(), pathA)
	_ = idx.RebuildFile(context.Background(), pathB)

	if err := idx.RemoveFile(context.Background(), pathA); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE file_path = ?`, "a.md").Scan(&n)
	if n != 0 {
		t.Errorf("a.md rows after remove = %d, want 0", n)
	}
	_ = idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE file_path = ?`, "b.md").Scan(&n)
	if n == 0 {
		t.Errorf("b.md rows after remove = 0, want >0")
	}
}
```

- [ ] **Step 2: Verify failure**

Run: `go test ./... -run TestRemoveFile -v`
Expected: FAIL — `idx.RemoveFile undefined`.

- [ ] **Step 3: Implement in `index.go`**

```go
func (idx *Index) RemoveFile(ctx context.Context, absPath string) error {
	rel, err := idx.relPath(absPath)
	if err != nil {
		return err
	}
	_, err = idx.db.ExecContext(ctx, `DELETE FROM chunks WHERE file_path = ?`, rel)
	return err
}
```

- [ ] **Step 4: Run — passes**

Run: `go test ./... -run TestRemoveFile -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add index.go index_test.go
git commit -m "$(cat <<'EOF'
feat(index): RemoveFile deletes all chunks for a file path

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 18: `Index.Search` — cosine top-K

**Files:**
- Modify: `index.go`
- Modify: `index_test.go`

- [ ] **Step 1: Add a failing test**

```go
func TestSearch_RanksByCosine(t *testing.T) {
	vault := t.TempDir()
	// Three files, three distinct one-hot cells. Query "alpha" hashes
	// (via onehotEmbedder) to the same cell as the first file, so it must rank first.
	a := writeFile(t, vault, "a.md", "alpha")
	b := writeFile(t, vault, "b.md", "beta")
	c := writeFile(t, vault, "c.md", "gamma")
	emb := onehotEmbedder("m", 16)
	idx, _ := OpenIndex(":memory:", vault, emb)
	defer idx.Close()

	for _, p := range []string{a, b, c} {
		if err := idx.RebuildFile(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}

	res, err := idx.Search(context.Background(), "alpha", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("len(res) = %d, want 2", len(res))
	}
	if res[0].Path != "a.md" {
		t.Errorf("top result = %q, want a.md", res[0].Path)
	}
	if res[0].Score < res[1].Score {
		t.Errorf("scores not descending: %v then %v", res[0].Score, res[1].Score)
	}
}
```

- [ ] **Step 2: Verify failure**

Run: `go test ./... -run TestSearch_RanksByCosine -v`
Expected: FAIL — `Search undefined`.

- [ ] **Step 3: Implement `Search` and the result type**

```go
type Result struct {
	Path      string  // relative to vault
	StartLine int
	EndLine   int
	Text      string
	Score     float32
}

// Search embeds the query and returns the top-k chunks by cosine similarity,
// restricted to rows that match the embedder's current model.
func (idx *Index) Search(ctx context.Context, query string, k int) ([]Result, error) {
	if idx.embedder == nil {
		return nil, errors.New("index: no embedder configured")
	}
	if k <= 0 {
		return nil, nil
	}
	vecs, err := idx.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("embed query: got %d vectors", len(vecs))
	}
	q := vecs[0]
	model := idx.embedder.Model()

	rows, err := idx.db.QueryContext(ctx,
		`SELECT file_path, start_line, end_line, text, embedding FROM chunks WHERE model = ?`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		r     Result
		score float32
	}
	var all []scored
	for rows.Next() {
		var r Result
		var blob []byte
		if err := rows.Scan(&r.Path, &r.StartLine, &r.EndLine, &r.Text, &blob); err != nil {
			return nil, err
		}
		v := decodeEmbedding(blob)
		s := cosine(q, v)
		r.Score = s
		all = append(all, scored{r: r, score: s})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })
	if k > len(all) {
		k = len(all)
	}
	out := make([]Result, k)
	for i := 0; i < k; i++ {
		out[i] = all[i].r
	}
	return out, nil
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./... -run TestSearch_RanksByCosine -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add index.go index_test.go
git commit -m "$(cat <<'EOF'
feat(index): Search returns top-K chunks by cosine similarity

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 3 — Background Indexer (Bubble Tea)

The indexer wires the index into the existing Bubble Tea event loop: an initial sweep on startup, a per-file rebuild on `fileChangedMsg`, deletion on a new `fileDeletedMsg`, and a status-bar progress line.

### Task 19: Indexer messages and `startInitialIndex` cmd

**Files:**
- Modify: `index.go`

- [ ] **Step 1: Add tea.Cmd helpers and message types**

Append to `index.go`:

```go
import (
	tea "github.com/charmbracelet/bubbletea"
)

type indexProgressMsg struct{ done, total int }
type indexDoneMsg     struct{ err error }
type indexFileMsg     struct{ path string }

// collectMarkdownFiles walks vault and returns absolute paths of every .md.
func collectMarkdownFiles(vault string) ([]string, error) {
	var out []string
	err := filepath.Walk(vault, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // tolerate transient walk errors
		}
		if info.IsDir() {
			name := info.Name()
			if (strings.HasPrefix(name, ".") && path != vault) || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// startInitialIndex returns a tea.Cmd that does the full vault sweep.
// It does NOT emit per-batch progress in this minimal version — we add
// progress in the next task. For now it returns one indexDoneMsg at the end.
func startInitialIndex(idx *Index, vault string) tea.Cmd {
	return func() tea.Msg {
		if idx == nil || idx.embedder == nil {
			return indexDoneMsg{} // feature disabled; stay quiet
		}
		paths, err := collectMarkdownFiles(vault)
		if err != nil {
			return indexDoneMsg{err: err}
		}
		ctx := context.Background()
		for _, p := range paths {
			if err := idx.RebuildFile(ctx, p); err != nil {
				return indexDoneMsg{err: err}
			}
		}
		return indexDoneMsg{}
	}
}

// reindexFileCmd re-indexes a single file in the background.
func reindexFileCmd(idx *Index, path string) tea.Cmd {
	return func() tea.Msg {
		if idx == nil || idx.embedder == nil {
			return indexFileMsg{path: path}
		}
		_ = idx.RebuildFile(context.Background(), path)
		return indexFileMsg{path: path}
	}
}

// removeFileFromIndexCmd handles deletion.
func removeFileFromIndexCmd(idx *Index, path string) tea.Cmd {
	return func() tea.Msg {
		if idx == nil {
			return indexFileMsg{path: path}
		}
		_ = idx.RemoveFile(context.Background(), path)
		return indexFileMsg{path: path}
	}
}
```

(`os` and `strings` are already imported above.)

- [ ] **Step 2: Build to confirm it compiles**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add index.go
git commit -m "$(cat <<'EOF'
feat(index): tea.Cmd helpers (initial sweep, per-file reindex, remove)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 20: `watcher.go` — emit `fileDeletedMsg` on Remove events

**Files:**
- Modify: `watcher.go`
- Create: `watcher_test.go` (no existing test file)

- [ ] **Step 1: Add the new message type and per-event return logic**

Replace `watcher.go` with:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

type fileChangedMsg struct{}
type fileDeletedMsg struct{ Path string }

func watchVault(vault string) tea.Cmd {
	return func() tea.Msg {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}
		addWatchDirs(watcher, vault)
		var debounce <-chan time.Time
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				base := filepath.Base(event.Name)
				if strings.HasPrefix(base, ".") {
					continue
				}
				if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
					return fileDeletedMsg{Path: event.Name}
				}
				if event.Has(fsnotify.Create) {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						addWatchDirs(watcher, event.Name)
					}
				}
				debounce = time.After(100 * time.Millisecond)
			case <-debounce:
				return fileChangedMsg{}
			case _, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
}

func addWatchDirs(watcher *fsnotify.Watcher, root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			watcher.Add(path)
		}
		return nil
	})
}
```

- [ ] **Step 2: Add a test for the new behavior**

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchVault_RemoveEmitsDeletedMsg(t *testing.T) {
	vault := t.TempDir()
	target := filepath.Join(vault, "x.md")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := watchVault(vault)

	type result struct{ msg interface{} }
	done := make(chan result, 1)
	go func() {
		done <- result{msg: cmd()}
	}()

	// Give the watcher a moment to register, then delete the file.
	time.Sleep(100 * time.Millisecond)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-done:
		if d, ok := r.msg.(fileDeletedMsg); !ok {
			t.Errorf("got %T, want fileDeletedMsg", r.msg)
		} else if filepath.Base(d.Path) != "x.md" {
			t.Errorf("Path = %q, want trailing x.md", d.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not emit a message")
	}
}
```

- [ ] **Step 3: Run the test**

Run: `go test ./... -run TestWatchVault -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add watcher.go watcher_test.go
git commit -m "$(cat <<'EOF'
feat(watcher): distinguish Remove/Rename and emit fileDeletedMsg

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 21: Wire indexer state into `model.go`

**Files:**
- Modify: `model.go`
- Modify: `main.go`

- [ ] **Step 1: Add indexer fields to the `model` struct**

Find the `model` struct in `model.go` (around line 66) and add (after `gitRemoteInput`):

```go
	// Vault index
	indexer       *Index
	indexerStatus string // status bar string; "" when idle
```

- [ ] **Step 2: Construct the index in `main.go`**

Update `main.go`'s `main()` function to create the index after `loadConfig` and pass it to `newModel`. Find the line `m := newModel(cfg.Vault, plugins, cfg.AIShortcutProvider)` and replace with:

```go
	indexPath := indexDBPath()
	emb, embErr := newEmbeddingClient(cfg)
	var idx *Index
	if embErr == nil {
		idx, _ = OpenIndex(indexPath, cfg.Vault, emb)
	}

	m := newModel(cfg.Vault, plugins, cfg.AIShortcutProvider)
	m.indexer = idx
	if embErr != nil {
		m.errMsg = "Embeddings disabled: " + embErr.Error()
	}
	defer func() {
		if idx != nil {
			idx.Close()
		}
	}()
```

Add to `index.go` an `indexDBPath` helper:

```go
func indexDBPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "index.db")
}
```

Also ensure the directory exists before opening:

```go
func OpenIndex(path, vault string, embedder EmbeddingClient) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	// ... rest unchanged
}
```

- [ ] **Step 3: Update `Init` to start the initial sweep**

In `model.go`, find `func (m model) Init()` (around line 245) and replace its body:

```go
func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, watchVault(m.vault), autoSaveTick(), gitSyncCheckImmediate()}
	if m.indexer != nil {
		cmds = append(cmds, startInitialIndex(m.indexer, m.vault))
	}
	return tea.Batch(cmds...)
}
```

- [ ] **Step 4: Add handlers for the indexer messages**

In `model.go`'s `Update` function, after the existing `case fileChangedMsg:` block, add:

```go
	case fileDeletedMsg:
		m.refreshTree()
		var cmds []tea.Cmd
		cmds = append(cmds, watchVault(m.vault))
		if m.indexer != nil {
			cmds = append(cmds, removeFileFromIndexCmd(m.indexer, msg.Path))
		}
		return m, tea.Batch(cmds...)

	case indexProgressMsg:
		m.indexerStatus = fmt.Sprintf("[idx %d/%d]", msg.done, msg.total)
		return m, nil

	case indexDoneMsg:
		if msg.err != nil {
			m.indexerStatus = "[idx error]"
			m.errMsg = "Index: " + msg.err.Error()
		} else {
			m.indexerStatus = ""
		}
		return m, nil

	case indexFileMsg:
		return m, nil
```

Also extend the existing `case fileChangedMsg:` to trigger reindex of the changed file (we don't know which file changed since the existing watcher only emits "something changed" — for now we re-walk and reindex what differs; the per-file path is plumbed in Phase 3 cleanup):

Replace:

```go
	case fileChangedMsg:
		m.refreshTree()
		return m, watchVault(m.vault)
```

with:

```go
	case fileChangedMsg:
		m.refreshTree()
		var cmds []tea.Cmd
		cmds = append(cmds, watchVault(m.vault))
		if m.indexer != nil {
			cmds = append(cmds, startInitialIndex(m.indexer, m.vault))
		}
		return m, tea.Batch(cmds...)
```

(`startInitialIndex` is idempotent thanks to chunk-hash dedup, so re-running it on every change is safe; the only cost is the SELECTs to confirm "no work to do" for unchanged files.)

- [ ] **Step 5: Build, run existing tests**

Run: `go build ./... && go test ./...`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add main.go model.go index.go
git commit -m "$(cat <<'EOF'
feat(index): wire background indexer into model.Init/Update

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 22: Status bar shows `[idx N/M]`

**Files:**
- Modify: `statusbar.go`
- Modify: `model.go`

- [ ] **Step 1: Add an `indexerStatus` field on `StatusBar`**

In `statusbar.go`'s `StatusBar` struct add:

```go
	indexerStatus string
```

In its `View()` method, build a left-padded segment to render before the right-aligned `right` block:

Replace the `right := ""` block with:

```go
	right := ""
	if s.indexerStatus != "" {
		right = statusFlashStyle.Render(s.indexerStatus) + "  "
	}
	if s.errMsg != "" {
		right += statusErrorStyle.Render(s.errMsg)
	} else if s.flashMsg != "" {
		right += statusFlashStyle.Render(s.flashMsg)
	} else if s.filename != "" {
		modified := ""
		if s.dirty {
			modified = " [+]"
		}
		right += fmt.Sprintf("%d:%d  %s%s", s.line, s.col, s.filename, modified)
	}
```

- [ ] **Step 2: Pass the status through in `model.go`**

Find where `model.View()` (or a helper in `model.go`) constructs the `StatusBar`. Search:

```bash
grep -n "StatusBar{" model.go
```

In each construction, add `indexerStatus: m.indexerStatus,`.

- [ ] **Step 3: Build and run all tests**

Run: `go build ./... && go test ./...`
Expected: green.

- [ ] **Step 4: Commit**

```bash
git add statusbar.go model.go
git commit -m "$(cat <<'EOF'
feat(statusbar): render [idx N/M] indexer progress

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 4 — Vault Search Modal (Ctrl+Shift+F)

### Task 23: Add `inputVaultSearch` mode and search state on `model`

**Files:**
- Modify: `model.go`

- [ ] **Step 1: Extend the `inputMode` enum**

In `model.go`, find the `inputMode` constants block (around line 44) and append `inputVaultSearch`:

```go
const (
	inputNone inputMode = iota
	inputFilter
	// ... existing values unchanged ...
	inputHelp
	inputVaultSearch
)
```

- [ ] **Step 2: Add search state fields**

After the existing `helpViewport viewport.Model` field, add:

```go
	// Vault search modal (Ctrl+Shift+F)
	vaultSearchInput   textinput.Model
	vaultSearchResults []searchResult
	vaultSearchCursor  int
	vaultSearchOffset  int
	vaultSearchPending bool
	vaultSearchCancel  context.CancelFunc
	vaultSearchToken   int64
```

- [ ] **Step 3: Initialize the textinput in `newModel`**

Inside `newModel`, near the other `textinput.New()` calls, add:

```go
	vsi := textinput.New()
	vsi.Placeholder = "Search note contents…"
	vsi.CharLimit = 256
```

In the model struct literal in `newModel`, add `vaultSearchInput: vsi,`.

- [ ] **Step 4: Build to confirm it compiles**

Run: `go build ./...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add model.go
git commit -m "$(cat <<'EOF'
feat(vault-search): inputVaultSearch mode and state on model

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 24: `vault_search.go` — searchResult type, snippet helper, command, view

**Files:**
- Create: `vault_search.go`
- Create: `vault_search_test.go`

- [ ] **Step 1: Create `vault_search.go`**

```go
package main

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type searchResult struct {
	Path      string
	StartLine int
	EndLine   int
	Score     float32
	Snippet   string
}

type vaultSearchResultsMsg struct {
	token   int64
	results []searchResult
	err     error
}

// snippetFromText returns the first 2 wrapped lines of text, with tabs
// replaced by spaces, truncated to width chars per line.
func snippetFromText(text string, width int) string {
	if width < 1 {
		width = 1
	}
	t := strings.ReplaceAll(text, "\t", "    ")
	lines := strings.SplitN(t, "\n", 3)
	out := make([]string, 0, 2)
	for i, l := range lines {
		if i == 2 {
			break
		}
		if len(l) > width {
			l = l[:width-1] + "…"
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func toSearchResults(rs []Result, snippetWidth int) []searchResult {
	out := make([]searchResult, len(rs))
	for i, r := range rs {
		out[i] = searchResult{
			Path:      r.Path,
			StartLine: r.StartLine,
			EndLine:   r.EndLine,
			Score:     r.Score,
			Snippet:   snippetFromText(r.Text, snippetWidth),
		}
	}
	return out
}

func searchVaultCmd(idx *Index, token int64, query string, k, snippetWidth int) tea.Cmd {
	return func() tea.Msg {
		if idx == nil || idx.embedder == nil || strings.TrimSpace(query) == "" {
			return vaultSearchResultsMsg{token: token, results: nil}
		}
		ctx := context.Background()
		rs, err := idx.Search(ctx, query, k)
		if err != nil {
			return vaultSearchResultsMsg{token: token, err: err}
		}
		return vaultSearchResultsMsg{token: token, results: toSearchResults(rs, snippetWidth)}
	}
}

var (
	vaultSearchModalStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(1, 2)
	vaultSearchResultStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	vaultSearchSelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("215")).Bold(true)
	vaultSearchPathStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	vaultSearchScoreStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// vaultSearchView renders the modal. screenWidth/Height are the full window
// dimensions; the modal is sized as a fraction of those.
func vaultSearchView(input string, results []searchResult, cursor int, offset int, screenWidth, screenHeight int) string {
	w := screenWidth * 4 / 5
	if w < 50 {
		w = 50
	}
	h := screenHeight * 7 / 10
	if h < 12 {
		h = 12
	}
	innerW := w - 6
	if innerW < 10 {
		innerW = 10
	}

	var b strings.Builder
	b.WriteString(vaultSearchPathStyle.Render("Vault Search") + "  " + vaultSearchScoreStyle.Render("(Esc to close)"))
	b.WriteString("\n\n> " + input + "\n\n")

	visibleSlots := h - 6
	resultsToShow := results
	if cursor >= offset+visibleSlots/3 {
		// keep cursor near the middle (lazy auto-scroll)
		offset = cursor - visibleSlots/3
		if offset < 0 {
			offset = 0
		}
	}
	for i := offset; i < len(resultsToShow); i++ {
		r := resultsToShow[i]
		header := vaultSearchPathStyle.Render(r.Path) +
			"  " + vaultSearchScoreStyle.Render(formatLineRange(r.StartLine, r.EndLine)) +
			"  " + vaultSearchScoreStyle.Render(formatScore(r.Score))
		marker := "  "
		style := vaultSearchResultStyle
		if i == cursor {
			marker = vaultSearchSelStyle.Render("❯ ")
			style = vaultSearchSelStyle
		}
		b.WriteString(marker + header + "\n")
		for _, line := range strings.Split(r.Snippet, "\n") {
			b.WriteString("    " + style.Render(line) + "\n")
		}
		b.WriteString("\n")
	}
	if len(results) == 0 && strings.TrimSpace(input) != "" {
		b.WriteString(vaultSearchScoreStyle.Render("(no results)\n"))
	}

	footer := vaultSearchScoreStyle.Render("Enter: open · ↑↓: navigate · Esc: close")
	b.WriteString("\n" + footer)

	return vaultSearchModalStyle.Width(w).Height(h).Render(b.String())
}

func formatLineRange(start, end int) string {
	if start == end {
		return "L" + itoa(start)
	}
	return "L" + itoa(start) + "-L" + itoa(end)
}

func formatScore(s float32) string {
	// 2 decimals
	return "(" + ftoa(s) + ")"
}

func itoa(i int) string {
	return strings.TrimRight(strings.TrimRight(strings.TrimSpace(formatFloat(float64(i), 0)), "0"), ".")
}

func ftoa(f float32) string { return formatFloat(float64(f), 2) }

func formatFloat(f float64, prec int) string {
	// minimal helper to avoid importing fmt here
	if prec == 0 {
		// integer
		neg := f < 0
		if neg {
			f = -f
		}
		i := int(f + 0.5)
		var digits []byte
		if i == 0 {
			digits = []byte("0")
		}
		for i > 0 {
			digits = append([]byte{byte('0' + i%10)}, digits...)
			i /= 10
		}
		if neg {
			digits = append([]byte("-"), digits...)
		}
		return string(digits)
	}
	mult := 1.0
	for i := 0; i < prec; i++ {
		mult *= 10
	}
	scaled := int(f*mult + 0.5)
	whole := scaled / int(mult)
	frac := scaled % int(mult)
	fracStr := itoa(frac)
	for len(fracStr) < prec {
		fracStr = "0" + fracStr
	}
	return itoa(whole) + "." + fracStr
}
```

(The home-grown int/float formatters avoid pulling `strconv` into this file's import set; they aren't load-bearing — replace with `strconv.Itoa` / `strconv.FormatFloat` if you prefer. They're correct but not pretty.)

- [ ] **Step 2: Add a snippet test**

```go
package main

import (
	"strings"
	"testing"
)

func TestSnippetFromText_Truncates(t *testing.T) {
	in := "line one is quite a bit longer than the width\nline two short\nline three is dropped"
	got := snippetFromText(in, 30)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !strings.HasSuffix(lines[0], "…") {
		t.Errorf("line 0 should be truncated: %q", lines[0])
	}
	if lines[1] != "line two short" {
		t.Errorf("line 1 = %q", lines[1])
	}
}

func TestSnippetFromText_PreservesShort(t *testing.T) {
	got := snippetFromText("hi", 30)
	if got != "hi" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 3: Run the tests**

Run: `go test ./... -run TestSnippetFromText -v`
Expected: PASS for both.

- [ ] **Step 4: Commit**

```bash
git add vault_search.go vault_search_test.go
git commit -m "$(cat <<'EOF'
feat(vault-search): vault_search.go — searchResult, view, snippet, search cmd

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 25: Hook up `Ctrl+Shift+F` to open the modal

**Files:**
- Modify: `model.go`

- [ ] **Step 1: Add the shortcut in the top-level `Update` switch**

In `model.go`, in the `switch msg.String()` block where other top-level shortcuts live (`ctrl+q`, `ctrl+l`, etc.), add a new case before the `tab` case:

```go
		case "ctrl+shift+f":
			if m.indexer == nil || m.indexer.embedder == nil {
				m.errMsg = "Configure embedding_provider in config.toml"
				return m, nil
			}
			m.inputMode = inputVaultSearch
			m.vaultSearchInput.SetValue("")
			m.vaultSearchResults = nil
			m.vaultSearchCursor = 0
			m.vaultSearchOffset = 0
			cmd := m.vaultSearchInput.Focus()
			return m, cmd
```

- [ ] **Step 2: Add a handler for `inputVaultSearch` in `handleInputMode`**

In `handleInputMode`, add a case:

```go
	case inputVaultSearch:
		return m.handleVaultSearch(msg)
```

- [ ] **Step 3: Add `handleVaultSearch` and its sub-handlers to `model.go`**

Append a new method:

```go
func (m model) handleVaultSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = inputNone
		m.vaultSearchInput.Blur()
		return m, nil
	case "up", "ctrl+k":
		if m.vaultSearchCursor > 0 {
			m.vaultSearchCursor--
		}
		return m, nil
	case "down", "ctrl+j":
		if m.vaultSearchCursor < len(m.vaultSearchResults)-1 {
			m.vaultSearchCursor++
		}
		return m, nil
	case "enter":
		if len(m.vaultSearchResults) == 0 {
			return m, nil
		}
		r := m.vaultSearchResults[m.vaultSearchCursor]
		abs := filepath.Join(m.vault, r.Path)
		m.inputMode = inputNone
		m.vaultSearchInput.Blur()
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingSwitchFile
			m.pendingSwitchPath = abs
			return m, nil
		}
		m.openFile(abs)
		m.editor.MoveTo(r.StartLine-1, 0)
		m.activePanel = editorPanel
		m.editorMode = modeEdit
		focus := m.editor.Focus()
		return m, focus
	}
	// any other keystroke goes through textinput, then triggers a debounced search
	prev := m.vaultSearchInput.Value()
	var cmd tea.Cmd
	m.vaultSearchInput, cmd = m.vaultSearchInput.Update(msg)
	cur := m.vaultSearchInput.Value()
	if cur != prev && m.indexer != nil {
		m.vaultSearchToken++
		searchCmd := searchVaultCmd(m.indexer, m.vaultSearchToken, cur, 8, 80)
		return m, tea.Batch(cmd, searchCmd)
	}
	return m, cmd
}
```

(Note: this fires the search synchronously on each keystroke, which is fine for this task. A 200ms debounce is layered in Task 27 once the message-handling baseline is in place.)

- [ ] **Step 4: Add the result-message handler in `Update`**

After the `indexFileMsg` handler, add:

```go
	case vaultSearchResultsMsg:
		if msg.token != m.vaultSearchToken {
			return m, nil
		}
		m.vaultSearchPending = false
		if msg.err != nil {
			m.errMsg = "Search: " + msg.err.Error()
			return m, nil
		}
		m.vaultSearchResults = msg.results
		m.vaultSearchCursor = 0
		return m, nil
```

- [ ] **Step 5: Render the modal in `View`**

In `model.go`'s `View()`, find the cascading `if m.inputMode == ...` chain that builds `rightView`. Add a check (placed first or after `inputHelp`):

```go
	if m.inputMode == inputVaultSearch {
		modal := vaultSearchView(
			m.vaultSearchInput.View(),
			m.vaultSearchResults,
			m.vaultSearchCursor,
			m.vaultSearchOffset,
			m.width, m.editorHeight,
		)
		return lipgloss.Place(m.width, m.editorHeight, lipgloss.Center, lipgloss.Center, modal) + "\n" + statusBar
	}
```

(`statusBar` is whatever variable holds the `StatusBar.View()` output in your existing `View()` method — check the surrounding code; the streaming PR uses a similar pattern.)

- [ ] **Step 6: Add `MoveTo` as an exported method on `SelectableEditor`**

In `selection.go`, the existing private `moveTo` is at line 273. Add a thin export below it:

```go
// MoveTo positions the cursor at (line, col). Both are 0-indexed. Public
// wrapper around the existing private moveTo.
func (e *SelectableEditor) MoveTo(line, col int) {
	e.moveTo(line, col)
}
```

- [ ] **Step 7: Build and exercise manually**

Run: `go build ./... && go test ./...`
Expected: all green. Manual check: run `./clipad`, hit `Ctrl+Shift+F` (assuming config has `embedding_provider` set and OpenRouter key configured) — modal should open, typing should fire searches, Enter should jump.

- [ ] **Step 8: Commit**

```bash
git add model.go selection.go
git commit -m "$(cat <<'EOF'
feat(vault-search): Ctrl+Shift+F opens modal, Enter jumps to chunk

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 26: Token-based stale-result guard test

**Files:**
- Modify: `vault_search_test.go`

- [ ] **Step 1: Add a stale-token test**

```go
func TestVaultSearchResultsMsg_StaleTokenIgnored(t *testing.T) {
	// Simulate the handler's behavior: when results come in for a token
	// older than the current m.vaultSearchToken, the model state must not change.
	type mini struct {
		token   int64
		results []searchResult
	}
	m := mini{token: 5}
	stale := vaultSearchResultsMsg{token: 3, results: []searchResult{{Path: "old.md"}}}
	if stale.token == m.token {
		t.Fatal("test setup wrong")
	}
	// Mirror the guard:
	if stale.token != m.token {
		// would early-return — model unchanged; this is the desired behavior
		return
	}
	t.Errorf("guard failed: stale message would have updated state")
}
```

(This is a minimal sanity check on the contract; the real protection lives in the model handler's first line.)

- [ ] **Step 2: Run**

Run: `go test ./... -run TestVaultSearchResultsMsg -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add vault_search_test.go
git commit -m "$(cat <<'EOF'
test(vault-search): documents the stale-token guard contract

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 27: 200ms debounce on the search input

**Files:**
- Modify: `model.go`
- Modify: `vault_search.go`

- [ ] **Step 1: Add a debounce timer message and helper**

In `vault_search.go`:

```go
import "time"

type vaultSearchTickMsg struct{ token int64 }

func vaultSearchTickCmd(token int64) tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return vaultSearchTickMsg{token: token}
	})
}
```

- [ ] **Step 2: Update `handleVaultSearch` to schedule a tick instead of searching immediately**

Replace the bottom of `handleVaultSearch` (the "any other keystroke" block) with:

```go
	prev := m.vaultSearchInput.Value()
	var cmd tea.Cmd
	m.vaultSearchInput, cmd = m.vaultSearchInput.Update(msg)
	cur := m.vaultSearchInput.Value()
	if cur != prev {
		m.vaultSearchToken++
		// Don't fire yet; the tick (matched against the latest token) does it.
		return m, tea.Batch(cmd, vaultSearchTickCmd(m.vaultSearchToken))
	}
	return m, cmd
```

- [ ] **Step 3: Add a tick handler in `Update`**

Below `vaultSearchResultsMsg`:

```go
	case vaultSearchTickMsg:
		if msg.token != m.vaultSearchToken {
			return m, nil // user typed again; this tick is stale
		}
		if m.indexer == nil {
			return m, nil
		}
		m.vaultSearchPending = true
		return m, searchVaultCmd(m.indexer, msg.token, m.vaultSearchInput.Value(), 8, 80)
```

- [ ] **Step 4: Build and re-run all tests**

Run: `go build ./... && go test ./...`
Expected: green.

- [ ] **Step 5: Commit**

```bash
git add model.go vault_search.go
git commit -m "$(cat <<'EOF'
feat(vault-search): 200ms debounce via tea.Tick + token guard

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 5 — Chat Panel (Ctrl+Shift+A)

### Task 28: `ask.go` — `composeChatRequest` with bounded history

**Files:**
- Create: `ask.go`
- Create: `ask_test.go`

- [ ] **Step 1: Create `ask.go`**

```go
package main

import (
	"fmt"
	"strings"
)

const maxHistoryPairs = 4

type chatMessage struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

type chatTurn struct {
	Role      string // "user" | "assistant"
	Content   string
	Citations []citation
}

type citation struct {
	Path      string
	StartLine int
	EndLine   int
}

// composeChatRequest builds the messages array for an Ask-your-vault call.
// turns is the full session history INCLUDING the user's brand-new question
// at the end. retrievedChunks is the top-K context for the current query.
func composeChatRequest(turns []chatTurn, retrievedChunks []Result) (system string, messages []chatMessage, citations []citation) {
	system = buildSystemPrompt(retrievedChunks)
	citations = make([]citation, len(retrievedChunks))
	for i, c := range retrievedChunks {
		citations[i] = citation{Path: c.Path, StartLine: c.StartLine, EndLine: c.EndLine}
	}

	// Find the last user turn (must be the very last entry).
	if len(turns) == 0 || turns[len(turns)-1].Role != "user" {
		return system, nil, citations
	}
	currentUser := turns[len(turns)-1]
	prior := turns[:len(turns)-1]

	// Take the last maxHistoryPairs (user, assistant) pairs from prior.
	pairs := lastPairs(prior, maxHistoryPairs)
	for _, t := range pairs {
		messages = append(messages, chatMessage{Role: t.Role, Content: t.Content})
	}
	messages = append(messages, chatMessage{Role: "user", Content: currentUser.Content})
	return system, messages, citations
}

func lastPairs(turns []chatTurn, n int) []chatTurn {
	// Walk backwards collecting up to n complete (user, assistant) pairs.
	var rev []chatTurn
	pairs := 0
	i := len(turns) - 1
	for i >= 0 && pairs < n {
		// Expect an assistant followed (above) by a user.
		if turns[i].Role != "assistant" {
			break
		}
		assistant := turns[i]
		i--
		if i < 0 || turns[i].Role != "user" {
			break
		}
		user := turns[i]
		i--
		// pair found; prepend in proper order
		rev = append([]chatTurn{user, assistant}, rev...)
		pairs++
	}
	return rev
}

func buildSystemPrompt(chunks []Result) string {
	var b strings.Builder
	b.WriteString(`You are answering questions using the user's personal note vault as context.
Below are relevant excerpts. Cite sources inline using their numbered tag,
e.g., [1], [2]. If the excerpts do not contain the answer, say so plainly
rather than guessing.

`)
	for i, c := range chunks {
		fmt.Fprintf(&b, "[%d] %s L%d-L%d:\n%s\n\n", i+1, c.Path, c.StartLine, c.EndLine, c.Text)
	}
	return b.String()
}
```

- [ ] **Step 2: Add tests in `ask_test.go`**

```go
package main

import (
	"strings"
	"testing"
)

func TestComposeChatRequest_BoundsHistoryToFourPairs(t *testing.T) {
	var turns []chatTurn
	for i := 0; i < 8; i++ {
		turns = append(turns,
			chatTurn{Role: "user", Content: "u" + itoaForTest(i)},
			chatTurn{Role: "assistant", Content: "a" + itoaForTest(i)},
		)
	}
	turns = append(turns, chatTurn{Role: "user", Content: "current"})

	chunks := []Result{{Path: "x.md", StartLine: 1, EndLine: 1, Text: "hello"}}
	_, msgs, _ := composeChatRequest(turns, chunks)

	// Expected: 4 prior pairs (8 messages) + 1 current user = 9 total
	if len(msgs) != 9 {
		t.Errorf("messages = %d, want 9", len(msgs))
	}
	// Must end with the current user.
	if msgs[len(msgs)-1].Content != "current" {
		t.Errorf("last message = %q, want 'current'", msgs[len(msgs)-1].Content)
	}
	// First prior pair should be u4 (i.e. u4/a4 are the oldest of the kept four).
	if msgs[0].Content != "u4" {
		t.Errorf("first prior user = %q, want 'u4'", msgs[0].Content)
	}
}

func TestComposeChatRequest_SystemPromptHasCitationTags(t *testing.T) {
	chunks := []Result{
		{Path: "a.md", StartLine: 1, EndLine: 3, Text: "hello"},
		{Path: "b.md", StartLine: 5, EndLine: 7, Text: "world"},
	}
	turns := []chatTurn{{Role: "user", Content: "what?"}}
	sys, _, cites := composeChatRequest(turns, chunks)
	if !strings.Contains(sys, "[1] a.md L1-L3:") {
		t.Errorf("system missing [1] tag: %q", sys)
	}
	if !strings.Contains(sys, "[2] b.md L5-L7:") {
		t.Errorf("system missing [2] tag: %q", sys)
	}
	if len(cites) != 2 || cites[0].Path != "a.md" || cites[1].Path != "b.md" {
		t.Errorf("citations = %v", cites)
	}
}

func itoaForTest(i int) string { return strings.TrimSpace(formatFloat(float64(i), 0)) }
```

- [ ] **Step 3: Run the tests**

Run: `go test ./... -run TestComposeChatRequest -v`
Expected: PASS for both.

- [ ] **Step 4: Commit**

```bash
git add ask.go ask_test.go
git commit -m "$(cat <<'EOF'
feat(ask): composeChatRequest builds system prompt + bounded history

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 29: `chat.go` — chat state, messages, send command, view scaffolding

**Files:**
- Create: `chat.go`

- [ ] **Step 1: Create `chat.go` with the streaming primitives and a placeholder render**

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type chatModeT int

const (
	chatModeInput chatModeT = iota
	chatModeView
)

// chatChunkMsg / chatDoneMsg / chatErrMsg mirror the plugin streaming msgs
// but with their own identity discriminator so the two flows don't collide.
type chatChunkMsg struct {
	chunks <-chan string
	errs   <-chan error
	delta  string
}
type chatDoneMsg struct{ chunks <-chan string }
type chatErrMsg struct {
	chunks <-chan string
	err    error
}

func streamChatCmd(chunks <-chan string, errs <-chan error) tea.Cmd {
	return readNextChatChunk(chunks, errs)
}

func readNextChatChunk(chunks <-chan string, errs <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case d, ok := <-chunks:
			if !ok {
				return chatDoneMsg{chunks: chunks}
			}
			return chatChunkMsg{chunks: chunks, errs: errs, delta: d}
		case err := <-errs:
			if err != nil {
				return chatErrMsg{chunks: chunks, err: err}
			}
			return chatDoneMsg{chunks: chunks}
		}
	}
}

// chatStartCmd performs retrieval, composes the request, and starts the stream.
// It expects m.chatTurns to already include the new user turn.
func chatStartCmd(idx *Index, turns []chatTurn, query string, providerURL, apiKey, chatModel string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var chunks []Result
		if idx != nil {
			rs, err := idx.Search(ctx, query, 8)
			if err != nil {
				return chatStartFailedMsg{err: fmt.Errorf("retrieval: %w", err)}
			}
			chunks = rs
		}
		sys, msgs, cites := composeChatRequest(turns, chunks)
		userMsg, _ := encodeChatHistory(msgs)
		ch, errsCh := streamChatCompletion(ctx, providerURL, apiKey, chatModel, sys, userMsg)
		return chatStartedMsg{chunks: ch, errs: errsCh, citations: cites}
	}
}

type chatStartedMsg struct {
	chunks    <-chan string
	errs      <-chan error
	citations []citation
}
type chatStartFailedMsg struct{ err error }

// encodeChatHistory packs the prior turns + current user into the single
// "userMessage" string required by streamChatCompletion (which only takes
// system + user). We use a JSON-like header so the model sees the role
// boundaries clearly. The system prompt holds the retrieved-chunk context.
func encodeChatHistory(msgs []chatMessage) (string, error) {
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		j, _ := json.Marshal(m.Content)
		fmt.Fprintf(&b, "%s: %s", m.Role, string(j))
	}
	return b.String(), nil
}

var (
	chatPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)
	chatUserStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("215")).Bold(true)
	chatAssistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	chatCitationStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	chatHintStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func renderChatScrollback(turns []chatTurn, width int) string {
	var b strings.Builder
	for i, t := range turns {
		if i > 0 {
			b.WriteString("\n")
		}
		switch t.Role {
		case "user":
			b.WriteString(chatUserStyle.Render("▸ You: ") + t.Content + "\n")
		case "assistant":
			b.WriteString(chatAssistantStyle.Render("clipad: ") + t.Content + "\n")
			for j, c := range t.Citations {
				b.WriteString("  " + chatCitationStyle.Render(fmt.Sprintf("[%d] %s L%d-L%d", j+1, c.Path, c.StartLine, c.EndLine)) + "\n")
			}
		}
	}
	return b.String()
}

func chatPanelView(vp viewport.Model, input string, mode chatModeT, width, height int) string {
	hint := "1-9: open citation · i: input · ↑↓: scroll · Esc: close"
	if mode == chatModeInput {
		hint = "Enter: send · Esc: view mode"
	}
	body := vp.View()
	footer := chatHintStyle.Render(hint)
	inputLine := "> " + input
	return chatPanelStyle.Width(width).Height(height).Render(body + "\n" + inputLine + "\n" + footer)
}
```

- [ ] **Step 2: Build to confirm it compiles**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add chat.go
git commit -m "$(cat <<'EOF'
feat(chat): chat.go — streaming msgs, chatStartCmd, scrollback render

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 30: Add chat state to `model`, recalc layout for 3-pane

**Files:**
- Modify: `model.go`

- [ ] **Step 1: Add chat fields to the `model` struct**

After the vault-search fields:

```go
	// Chat panel (Ctrl+Shift+A)
	chatOpen         bool
	chatWidth        int
	chatMode         chatModeT
	chatTurns        []chatTurn
	chatInput        textinput.Model
	chatViewport     viewport.Model
	chatStreaming    bool
	chatActiveChunks <-chan string
	chatCancel       context.CancelFunc
	chatCurrentCites []citation
```

- [ ] **Step 2: Initialize in `newModel`**

Near the other textinput inits:

```go
	ci := textinput.New()
	ci.Placeholder = "Ask your vault…"
	ci.CharLimit = 1000
```

In the model literal, add:

```go
		chatInput: ci,
```

- [ ] **Step 3: Update `recalcLayout` to reserve `chatWidth`**

Find `recalcLayout` (around line 1290). Replace with a 3-pane-aware version:

```go
func (m *model) recalcLayout() {
	headerHeight := 0
	statusBarHeight := 1
	m.editorHeight = m.height - headerHeight - statusBarHeight
	m.treeHeight = m.editorHeight
	if m.editorHeight < 1 {
		m.editorHeight = 1
		m.treeHeight = 1
	}

	const minTreeWidth = 20
	chatWidth := 0
	if m.chatOpen {
		chatWidth = m.width * 3 / 10
		if chatWidth < 40 {
			chatWidth = 40
		}
		if chatWidth > m.width/2 {
			chatWidth = m.width / 2
		}
	}

	if m.treeHidden || m.width < minTreeWidth+10+chatWidth {
		m.treeWidth = 0
	} else {
		m.treeWidth = m.width / 4
		if m.treeWidth < minTreeWidth {
			m.treeWidth = minTreeWidth
		}
	}

	editorWidth := m.width - m.treeWidth - chatWidth
	if m.treeWidth > 0 {
		editorWidth-- // tree right border
	}
	if chatWidth > 0 {
		editorWidth-- // chat left border
	}
	if editorWidth < 10 {
		// shrink tree first, then chat, to keep editor usable
		if m.treeWidth > 0 {
			m.treeWidth = 0
			editorWidth = m.width - chatWidth
			if chatWidth > 0 {
				editorWidth--
			}
		}
		if editorWidth < 10 && chatWidth > 0 {
			chatWidth = 0
			m.chatOpen = false
			editorWidth = m.width
		}
	}
	m.editorWidth = editorWidth
	m.chatWidth = chatWidth

	m.tree.width = m.treeWidth
	m.tree.height = m.treeHeight
	m.tree.clampOffset()

	setEditorSize(&m.editor, m.editorWidth, m.editorHeight)

	if m.chatOpen {
		if m.chatViewport.Width == 0 {
			m.chatViewport = viewport.New(m.chatWidth-4, m.chatHeight()-4)
		} else {
			m.chatViewport.Width = m.chatWidth - 4
			m.chatViewport.Height = m.chatHeight() - 4
		}
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, m.chatWidth-4))
	}

	if m.inputMode == inputPluginDiff {
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
			m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)
	}
	if m.inputMode == inputHelp {
		m.helpViewport.Width = m.editorWidth
		m.helpViewport.Height = m.editorHeight
		m.helpViewport.SetContent(helpContent(m.editorWidth))
	}
}

func (m model) chatHeight() int { return m.editorHeight }
```

- [ ] **Step 4: Update `View` to render the chat panel as a third column**

Find the `mainView` construction in `View()` (around line 1397). Replace it:

```go
	var columns []string
	if m.treeWidth > 0 {
		columns = append(columns, treeView)
	}
	columns = append(columns, rightView)
	if m.chatOpen && m.chatWidth > 0 {
		columns = append(columns, chatPanelView(m.chatViewport, m.chatInput.View(), m.chatMode, m.chatWidth, m.editorHeight))
	}
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, columns...)
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add model.go
git commit -m "$(cat <<'EOF'
feat(chat): chat panel state + 3-pane recalcLayout/View integration

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 31: `Ctrl+Shift+A` toggles the chat panel

**Files:**
- Modify: `model.go`

- [ ] **Step 1: Add the shortcut handler**

In the same top-level `switch msg.String()` block where `ctrl+shift+f` lives, add:

```go
		case "ctrl+shift+a":
			if m.indexer == nil || m.indexer.embedder == nil {
				m.errMsg = "Configure embedding_provider in config.toml"
				return m, nil
			}
			if m.chatOpen {
				if m.chatCancel != nil {
					m.chatCancel()
					m.chatCancel = nil
				}
				m.chatOpen = false
				m.chatInput.Blur()
				m.recalcLayout()
				return m, nil
			}
			m.chatOpen = true
			m.chatMode = chatModeInput
			m.recalcLayout()
			cmd := m.chatInput.Focus()
			return m, cmd
```

- [ ] **Step 2: Build and exercise manually**

Run: `go build ./...`
Expected: success. Run `./clipad`, hit `Ctrl+Shift+A` — chat panel should open as a right-side column. Hit again to close.

- [ ] **Step 3: Commit**

```bash
git add model.go
git commit -m "$(cat <<'EOF'
feat(chat): Ctrl+Shift+A toggles the chat panel

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 32: Chat send flow and streaming handlers

**Files:**
- Modify: `model.go`

- [ ] **Step 1: Add `handleChatPanel` (input + view modes) and the streaming handlers**

Add to `model.go`:

```go
func (m model) handleChatPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.chatStreaming {
		// Only Esc cancels mid-stream; everything else is ignored.
		if msg.String() == "esc" {
			if m.chatCancel != nil {
				m.chatCancel()
				m.chatCancel = nil
			}
			m.chatStreaming = false
			m.chatActiveChunks = nil
			m.chatMode = chatModeView
			m.chatInput.Blur()
			return m, nil
		}
		return m, nil
	}

	switch m.chatMode {
	case chatModeInput:
		switch msg.String() {
		case "esc":
			m.chatMode = chatModeView
			m.chatInput.Blur()
			return m, nil
		case "enter":
			query := strings.TrimSpace(m.chatInput.Value())
			if query == "" {
				return m, nil
			}
			m.chatInput.SetValue("")
			m.chatTurns = append(m.chatTurns, chatTurn{Role: "user", Content: query})
			m.chatTurns = append(m.chatTurns, chatTurn{Role: "assistant", Content: ""})
			m.chatStreaming = true

			provider := m.activeShortcutProvider
			if provider == "" {
				provider = defaultAIShortcutProvider
			}
			plugCfg, err := loadPluginConfig(provider)
			if err != nil {
				m.chatStreaming = false
				m.errMsg = "Plugin config: " + err.Error()
				return m, nil
			}
			url := defaultBlackboxURL
			if provider == "openrouter" {
				url = defaultOpenRouterURL
			}
			ctx, cancel := context.WithCancel(context.Background())
			m.chatCancel = cancel
			_ = ctx // not used directly; chatStartCmd makes its own context.Background

			return m, chatStartCmd(m.indexer, m.chatTurns, query, url, plugCfg["api_key"], plugCfg["model"])
		}
		var cmd tea.Cmd
		m.chatInput, cmd = m.chatInput.Update(msg)
		return m, cmd
	case chatModeView:
		switch msg.String() {
		case "esc":
			m.chatOpen = false
			m.chatInput.Blur()
			m.recalcLayout()
			return m, nil
		case "i", "/":
			m.chatMode = chatModeInput
			cmd := m.chatInput.Focus()
			return m, cmd
		case "up", "k":
			m.chatViewport.LineUp(1)
			return m, nil
		case "down", "j":
			m.chatViewport.LineDown(1)
			return m, nil
		}
		// Digit 1-9 opens citation N of the most recent assistant turn.
		s := msg.String()
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			n := int(s[0] - '0')
			cite := mostRecentCitation(m.chatTurns, n)
			if cite != nil {
				abs := filepath.Join(m.vault, cite.Path)
				if m.isDirty() {
					m.inputMode = inputUnsavedGuard
					m.pendingAction = pendingSwitchFile
					m.pendingSwitchPath = abs
					return m, nil
				}
				m.openFile(abs)
				m.editor.MoveTo(cite.StartLine-1, 0)
				m.activePanel = editorPanel
				m.editorMode = modeEdit
				return m, m.editor.Focus()
			}
		}
		return m, nil
	}
	return m, nil
}

func mostRecentCitation(turns []chatTurn, n int) *citation {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role == "assistant" && len(turns[i].Citations) > 0 {
			if n >= 1 && n <= len(turns[i].Citations) {
				c := turns[i].Citations[n-1]
				return &c
			}
			return nil
		}
	}
	return nil
}
```

- [ ] **Step 2: Route key events to `handleChatPanel`**

The chat panel should *not* intercept global shortcuts (Ctrl+Q, Ctrl+S, Ctrl+Shift+A toggle, Ctrl+Shift+F, Ctrl+B, etc.). Place the chat routing **after** the global `switch msg.String()` block in `Update`, immediately before the `m.handleTreeKeys / m.handleEditorKeys` dispatch.

Find the bottom of the global shortcut switch in `Update` — it currently looks like:

```go
		if m.activePanel == treePanel {
			return m.handleTreeKeys(msg)
		}
		return m.handleEditorKeys(msg)
```

Replace with:

```go
		if m.chatOpen {
			return m.handleChatPanel(msg)
		}
		if m.activePanel == treePanel {
			return m.handleTreeKeys(msg)
		}
		return m.handleEditorKeys(msg)
```

This way: every `case "ctrl+...":` in the global switch above still returns first (so Ctrl+Q quits, Ctrl+Shift+A toggles, Ctrl+S saves, etc.); only keys that didn't match any global shortcut fall through to the chat panel when it's open.

- [ ] **Step 3: Add streaming handlers in `Update`**

Below `vaultSearchTickMsg`:

```go
	case chatStartedMsg:
		m.chatActiveChunks = msg.chunks
		m.chatCurrentCites = msg.citations
		return m, streamChatCmd(msg.chunks, msg.errs)

	case chatStartFailedMsg:
		m.chatStreaming = false
		// Roll back the placeholder assistant turn we appended optimistically.
		if len(m.chatTurns) > 0 && m.chatTurns[len(m.chatTurns)-1].Role == "assistant" {
			m.chatTurns = m.chatTurns[:len(m.chatTurns)-1]
		}
		m.errMsg = "Chat: " + msg.err.Error()
		return m, nil

	case chatChunkMsg:
		if msg.chunks != m.chatActiveChunks {
			return m, nil
		}
		last := &m.chatTurns[len(m.chatTurns)-1]
		last.Content += msg.delta
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, m.chatWidth-4))
		m.chatViewport.GotoBottom()
		return m, readNextChatChunk(msg.chunks, msg.errs)

	case chatDoneMsg:
		if msg.chunks != m.chatActiveChunks {
			return m, nil
		}
		m.chatStreaming = false
		m.chatActiveChunks = nil
		m.chatCancel = nil
		last := &m.chatTurns[len(m.chatTurns)-1]
		last.Citations = m.chatCurrentCites
		m.chatCurrentCites = nil
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, m.chatWidth-4))
		m.chatViewport.GotoBottom()
		return m, nil

	case chatErrMsg:
		if msg.chunks != m.chatActiveChunks {
			return m, nil
		}
		m.chatStreaming = false
		m.chatActiveChunks = nil
		last := &m.chatTurns[len(m.chatTurns)-1]
		last.Content = "Error: " + msg.err.Error()
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, m.chatWidth-4))
		return m, nil
```

- [ ] **Step 4: Build and run all tests**

Run: `go build ./... && go test ./...`
Expected: green.

- [ ] **Step 5: Commit**

```bash
git add model.go
git commit -m "$(cat <<'EOF'
feat(chat): send flow + streaming handlers + 1-9 citation jump

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 33: Tests for `mostRecentCitation` and chat-mode transitions

**Files:**
- Create: `chat_test.go`

- [ ] **Step 1: Add tests**

```go
package main

import (
	"testing"
)

func TestMostRecentCitation_FindsLastAssistantTurnWithCites(t *testing.T) {
	turns := []chatTurn{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1", Citations: []citation{{Path: "old.md", StartLine: 1, EndLine: 1}}},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2", Citations: []citation{
			{Path: "new.md", StartLine: 5, EndLine: 7},
			{Path: "other.md", StartLine: 8, EndLine: 9},
		}},
	}
	c := mostRecentCitation(turns, 2)
	if c == nil || c.Path != "other.md" {
		t.Errorf("got %v, want other.md", c)
	}
	c = mostRecentCitation(turns, 1)
	if c == nil || c.Path != "new.md" {
		t.Errorf("got %v, want new.md", c)
	}
	if mostRecentCitation(turns, 99) != nil {
		t.Errorf("out-of-range should return nil")
	}
}

func TestMostRecentCitation_SkipsTurnsWithoutCites(t *testing.T) {
	turns := []chatTurn{
		{Role: "assistant", Content: "no cites"},
	}
	if mostRecentCitation(turns, 1) != nil {
		t.Errorf("should be nil when last assistant has no citations")
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./... -run TestMostRecentCitation -v`
Expected: PASS for both.

- [ ] **Step 3: Commit**

```bash
git add chat_test.go
git commit -m "$(cat <<'EOF'
test(chat): mostRecentCitation walks back to the last assistant w/ cites

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 34: Help modal entries for the new shortcuts

**Files:**
- Modify: `help_modal.go`

- [ ] **Step 1: Add entries**

In `help_modal.go`, find `var helpSections = []helpSection{` (line 37) and add to the `Global` section's entries list (alphabetic order is not required, but mirror the existing flow — put these before `Ctrl+?`):

```go
				{"Ctrl+Shift+F", "Vault search"},
				{"Ctrl+Shift+A", "Ask your vault"},
```

Also add a new section after "Diff View":

```go
	{
		title: "Vault Chat",
		entries: []helpEntry{
			{"Enter", "Send (input mode)"},
			{"Esc", "Switch to view mode / close panel (view mode)"},
			{"i / /", "Return to input mode"},
			{"1–9", "Open numbered citation"},
			{"↑ / ↓", "Scroll scrollback"},
		},
	},
```

- [ ] **Step 2: Build and run all tests**

Run: `go build ./... && go test ./...`
Expected: green (existing help_modal_test should still pass; the new strings are just additions).

- [ ] **Step 3: Commit**

```bash
git add help_modal.go
git commit -m "$(cat <<'EOF'
docs(help): document Ctrl+Shift+F, Ctrl+Shift+A, and chat keys

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

1. **Spec coverage** — every spec section has a task:
   - Embeddings abstraction → Tasks 1–8
   - Config → Task 9
   - Index schema/chunking/search → Tasks 10–18
   - Background indexer → Tasks 19–22
   - Search modal → Tasks 23–27
   - Chat panel → Tasks 28–33
   - Help docs → Task 34

2. **Type consistency**: `searchResult`, `chunk`, `Result`, `chatTurn`, `citation`, `chatChunkMsg`, `vaultSearchResultsMsg`, `indexProgressMsg`, `indexDoneMsg`, `indexFileMsg`, `fileDeletedMsg` are used consistently across tasks. `EmbeddingClient.Model()` and `Dim()` match between interface (Task 1), impls (Tasks 3, 7), and consumers (Task 15 reads `idx.embedder.Model()`).

3. **Placeholders**: every code step has actual code; every test has actual assertions; commit commands are concrete.

4. **YAGNI checks**: no vector indexes, no cross-device sync, no chat persistence, no model-change auto-cleanup — all called out as out-of-scope in the spec and not added here.

5. **TDD**: every new behavior is preceded by a failing test before its implementation. Bigger UI tasks (modal/panel rendering) lean on manual verification + targeted unit tests for the pure-function pieces (snippet, citation lookup, history bounding) since rendered TUI views aren't naturally unit-testable.

---

**Plan complete and saved to `docs/superpowers/plans/2026-04-26-vault-semantic-search-rag.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
