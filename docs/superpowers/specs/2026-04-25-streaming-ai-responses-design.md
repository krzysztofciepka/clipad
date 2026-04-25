# Streaming AI Responses Into the Diff View — Design

**Date:** 2026-04-25
**Status:** Approved
**Source:** Task 23

## Goal

Replace the current blocking POST that backs `OpenRouterPlugin.Run`, `BlackboxPlugin.Run`, and `runShortcutCmd` with Server-Sent Events streaming. After the user picks a plugin or shortcut, the diff view opens immediately and the right pane fills progressively as tokens arrive. `Esc` cancels mid-stream and discards partial output.

## Non-Goals

- Changing the plugin selection UX, the prompt input flow, or the diff accept/reject keys.
- Persisting partial streams or offering "keep partial as result" prompts.
- Streaming for any provider not already supported (Blackbox, OpenRouter).

## Architecture

```
┌─────────────────────────────┐
│   model.go (Bubble Tea)     │
│  - pluginCancel CancelFunc  │
│  - pluginChunkMsg loop      │
│  - opens diff immediately   │
└──────────┬──────────────────┘
           │ tea.Cmd reads from chunks chan
           ▼
┌─────────────────────────────┐    ┌─────────────────────────────┐
│ plugin_openrouter.go        │    │ plugin_blackbox.go          │
│ Run(ctx, ...)               │    │ Run(ctx, ...)               │
│  → streamChatCompletion     │    │  → streamChatCompletion     │
└──────────┬──────────────────┘    └──────────┬──────────────────┘
           │                                  │
           └──────────────┬───────────────────┘
                          ▼
        ┌─────────────────────────────────────┐
        │ plugin_stream.go (NEW)              │
        │ streamChatCompletion(ctx, url, ...) │
        │  → (<-chan string, <-chan error)    │
        │  - SSE parser                       │
        │  - Content-Type fallback            │
        │  - http.NewRequestWithContext       │
        └─────────────────────────────────────┘
```

## Plugin Interface

`Plugin.Run` becomes streaming-only. There is no separate `RunStream`; the single signature carries both streaming and (internally fallback) blocking semantics.

```go
type Plugin interface {
    Name() string
    Description() string
    ConfigFields() []ConfigField
    Run(ctx context.Context, content, prompt string, cfg map[string]string) (<-chan string, <-chan error)
}
```

Each provider's `Run` is a thin wrapper that selects the URL, builds the system+user messages, and delegates to the shared SSE primitive:

```go
func (p *OpenRouterPlugin) Run(ctx context.Context, content, prompt string, cfg map[string]string) (<-chan string, <-chan error) {
    url := p.BaseURL
    if url == "" {
        url = defaultOpenRouterURL
    }
    sys := "You are a note editor. Apply the following transformation to the note provided by the user. Return only the transformed note content, no explanations."
    user := fmt.Sprintf("Instruction: %s\n\nNote:\n%s", prompt, content)
    return streamChatCompletion(ctx, url, cfg["api_key"], cfg["model"], sys, user)
}
```

`BlackboxPlugin.Run` is analogous (different default URL, same system prompt as today).

## Shared SSE Primitive (`plugin_stream.go`)

```go
func streamChatCompletion(ctx context.Context, url, apiKey, model, systemPrompt, userMessage string) (<-chan string, <-chan error)
```

**Behavior:**

1. Builds a chat-completion request body with `"stream": true` and the standard messages array.
2. Issues `http.NewRequestWithContext(ctx, "POST", url, ...)` with `Authorization: Bearer <apiKey>`, `Content-Type: application/json`, `Accept: text/event-stream`.
3. No `http.Client.Timeout` — context handles cancellation, and a streaming response can take minutes.
4. On non-2xx response: read body (truncated to 200 chars), send error to `errs`, close both channels, return.
5. On 2xx with `Content-Type` not starting with `text/event-stream`: parse body as the regular blocking response shape (`choices[0].message.content`), emit the full content as a single chunk, close both channels.
6. On 2xx SSE: read line-by-line with `bufio.Scanner`. Skip blank lines and lines starting with `:` (comment / keep-alive). On `data: [DONE]`, close cleanly. On `data: {json}`, unmarshal and extract `choices[0].delta.content`; if non-empty, send to `chunks`. On malformed JSON in a single frame, skip and continue.
7. All sends respect `ctx.Done()`:

   ```go
   select {
   case chunks <- delta:
   case <-ctx.Done():
       return
   }
   ```

   This is required to avoid deadlocking the goroutine after cancellation if the chunk consumer has already moved on.
8. The `errs` channel is buffered (size 1) so the goroutine can `errs <- err; return` without blocking on a non-listening reader.

The default `bufio.Scanner` buffer is large enough for individual SSE frames in practice; if a provider sends very long deltas this can be revisited, but it is YAGNI for now.

## Shortcut Wiring

