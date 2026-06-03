# Vault Agent — Design Spec (Task 20)

**Date:** 2026-06-03
**Status:** Approved, ready for implementation planning

## Summary

Turn the existing Ctrl+K "ask your vault" chat panel into a conversational **AI
agent** that both answers questions about the user's notes and *manages* them by
running shell commands. The agent uses native OpenAI-compatible tool-calling
against the user's configured provider (blackbox.ai by default). It exposes two
tools — `search_vault` (semantic retrieval) and `bash` (run a command in the
vault). The same change fixes a long-standing bug where the embeddings index
keeps chunks for deleted notes and cites files that no longer exist.

## Goals

- A continuous, multi-turn agent conversation in the existing right-hand panel,
  opened with the existing **Ctrl+K** keybinding.
- The agent can manage the notes directory and individual note contents by
  running bash commands (`cd`, `mv`, `cp`, `cat`, `sed`, `awk`, etc.).
- The agent can answer questions about the notes via semantic search, which
  becomes one of its tools rather than a hard-wired one-shot flow.
- Fix stale embeddings: never retrieve or cite chunks for files that no longer
  exist on disk.

## Non-goals (YAGNI)

- Token-by-token streaming of the final answer (per-turn non-streaming is fine).
- Per-command confirmation prompts (commands auto-run, scoped to the vault).
- Auto-reconcile of the index after file ops / a `/reindex` rebuild command
  (can be added later; out of scope for the MVP).
- Tools beyond `search_vault` and `bash`.
- Conversation persistence across app restarts.
- Conversation summarization / token-window compaction.

## Background: current state

- **Provider plumbing.** `loadPluginConfig(provider)` yields `api_key` + `model`;
  the URL is `defaultBlackboxURL` or `defaultOpenRouterURL`. The active provider
  is `m.activeShortcutProvider` (persisted as `ai_shortcut_provider`).
- **Streaming.** `streamChatCompletion(ctx, url, key, model, system, user)` in
  `plugin_stream.go` is text-only (system + user), no tool support. It stays as
  is for AI shortcuts/plugins.
- **Existing chat panel.** `chat.go` + `model.go` `handleChatPanel`: a one-shot
  RAG flow — `chatStartCmd` retrieves top-K chunks, `composeChatRequest` /
  `encodeChatHistory` pack them into a single system+user request, the answer
  streams back, citations render and `1`–`9` open them. Opening the panel
  currently requires `m.indexer.embedder != nil`.
- **Index.** `index.go`: SQLite chunks table keyed by `file_path` (vault-
  relative). `Search` does cosine top-K. `RebuildFile` re-embeds a single file.
  `RemoveFile` deletes a file's chunks. The startup sweep (`startInitialIndex` →
  `RebuildFile` per `.md`) and in-app delete/rename hooks (`removeFileFromIndexCmd`
  / `reindexFileCmd`) are the **only** things that touch the index. Nothing prunes
  chunks for files deleted outside clipad — orphans accumulate. This worsens once
  the agent itself renames/removes files via `bash`.
- **Tool-calling support.** Verified (blackbox.ai docs): the API is OpenAI-
  compatible with full function/tool calling and JSON mode. Native tool-calling
  is viable; no prompt-protocol fallback needed.

## Architecture

### Panel = agent (always agentic)

The Ctrl+K panel hosts the agent. Every user message runs through a tool-calling
loop; the model decides whether to call `search_vault`, `bash`, or just answer.
The panel opens regardless of embedder configuration (file ops work without
embeddings; `search_vault` reports a clear error if embeddings are unconfigured).

The old one-shot retrieval flow is removed: `chatStartCmd`, `composeChatRequest`,
and `encodeChatHistory` are replaced by the agent loop and the `search_vault`
tool. Citation rendering and the `1`–`9` open-citation UX are retained, sourced
from `search_vault` tool results.

### Conversation state

A real OpenAI messages array held for the session:

- `system` — the agent system prompt (below).
- `user` — each user message.
- `assistant` — model replies, including `tool_calls`.
- `tool` — tool results (keyed by `tool_call_id`).

Held in memory for the session. `/clear` resets to just the system message.
Reuses the active provider's `api_key`, `model`, and URL.

### Tools

**`search_vault(query: string, k?: integer)`**
- Default `k = 8`.
- **Prunes orphans first** (see Index Freshness), then `idx.Search(query, k)`.
- Returns a structured result the model can cite: for each hit, an index tag,
  `path`, `start_line`–`end_line`, and `text`.
