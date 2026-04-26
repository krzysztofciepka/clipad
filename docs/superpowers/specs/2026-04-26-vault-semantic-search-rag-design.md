# Vault-Wide Semantic Search + "Ask Your Vault" (RAG) — Design

**Date:** 2026-04-26
**Status:** Approved
**Source:** Task 24

## Goal

Replace the filename-only `/` filter with two new capabilities:

1. **Semantic search** (`Ctrl+Shift+F`) — vault-wide content search via embeddings. Modal lists ranked chunks; Enter opens the file at the chunk's start line.
2. **Ask your vault** (`Ctrl+Shift+A`) — right-side chat panel that answers questions using retrieved chunks as context, streaming the response and showing numbered citations.

Both are backed by a per-device SQLite index of chunk embeddings, built lazily in the background and updated incrementally on file changes.

## Non-Goals

- Cross-device index synchronization. Each device builds its own.
- Vector indexes (HNSW/IVF). Brute-force cosine over `<10K` chunks is fine.
- Hybrid keyword+semantic search. Pure semantic only; BM25 can layer later.
- Embeddings of non-`.md` files.
- Saving / exporting / multi-session chat history.

## High-Level Architecture

```
┌──────────────────────────────────────────────────────────────┐
│ model.go                                                     │
│  ├─ inputVaultSearch (modal, Ctrl+Shift+F)                   │
│  ├─ chatPanel (right-split, Ctrl+Shift+A toggles)            │
│  ├─ indexer status in statusbar  ("[idx 47/312]")            │
│  └─ stale-message guards via channel identity                │
└──────────┬───────────────────────────┬───────────────────────┘
           │                           │
           ▼                           ▼
┌──────────────────────┐    ┌────────────────────────────────┐
│ index.go (NEW)       │    │ ask.go (NEW)                   │
│  - chunkFile()       │    │  - composeChatRequest()        │
│  - RebuildFile()     │◄──►│  - bounded last-4-pairs        │
│  - RemoveFile()      │    │  - reuses streamChatCompletion │
│  - Search(q, k)      │    └────────────────┬───────────────┘
│  - SQLite store      │                     │
└──────────┬───────────┘                     │
           │                                 │
           ▼                                 ▼
┌──────────────────────────────────────────────────────────────┐
│ embeddings.go (NEW)                                          │
│   type EmbeddingClient interface {                           │
│     Embed(ctx, []string) ([][]float32, error)                │
│     Model() string                                           │
│     Dim() int                                                │
│   }                                                          │
│   - OpenRouterEmbeddings   (default; reuses openrouter key)  │
│   - OllamaEmbeddings                                         │
└──────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────┐
│ ~/.config/clipad/index.db (modernc.org/sqlite, no cgo)       │
│   chunks(id, file_path, start_line, end_line, text,          │
│          chunk_hash, embedding BLOB, model, dim, updated_at) │
│   meta(key, value)                                           │
└──────────────────────────────────────────────────────────────┘
```

**Key choices:**

- Pure-Go SQLite via `modernc.org/sqlite` — no cgo, matches existing build.
- Embeddings stored as little-endian `float32` byte blobs. Cosine in Go.
- `EmbeddingClient` interface keeps index code provider-agnostic.
- Content-addressed chunks via `chunk_hash` so file edits only re-embed deltas.
- `streamChatCompletion` from `plugin_stream.go` powers the chat answer stream — no new SSE plumbing.

## Embeddings Abstraction

```go
// embeddings.go
type EmbeddingClient interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Model() string
    Dim() int
}

func newEmbeddingClient(cfg Config) (EmbeddingClient, error) {
    switch cfg.EmbeddingProvider {
    case "ollama":
        return &OllamaEmbeddings{BaseURL: cfg.OllamaURL, ModelName: cfg.EmbeddingModel}, nil
    case "openrouter", "":
        keyCfg, err := loadPluginConfig("openrouter")
        if err != nil { return nil, fmt.Errorf("openrouter embeddings need plugin config: %w", err) }
        return &OpenRouterEmbeddings{APIKey: keyCfg["api_key"], ModelName: cfg.EmbeddingModel}, nil
    default:
        return nil, fmt.Errorf("unknown embedding_provider: %q", cfg.EmbeddingProvider)
    }
}
```