`runShortcutCmd` parallels `Plugin.Run` rather than going through it (the only difference is the system prompt, and routing through the plugin would require smuggling the prompt through the interface). It now returns raw channels rather than a `tea.Cmd`:

```go
func runShortcutCmd(ctx context.Context, shortcut AIShortcut, content, provider string, cfg map[string]string) (<-chan string, <-chan error) {
    sys := "You are a text processing assistant. Apply the following instruction to the provided text. Return ONLY the processed text, nothing else."
    user := fmt.Sprintf("Instruction: %s\n\nText:\n%s", shortcut.Prompt, content)
    var url string
    switch provider {
    case "openrouter":
        url = defaultOpenRouterURL
    default:
        url = defaultBlackboxURL
    }
    return streamChatCompletion(ctx, url, cfg["api_key"], cfg["model"], sys, user)
}
```

The Bubble Tea wiring (next section) consumes channels from either path identically.

`callOpenRouter` and `callBlackbox` are deleted. They have no remaining callers — both plugin runs and shortcut runs go through `streamChatCompletion`, and the fallback path is internal to that function.

## Bubble Tea Wiring

### Messages (in `plugin.go`)

```go
type pluginChunkMsg struct {
    chunks <-chan string
    errs   <-chan error
    delta  string
}
type pluginDoneMsg struct{}
type pluginErrMsg  struct{ err error }
```

`pluginResultMsg` and `shortcutResultMsg` are removed; both paths now use the same three messages.

### Drain Loop

```go
func streamPluginCmd(chunks <-chan string, errs <-chan error) tea.Cmd {
    return readNextChunk(chunks, errs)
}

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
```

### Stream Start

In `handlePluginPrompt` (and the shortcut paths in `handlePluginConfig` / `handleShortcutSelect`):

```go
ctx, cancel := context.WithCancel(context.Background())
m.pluginCancel = cancel
m.pluginProcessing = true
m.pluginDiffOriginal = content
m.pluginDiffResult = ""
m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(content, "", m.editorWidth, m.editorHeight)
m.inputMode = inputPluginDiff   // open diff view immediately
chunks, errs := pluginInst.Run(ctx, content, prompt, cfg)
m.activeChunks = chunks         // identity used to discard trailing messages from superseded streams
return m, streamPluginCmd(chunks, errs)
```

### Stale-message guard

Trailing messages from a cancelled or completed stream can arrive after the model has already cleaned up — or, in the edge case where the user starts a second stream immediately, while a *different* stream is now active. Comparing `pluginProcessing` alone is insufficient because it would let an old chunk leak into a new diff.

Channel identity is the correct discriminator. The model gains:

```go
activeChunks <-chan string  // set at stream start; nil when no stream active
```

It is set when the stream starts (alongside `pluginCancel`), and cleared on stream end / cancel / error. Each handler checks `msg.chunks == m.activeChunks` before acting.

### Update Handlers

```go
case pluginChunkMsg:
    if msg.chunks != m.activeChunks {
        return m, nil // stale: superseded or cancelled stream
    }
    m.pluginDiffResult += msg.delta
    rightWidth := /* same calculation as newDiffViewports right pane */
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
    // otherwise: diff stays open, user reviews and accepts/rejects via existing handlePluginDiff
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

`pluginDoneMsg` and `pluginErrMsg` therefore also carry the `chunks` channel for identity comparison:

```go
type pluginDoneMsg struct{ chunks <-chan string }
type pluginErrMsg  struct{ chunks <-chan string; err error }
```

`readNextChunk` populates this field on every emitted message.

The empty-result check (`m.pluginDiffResult == ""`) covers the case where a stream was cancelled before any content arrived but the goroutine still produced a clean `pluginDoneMsg`.

## Cancellation & Key Gating

The current `model.go` short-circuits all key messages when `pluginProcessing` is true:

```go
case tea.KeyMsg:
    if m.pluginProcessing { return m, nil }
```

This must be changed so `Esc` and `Ctrl+Q` reach the model:

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
            // existing dirty-guard / quit logic
            if m.isDirty() {
                m.inputMode = inputUnsavedGuard
                m.pendingAction = pendingQuit
                return m, nil
            }
            return m, tea.Quit
        }
        return m, nil
    }
    // ...rest unchanged
```

**Mouse messages** are still dropped while streaming (same as today).

**Manual scrolling (`up`/`down`/`k`/`j`) is disabled while streaming.** This is a consequence of the auto-scroll-only choice: any user attempt to scroll mid-stream would be undone on the next chunk's `GotoBottom`. Once the stream completes (`pluginProcessing = false`), the existing `handlePluginDiff` resumes accepting `y/n/up/down/k/j/esc`.

**Cancellation flow ordering:**

