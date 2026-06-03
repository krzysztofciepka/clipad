# Vault Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the Ctrl+K "ask your vault" panel into a continuous AI agent that answers questions about notes (via a `search_vault` tool) and manages them (via a `bash` tool), using native OpenAI-compatible tool-calling; and fix stale embeddings by pruning chunks for deleted files.

**Architecture:** A non-streaming, tools-aware completion call (`runAgentTurn`) drives an agentic loop in a goroutine. The loop emits UI events over a channel (mirroring the existing chat-stream pattern). Two tools: `search_vault` (prune orphans, then semantic search) and `bash` (run a command with cwd=vault, a best-effort escape guard, a timeout, and truncated output). The existing one-shot RAG flow is removed; the panel's conversation state becomes a real OpenAI messages array.

**Tech Stack:** Go, Charm Bubble Tea/Lipgloss/Glamour, `modernc.org/sqlite`, blackbox.ai / OpenRouter (OpenAI-compatible chat completions with tool-calling).

**Spec:** `docs/superpowers/specs/2026-06-03-vault-agent-design.md`

---

## File Structure

- **Create `agent_exec.go`** — `vaultGuard` + `runBashInVault` (pure, unit-testable shell execution scoped to the vault).
- **Create `agent_api.go`** — message/tool types, `agentTools()`, `runAgentTurn` (tools-aware non-streaming completion).
- **Create `agent.go`** — the agent loop (`runAgentLoop`), tool dispatch, event types, slash-command parsing, system prompt, `applyAgentEvent` (event → display turn), and Trace rendering.
- **Create `agent_exec_test.go`, `agent_api_test.go`, `agent_test.go`** — tests for the above.
- **Modify `index.go`** — add `Index.PruneOrphans`.
- **Modify `index_test.go`** — add `PruneOrphans` test.
- **Modify `chat.go`** — add `Trace` to `chatTurn`, render it; remove obsolete one-shot stream plumbing (Task 12).
- **Modify `ask.go`** — remove obsolete `composeChatRequest`/`lastPairs`/`buildSystemPrompt`/`chatMessage` (Task 12).
- **Modify `model.go`** — agent state fields; Ctrl+K opens the agent (drop embedder gate); replace the one-shot handlers with agent-event handling; Esc cancels.
- **Modify `chat_test.go`** — remove tests for deleted functions (Task 12).
- **Modify `README.md`** — document the agent.

Shared constants (declared in `agent.go`):
```go
const (
	maxToolCalls  = 25              // hard cap on tool calls per user message
	bashTimeout   = 30              // seconds; per-command wall clock
	maxToolOutput = 4096            // bytes; tool output truncated before returning to model
)
```

---

## Task 1: vaultGuard — best-effort escape guard

**Files:**
- Create: `agent_exec.go`
- Test: `agent_exec_test.go`

- [ ] **Step 1: Write the failing test**

Create `agent_exec_test.go`:

```go
package main

import "testing"

func TestVaultGuard_AllowsInVaultCommands(t *testing.T) {
	vault := "/home/u/vault"
	ok := []string{
		"ls",
		"mv 'Task 9.md' 'Task 1.md'",
		"cat ./notes/x.md",
		"sed -i 's/a/b/' notes/x.md",
		"cat /home/u/vault/a.md", // absolute, but inside vault
	}
	for _, c := range ok {
		if err := vaultGuard(vault, c); err != nil {
			t.Errorf("vaultGuard(%q) = %v, want nil", c, err)
		}
	}
}

func TestVaultGuard_RejectsEscapes(t *testing.T) {
	vault := "/home/u/vault"
	bad := []string{
		"sudo rm -rf x",
		"cat /etc/passwd",
		"cat ../../secret",
		"mv a.md ../outside.md",
		"cat ~/secrets",
	}
	for _, c := range bad {
		if err := vaultGuard(vault, c); err == nil {
			t.Errorf("vaultGuard(%q) = nil, want error", c)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestVaultGuard -v`
Expected: FAIL — `undefined: vaultGuard`.

- [ ] **Step 3: Write the minimal implementation**

Create `agent_exec.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var sudoRE = regexp.MustCompile(`(^|\s)sudo(\s|$)`)

// vaultGuard returns a non-nil error if cmd should not run because it could
// escape the vault or is disallowed. This is a best-effort guard against
// accidents — cwd=vault is the real scoping mechanism, and a determined
// command could still escape. It rejects: sudo, "~" references, absolute
// paths not under the vault, and ".." traversal that escapes the vault root.
func vaultGuard(vault, cmd string) error {
	if sudoRE.MatchString(cmd) {
		return fmt.Errorf("blocked: sudo is not allowed")
	}
	for _, tok := range strings.Fields(cmd) {
		t := strings.Trim(tok, "'\"")
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "~") {
			return fmt.Errorf("blocked: home (~) reference %q escapes the vault", t)
		}
		var abs string
		if filepath.IsAbs(t) {
			abs = filepath.Clean(t)
		} else if strings.Contains(t, "..") {
			abs = filepath.Clean(filepath.Join(vault, t))
		} else {
			continue
		}
		if !underDir(vault, abs) {
			return fmt.Errorf("blocked: path %q is outside the vault", t)
		}
	}
	return nil
}

// underDir reports whether path is the base dir or inside it.
func underDir(base, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestVaultGuard -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent_exec.go agent_exec_test.go
git commit -m "feat(agent): vaultGuard best-effort escape guard"
```

---

## Task 2: runBashInVault — scoped, timed shell execution

**Files:**
- Modify: `agent_exec.go`
- Test: `agent_exec_test.go`

- [ ] **Step 1: Write the failing test**

Append to `agent_exec_test.go`:

```go
import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunBashInVault_RunsInVaultCwd(t *testing.T) {
	vault := t.TempDir()
	out, code, err := runBashInVault(context.Background(), vault, "pwd", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out) != vault {
		t.Errorf("pwd = %q, want %q", strings.TrimSpace(out), vault)
	}
}

func TestRunBashInVault_CapturesStderrAndExitCode(t *testing.T) {
	vault := t.TempDir()
	out, code, err := runBashInVault(context.Background(), vault, "echo oops >&2; exit 3", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(out, "oops") {
		t.Errorf("output %q missing stderr", out)
	}
}

func TestRunBashInVault_TimesOut(t *testing.T) {
	vault := t.TempDir()
	_, code, err := runBashInVault(context.Background(), vault, "sleep 5", 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 {
		t.Errorf("timed-out command should not report exit 0")
	}
}

func TestRunBashInVault_TruncatesOutput(t *testing.T) {
	vault := t.TempDir()
	out, _, err := runBashInVault(context.Background(), vault, "yes x | head -c 100000", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > maxToolOutput+64 {
		t.Errorf("output not truncated: %d bytes", len(out))
	}
}
```