| Field | OpenRouter (default) | Ollama |
|---|---|---|
| Endpoint | `https://openrouter.ai/api/v1/embeddings` | `http://localhost:11434/api/embeddings` |
| Auth | `Authorization: Bearer <api_key>` (reuses OpenRouter plugin key) | none |
| Default model | `qwen/qwen3-embedding-8b` (4096-dim) | `nomic-embed-text` (768-dim) |
| Request shape | `{"model": M, "input": [...]}` | `{"model": M, "prompt": "..."}` (per-text) |
| Response shape | `{"data": [{"embedding": [...]}, ...]}` | `{"embedding": [...]}` |
| Batching | Up to 100 inputs per call | Per-text loop |

`OpenRouterEmbeddings.Embed` batches in groups of 100. `OllamaEmbeddings.Embed` loops internally so callers see the same `texts in → vectors out` shape. `Model()` returns the configured model string for storage in the DB row's `model` column; rows whose `model` mismatches the current configuration are ignored on read and not pruned automatically.

## Config (`config.go`)

```go
type Config struct {
    Vault              string     `toml:"vault"`
    GitRemote          string     `toml:"git_remote,omitempty"`
    LastSync           *time.Time `toml:"last_sync,omitempty"`
    AIShortcutProvider string     `toml:"ai_shortcut_provider,omitempty"`

    // NEW
    EmbeddingProvider  string     `toml:"embedding_provider,omitempty"`  // "openrouter" | "ollama"
    EmbeddingModel     string     `toml:"embedding_model,omitempty"`
    OllamaURL          string     `toml:"ollama_url,omitempty"`
}
```

Defaults filled in `loadConfig`:

- `EmbeddingProvider` → `"openrouter"`
- `EmbeddingModel` → `"qwen/qwen3-embedding-8b"` if provider=openrouter, `"nomic-embed-text"` if provider=ollama
- `OllamaURL` → `"http://localhost:11434"`

The OpenRouter API key is reused from `~/.config/clipad/plugins/openrouter.toml` — the same key already used for chat. No second key for the common case. If that file is missing and `EmbeddingProvider == "openrouter"`, `newEmbeddingClient` returns an error → indexer stays nil → search/chat surface a config error.

## Index: Schema and Chunking

### Schema

```sql
CREATE TABLE chunks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path   TEXT NOT NULL,         -- relative to vault
    start_line  INTEGER NOT NULL,      -- 1-indexed inclusive
    end_line    INTEGER NOT NULL,      -- 1-indexed inclusive
    text        TEXT NOT NULL,
    chunk_hash  TEXT NOT NULL,         -- sha256(text)[:16]
    embedding   BLOB NOT NULL,         -- little-endian float32 array
    model       TEXT NOT NULL,         -- e.g. "qwen/qwen3-embedding-8b"
    dim         INTEGER NOT NULL,      -- 4096 / 768; reject corrupt rows fast
    updated_at  INTEGER NOT NULL       -- unix seconds
);
CREATE INDEX idx_chunks_file ON chunks(file_path);
CREATE INDEX idx_chunks_hash ON chunks(file_path, chunk_hash);

CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- meta keys: "schema_version", "embedding_model", "vault_path"
```

### Chunking algorithm (`chunkFile(text string) []chunk`)