- If `idx.embedder == nil`: returns a tool error string ("semantic search is not
  configured; set embedding_provider in config.toml") so the agent can continue
  with bash. The loop does not abort.
- Hits are recorded as `citation{Path,StartLine,EndLine}` on the assistant turn
  so `1`–`9` still opens them.

**`bash(cmd: string)`**
- Runs `bash -c <cmd>` with **cwd = vault**, **stdin = /dev/null**, **30s
  timeout**.
- `vaultGuard(vault, cmd)` runs first; on rejection the tool returns an error
  string (the loop continues, the model sees why it was blocked).
- Combined stdout+stderr captured and **truncated to ~4 KB** before returning to
  the model; the exit code is included.
- Rendered inline in the scrollback: `$ <cmd>` then `✓ (exit 0)` or the output /
  a `✗ (exit N)` marker.

### The loop

`runAgentTurn` (in `agent_api.go`) performs one non-streaming POST with the full
messages array plus the `tools` schema, and returns the assistant message
(content and/or `tool_calls`).

The session loop (a `tea.Cmd` goroutine, cancellable via context on Esc) emits
events over a channel for live UI updates:

1. POST messages + tools → assistant message.
2. If it has `tool_calls`: for each, emit `toolStart`, execute the tool, append a
   `tool` message, emit `toolResult`. Loop to 1.
3. If it has content and no tool calls: emit it as the final answer; done.
4. **Hard cap: 25 tool calls per user message.** On reaching it, stop with a
   notice appended to the transcript.

Events: `assistantText`, `toolStart{name,cmd}`, `toolResult{ok,output}`,
`done`, `error`. The model's `Update` applies each event, re-renders the
viewport, and reads the next event (mirroring the existing chat-stream pattern).

### vaultGuard (best-effort scoping)

`cwd = vault` is the primary mechanism. `vaultGuard` is a guardrail against
accidents, **not** a security sandbox. It rejects, before running:

- `sudo` (any token).
- Absolute paths not under the vault root.
- `..` traversal that escapes the vault root.

The spec is explicit that a determined command could still escape; the agent is
the user's own and the guard exists to contain stray operations to the vault.

### Index freshness — stale-embeddings fix

New method **`Index.PruneOrphans(ctx) (removed int, err error)`**:

- `SELECT DISTINCT file_path FROM chunks`.
- For each, `os.Stat(filepath.Join(vault, file_path))`; if it does not exist,
  `DELETE FROM chunks WHERE file_path = ?`.
- Returns the number of files pruned.

Called automatically at the start of every `search_vault` invocation. Cheap (one
stat per distinct file, no API calls). This guarantees the agent never retrieves
or cites a deleted note — the reported bug.

Out of scope (documented limitation): renamed/newly created notes are not
re-embedded until the next startup sweep or in-app edit. A manual `/reindex`
(prune + full rebuild) can be added later if desired.

### Slash commands

Parsed in the agent input before any request is sent:

- `/clear` — reset the conversation (keep only the system message); panel stays
  open.
- `/exit` — close the panel (same as Esc).
- `/model` — show the model the agent is currently using.
- `/help` — list the slash commands and a one-line description of the agent.
- Unknown `/x` — inline hint, no request sent.

### System prompt

Explains the agent's role and constraints:

- You manage the user's notes vault located at `<vault path>`.
- The working directory is the vault and you are confined to it.
- Use `search_vault` to answer questions about the notes; cite results by their
  numbered tag.
- Use `bash` (cd/mv/cp/cat/sed/awk, etc.) to inspect and modify files; prefer
  inspecting before destructive changes.
- Finish each task with a short plain-text summary of what you did.

## UI

The existing chat panel layout (right column, input/view modes) is reused. The
scrollback renders:

- `You: …` user turns.
- `clipad: …` assistant text.
- Inline tool activity for `bash`: `$ <cmd>` and `✓ (exit 0)` / output / `✗`.
- Citations under assistant turns (from `search_vault`), opened with `1`–`9`.

Per-turn requests are non-streaming, but tool/answer **events** stream into the
viewport so the user sees commands run live. Esc during a run cancels the
context and stops the loop.

## File layout

- `agent.go` — conversation types, system prompt builder, the session loop
  `tea.Cmd` + event channel, slash-command parsing, tool-activity rendering.
- `agent_exec.go` — `runBashInVault` + `vaultGuard` (pure, unit-testable).
- `agent_api.go` — `runAgentTurn`: tools-aware non-streaming completion (build
  the `tools` array, POST, parse `tool_calls` / content).
- `index.go` — add `PruneOrphans`.
- `chat.go` / `ask.go` — repurposed into the agent panel; obsolete one-shot
  retrieval (`chatStartCmd`, `composeChatRequest`, `encodeChatHistory`) removed;
  citation rendering and `1`–`9` retained.
- `model.go` — Ctrl+K opens the agent; handle agent events; drop the embedder
  gate on opening the panel.

## Testing (TDD)

- `vaultGuard`: accepts in-vault commands; rejects `sudo`, absolute paths outside
  the vault, `..` escapes.
- `runBashInVault`: runs in the vault cwd; enforces the timeout; truncates large
  output; reports exit code.
- `PruneOrphans`: removes chunks for missing files, keeps chunks for existing
  files, returns the correct count.
- `search_vault` tool: result formatting, citation extraction, graceful error
  when no embedder.
- Tool-call message assembly: `assistant` `tool_calls` + matching `tool` result
  messages are well-formed.
- Loop bound: stops at 25 tool calls.
- Slash-command parsing: `/clear`, `/exit`, `/model`, `/help`, unknown.
- Tool-activity rendering: bash command + output rendered as designed.

## Open risks

- **Guard is best-effort**, not a sandbox (documented above).
- **Token growth**: long sessions grow the messages array unbounded; mitigated by
  `/clear`. Compaction is out of scope.
- **Model capability**: the user's configured provider model must support
  tool-calling; blackbox.ai does. A weak model may loop or misuse tools; the
  25-call cap bounds the blast radius.