Note: the existing `import "testing"` line at the top of the file is replaced by this grouped import block. Keep a single import block in the file.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestRunBashInVault -v`
Expected: FAIL — `undefined: runBashInVault` and `undefined: maxToolOutput`.

- [ ] **Step 3: Write the minimal implementation**

`maxToolOutput` is declared in `agent.go` (Task 8); to let Tasks 2–7 build before Task 8, add the constants block now at the top of `agent_exec.go` and remove it from the Task 8 instructions if already present. Add to `agent_exec.go`:

```go
const (
	maxToolCalls  = 25 // hard cap on tool calls per user message
	maxToolOutput = 4096
	bashTimeout   = 30 // seconds; used as `bashTimeout * time.Second` in model wiring
)

// runBashInVault runs `bash -c cmd` with cwd=vault, no stdin, a wall-clock
// timeout, and combined stdout+stderr truncated to maxToolOutput bytes.
// Returns the (possibly truncated) output and the process exit code. err is
// non-nil only for failures to start/execute the process itself — a non-zero
// exit or a timeout is reported via the exit code, not err.
func runBashInVault(ctx context.Context, vault, cmd string, timeout time.Duration) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := exec.CommandContext(ctx, "bash", "-c", cmd)
	c.Dir = vault
	c.Stdin = nil // no stdin; reads return EOF immediately

	out, err := c.CombinedOutput()
	output := truncateBytes(out, maxToolOutput)

	if ctx.Err() == context.DeadlineExceeded {
		return output + "\n[timed out after " + timeout.String() + "]", 124, nil
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return output, ee.ExitCode(), nil
		}
		return output, -1, fmt.Errorf("run: %w", err)
	}
	return output, 0, nil
}

// truncateBytes returns b as a string, truncated to max bytes with a marker.
func truncateBytes(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "\n[output truncated]"
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestRunBashInVault -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent_exec.go agent_exec_test.go
git commit -m "feat(agent): runBashInVault scoped/timed shell execution"
```

---

## Task 3: Index.PruneOrphans — drop chunks for deleted files

**Files:**
- Modify: `index.go`
- Test: `index_test.go`

- [ ] **Step 1: Write the failing test**

Append to `index_test.go`:

```go
func TestPruneOrphans_RemovesChunksForMissingFiles(t *testing.T) {
	vault := t.TempDir()
	writeFile(t, vault, "alive.md", "para one.\n\npara two.")
	emb := onehotEmbedder("test-model", 8)
	idx, err := OpenIndex(":memory:", vault, emb)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	// Index both files, then delete one from disk.
	gone := writeFile(t, vault, "gone.md", "dead chunk.")
	if _, err := idx.RebuildFile(context.Background(), filepath.Join(vault, "alive.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.RebuildFile(context.Background(), gone); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	removed, err := idx.PruneOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("pruned files = %d, want 1", removed)
	}

	var goneCount, aliveCount int
	idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE file_path = 'gone.md'`).Scan(&goneCount)
	idx.db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE file_path = 'alive.md'`).Scan(&aliveCount)
	if goneCount != 0 {
		t.Errorf("gone.md chunks = %d, want 0", goneCount)
	}
	if aliveCount == 0 {
		t.Errorf("alive.md chunks = 0, want > 0")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestPruneOrphans -v`
Expected: FAIL — `idx.PruneOrphans undefined`.

- [ ] **Step 3: Write the minimal implementation**

Add to `index.go` (after `RemoveFile`):

```go
// PruneOrphans deletes all chunks whose file_path no longer exists under the
// vault on disk. Returns the number of distinct files pruned. Cheap: one stat
// per distinct file, no embedding calls.
func (idx *Index) PruneOrphans(ctx context.Context) (int, error) {
	rows, err := idx.db.QueryContext(ctx, `SELECT DISTINCT file_path FROM chunks`)
	if err != nil {
		return 0, fmt.Errorf("select distinct paths: %w", err)
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, err
		}
		paths = append(paths, p)
	}
	rows.Close()

	removed := 0
	for _, rel := range paths {
		if _, err := os.Stat(filepath.Join(idx.vault, rel)); os.IsNotExist(err) {
			if _, err := idx.db.ExecContext(ctx, `DELETE FROM chunks WHERE file_path = ?`, rel); err != nil {
				return removed, fmt.Errorf("delete %q: %w", rel, err)
			}
			removed++
		}
	}
	return removed, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestPruneOrphans -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add index.go index_test.go
git commit -m "feat(index): PruneOrphans drops chunks for deleted files"
```

---

## Task 4: Agent message + tool types and the tool schema

**Files:**
- Create: `agent_api.go`
- Test: `agent_api_test.go`

- [ ] **Step 1: Write the failing test**

Create `agent_api_test.go`:

```go
package main

import (
	"encoding/json"
	"testing"
)

func TestAgentTools_HasBashAndSearchVault(t *testing.T) {
	tools := agentTools()
	names := map[string]bool{}
	for _, tl := range tools {
		if tl.Type != "function" {
			t.Errorf("tool type = %q, want function", tl.Type)
		}
		names[tl.Function.Name] = true
	}
	for _, want := range []string{"bash", "search_vault"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

func TestAgentMessage_MarshalsToolCall(t *testing.T) {
	m := agentMessage{
		Role: "assistant",
		ToolCalls: []agentToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: agentToolFunction{Name: "bash", Arguments: `{"cmd":"ls"}`},
		}},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var round map[string]any
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatal(err)
	}
	if round["role"] != "assistant" {
		t.Errorf("role = %v", round["role"])
	}
	if _, ok := round["tool_calls"]; !ok {
		t.Errorf("tool_calls missing in %s", b)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run 'TestAgentTools|TestAgentMessage' -v`
Expected: FAIL — undefined `agentTools`, `agentMessage`, etc.

- [ ] **Step 3: Write the minimal implementation**

Create `agent_api.go`:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type agentMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []agentToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type agentToolCall struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function agentToolFunction `json:"function"`
}

type agentToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type agentTool struct {
	Type     string        `json:"type"`
	Function agentToolSpec `json:"function"`
}

type agentToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// agentTools returns the tool schema advertised to the model.
func agentTools() []agentTool {
	return []agentTool{
		{Type: "function", Function: agentToolSpec{
			Name:        "bash",
			Description: "Run a bash command in the notes vault (cwd is the vault root). Use for managing files: cd, ls, mv, cp, cat, sed, awk, etc. Confined to the vault.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cmd": map[string]any{
						"type":        "string",
						"description": "The bash command to run.",
					},
				},
				"required": []string{"cmd"},
			},
		}},
		{Type: "function", Function: agentToolSpec{
			Name:        "search_vault",
			Description: "Semantic search over the user's notes. Use to answer questions about note content. Returns numbered excerpts with file paths and line ranges to cite.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query.",
					},
					"k": map[string]any{
						"type":        "integer",
						"description": "Number of excerpts to return (default 8).",
					},
				},
				"required": []string{"query"},
			},
		}},
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run 'TestAgentTools|TestAgentMessage' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent_api.go agent_api_test.go
git commit -m "feat(agent): tool-calling message types and tool schema"
```

---

## Task 5: runAgentTurn — tools-aware non-streaming completion

**Files:**
- Modify: `agent_api.go`
- Test: `agent_api_test.go`

- [ ] **Step 1: Write the failing test**

Append to `agent_api_test.go`:

```go
import (
	"context"
	"net/http"
	"net/http/httptest"
)

func TestRunAgentTurn_ParsesToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"ls\"}"}}]}}]}`)
	}))
	defer server.Close()

	msg, err := runAgentTurn(context.Background(), server.URL, "k", "m",
		[]agentMessage{{Role: "user", Content: "list files"}}, agentTools())
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "bash" {
		t.Fatalf("got %+v, want one bash tool call", msg)
	}
}