1. Split file content on `\n\n+` (one or more blank lines) to get paragraph candidates. Track 1-indexed `start_line` / `end_line` per candidate.
2. For each paragraph:
   - If `len(text) <= maxChunkChars` (≈2000 chars, ~500 tokens): emit as one chunk.
   - Else split into sub-chunks: walk by lines and accumulate until adding the next line would exceed `maxChunkChars`. Emit, advance, repeat. Each sub-chunk gets its own `start_line` / `end_line`.
3. Skip empty / whitespace-only chunks.
4. Hash: `chunk_hash = sha256(text)[:16]`.

**Why character-cap, not token-cap:** counting tokens means a tokenizer dependency we don't otherwise need. 4 chars ≈ 1 token is a safe overestimate; 2000 chars ≈ 500 tokens leaves headroom under any embedding model's context limit.

### Incremental update flow (`Index.RebuildFile`)

```go
func (idx *Index) RebuildFile(ctx context.Context, path string) error {
    text := readFile(path)
    new := chunkFile(text)               // []chunk with start, end, text, hash

    existing := selectChunkHashes(path)  // map[hash]rowID, current model only

    var toEmbed []chunk
    var keepIDs []int64
    for _, c := range new {
        if id, ok := existing[c.hash]; ok {
            keepIDs = append(keepIDs, id)
        } else {
            toEmbed = append(toEmbed, c)
        }
    }
    deleteChunksNotIn(path, keepIDs)

    if len(toEmbed) == 0 { return nil }
    embs, err := embedder.Embed(ctx, textsOf(toEmbed))
    if err != nil { return err }
    insertChunks(path, toEmbed, embs)
    return nil
}
```

A typed-character edit that re-saves only one paragraph's chunk → one new row inserted, all other rows untouched, one embedding API call.

### Search (`Index.Search`)

```go
func (idx *Index) Search(ctx context.Context, query string, k int) ([]Result, error) {
    qvec, err := embedder.Embed(ctx, []string{query})  // single-element batch
    if err != nil { return nil, err }
    // SELECT id, file_path, start_line, end_line, text, embedding
    // FROM chunks WHERE model = ?
    // brute-force cosine in Go, top-K via container/heap
    return topK, nil
}
```

For 10K chunks of 4096-dim: ~40M FLOPs ≈ a few ms. No vector index needed.

### File deletion (`Index.RemoveFile`)

`Index.RemoveFile(path)` deletes all rows where `file_path = path`. Triggered from a new `fileDeletedMsg` (a small extension to `watcher.go` to distinguish `Remove` events).

## Background Indexer

Long-running goroutine driven by Bubble Tea messages, mirroring `gitSync` and `autoSave` patterns.

### State on `model`

```go
indexer        *Index           // DB handle + embedder
indexerStatus  string           // status bar string; "" when idle
indexerCancel  context.CancelFunc
indexerErr     string           // last error
```

### Messages (in `index.go`)

```go
type indexProgressMsg struct{ done, total int }
type indexDoneMsg     struct{ err error }
type indexFileMsg     struct{ path string }
```

### Lifecycle

1. **Startup.** `newModel` constructs the embedder via `newEmbeddingClient(cfg)`. If it returns an error, `indexer` is nil and search/chat surface a one-line "configure embedding provider" error when the user opens them.
2. **Initial sweep.** `model.Init()` returns `tea.Batch(..., startInitialIndex(idx, vault))`. That command walks the vault, computes `chunkFile()` per file, compares against existing rows (model-matched), builds the global `toEmbed` set. If empty, returns `indexDoneMsg{nil}` immediately. Else embeds in batches in a goroutine, emitting `indexProgressMsg{done, total}` per batch; final message `indexDoneMsg{}`.
3. **Per-file updates.** Existing `fileChangedMsg` handler triggers `reindexFileCmd(path)` → `idx.RebuildFile(ctx, path)` → emits `indexFileMsg{path}`. Multiple changes in quick succession run sequentially in the same goroutine pool (index writes are serialized).
4. **Cancellation.** `indexerCancel` aborts in-flight work on shutdown. Each file's `RebuildFile` is atomic via SQLite transaction.
5. **Status bar.** `statusbar.go` renders `[idx 47/312]` while building, hidden when idle. Errors render as `[idx error]` with full message available in `errMsg`.