1. User hits `Esc`. The key-msg branch above runs `pluginCancel()` and clears model state synchronously.
2. `streamChatCompletion`'s in-flight HTTP read aborts with `context.Canceled`.
3. The goroutine sends the error to the buffered `errs` channel and returns. Both channels close.
4. The recursive `readNextChunk` `tea.Cmd` is still in flight. When it fires, it produces `pluginDoneMsg` or `pluginErrMsg`.
5. The handlers above see `m.pluginProcessing == false` and return early without touching state.

This is why the "stale message" guards in the chunk/done/err handlers are mandatory: the goroutine does not drive cleanup; the Esc handler does, and trailing messages from the drain goroutine arrive after the fact.

## Error Handling

| Source | Behavior |
|---|---|
| `http.Do` network failure | `errs <- err`; channels close; model shows "Plugin error: ..." and resets. |
| Non-2xx response | Read body (truncated to 200 chars), `errs <- fmt.Errorf("API error (HTTP %d): %s", ...)`; same reset path. |
| Malformed JSON in a single SSE frame | Skip and continue parsing. Tolerance is the spec. |
| `ctx.Canceled` mid-stream | Goroutine exits cleanly. Model already cleaned up; trailing message ignored via stale guard. |
| `data: [DONE]` | Goroutine exits cleanly; channels close; `pluginDoneMsg` fires; diff stays open for review. |
| 200 + non-SSE Content-Type | Parse as blocking `{choices[0].message.content}`; emit one chunk; close. |

## Testing

### New: `plugin_stream_test.go`

Targets the SSE parser directly via `httptest.NewServer`. Tests:

1. **Multi-chunk happy path** — server flushes
   ```
   data: {"choices":[{"delta":{"content":"Hello"}}]}\n\n
   data: {"choices":[{"delta":{"content":" world"}}]}\n\n
   data: [DONE]\n\n
   ```
   Assert: drained `chunks` concatenates to `"Hello world"`; `errs` closes without value.

2. **Cancel mid-stream** — server flushes one chunk, then blocks on a sentinel. Caller cancels ctx. Assert: `chunks` closes, `errs` either closes or yields `context.Canceled`, and the server-side handler observes the request torn down (e.g., by checking `r.Context().Done()`).

3. **Non-streaming fallback** — server responds `Content-Type: application/json` with the regular blocking JSON shape. Assert: exactly one chunk equal to the full content; `errs` closes clean.

4. **Malformed frame tolerance** — body contains one valid frame, one frame with broken JSON, one valid frame, then `[DONE]`. Assert: `chunks` = concatenation of the two valid deltas; no error.

5. **Auth error (non-2xx)** — server returns 401 with body. Assert: `errs` receives an error containing the body snippet; no chunks produced.

6. **Comment / keep-alive line skipping** — body interleaves `: keep-alive\n\n` between data frames. Assert: no spurious chunks; normal output.

### Updated: `plugin_blackbox_test.go` and `plugin_openrouter_test.go`

The existing `Run_Success` / `Run_Error` / `Run_EmptyChoices` tests are rewritten to drain the new channel-returning `Run`. Each provider keeps a smoke test (one happy-path streaming run) verifying URL, auth header, model name, and request body shape. Deeper SSE coverage lives in `plugin_stream_test.go` to avoid duplicating identical behavior across providers.

## Files Touched

| File | Change |
|---|---|
| `plugin.go` | New `Plugin.Run` signature; new `pluginChunkMsg` / `pluginDoneMsg` / `pluginErrMsg`; remove `pluginResultMsg`; replace `runPluginCmd` with `streamPluginCmd` + `readNextChunk`. |
| `plugin_stream.go` | **New.** `streamChatCompletion`, SSE parser, Content-Type fallback. |
| `plugin_openrouter.go` | `Run` becomes a thin wrapper around `streamChatCompletion`; delete `callOpenRouter`. |
| `plugin_blackbox.go` | `Run` becomes a thin wrapper around `streamChatCompletion`; delete `callBlackbox`; keep `truncate` (still used by the SSE primitive). |
| `shortcuts.go` | `runShortcutCmd` returns channels; remove `shortcutResultMsg`. |
| `plugin_input.go` | At stream start: create context, store `pluginCancel`, build empty diff viewports, set `inputMode = inputPluginDiff`, return `streamPluginCmd`. |
| `model.go` | Add `pluginCancel context.CancelFunc` and `activeChunks <-chan string` fields; rewrite key gating in `tea.KeyMsg` branch to allow Esc/Ctrl+Q during streaming; replace `pluginResultMsg` / `shortcutResultMsg` handlers with `pluginChunkMsg` / `pluginDoneMsg` / `pluginErrMsg`. |
| `plugin_stream_test.go` | **New.** SSE parser tests. |
| `plugin_blackbox_test.go` | Rewrite to drain channels; keep one streaming smoke test. |
| `plugin_openrouter_test.go` | Rewrite to drain channels; keep one streaming smoke test. |