func TestRunAgentTurn_ParsesContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"all done"}}]}`)
	}))
	defer server.Close()

	msg, err := runAgentTurn(context.Background(), server.URL, "k", "m",
		[]agentMessage{{Role: "user", Content: "hi"}}, agentTools())
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "all done" || len(msg.ToolCalls) != 0 {
		t.Fatalf("got %+v, want plain content", msg)
	}
}

func TestRunAgentTurn_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"bad key"}`)
	}))
	defer server.Close()

	_, err := runAgentTurn(context.Background(), server.URL, "bad", "m",
		[]agentMessage{{Role: "user", Content: "hi"}}, agentTools())
	if err == nil {
		t.Fatal("expected error on 401")
	}
}
```

Merge these imports into the file's existing import block (`encoding/json`, `testing` already present; add `context`, `fmt`, `net/http`, `net/http/httptest`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestRunAgentTurn -v`
Expected: FAIL — `undefined: runAgentTurn`.

- [ ] **Step 3: Write the minimal implementation**

Add to `agent_api.go`:

```go
// runAgentTurn performs one non-streaming chat completion with tools enabled
// and returns the assistant message (which may carry content, tool_calls, or
// both).
func runAgentTurn(ctx context.Context, url, apiKey, model string, messages []agentMessage, tools []agentTool) (agentMessage, error) {
	body, err := json.Marshal(map[string]any{
		"model":       model,
		"messages":    messages,
		"tools":       tools,
		"tool_choice": "auto",
		"stream":      false,
	})
	if err != nil {
		return agentMessage{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return agentMessage{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return agentMessage{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return agentMessage{}, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message agentMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return agentMessage{}, fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return agentMessage{}, fmt.Errorf("no choices in response")
	}
	return result.Choices[0].Message, nil
}
```