### Concurrency

- One indexer goroutine at a time. Re-entrancy prevented by `indexerCancel != nil` check.
- `fileChangedMsg` while a sweep is in flight → enqueue (`pendingFiles []string`), drain after sweep finishes. Watcher already debounces 100ms; no second debounce.

### Handlers

```go
case indexProgressMsg:
    m.indexerStatus = fmt.Sprintf("[idx %d/%d]", msg.done, msg.total)
    return m, nil

case indexDoneMsg:
    if msg.err != nil { m.indexerStatus = "[idx error]"; m.errMsg = "Index: " + msg.err.Error() }
    else              { m.indexerStatus = "" }
    return m, nil

case indexFileMsg:
    return m, nil
```

### Error handling

| Failure | Behavior |
|---|---|
| Embedding API 401 / 429 / 5xx | Surface in status bar; abort current sweep; user sees error in `errMsg`. Next `fileChangedMsg` retries that file. |
| DB write fails | Same — log to `errMsg`, abort current file's transaction, continue. |
| Ollama not running | Same as 5xx (connection refused). |
| Embedder config missing on startup | `indexer == nil`; feature disabled until config completes. Search/Ask key handlers show config error. |

## Semantic Search Modal (`Ctrl+Shift+F`)

A new `inputMode` value, full-screen modal in clipad's existing pattern.

### Trigger

`Ctrl+Shift+F` from any non-input state (top-level `Update` switch). If `m.indexer == nil`, set `errMsg = "Configure embedding_provider in config.toml"` and bail.

### State on `model`

```go
vaultSearchInput   textinput.Model
vaultSearchResults []searchResult       // {path, startLine, endLine, snippet, score}
vaultSearchCursor  int
vaultSearchOffset  int
vaultSearchPending bool                 // request in flight
vaultSearchCancel  context.CancelFunc
vaultSearchToken   int64                // monotonic; gates stale results
```

### Result struct

```go
type searchResult struct {
    Path      string   // relative to vault
    StartLine int
    EndLine   int
    Score     float32
    Snippet   string   // first 2 wrapped lines of chunk text
}
```

### View

```
┌─ Vault Search ──────────────────────────────────── Esc ┐
│ > how do I configure git remote_                       │
│                                                        │
│ ❯ docs/git-sync.md  L42-L51   (0.87)                   │
│   ...the remote is configured via `git remote add`...  │
│                                                        │
│   notes/setup.md    L8-L14    (0.72)                   │
│   ...point your repo at any git remote and run...      │
│                                                        │
│   3 more results  ↓                                    │
│                                                        │
│ Enter: open · ↑↓: navigate · Esc: close                │
└────────────────────────────────────────────────────────┘
```

- ~80% width, ~70% height, centered. Same lipgloss modal scaffold as help / plugin modals.
- Top: query input, focused on open. Placeholder `"Search note contents…"`.
- Body: ranked list. Each result is 3 lines: header (`path  Lstart-Lend  (score)`), 2-line snippet truncated to width-2. Cursor (`❯`) on selected. Auto-scrolls to keep cursor visible (`filterCursor` / `filterOffset` pattern).
- Footer: keybinding hint.

### Query execution (debounced, async)

- Each keystroke increments `vaultSearchToken`, cancels any in-flight request via `vaultSearchCancel`, schedules a 200ms-debounced `searchVaultCmd(token, query, k=8)`.
- The cmd: `qvec ← embedder.Embed(query)`, `top8 ← idx.Search(qvec)`. Returns `vaultSearchResultsMsg{token, results, err}`.
- Handler:

```go
case vaultSearchResultsMsg:
    if msg.token != m.vaultSearchToken { return m, nil }   // stale
    m.vaultSearchPending = false
    if msg.err != nil { m.errMsg = "Search: " + msg.err.Error(); return m, nil }
    m.vaultSearchResults = msg.results
    m.vaultSearchCursor = 0
    return m, nil
```

Token-based stale guard mirrors the `activeChunks` pattern from Task 23.

### Snippet generation

`chunk.text` truncated to 2 lines × `(modalWidth-2)` chars; preserves newlines, replaces tabs with spaces.

### Key bindings

| Key | Action |
|---|---|
| typing | input + debounced re-search |
| `↑` / `↓` | move cursor in results |
| `Enter` | close modal, open `Path`, scroll editor cursor to `StartLine` |
| `Esc` | close modal, restore previous state |
| `Ctrl+Q` | existing dirty-guard / quit |

### Jump to chunk

On Enter: `m.inputMode = inputNone`, `m.openFile(absPath)`, then `editor.MoveCursorTo(StartLine, 0)` (existing helper used by replace flow). If a different file is opened, `openFile` runs the unsaved-changes guard first; cursor jump runs after the file loads.

## Chat Panel (`Ctrl+Shift+A`)

Right-side split panel; the layout becomes 3-pane: `tree | editor | chat`. Diff view (`inputPluginDiff`) coexists — they occupy independent regions.

### Layout (`recalcLayout`)

```
treeHidden=false, chatOpen=false  →  tree(20) │ editor(rest)
treeHidden=false, chatOpen=true   →  tree(20) │ editor(rest)         │ chat(40)
treeHidden=true,  chatOpen=true   │           editor(rest)           │ chat(40)
```

`chatWidth = max(40, screenWidth*0.3)`; minimum 30 columns or panel doesn't open and shows `errMsg = "Window too narrow for chat panel"`.

### State on `model`

```go
chatOpen           bool
chatWidth          int
chatMode           chatModeT          // chatModeInput | chatModeView
chatTurns          []chatTurn         // process-scoped; persists across panel open/close, cleared on clipad exit
chatInput          textinput.Model
chatViewport       viewport.Model
chatStreaming      bool
chatActiveChunks   <-chan string      // stale guard
chatCancel         context.CancelFunc
chatCurrentCites   []citation         // for the in-flight assistant turn
```

```go
type chatTurn struct {
    Role      string       // "user" | "assistant"
    Content   string
    Citations []citation   // populated only for assistant turns
}
type citation struct {
    Path      string  // relative to vault
    StartLine int
    EndLine   int
}
```

### Trigger

`Ctrl+Shift+A`:

- `m.indexer == nil` → set `errMsg = "Configure embedding_provider in config.toml"`; bail.
- `chatOpen == false` → set `chatOpen = true`, `chatMode = chatModeInput`, focus `chatInput`, recalc layout, build viewport.
- `chatOpen == true` → close (cancel any in-flight stream first), recalc layout. `chatTurns` is preserved so the next reopen shows the same scrollback; only clipad shutdown clears it.

### Send flow (Enter in `chatInput`)

1. Read `query := chatInput.Value()`, clear input.
2. Append `{Role: "user", Content: query}` to `chatTurns`.
3. Retrieve top-8 chunks: `chunks, _ := idx.Search(ctx, query, 8)`. Build `m.chatCurrentCites` from those (in score order).
4. Compose messages:
   - `system`: contains the retrieved chunk excerpts, numbered, with citation instruction.
   - Last 4 user/assistant pairs from `chatTurns` (excluding the brand-new user turn).
   - `user`: the current query.
5. Append placeholder `{Role: "assistant", Content: ""}` to `chatTurns`.
6. `ctx, cancel := context.WithCancel(...)`; `m.chatCancel = cancel`; `m.chatStreaming = true`.
7. Call `streamChatCompletion(ctx, providerURL, apiKey, chatModelFromActiveProvider, systemPrompt, userMessage)`. The chat provider is the `ai_shortcut_provider` value (Blackbox by default), reusing its plugin config.
8. Return `streamChatCmd(chunks, errs)`.