(`truncate` already exists in `plugin_stream.go`.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestRunAgentTurn -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent_api.go agent_api_test.go
git commit -m "feat(agent): runAgentTurn tools-aware completion call"
```

---

## Task 6: Tool argument parsing + tool dispatch

**Files:**
- Create: `agent.go`
- Test: `agent_test.go`

This task adds argument parsing and a `dispatchTool` function that executes one tool call and returns the tool-result string plus any citations. It does NOT yet add the loop (Task 8).

- [ ] **Step 1: Write the failing test**

Create `agent_test.go`:

```go
package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDispatchTool_Bash(t *testing.T) {
	vault := t.TempDir()
	deps := agentDeps{vault: vault, timeout: 5 * time.Second}
	out, cites, ok := dispatchTool(context.Background(), deps, "bash", `{"cmd":"echo hi"}`)
	if !ok {
		t.Errorf("ok = false, want true")
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("output %q missing 'hi'", out)
	}
	if cites != nil {
		t.Errorf("bash should produce no citations, got %v", cites)
	}
}

func TestDispatchTool_BashGuardRejects(t *testing.T) {
	vault := t.TempDir()
	deps := agentDeps{vault: vault, timeout: 5 * time.Second}
	out, _, ok := dispatchTool(context.Background(), deps, "bash", `{"cmd":"sudo rm x"}`)
	if ok {
		t.Errorf("guarded command should report ok=false")
	}
	if !strings.Contains(out, "blocked") {
		t.Errorf("output %q should explain the block", out)
	}
}

func TestDispatchTool_SearchVaultNoEmbedder(t *testing.T) {
	deps := agentDeps{idx: nil}
	out, _, ok := dispatchTool(context.Background(), deps, "search_vault", `{"query":"taxes"}`)
	if ok {
		t.Errorf("search without embedder should report ok=false")
	}
	if !strings.Contains(strings.ToLower(out), "not configured") {
		t.Errorf("output %q should explain missing config", out)
	}
}

func TestDispatchTool_SearchVaultReturnsCitations(t *testing.T) {
	vault := t.TempDir()
	writeFile(t, vault, "a.md", "Taxes are due in April.\n\nUnrelated paragraph.")
	emb := onehotEmbedder("test-model", 8)
	idx, err := OpenIndex(":memory:", vault, emb)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.RebuildFile(context.Background(), vault+"/a.md"); err != nil {
		t.Fatal(err)
	}
	deps := agentDeps{idx: idx, vault: vault}
	out, cites, ok := dispatchTool(context.Background(), deps, "search_vault", `{"query":"Taxes are due in April.","k":2}`)
	if !ok {
		t.Fatalf("ok=false, out=%q", out)
	}
	if len(cites) == 0 {
		t.Errorf("expected citations, got none")
	}
	if !strings.Contains(out, "a.md") {
		t.Errorf("output %q should reference a.md", out)
	}
}

func TestDispatchTool_UnknownTool(t *testing.T) {
	out, _, ok := dispatchTool(context.Background(), agentDeps{}, "nope", `{}`)
	if ok || !strings.Contains(out, "unknown tool") {
		t.Errorf("unknown tool should report ok=false with message, got ok=%v out=%q", ok, out)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestDispatchTool -v`
Expected: FAIL — undefined `agentDeps`, `dispatchTool`.

- [ ] **Step 3: Write the minimal implementation**

Create `agent.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// agentDeps bundles everything a tool call or the loop needs.
type agentDeps struct {
	url     string
	apiKey  string
	model   string
	vault   string
	idx     *Index
	timeout time.Duration
}

// dispatchTool executes a single tool call and returns the tool-result text
// (always non-empty, fed back to the model), any citations (search_vault only),
// and whether the tool succeeded. A failure (ok=false) still returns a message
// describing why so the model can recover.
func dispatchTool(ctx context.Context, deps agentDeps, name, argsJSON string) (string, []citation, bool) {
	switch name {
	case "bash":
		cmd, err := parseBashArgs(argsJSON)
		if err != nil {
			return "error: " + err.Error(), nil, false
		}
		if err := vaultGuard(deps.vault, cmd); err != nil {
			return err.Error(), nil, false
		}
		out, code, err := runBashInVault(ctx, deps.vault, cmd, deps.timeout)
		if err != nil {
			return "error: " + err.Error(), nil, false
		}
		return fmt.Sprintf("exit %d\n%s", code, out), nil, code == 0

	case "search_vault":
		query, k, err := parseSearchArgs(argsJSON)
		if err != nil {
			return "error: " + err.Error(), nil, false
		}
		if deps.idx == nil || deps.idx.embedder == nil {
			return "semantic search is not configured (set embedding_provider in config.toml)", nil, false
		}
		_, _ = deps.idx.PruneOrphans(ctx)
		results, err := deps.idx.Search(ctx, query, k)
		if err != nil {
			return "error: " + err.Error(), nil, false
		}
		return formatSearchResults(results)

	default:
		return "unknown tool: " + name, nil, false
	}
}

func parseBashArgs(argsJSON string) (string, error) {
	var a struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("bad bash arguments: %w", err)
	}
	if strings.TrimSpace(a.Cmd) == "" {
		return "", fmt.Errorf("bash: empty cmd")
	}
	return a.Cmd, nil
}

func parseSearchArgs(argsJSON string) (string, int, error) {
	var a struct {
		Query string `json:"query"`
		K     int    `json:"k"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", 0, fmt.Errorf("bad search arguments: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return "", 0, fmt.Errorf("search_vault: empty query")
	}
	if a.K <= 0 {
		a.K = 8
	}
	return a.Query, a.K, nil
}

// formatSearchResults renders hits as numbered excerpts and extracts citations.
func formatSearchResults(results []Result) (string, []citation, bool) {
	if len(results) == 0 {
		return "No matching notes found.", nil, true
	}
	var b strings.Builder
	cites := make([]citation, len(results))
	for i, r := range results {
		fmt.Fprintf(&b, "[%d] %s L%d-L%d:\n%s\n\n", i+1, r.Path, r.StartLine, r.EndLine, r.Text)
		cites[i] = citation{Path: r.Path, StartLine: r.StartLine, EndLine: r.EndLine}
	}
	return strings.TrimSpace(b.String()), cites, true
}
```

Note: the `maxToolCalls`/`maxToolOutput` constants were added in `agent_exec.go` (Task 2). Do not redeclare them here.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestDispatchTool -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent.go agent_test.go
git commit -m "feat(agent): tool argument parsing and dispatch"
```

---

## Task 7: System prompt + slash-command parsing

**Files:**
- Modify: `agent.go`
- Test: `agent_test.go`

- [ ] **Step 1: Write the failing test**

Append to `agent_test.go`:

```go
func TestAgentSystemPrompt_MentionsVaultAndTools(t *testing.T) {
	p := agentSystemPrompt("/home/u/notes")
	for _, want := range []string{"/home/u/notes", "search_vault", "bash"} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestParseSlashCommand(t *testing.T) {
	cases := map[string]slashCommand{
		"/clear":     slashClear,
		"/exit":      slashExit,
		"/model":     slashModel,
		"/help":      slashHelp,
		"/bogus":     slashUnknown,
		"not a slash": slashNone,
	}
	for in, want := range cases {
		if got := parseSlashCommand(in); got != want {
			t.Errorf("parseSlashCommand(%q) = %v, want %v", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run 'TestAgentSystemPrompt|TestParseSlashCommand' -v`
Expected: FAIL — undefined `agentSystemPrompt`, `parseSlashCommand`, slash constants.

- [ ] **Step 3: Write the minimal implementation**

Add to `agent.go`:

```go
type slashCommand int

const (
	slashNone slashCommand = iota // not a slash command
	slashUnknown
	slashClear
	slashExit
	slashModel
	slashHelp
)

func parseSlashCommand(input string) slashCommand {
	s := strings.TrimSpace(input)
	if !strings.HasPrefix(s, "/") {
		return slashNone
	}
	switch s {
	case "/clear":
		return slashClear
	case "/exit":
		return slashExit
	case "/model":
		return slashModel
	case "/help":
		return slashHelp
	default:
		return slashUnknown
	}
}

const agentHelpText = "Commands: /clear (reset conversation), /exit (close), /model (show model), /help. " +
	"Ask about your notes or tell me to manage files (rename, move, edit) in your vault."

func agentSystemPrompt(vault string) string {
	return fmt.Sprintf(`You are clipad's note-vault agent. You help the user manage and ask questions about their personal Markdown notes.

The notes vault is located at: %s
Your working directory is the vault root, and you are confined to it.

Tools:
- search_vault(query, k): semantic search over the notes. Use it to answer questions about note content. Cite excerpts by their numbered tag, e.g. [1].
- bash(cmd): run a shell command in the vault (cd, ls, mv, cp, cat, sed, awk, etc.). Use it to inspect and modify files. Inspect with read-only commands before making destructive changes.

Guidelines:
- For questions about the notes, prefer search_vault. For file management and content edits, use bash.
- Quote filenames that contain spaces.
- When you finish a task, end with a short plain-text summary of what you did.`, vault)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run 'TestAgentSystemPrompt|TestParseSlashCommand' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent.go agent_test.go
git commit -m "feat(agent): system prompt and slash-command parsing"
```

---

## Task 8: The agent loop + event types

**Files:**
- Modify: `agent.go`
- Test: `agent_test.go`

- [ ] **Step 1: Write the failing test**

Append to `agent_test.go`:

```go
import "net/http/httptest" // add to the import block
import "net/http"          // add to the import block

// scriptedServer returns canned JSON responses in sequence, one per request.
func scriptedServer(t *testing.T, responses []string) *httptest.Server {
	t.Helper()
	i := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if i >= len(responses) {
			t.Fatalf("unexpected request #%d", i+1)
		}
		fmt.Fprint(w, responses[i])
		i++
	}))
}

func collectEvents(events <-chan agentEvent) []agentEvent {
	var out []agentEvent
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func TestRunAgentLoop_BashThenAnswer(t *testing.T) {
	vault := t.TempDir()
	server := scriptedServer(t, []string{
		`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"echo hi\"}"}}]}}]}`,
		`{"choices":[{"message":{"role":"assistant","content":"Done."}}]}`,
	})
	defer server.Close()

	deps := agentDeps{url: server.URL, apiKey: "k", model: "m", vault: vault, timeout: 5 * time.Second}
	events := make(chan agentEvent)
	msgs := []agentMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "say hi"}}
	go runAgentLoop(context.Background(), deps, msgs, events)
	evs := collectEvents(events)

	var sawToolStart, sawToolResult, sawDone bool
	var finalText string
	for _, ev := range evs {
		switch ev.Kind {
		case evToolStart:
			sawToolStart = true
		case evToolResult:
			sawToolResult = true
		case evAssistantText:
			finalText = ev.Text
		case evDone:
			sawDone = true
		}
	}
	if !sawToolStart || !sawToolResult || !sawDone {
		t.Errorf("missing events: start=%v result=%v done=%v", sawToolStart, sawToolResult, sawDone)
	}
	if finalText != "Done." {
		t.Errorf("final text = %q, want Done.", finalText)
	}
}

func TestRunAgentLoop_StopsAtToolCallCap(t *testing.T) {
	vault := t.TempDir()
	// Always returns a tool call; loop must stop at the cap rather than forever.
	responses := make([]string, maxToolCalls+1)
	for i := range responses {
		responses[i] = `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"echo x\"}"}}]}}]}`
	}
	server := scriptedServer(t, responses)
	defer server.Close()

	deps := agentDeps{url: server.URL, apiKey: "k", model: "m", vault: vault, timeout: 5 * time.Second}
	events := make(chan agentEvent)
	msgs := []agentMessage{{Role: "user", Content: "loop"}}
	go runAgentLoop(context.Background(), deps, msgs, events)
	evs := collectEvents(events)

	calls := 0
	sawDone := false
	for _, ev := range evs {
		if ev.Kind == evToolStart {
			calls++
		}
		if ev.Kind == evDone {
			sawDone = true
		}
	}
	if calls > maxToolCalls {
		t.Errorf("tool calls = %d, want <= %d", calls, maxToolCalls)
	}
	if !sawDone {
		t.Errorf("loop did not terminate with evDone")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestRunAgentLoop -v`
Expected: FAIL — undefined `agentEvent`, `evToolStart`, `runAgentLoop`, etc.

- [ ] **Step 3: Write the minimal implementation**

Add to `agent.go`:

```go
type agentEventKind int

const (
	evAssistantText agentEventKind = iota
	evToolStart
	evToolResult
	evDone
	evError
)

type agentEvent struct {
	Kind     agentEventKind
	Text     string         // evAssistantText
	Tool     string         // evToolStart / evToolResult
	Label    string         // evToolStart: the bash cmd or search query
	Output   string         // evToolResult
	OK       bool           // evToolResult
	Cites    []citation     // evToolResult (search_vault)
	Messages []agentMessage // evDone: final conversation to persist
	Err      error          // evError
}

// runAgentLoop drives the tool-calling loop, emitting events for the UI and
// closing the channel when finished. msgs must already include the system and
// the new user message. The loop persists the full conversation via evDone.
func runAgentLoop(ctx context.Context, deps agentDeps, msgs []agentMessage, events chan<- agentEvent) {
	defer close(events)

	send := func(ev agentEvent) bool {
		select {
		case events <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	calls := 0
	for {
		assistant, err := runAgentTurn(ctx, deps.url, deps.apiKey, deps.model, msgs, agentTools())
		if err != nil {
			send(agentEvent{Kind: evError, Err: err})
			return
		}
		msgs = append(msgs, assistant)

		if strings.TrimSpace(assistant.Content) != "" {
			if !send(agentEvent{Kind: evAssistantText, Text: assistant.Content}) {
				return
			}
		}

		if len(assistant.ToolCalls) == 0 {
			send(agentEvent{Kind: evDone, Messages: msgs})
			return
		}

		for _, tc := range assistant.ToolCalls {
			if calls >= maxToolCalls {
				send(agentEvent{Kind: evAssistantText, Text: "(stopped: reached the tool-call limit)"})
				send(agentEvent{Kind: evDone, Messages: msgs})
				return
			}
			calls++

			label := toolLabel(tc)
			if !send(agentEvent{Kind: evToolStart, Tool: tc.Function.Name, Label: label}) {
				return
			}
			out, cites, ok := dispatchTool(ctx, deps, tc.Function.Name, tc.Function.Arguments)
			if !send(agentEvent{Kind: evToolResult, Tool: tc.Function.Name, Output: out, OK: ok, Cites: cites}) {
				return
			}
			msgs = append(msgs, agentMessage{Role: "tool", ToolCallID: tc.ID, Content: out})
		}
	}
}

// toolLabel derives a short display label from a tool call's arguments.
func toolLabel(tc agentToolCall) string {
	switch tc.Function.Name {
	case "bash":
		if cmd, err := parseBashArgs(tc.Function.Arguments); err == nil {
			return cmd
		}
	case "search_vault":
		if q, _, err := parseSearchArgs(tc.Function.Arguments); err == nil {
			return q
		}
	}
	return tc.Function.Arguments
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestRunAgentLoop -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent.go agent_test.go
git commit -m "feat(agent): tool-calling loop with event stream and call cap"
```

---

## Task 9: Trace on chatTurn + applyAgentEvent + rendering

**Files:**
- Modify: `chat.go` (add `Trace` to `chatTurn`, render it)
- Modify: `agent.go` (add `applyAgentEvent`)
- Test: `agent_test.go`

- [ ] **Step 1: Write the failing test**

Append to `agent_test.go`:

```go
func TestApplyAgentEvent_AppendsTraceAndContent(t *testing.T) {
	turns := []chatTurn{
		{Role: "user", Content: "rename stuff"},
		{Role: "assistant"}, // in-flight placeholder
	}
	turns = applyAgentEvent(turns, agentEvent{Kind: evToolStart, Tool: "bash", Label: "mv a b"})
	turns = applyAgentEvent(turns, agentEvent{Kind: evToolResult, Tool: "bash", Output: "exit 0\n", OK: true})
	turns = applyAgentEvent(turns, agentEvent{Kind: evAssistantText, Text: "Renamed."})

	last := turns[len(turns)-1]
	if len(last.Trace) != 2 {
		t.Fatalf("trace lines = %d, want 2", len(last.Trace))
	}
	if last.Trace[0].Kind != "cmd" || !strings.Contains(last.Trace[0].Text, "mv a b") {
		t.Errorf("trace[0] = %+v", last.Trace[0])
	}
	if last.Content != "Renamed." {
		t.Errorf("content = %q, want Renamed.", last.Content)
	}
}

func TestApplyAgentEvent_SearchAttachesCitations(t *testing.T) {
	turns := []chatTurn{{Role: "user", Content: "q"}, {Role: "assistant"}}
	cites := []citation{{Path: "a.md", StartLine: 1, EndLine: 2}}
	turns = applyAgentEvent(turns, agentEvent{Kind: evToolStart, Tool: "search_vault", Label: "taxes"})
	turns = applyAgentEvent(turns, agentEvent{Kind: evToolResult, Tool: "search_vault", OK: true, Cites: cites})
	last := turns[len(turns)-1]
	if len(last.Citations) != 1 || last.Citations[0].Path != "a.md" {
		t.Errorf("citations = %+v, want a.md", last.Citations)
	}
}

func TestRenderChatScrollback_ShowsTrace(t *testing.T) {
	turns := []chatTurn{
		{Role: "assistant", Content: "ok", Trace: []traceLine{
			{Kind: "cmd", Text: "$ ls"},
			{Kind: "result", Text: "✓ (exit 0)", OK: true},
		}},
	}
	out := renderChatScrollback(turns, 60, false)
	if !strings.Contains(out, "$ ls") {
		t.Errorf("scrollback missing command line: %q", out)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run 'TestApplyAgentEvent|TestRenderChatScrollback_ShowsTrace' -v`
Expected: FAIL — `traceLine` undefined, `applyAgentEvent` undefined, `chatTurn` has no `Trace`.

- [ ] **Step 3a: Add Trace to chatTurn (chat.go)**

In `chat.go`, change the `chatTurn` struct and add `traceLine`:

```go
type chatTurn struct {
	Role      string // "user" | "assistant"
	Content   string
	Citations []citation
	Trace     []traceLine // tool activity for assistant turns (in order)
}

type traceLine struct {
	Kind string // "cmd" | "result" | "search"
	Text string
	OK   bool
}
```

- [ ] **Step 3b: Render Trace in renderChatScrollback (chat.go)**

In `renderChatScrollback`, inside the `case "assistant":` branch, render the trace before the content. Replace the assistant branch body with:

```go
		case "assistant":
			for _, tl := range t.Trace {
				style := chatHintStyle
				if tl.Kind == "result" && !tl.OK {
					style = chatUserStyle
				}
				b.WriteString("  " + style.Render(wordWrap(tl.Text, width-2)) + "\n")
			}
			content := t.Content
			// While streaming, show a placeholder for the empty in-flight turn.
			if content == "" && len(t.Trace) == 0 && streaming && i == len(turns)-1 {
				content = "(thinking…)"
			}
			if content != "" {
				body := wordWrap("clipad: "+content, width)
				b.WriteString(chatAssistantStyle.Render(body) + "\n")
			}
			for j, c := range t.Citations {
				cite := fmt.Sprintf("[%d] %s L%d-L%d", j+1, c.Path, c.StartLine, c.EndLine)
				b.WriteString("  " + chatCitationStyle.Render(wordWrap(cite, width-2)) + "\n")
			}
```

- [ ] **Step 3c: Add applyAgentEvent (agent.go)**

```go
// applyAgentEvent folds one UI event into the display turns, mutating the last
// (in-flight assistant) turn. Returns the updated slice.
func applyAgentEvent(turns []chatTurn, ev agentEvent) []chatTurn {
	if len(turns) == 0 || turns[len(turns)-1].Role != "assistant" {
		return turns
	}
	last := &turns[len(turns)-1]
	switch ev.Kind {
	case evToolStart:
		if ev.Tool == "search_vault" {
			last.Trace = append(last.Trace, traceLine{Kind: "search", Text: "🔍 search_vault: " + ev.Label, OK: true})
		} else {
			last.Trace = append(last.Trace, traceLine{Kind: "cmd", Text: "$ " + ev.Label})
		}
	case evToolResult:
		if ev.Tool == "search_vault" {
			last.Citations = append(last.Citations, ev.Cites...)
			last.Trace = append(last.Trace, traceLine{Kind: "result", Text: fmt.Sprintf("  → %d result(s)", len(ev.Cites)), OK: ev.OK})
			return turns
		}
		text := "✓ (exit 0)"
		if !ev.OK {
			text = "✗ " + firstLine(ev.Output)
		} else if out := strings.TrimSpace(stripExitLine(ev.Output)); out != "" {
			text = out
		}
		last.Trace = append(last.Trace, traceLine{Kind: "result", Text: text, OK: ev.OK})
	case evAssistantText:
		if last.Content == "" {
			last.Content = ev.Text
		} else {
			last.Content += "\n" + ev.Text
		}
	}
	return turns
}

// stripExitLine drops a leading "exit N\n" prefix produced by the bash tool.
func stripExitLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 && strings.HasPrefix(s, "exit ") {
		return s[i+1:]
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run 'TestApplyAgentEvent|TestRenderChatScrollback|TestMostRecentCitation' -v`
Expected: PASS (existing citation tests still green).

- [ ] **Step 5: Commit**

```bash
git add chat.go agent.go agent_test.go
git commit -m "feat(agent): trace rendering and event-to-turn folding"
```

---

## Task 10: Model state + start command + Esc cancel

**Files:**
- Modify: `model.go`
- Modify: `agent.go` (add `startAgentLoop` + msg types)
- Test: none (wiring; verified by build + Task 13 manual run). Pure helpers are already covered.

- [ ] **Step 1: Add agent state fields to the model struct (model.go)**

In the `// Chat panel (Ctrl+Shift+A)` block of the `model` struct (around `model.go:188`), add after `chatCurrentCites`:

```go
	// Agent (the Ctrl+K panel runs an agentic tool-calling loop)
	agentMessages []agentMessage    // full OpenAI conversation incl. system
	agentEvents   <-chan agentEvent // active event stream (nil when idle)
	agentCancel   context.CancelFunc
```

- [ ] **Step 2: Add start plumbing + tea messages (agent.go)**

Add to `agent.go`:

```go
import tea "github.com/charmbracelet/bubbletea" // add to the import block

// agentStartedMsg carries the freshly created event channel.
type agentStartedMsg struct{ events <-chan agentEvent }

// agentEventMsg carries one event plus its channel (identity token).
type agentEventMsg struct {
	events <-chan agentEvent
	ev     agentEvent
}

// agentClosedMsg fires when the channel closes without a terminal event
// (e.g. cancelled).
type agentClosedMsg struct{ events <-chan agentEvent }

// startAgentCmd launches the loop goroutine and returns the channel.
func startAgentCmd(ctx context.Context, deps agentDeps, msgs []agentMessage) tea.Cmd {
	return func() tea.Msg {
		events := make(chan agentEvent)
		go runAgentLoop(ctx, deps, msgs, events)
		return agentStartedMsg{events: events}
	}
}

// readNextAgentEvent blocks for the next event from the channel.
func readNextAgentEvent(events <-chan agentEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return agentClosedMsg{events: events}
		}
		return agentEventMsg{events: events, ev: ev}
	}
}
```

- [ ] **Step 3: Verify the project builds**

Run: `go build ./...`
Expected: builds (new code unused by model yet, but compiles).

- [ ] **Step 4: Commit**

```bash
git add model.go agent.go
git commit -m "feat(agent): model state fields and loop start plumbing"
```

---

## Task 11: Rewire the Ctrl+K panel to the agent

**Files:**
- Modify: `model.go`
- Test: none (integration; covered by build + Task 13 manual verification).

- [ ] **Step 1: Drop the embedder gate so the panel always opens (model.go)**

The Ctrl+K case starts at `model.go:816` with `case "ctrl+k":` then an embedder check before the `m.chatOpen` toggle at ~`model.go:821`. Remove the embedder guard so the panel opens regardless. The case body becomes:

```go
		case "ctrl+k":
			if m.chatOpen {
				if m.agentCancel != nil {
					m.agentCancel()
					m.agentCancel = nil
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

(If the lines `816-820` contained `if m.indexer == nil || m.indexer.embedder == nil { ... }` guarding this case, delete them.)

- [ ] **Step 2: Replace the input-mode handler in handleChatPanel (model.go)**

Replace the `chatStreaming` Esc block (top of `handleChatPanel`, ~`model.go:1122-1135`) to cancel the agent:

```go
	if m.chatStreaming {
		if msg.String() == "esc" {
			if m.agentCancel != nil {
				m.agentCancel()
				m.agentCancel = nil
			}
			m.chatStreaming = false
			m.agentEvents = nil
			m.chatMode = chatModeView
			m.chatInput.Blur()
			return m, nil
		}
		return m, nil
	}
```

Then replace the `case "enter":` block inside `case chatModeInput:` (~`model.go:1144-1185`) with:

```go
		case "enter":
			input := strings.TrimSpace(m.chatInput.Value())
			if input == "" {
				return m, nil
			}
			if sc := parseSlashCommand(input); sc != slashNone {
				m.chatInput.SetValue("")
				return m.handleAgentSlash(sc)
			}
			m.chatInput.SetValue("")

			// Display turns.
			m.chatTurns = append(m.chatTurns, chatTurn{Role: "user", Content: input})
			m.chatTurns = append(m.chatTurns, chatTurn{Role: "assistant"})

			// Resolve provider config.
			provider := m.activeShortcutProvider
			if provider == "" {
				provider = defaultAIShortcutProvider
			}
			plugCfg, err := loadPluginConfig(provider)
			if err != nil {
				m.errMsg = "Plugin config: " + err.Error()
				m.chatTurns = m.chatTurns[:len(m.chatTurns)-2] // roll back user+assistant
				return m, nil
			}
			url := defaultBlackboxURL
			if provider == "openrouter" {
				url = defaultOpenRouterURL
			}

			// Conversation history (ensure a system message at the head).
			if len(m.agentMessages) == 0 {
				m.agentMessages = []agentMessage{{Role: "system", Content: agentSystemPrompt(m.vault)}}
			}
			m.agentMessages = append(m.agentMessages, agentMessage{Role: "user", Content: input})

			m.chatStreaming = true
			ctx, cancel := context.WithCancel(context.Background())
			m.agentCancel = cancel
			deps := agentDeps{
				url:     url,
				apiKey:  plugCfg["api_key"],
				model:   plugCfg["model"],
				vault:   m.vault,
				idx:     m.indexer,
				timeout: bashTimeout * time.Second,
			}

			innerW := m.chatWidth - 4
			if innerW < 1 {
				innerW = 1
			}
			m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, m.chatStreaming))
			m.chatViewport.GotoBottom()

			return m, startAgentCmd(ctx, deps, m.agentMessages)
```

Note: add `"time"` to `model.go`'s imports if not already present (it is — `autoSaveTickMsg` uses it).

- [ ] **Step 3: Add handleAgentSlash (model.go)**

Add a method near `handleChatPanel`:

```go
// handleAgentSlash processes a recognized slash command typed in the agent
// input. Returns the updated model.
func (m model) handleAgentSlash(sc slashCommand) (tea.Model, tea.Cmd) {
	innerW := m.chatWidth - 4
	if innerW < 1 {
		innerW = 1
	}
	switch sc {
	case slashExit:
		if m.agentCancel != nil {
			m.agentCancel()
			m.agentCancel = nil
		}
		m.chatOpen = false
		m.chatInput.Blur()
		m.recalcLayout()
		return m, nil
	case slashClear:
		m.chatTurns = nil
		m.agentMessages = nil
		m.chatViewport.SetContent("")
		return m, nil
	case slashModel:
		provider := m.activeShortcutProvider
		if provider == "" {
			provider = defaultAIShortcutProvider
		}
		modelName := "(unset)"
		if cfg, err := loadPluginConfig(provider); err == nil && cfg["model"] != "" {
			modelName = cfg["model"]
		}
		m.chatTurns = append(m.chatTurns, chatTurn{Role: "assistant", Content: fmt.Sprintf("Model: %s (provider: %s)", modelName, provider)})
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, false))
		m.chatViewport.GotoBottom()
		return m, nil
	case slashHelp:
		m.chatTurns = append(m.chatTurns, chatTurn{Role: "assistant", Content: agentHelpText})
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, false))
		m.chatViewport.GotoBottom()
		return m, nil
	default: // slashUnknown
		m.chatTurns = append(m.chatTurns, chatTurn{Role: "assistant", Content: "Unknown command. " + agentHelpText})
		m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, false))
		m.chatViewport.GotoBottom()
		return m, nil
	}
}
```

(`fmt` is already imported in `model.go`.)

- [ ] **Step 4: Replace the chat msg handlers with agent-event handlers (model.go)**

Delete the five cases `chatStartedMsg`, `chatStartFailedMsg`, `chatChunkMsg`, `chatDoneMsg`, `chatErrMsg` (`model.go:437-495`) and replace with:

```go
	case agentStartedMsg:
		m.agentEvents = msg.events
		return m, readNextAgentEvent(msg.events)

	case agentEventMsg:
		if msg.events != m.agentEvents {
			return m, nil // superseded stream
		}
		innerW := m.chatWidth - 4
		if innerW < 1 {
			innerW = 1
		}
		switch msg.ev.Kind {
		case evDone:
			m.agentMessages = msg.ev.Messages
			m.chatStreaming = false
			m.agentEvents = nil
			m.agentCancel = nil
			m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, false))
			m.chatViewport.GotoBottom()
			return m, nil
		case evError:
			m.chatStreaming = false
			m.agentEvents = nil
			m.agentCancel = nil
			m.chatTurns = applyAgentEvent(m.chatTurns, agentEvent{Kind: evAssistantText, Text: "Error: " + msg.ev.Err.Error()})
			m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, false))
			m.chatViewport.GotoBottom()
			return m, nil
		default:
			m.chatTurns = applyAgentEvent(m.chatTurns, msg.ev)
			m.chatViewport.SetContent(renderChatScrollback(m.chatTurns, innerW, m.chatStreaming))
			m.chatViewport.GotoBottom()
			return m, readNextAgentEvent(msg.events)
		}

	case agentClosedMsg:
		if msg.events != m.agentEvents {
			return m, nil
		}
		m.chatStreaming = false
		m.agentEvents = nil
		m.agentCancel = nil
		return m, nil
```

- [ ] **Step 5: Build and run the full test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build OK; tests for `chatStartCmd`/`composeChatRequest`/`encodeChatHistory` will now FAIL to compile because the model no longer references them but they still exist. That's fixed in Task 12. If `go build ./...` fails because `chatStartCmd`/`streamChatCmd`/`readNextChatChunk` are now unused, leave them for Task 12 (unused package-level funcs do not break a Go build; only unused imports/locals do). So `go build ./...` should pass. Proceed.

- [ ] **Step 6: Commit**

```bash
git add model.go
git commit -m "feat(agent): rewire Ctrl+K panel to the agentic loop"
```

---

## Task 12: Remove obsolete one-shot RAG code

**Files:**
- Modify: `chat.go`, `ask.go`, `chat_test.go`

- [ ] **Step 1: Delete obsolete functions/types**

In `chat.go`, delete (no longer referenced):
- `chatChunkMsg`, `chatDoneMsg`, `chatErrMsg`, `chatStartedMsg`, `chatStartFailedMsg` types.
- `streamChatCmd`, `readNextChatChunk`, `chatStartCmd`, `encodeChatHistory` functions.

Keep: `chatModeT` + constants, `chatTurn`, `traceLine`, `citation`, `renderChatScrollback`, `chatPanelView`, `mostRecentCitation`, and the lipgloss styles.

In `ask.go`, delete: `maxHistoryPairs`, `chatMessage` type, `composeChatRequest`, `lastPairs`, `buildSystemPrompt`. (The entire file becomes empty of declarations — delete `ask.go` itself.)

```bash
git rm ask.go
```

- [ ] **Step 2: Delete obsolete tests**

In `chat_test.go`, delete: `TestEncodeChatHistory_SingleTurnIsBareQuery`, `TestEncodeChatHistory_MultiTurnIsTranscript`, `TestComposeChatRequest_StripsAssistantPlaceholder`. Keep `TestMostRecentCitation_*` and the `contains` helper (still used by them).

If `contains` becomes unused after deletions, delete it too (check: `grep -n "contains(" *_test.go`).

- [ ] **Step 3: Build and run the whole suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS, no unused-symbol or undefined-symbol errors.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor(agent): remove obsolete one-shot RAG chat code"
```

---

## Task 13: Docs + manual verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README**

In `README.md`, update the keybindings table and add an Agent section. Change the `Ctrl+K` row (currently the ask-vault chat) to:

```
| `Ctrl+K` | Open the notes **agent** panel (ask about or manage your notes) |
```

Add under "Plugins" (or a new "## Agent" section):

```markdown
## Agent

Press `Ctrl+K` to open the agent — a continuous chat in the right-hand panel
that can both answer questions about your notes and manage them. It uses your
active AI provider (blackbox.ai by default) with native tool-calling and has two
tools:

- **search_vault** — semantic search over your notes (cited inline; press `1`–`9`
  to open a citation). Requires `embedding_provider` configured; before each
  search it prunes index entries for files that no longer exist.
- **bash** — runs shell commands (cd, mv, cp, cat, sed, awk, …) in your vault to
  inspect and edit notes. Commands run with the vault as the working directory
  and a best-effort guard that blocks paths escaping the vault and `sudo`.

Example: *"rename all Task <N> files so N is sequential starting from 1, only in
the Prywatne directory."*

Slash commands: `/clear` (reset the conversation), `/exit` (close), `/model`
(show the model), `/help`. Press `Esc` to stop a run.

The agent's bash commands run automatically and are scoped to the vault by
working directory plus a heuristic guard — this is a safety rail against
accidents, not a security sandbox.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs(agent): document the Ctrl+K notes agent"
```

- [ ] **Step 3: Manual verification (full app)**

Build and run against a scratch vault:

```bash
go build -o clipad .
```

Then, with a configured blackbox/openrouter provider:
1. Launch `./clipad`, press `Ctrl+K`. The panel opens even if embeddings are unconfigured.
2. Type `/help` → see the command list.
3. Ask a question about your notes → the agent calls `search_vault`, shows a 🔍 line, answers with `[1]` citations; press `1` to open the cited file.
4. Ask it to "create a file test-agent.md with a heading" → see `$ ...` command lines and `✓ (exit 0)`, then a summary; confirm the file exists.
5. Delete a note on disk, then ask a question that would have matched it → confirm the deleted note is not cited (pruned).
6. Start a long task and press `Esc` → the run stops.
7. `/clear` → transcript resets; `/exit` → panel closes.

Expected: all behaviors as described; no panics.

---

## Self-Review

**Spec coverage:**
- Merge into Ctrl+K panel, continuous conversation → Tasks 10–11.
- Always-agentic loop, two tools → Tasks 6, 8.
- `search_vault` tool + citations + 1–9 → Tasks 6, 9, 11.
- `bash` tool, cwd=vault, guard, timeout, truncation → Tasks 1, 2, 6.
- Native tool-calling completion → Tasks 4, 5.
- Stale-embeddings fix (PruneOrphans on search) → Tasks 3, 6.
- Slash commands /clear /exit /model /help → Tasks 7, 11.
- System prompt → Task 7.
- Panel opens without embedder → Task 11.
- 25-call cap → Task 8.
- Remove obsolete one-shot flow → Task 12.
- Docs → Task 13.

**Placeholder scan:** none — every code step contains complete code.

**Type consistency:** `agentMessage`/`agentToolCall`/`agentToolFunction`/`agentTool`/`agentToolSpec` (Task 4) are used unchanged in Tasks 5, 8. `agentDeps` fields (`url,apiKey,model,vault,idx,timeout`) are consistent across Tasks 6, 8, 10, 11. `agentEvent` fields (`Kind,Text,Tool,Label,Output,OK,Cites,Messages,Err`) consistent across Tasks 8, 9, 11. `traceLine{Kind,Text,OK}` consistent across Tasks 9, 11. `dispatchTool` signature `(ctx, deps, name, argsJSON) (string, []citation, bool)` consistent across Tasks 6, 8. Constants `maxToolCalls`/`maxToolOutput` declared once in `agent_exec.go` (Task 2); `bashTimeout` is used as `bashTimeout * time.Second` in Task 11 — **declared as the integer `30` in the shared const block** (see File Structure); ensure `agent_exec.go`'s const block includes `bashTimeout = 30`.

**Note on bashTimeout:** the File Structure const block lists `bashTimeout = 30`. Task 2 adds a const block to `agent_exec.go` with `maxToolCalls`/`maxToolOutput` — **add `bashTimeout = 30` there too** so Task 11's `bashTimeout * time.Second` compiles. Tasks 1/2/6 pass an explicit `time.Duration` to `runBashInVault`/`agentDeps.timeout`, so they don't depend on the constant's unit.