### System prompt template (`ask.go`)

```
You are answering questions using the user's personal note vault as context.
Below are relevant excerpts. Cite sources inline using their numbered tag,
e.g., [1], [2]. If the excerpts do not contain the answer, say so plainly
rather than guessing.

[1] docs/git-sync.md L42-L51:
<chunk text>

[2] notes/setup.md L8-L14:
<chunk text>
...
```

### Streaming messages (separate from plugin streaming)

```go
type chatChunkMsg struct{ chunks <-chan string; errs <-chan error; delta string }
type chatDoneMsg  struct{ chunks <-chan string }
type chatErrMsg   struct{ chunks <-chan string; err error }
```

### Handlers

```go
case chatChunkMsg:
    if msg.chunks != m.chatActiveChunks { return m, nil }
    last := &m.chatTurns[len(m.chatTurns)-1]
    last.Content += msg.delta
    m.refreshChatViewport()
    return m, readNextChatChunk(msg.chunks, msg.errs)

case chatDoneMsg:
    if msg.chunks != m.chatActiveChunks { return m, nil }
    m.chatStreaming = false
    m.chatActiveChunks = nil
    last := &m.chatTurns[len(m.chatTurns)-1]
    last.Citations = m.chatCurrentCites
    m.chatCurrentCites = nil
    m.refreshChatViewport()
    return m, nil

case chatErrMsg:
    if msg.chunks != m.chatActiveChunks { return m, nil }
    m.chatStreaming = false
    last := &m.chatTurns[len(m.chatTurns)-1]
    last.Content = "Error: " + msg.err.Error()
    m.refreshChatViewport()
    return m, nil
```

### Two-mode key handling

| Mode | Key | Action |
|---|---|---|
| input | type | textinput edits |
| input | `Enter` | send (steps 1–8) |
| input | `Esc` | switch to view mode; if streaming, cancel via `chatCancel()` |
| view | `↑/↓` `k/j` | scroll viewport |
| view | `1`–`9` | open citation N of the *most recent assistant turn*: `m.openFile(cite.Path)`, jump to `cite.StartLine` |
| view | `i` or `/` | return to input mode |
| view | `Esc` | close chat panel (`chatOpen = false`) |
| any | `Tab` | cycles tree → editor → chat |
| any | `Ctrl+Shift+A` | toggle close |
| any | `Ctrl+Q` | existing dirty-guard / quit |

### Render (`chat.go`)

```
┌─ Ask your vault ──────────────────────────────── Esc ┐
│ ▸ You: how do I configure git remote?                │
│                                                      │
│ clipad: Set the remote with `git remote add` from    │
│ inside the vault directory; clipad reads             │
│ git_remote from config and runs sync via [1].        │
│                                                      │
│   [1] docs/git-sync.md L42-L51                       │
│   [2] notes/setup.md   L8-L14                        │
│ ─────────────────────────────────                    │
│ ▸ You: ...                                           │
├──────────────────────────────────────────────────────┤
│ > _                                                  │
└──────────────────────────────────────────────────────┘
   1-9: open citation · i: input · ↑↓: scroll · Esc: close
```

The chat input (bottom) and viewport (top, scrollable) are stacked. Streaming renders character-by-character into the in-flight assistant turn; viewport `GotoBottom` on each chunk.

### Cancellation

`Esc` while streaming aborts the stream context. The partial assistant turn becomes a final turn with whatever content arrived so far — not deleted, since users may still want it. The next message picks up cleanly.

## Files Touched

| File | Change |
|---|---|
| `index.go` | **NEW.** SQLite open/migrate, `chunkFile`, `Index.RebuildFile`, `Index.RemoveFile`, `Index.Search`, cosine top-K, schema/migrations. |
| `embeddings.go` | **NEW.** `EmbeddingClient` interface, `OpenRouterEmbeddings`, `OllamaEmbeddings`, `newEmbeddingClient(cfg)`. |
| `ask.go` | **NEW.** System-prompt builder, `composeChatRequest(turns, query, chunks)`, last-4-pairs message array helper. |
| `chat.go` | **NEW.** Chat panel rendering (lipgloss), viewport refresh, citation render, two-mode key handling. |
| `vault_search.go` | **NEW.** Search modal rendering, debounced search command, result-list rendering. |
| `config.go` | Add `EmbeddingProvider`, `EmbeddingModel`, `OllamaURL` fields + defaults. |
| `model.go` | Add indexer/search/chat state fields; `Ctrl+Shift+F` and `Ctrl+Shift+A` shortcuts; `recalcLayout` for 3-pane; `Update` handlers for new messages; `View` integrates chat panel + status bar `[idx N/M]`. |
| `watcher.go` | Distinguish create/write from delete; emit `fileDeletedMsg{path}` for removes. |
| `statusbar.go` | Render `[idx N/M]` indexer status when non-empty. |
| `help_modal.go` | Document `Ctrl+Shift+F` and `Ctrl+Shift+A`. |
| `main.go` | Construct `Index` after `newModel`; pass into model; defer `Index.Close()` on shutdown. |
| `go.mod` | Add `modernc.org/sqlite`. |

## Testing

Unit tests live alongside their code (matching project's `*_test.go` pattern).

| Test file | Scope |
|---|---|
| `index_test.go` | `chunkFile` table tests (paragraph splits, oversize splits, unicode, edge cases — empty file, single line, only whitespace, single huge paragraph). `Index.RebuildFile` against `:memory:` SQLite: insert N chunks, modify one paragraph in source, assert exactly one new row inserted + correct rows deleted. `Index.Search` correctness with synthetic embeddings (build a tiny vector space by hand). `Index.RemoveFile` deletes all rows for a path. |
| `embeddings_test.go` | `OpenRouterEmbeddings` against `httptest.NewServer`: batching (101 inputs → 2 calls), auth header, error response. `OllamaEmbeddings` against `httptest.NewServer`: per-text loop, response shape, connection-refused fallback. `newEmbeddingClient` with each `EmbeddingProvider` value, including unknown → error. |
| `ask_test.go` | `composeChatRequest`: bounded history (8 pairs in → only last 4 in messages), system prompt formatting, citation numbering matches chunk order. |
| `vault_search_test.go` | Debounce token: simulate fast typing → only the last token's results applied. Stale results dropped. Snippet formatting (truncation, newline preservation). |
| `chat_test.go` | Two-mode key handling: input vs view mode transitions, `1`–`9` jump-to-citation correctly resolves Path/StartLine, `Esc` while streaming cancels via `chatCancel`. Streaming append into `chatTurns[-1].Content`, citations attached on `chatDoneMsg`. |
| `watcher_test.go` (extend) | `Remove` events emit `fileDeletedMsg`; create/write still emit `fileChangedMsg`. |

End-to-end test (one): `TestVaultSearch_HappyPath` builds a temp vault with three notes, mocks the embedding client to return deterministic vectors, runs `Search("alpha")`, asserts the right note ranks first.

## Out of Scope (YAGNI)

- Vector index (HNSW/IVF). Brute-force is fine until vaults exceed ~50K chunks.
- Cross-device index sync. Each device rebuilds; cost is ~$0.001 for a 1000-chunk vault.
- Re-embedding the whole vault on model change — handled lazily on next sweep via `WHERE model = ?` (rows with stale model are skipped on read; new chunks insert with the new model name; manual reset is a future feature).
- Embeddings of non-`.md` files.
- Hybrid keyword+semantic search.
- Saving / exporting chat sessions; multiple concurrent chat sessions.
