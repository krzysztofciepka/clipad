# Fabric Patterns in the AI Shortcut Picker — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the blackbox.ai provider from clipad, and list the user's installed Fabric patterns in the Ctrl+G AI shortcut picker so any pattern can be run against the current note through the same AI provider native shortcuts use.

**Architecture:** Fabric patterns are read straight off disk (`~/.config/fabric/patterns/<name>/system.md`) — no fabric CLI involvement. The picker's cursor changes from an index into `m.shortcuts` to an index into a flat `[]shortcutRow` that mixes editable shortcuts, a non-selectable section header, and read-only patterns, with a scroll window and a `/` fuzzy filter. All three AI-run entry points (shortcut, pattern, deferred run resumed after the provider config wizard) funnel through one `startAIRun` helper.

**Tech Stack:** Go, Bubble Tea / Bubbles `textinput` / Lipgloss, `github.com/sahilm/fuzzy` (already a dependency, used by `filter.go`), `github.com/pelletier/go-toml/v2`.

**Spec:** `docs/superpowers/specs/2026-08-05-fabric-patterns-design.md`

## Global Constraints

- **No git operations.** Do not branch, stage, commit, or push. The user handles all git themselves. Every task ends at "tests pass", not at a commit.
- Package is flat `package main` at the repo root; tests live beside their source as `<name>_test.go`.
- Full verification command for every task: `go build ./... && go vet ./... && go test ./...` from `/home/kc/repos/clipad`.
- Tests must never read the user's real `~/.config/fabric` or `~/.config/clipad`. Use `t.TempDir()` plus `t.Setenv("FABRIC_CONFIG_HOME", ...)` / `t.Setenv("XDG_CONFIG_HOME", ...)`.
- Existing model-level tests build their model via the `newTestModel(t)` helper — reuse it rather than constructing `model{}` literals.
- Comments explain *why*, not *what*, matching the density of the surrounding files.

---

### Task 1: Remove the blackbox provider

Deleting `BlackboxPlugin` from the plugin slice removes it from both the Ctrl+Space plugin picker and the Ctrl+G provider cycle, since both iterate `m.plugins`. The catch is that existing `config.toml` files say `ai_shortcut_provider = "blackbox"`, which would then fail at run time with `Unknown AI shortcut provider: blackbox`. `resolveShortcutProvider` maps any unregistered name back to the default.

**Files:**
- Delete: `plugin_blackbox.go`, `plugin_blackbox_test.go`
- Modify: `main.go:164-168`, `config.go:25`, `shortcuts.go:96-108`, `shortcut_provider.go`, `model.go` (the `m := model{` literal in `newModel`)
- Test: `shortcut_provider_test.go` (new test + retarget existing), `config_test.go:116-118`, `plugin_test.go:9-10`, `shortcuts_modal_test.go` (4 call sites)

**Interfaces:**
- Consumes: nothing.
- Produces: `func resolveShortcutProvider(name string, plugins []Plugin) string`; `defaultAIShortcutProvider == "openrouter"`.

- [ ] **Step 1: Write the failing test**

Append to `shortcut_provider_test.go`:

```go
func TestResolveShortcutProvider_KeepsRegisteredName(t *testing.T) {
	plugins := []Plugin{&OpenRouterPlugin{}, &OpenCodePlugin{}}
	if got := resolveShortcutProvider("opencode", plugins); got != "opencode" {
		t.Errorf("resolveShortcutProvider(opencode) = %q, want opencode", got)
	}
}

func TestResolveShortcutProvider_FallsBackForRemovedProvider(t *testing.T) {
	plugins := []Plugin{&OpenRouterPlugin{}, &OpenCodePlugin{}}
	// Configs written before blackbox was removed still name it.
	if got := resolveShortcutProvider("blackbox", plugins); got != defaultAIShortcutProvider {
		t.Errorf("resolveShortcutProvider(blackbox) = %q, want %q", got, defaultAIShortcutProvider)
	}
	if got := resolveShortcutProvider("", plugins); got != defaultAIShortcutProvider {
		t.Errorf("resolveShortcutProvider(\"\") = %q, want %q", got, defaultAIShortcutProvider)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestResolveShortcutProvider -v`
Expected: FAIL — `undefined: resolveShortcutProvider`.

- [ ] **Step 3: Add `resolveShortcutProvider`**

Append to `shortcut_provider.go`:

```go
// resolveShortcutProvider maps a configured provider name onto a registered
// plugin. A config naming a provider that no longer ships (blackbox, removed
// in favour of openrouter) resolves to the default instead of failing at run
// time. The config file is left alone; the resolved value is persisted the
// next time the user cycles providers with 'p'.
func resolveShortcutProvider(name string, plugins []Plugin) string {
	if pluginByName(plugins, name) != nil {
		return name
	}
	return defaultAIShortcutProvider
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestResolveShortcutProvider -v`
Expected: PASS.

- [ ] **Step 5: Delete the plugin**

```bash
rm plugin_blackbox.go plugin_blackbox_test.go
```

In `main.go`, the plugin slice becomes:

```go
	plugins := []Plugin{
		&OpenRouterPlugin{},
		&OpenCodePlugin{},
	}
```

- [ ] **Step 6: Repoint the default provider**

In `config.go`, change the constant:

```go
	defaultAIShortcutProvider       = "openrouter"
```

In `shortcuts.go`, replace the `shortcutProviderURL` comment and default case:

```go
// shortcutProviderURL maps a shortcut provider name to its chat-completion
// endpoint. Unknown providers fall back to openrouter. Keep these cases in
// sync with the registered Plugin endpoints (plugin_*.go).
func shortcutProviderURL(provider string) string {
	switch provider {
	case "opencode":
		return defaultOpenCodeURL
	default:
		return defaultOpenRouterURL
	}
}
```

- [ ] **Step 7: Resolve the provider in `newModel`**

In `model.go`, inside the `m := model{` literal, change the `activeShortcutProvider` field:

```go
		activeShortcutProvider:   resolveShortcutProvider(activeShortcutProvider, plugins),
```

- [ ] **Step 8: Retarget the blackbox-referencing tests**

- `config_test.go:116-118` — the default assertion becomes `"openrouter"`.
- `plugin_test.go:9-10` — `pluginConfigPath("blackbox")` / `.../blackbox.toml` become `"openrouter"` / `.../openrouter.toml`.
- `shortcuts_modal_test.go` — the four `shortcutSelectorView(..., "blackbox", ...)` calls take `"openrouter"`.
- `shortcut_provider_test.go` — every `"blackbox"` string becomes `"openrouter"`, every `"openrouter"` in the *second* slot becomes `"opencode"`, `&BlackboxPlugin{}` becomes `&OpenRouterPlugin{}`, `&OpenRouterPlugin{}` becomes `&OpenCodePlugin{}`, and the seeded `blackbox.toml` fixture files become `openrouter.toml`. The assertions keep their shape — only the provider names shift.

- [ ] **Step 9: Update the README's blackbox references**

- Line 12 — feature bullet becomes:
  `- **Plugin system** with OpenRouter and OpenCode Zen integrations for LLM-powered note transformation (rephrase, translate, redraft)`
- Lines 203-214 — delete the entire `### Blackbox` section (heading through the `blackbox.toml` line and its trailing blank line).
- Line 233 — `(Blackbox ⇄ OpenRouter)` becomes `(OpenRouter ⇄ OpenCode Zen)`.
- Line 247 — `active AI provider (blackbox.ai by default)` becomes `active AI provider (OpenRouter by default)`.

- [ ] **Step 10: Verify the whole tree**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS, with no remaining hits from `grep -rn "blackbox\|Blackbox" --include="*.go" --include="*.md" . | grep -v docs/superpowers/plans` (historical plan documents keep their references).

---

### Task 2: Discover fabric patterns on disk

Read patterns directly rather than shelling out to fabric, so output flows through clipad's own AI provider. Listing must stay cheap: one readdir plus one explanations file, no reading of ~250 prompt bodies.

**Files:**
- Create: `fabric.go`
- Test: `fabric_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `type FabricPattern struct { Name, Description string }`; `func fabricPatternsDir() string`; `func parsePatternExplanations(data []byte) map[string]string`; `func listFabricPatterns(dir string) []FabricPattern`.

- [ ] **Step 1: Write the failing tests**

Create `fabric_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writePattern creates dir/name/system.md with the given body.
func writePattern(t *testing.T, dir, name, system string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "system.md"), []byte(system), 0o644); err != nil {
		t.Fatalf("write system.md for %s: %v", name, err)
	}
}

func TestListFabricPatterns_SortedAndFiltered(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# SUMMARIZE")
	writePattern(t, dir, "analyze_claims", "# ANALYZE")
	// Empty system.md: not a usable pattern.
	writePattern(t, dir, "hollow", "")
	// Directory without a system.md at all.
	if err := os.MkdirAll(filepath.Join(dir, "notapattern"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Loose files alongside the pattern directories.
	if err := os.WriteFile(filepath.Join(dir, "loaded"), nil, 0o644); err != nil {
		t.Fatalf("write loaded: %v", err)
	}

	got := listFabricPatterns(dir)
	if len(got) != 2 {
		t.Fatalf("got %d patterns (%v), want 2", len(got), got)
	}
	if got[0].Name != "analyze_claims" || got[1].Name != "summarize" {
		t.Errorf("names = [%s, %s], want [analyze_claims, summarize]", got[0].Name, got[1].Name)
	}
}

func TestListFabricPatterns_MissingDirIsNotAnError(t *testing.T) {
	if got := listFabricPatterns(filepath.Join(t.TempDir(), "absent")); got != nil {
		t.Errorf("got %v, want nil when fabric is not installed", got)
	}
}

func TestListFabricPatterns_AttachesExplanations(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# SUMMARIZE")
	writePattern(t, dir, "undocumented", "# X")
	explanations := "# Brief one-line summary\n\n" +
		"1. **summarize**: Summarize content into sections.\n" +
		"2. **absent_pattern**: Not installed here.\n"
	if err := os.WriteFile(filepath.Join(dir, "pattern_explanations.md"), []byte(explanations), 0o644); err != nil {
		t.Fatalf("write explanations: %v", err)
	}

	got := listFabricPatterns(dir)
	if len(got) != 2 {
		t.Fatalf("got %d patterns, want 2", len(got))
	}
	if got[0].Description != "Summarize content into sections." {
		t.Errorf("summarize description = %q", got[0].Description)
	}
	if got[1].Description != "" {
		t.Errorf("undocumented description = %q, want empty", got[1].Description)
	}
}

func TestParsePatternExplanations_IgnoresNonEntryLines(t *testing.T) {
	in := []byte("# Heading\n\n- Key pattern to use: **suggest_pattern**, suggests patterns.\n" +
		"12. **analyze_claims**: Analyse and rate truth claims.\n")
	got := parsePatternExplanations(in)
	if len(got) != 1 {
		t.Fatalf("got %d entries (%v), want 1", len(got), got)
	}
	if got["analyze_claims"] != "Analyse and rate truth claims." {
		t.Errorf("analyze_claims = %q", got["analyze_claims"])
	}
}

func TestFabricPatternsDir_HonoursEnvOverride(t *testing.T) {
	t.Setenv("FABRIC_CONFIG_HOME", "/custom/fabric")
	if got := fabricPatternsDir(); got != "/custom/fabric/patterns" {
		t.Errorf("fabricPatternsDir() = %q, want /custom/fabric/patterns", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestListFabricPatterns|TestParsePatternExplanations|TestFabricPatternsDir' -v`
Expected: FAIL — `undefined: listFabricPatterns` and friends.

- [ ] **Step 3: Write the implementation**

Create `fabric.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FabricPattern is one pattern discovered on disk. Only the metadata needed to
// render a picker row lives here; the prompt bodies are read by
// loadFabricPattern when a pattern actually runs, so opening the picker costs
// one readdir rather than hundreds of file reads.
type FabricPattern struct {
	Name        string
	Description string
}

// fabricPatternsDir returns the directory fabric keeps its patterns in.
// Clipad reads the files directly and never invokes the fabric CLI.
func fabricPatternsDir() string {
	if home := os.Getenv("FABRIC_CONFIG_HOME"); home != "" {
		return filepath.Join(home, "patterns")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "fabric", "patterns")
}

// explanationRe matches one entry of fabric's pattern_explanations.md, e.g.
// "12. **analyze_claims**: Analyse and rate truth claims."
var explanationRe = regexp.MustCompile(`^\s*\d+\.\s+\*\*([^*]+)\*\*:\s*(.+)$`)

// parsePatternExplanations extracts pattern name to one-line description.
// Lines that do not match the numbered-entry shape are skipped, so headings
// and prose in the file are ignored.
func parsePatternExplanations(data []byte) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		m := explanationRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out[strings.TrimSpace(m[1])] = strings.TrimSpace(m[2])
	}
	return out
}

// listFabricPatterns returns every pattern in dir. A subdirectory qualifies
// when it holds a non-empty system.md. A missing or unreadable dir yields nil:
// clipad works fine without fabric installed, the picker just omits the
// section. os.ReadDir returns entries sorted by filename, so the result is
// already alphabetical.
func listFabricPatterns(dir string) []FabricPattern {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	descriptions := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(dir, "pattern_explanations.md")); err == nil {
		descriptions = parsePatternExplanations(data)
	}
	var patterns []FabricPattern
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, e.Name(), "system.md"))
		if err != nil || info.Size() == 0 {
			continue
		}
		patterns = append(patterns, FabricPattern{
			Name:        e.Name(),
			Description: descriptions[e.Name()],
		})
	}
	return patterns
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run 'TestListFabricPatterns|TestParsePatternExplanations|TestFabricPatternsDir' -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Verify the whole tree**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 3: Load and run a fabric pattern

Fabric's semantics differ from clipad's native shortcuts: `system.md` *is* the system prompt, and the input is the user message. `runShortcutStream` instead wraps a one-line instruction in clipad's own system prompt. Both hit the same provider endpoint.

**Files:**
- Modify: `fabric.go`
- Test: `fabric_test.go`

**Interfaces:**
- Consumes: `fabricPatternsDir`, `shortcutProviderURL` (`shortcuts.go`), `streamChatCompletion` (`plugin_stream.go`).
- Produces: `func loadFabricPattern(dir, name string) (system, user string, err error)`; `func runFabricPatternStream(ctx context.Context, system, user, content, provider string, config map[string]string) (<-chan string, <-chan error)`.

- [ ] **Step 1: Write the failing tests**

Append to `fabric_test.go` (and add `"context"`, `"encoding/json"`, `"net/http"`, `"net/http/httptest"`, `"strings"` to its imports):

```go
func TestLoadFabricPattern_ReturnsSystemVerbatim(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# IDENTITY\n\nYou summarize.\n")

	system, user, err := loadFabricPattern(dir, "summarize")
	if err != nil {
		t.Fatalf("loadFabricPattern: %v", err)
	}
	if system != "# IDENTITY\n\nYou summarize.\n" {
		t.Errorf("system = %q, want the file verbatim", system)
	}
	if user != "" {
		t.Errorf("user = %q, want empty when there is no user.md", user)
	}
}

func TestLoadFabricPattern_BlankUserMdIsIgnored(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# SYS")
	if err := os.WriteFile(filepath.Join(dir, "summarize", "user.md"), []byte("\n  \n"), 0o644); err != nil {
		t.Fatalf("write user.md: %v", err)
	}

	_, user, err := loadFabricPattern(dir, "summarize")
	if err != nil {
		t.Fatalf("loadFabricPattern: %v", err)
	}
	if user != "" {
		t.Errorf("user = %q, want empty for a whitespace-only user.md", user)
	}
}

func TestLoadFabricPattern_KeepsPopulatedUserMd(t *testing.T) {
	dir := t.TempDir()
	writePattern(t, dir, "summarize", "# SYS")
	if err := os.WriteFile(filepath.Join(dir, "summarize", "user.md"), []byte("EXTRA CONTEXT"), 0o644); err != nil {
		t.Fatalf("write user.md: %v", err)
	}

	_, user, err := loadFabricPattern(dir, "summarize")
	if err != nil {
		t.Fatalf("loadFabricPattern: %v", err)
	}
	if user != "EXTRA CONTEXT" {
		t.Errorf("user = %q, want EXTRA CONTEXT", user)
	}
}

func TestLoadFabricPattern_MissingPatternErrors(t *testing.T) {
	if _, _, err := loadFabricPattern(t.TempDir(), "nope"); err == nil {
		t.Error("loadFabricPattern on a missing pattern returned nil error")
	}
}

func TestRunFabricPatternStream_SendsSystemMdAsSystemMessage(t *testing.T) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Messages []message `json:"messages"`
	}
	captured := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		captured <- req
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	// shortcutProviderURL has no test seam, so exercise the message assembly
	// against streamChatCompletion directly through the same code path by
	// pointing the provider at the test server via a temporary override.
	origURL := fabricStreamURL
	fabricStreamURL = func(string) string { return server.URL }
	defer func() { fabricStreamURL = origURL }()

	chunks, errs := runFabricPatternStream(context.Background(),
		"# IDENTITY\n\nYou summarize.", "", "note body",
		"openrouter", map[string]string{"api_key": "k", "model": "m"})
	for range chunks {
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
	default:
	}

	req := <-captured
	if len(req.Messages) < 2 {
		t.Fatalf("got %d messages, want 2", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "# IDENTITY\n\nYou summarize." {
		t.Errorf("system message = %+v, want system.md verbatim", req.Messages[0])
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != "note body" {
		t.Errorf("user message = %+v, want the note body alone", req.Messages[1])
	}
}

func TestRunFabricPatternStream_PrependsUserMd(t *testing.T) {
	if got := fabricUserMessage("PREAMBLE", "note body"); got != "PREAMBLE\n\nnote body" {
		t.Errorf("fabricUserMessage = %q, want PREAMBLE\\n\\nnote body", got)
	}
	if got := fabricUserMessage("", "note body"); got != "note body" {
		t.Errorf("fabricUserMessage with no user.md = %q, want note body", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestLoadFabricPattern|TestRunFabricPatternStream' -v`
Expected: FAIL — `undefined: loadFabricPattern`, `undefined: fabricStreamURL`, `undefined: fabricUserMessage`.

- [ ] **Step 3: Write the implementation**

Append to `fabric.go` (and add `"context"` and `"fmt"` to its imports):

```go
// loadFabricPattern reads a pattern's prompt bodies. user is empty when the
// pattern has no user.md or it holds only whitespace — all but one of the
// stock patterns ship an empty user.md.
func loadFabricPattern(dir, name string) (system, user string, err error) {
	sys, err := os.ReadFile(filepath.Join(dir, name, "system.md"))
	if err != nil {
		return "", "", fmt.Errorf("reading fabric pattern %s: %w", name, err)
	}
	if usr, err := os.ReadFile(filepath.Join(dir, name, "user.md")); err == nil {
		if strings.TrimSpace(string(usr)) != "" {
			user = string(usr)
		}
	}
	return string(sys), user, nil
}

// fabricStreamURL resolves the provider endpoint. It is a variable so tests
// can point a pattern run at an httptest server.
var fabricStreamURL = shortcutProviderURL

// fabricUserMessage assembles the user message for a pattern run: the note
// body, prefixed by user.md when the pattern ships a non-empty one.
func fabricUserMessage(user, content string) string {
	if user == "" {
		return content
	}
	return user + "\n\n" + content
}

// runFabricPatternStream runs a fabric pattern through the AI-shortcut
// provider. Unlike runShortcutStream, which wraps a one-line instruction in
// clipad's own system prompt, a pattern's system.md *is* the system prompt —
// that is how fabric itself invokes them.
func runFabricPatternStream(ctx context.Context, system, user, content, provider string, config map[string]string) (<-chan string, <-chan error) {
	return streamChatCompletion(ctx, fabricStreamURL(provider),
		config["api_key"], config["model"], system, fabricUserMessage(user, content))
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run 'TestLoadFabricPattern|TestRunFabricPatternStream' -v`
Expected: PASS (6 tests).

If `TestRunFabricPatternStream_SendsSystemMdAsSystemMessage` hangs or the field names do not line up, read `plugin_stream.go` for the exact request body shape and SSE termination `streamChatCompletion` expects, and mirror `plugin_openrouter_test.go`'s server stub — it is the known-good example in this repo.

- [ ] **Step 5: Verify the whole tree**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 4: The picker row model

The picker cursor currently indexes `m.shortcuts` directly. Mixing in a section header and patterns makes it a row index. This task is pure data — no UI, no model state — so it is fully unit-testable before anything is rewired.

**Files:**
- Create: `shortcut_rows.go`
- Test: `shortcut_rows_test.go`

**Interfaces:**
- Consumes: `AIShortcut` (`shortcuts.go`), `FabricPattern` (`fabric.go`), `github.com/sahilm/fuzzy`.
- Produces:
  - `type rowKind int` with `rowShortcut`, `rowHeader`, `rowFabric`
  - `type shortcutRow struct { kind rowKind; index int; name, description string }`
  - `const fabricSectionTitle = "── Fabric patterns ──"`
  - `func buildShortcutRows(shortcuts []AIShortcut, patterns []FabricPattern, filter string) []shortcutRow`
  - `func nextSelectableRow(rows []shortcutRow, cursor, delta int) int`
  - `func clampSelectableRow(rows []shortcutRow, cursor int) int`
  - `func clampShortcutOffset(cursor, offset, visible int) int`
  - `func visibleShortcutRows(height int) int`
  - `func selectedRow(rows []shortcutRow, cursor int) shortcutRow`

- [ ] **Step 1: Write the failing tests**

Create `shortcut_rows_test.go`:

```go
package main

import "testing"

func testRowInputs() ([]AIShortcut, []FabricPattern) {
	return []AIShortcut{
			{Name: "tldr", Description: "Add a TL;DR at the top"},
			{Name: "critique", Description: "Flag issues"},
		}, []FabricPattern{
			{Name: "summarize", Description: "Summarize content"},
			{Name: "extract_wisdom", Description: "Pull out insights"},
		}
}

func TestBuildShortcutRows_HeaderSeparatesSections(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5 (2 shortcuts + header + 2 patterns)", len(rows))
	}
	kinds := []rowKind{rowShortcut, rowShortcut, rowHeader, rowFabric, rowFabric}
	for i, want := range kinds {
		if rows[i].kind != want {
			t.Errorf("row %d kind = %v, want %v", i, rows[i].kind, want)
		}
	}
	if rows[2].name != fabricSectionTitle {
		t.Errorf("header name = %q, want %q", rows[2].name, fabricSectionTitle)
	}
	if rows[3].index != 0 || rows[4].index != 1 {
		t.Errorf("pattern indexes = [%d, %d], want [0, 1]", rows[3].index, rows[4].index)
	}
}

func TestBuildShortcutRows_NoHeaderWithoutPatterns(t *testing.T) {
	shortcuts, _ := testRowInputs()
	rows := buildShortcutRows(shortcuts, nil, "")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, r := range rows {
		if r.kind != rowShortcut {
			t.Errorf("row %d kind = %v, want rowShortcut", i, r.kind)
		}
	}
}

func TestBuildShortcutRows_FilterNarrowsBothSections(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "summ")
	// "summarize" matches; no shortcut does, so the header leads.
	if len(rows) != 2 {
		t.Fatalf("got %d rows (%v), want 2", len(rows), rows)
	}
	if rows[0].kind != rowHeader {
		t.Errorf("row 0 kind = %v, want rowHeader", rows[0].kind)
	}
	if rows[1].kind != rowFabric || rows[1].name != "summarize" {
		t.Errorf("row 1 = %+v, want the summarize pattern", rows[1])
	}
}

func TestBuildShortcutRows_FilterExcludingAllPatternsDropsHeader(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "tldr")
	if len(rows) != 1 {
		t.Fatalf("got %d rows (%v), want 1", len(rows), rows)
	}
	if rows[0].kind != rowShortcut || rows[0].name != "tldr" {
		t.Errorf("row 0 = %+v, want the tldr shortcut", rows[0])
	}
}

func TestBuildShortcutRows_FilterMatchingNothingReturnsEmpty(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	if rows := buildShortcutRows(shortcuts, patterns, "zzzzqqq"); len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestNextSelectableRow_SkipsHeader(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if got := nextSelectableRow(rows, 1, 1); got != 3 {
		t.Errorf("down from 1 = %d, want 3 (header at 2 skipped)", got)
	}
	if got := nextSelectableRow(rows, 3, -1); got != 1 {
		t.Errorf("up from 3 = %d, want 1 (header at 2 skipped)", got)
	}
}

func TestNextSelectableRow_StopsAtEnds(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if got := nextSelectableRow(rows, 0, -1); got != 0 {
		t.Errorf("up from 0 = %d, want 0", got)
	}
	if got := nextSelectableRow(rows, 4, 1); got != 4 {
		t.Errorf("down from 4 = %d, want 4", got)
	}
}

func TestClampSelectableRow_MovesOffHeader(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if got := clampSelectableRow(rows, 2); got != 1 {
		t.Errorf("clamp on header = %d, want 1 (prefers backwards)", got)
	}
	if got := clampSelectableRow(rows, 99); got != 4 {
		t.Errorf("clamp past end = %d, want 4", got)
	}
	// Header first: the only way off it is forwards.
	patternsOnly := buildShortcutRows(nil, patterns, "")
	if got := clampSelectableRow(patternsOnly, 0); got != 1 {
		t.Errorf("clamp on leading header = %d, want 1", got)
	}
	if got := clampSelectableRow(nil, 3); got != 0 {
		t.Errorf("clamp on empty rows = %d, want 0", got)
	}
}

func TestClampShortcutOffset_KeepsCursorVisible(t *testing.T) {
	if got := clampShortcutOffset(12, 0, 10); got != 3 {
		t.Errorf("scrolling down = %d, want 3", got)
	}
	if got := clampShortcutOffset(2, 5, 10); got != 2 {
		t.Errorf("scrolling up = %d, want 2", got)
	}
	if got := clampShortcutOffset(4, 0, 10); got != 0 {
		t.Errorf("cursor already visible = %d, want 0", got)
	}
	if got := clampShortcutOffset(4, 0, 0); got != 0 {
		t.Errorf("zero-height window = %d, want 0", got)
	}
}

func TestVisibleShortcutRows_ReservesFooter(t *testing.T) {
	if got := visibleShortcutRows(20); got != 18 {
		t.Errorf("visibleShortcutRows(20) = %d, want 18", got)
	}
	if got := visibleShortcutRows(1); got != 1 {
		t.Errorf("visibleShortcutRows(1) = %d, want 1 (never below one row)", got)
	}
}

func TestSelectedRow_OutOfRangeIsNotSelectable(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if got := selectedRow(rows, 0); got.kind != rowShortcut || got.index != 0 {
		t.Errorf("selectedRow(0) = %+v", got)
	}
	if got := selectedRow(rows, 99); got.kind != rowHeader {
		t.Errorf("selectedRow(99) kind = %v, want rowHeader (unselectable sentinel)", got.kind)
	}
	if got := selectedRow(nil, 0); got.kind != rowHeader {
		t.Errorf("selectedRow on empty rows kind = %v, want rowHeader", got.kind)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestBuildShortcutRows|TestNextSelectableRow|TestClampSelectableRow|TestClampShortcutOffset|TestVisibleShortcutRows|TestSelectedRow' -v`
Expected: FAIL — `undefined: buildShortcutRows` and friends.

- [ ] **Step 3: Write the implementation**

Create `shortcut_rows.go`:

```go
package main

import (
	"github.com/sahilm/fuzzy"
)

type rowKind int

const (
	rowShortcut rowKind = iota
	rowHeader
	rowFabric
)

// fabricSectionTitle labels the read-only fabric patterns below the user's own
// shortcuts.
const fabricSectionTitle = "── Fabric patterns ──"

// shortcutRow is one line of the Ctrl+G picker. The picker mixes editable
// shortcuts with read-only fabric patterns under a non-selectable header, so
// the cursor indexes rows rather than either underlying slice.
type shortcutRow struct {
	kind        rowKind
	index       int // into shortcuts (rowShortcut) or patterns (rowFabric); -1 for rowHeader
	name        string
	description string
}

// nameSource adapts a name slice to the fuzzy matcher, mirroring fileSource in
// filter.go.
type nameSource []string

func (n nameSource) String(i int) string { return n[i] }
func (n nameSource) Len() int            { return len(n) }

// matchIndexes returns the indexes of names matching filter, in fuzzy-rank
// order. An empty filter keeps everything in its original order.
func matchIndexes(names []string, filter string) []int {
	if filter == "" {
		idx := make([]int, len(names))
		for i := range names {
			idx[i] = i
		}
		return idx
	}
	matches := fuzzy.FindFrom(filter, nameSource(names))
	idx := make([]int, len(matches))
	for i, m := range matches {
		idx[i] = m.Index
	}
	return idx
}

// buildShortcutRows flattens the user's shortcuts and the installed fabric
// patterns into picker rows. The section header is emitted only when a pattern
// row follows it, so a vault without fabric installed — or a filter that
// excludes every pattern — leaves the picker looking exactly as it did before
// patterns existed.
func buildShortcutRows(shortcuts []AIShortcut, patterns []FabricPattern, filter string) []shortcutRow {
	names := make([]string, len(shortcuts))
	for i, s := range shortcuts {
		names[i] = s.Name
	}
	var rows []shortcutRow
	for _, i := range matchIndexes(names, filter) {
		rows = append(rows, shortcutRow{
			kind:        rowShortcut,
			index:       i,
			name:        shortcuts[i].Name,
			description: shortcuts[i].Description,
		})
	}

	patternNames := make([]string, len(patterns))
	for i, p := range patterns {
		patternNames[i] = p.Name
	}
	matched := matchIndexes(patternNames, filter)
	if len(matched) == 0 {
		return rows
	}
	rows = append(rows, shortcutRow{kind: rowHeader, index: -1, name: fabricSectionTitle})
	for _, i := range matched {
		rows = append(rows, shortcutRow{
			kind:        rowFabric,
			index:       i,
			name:        patterns[i].Name,
			description: patterns[i].Description,
		})
	}
	return rows
}

// nextSelectableRow moves from cursor by delta (+1 or -1), skipping header
// rows. Returns cursor unchanged when no selectable row lies that way.
func nextSelectableRow(rows []shortcutRow, cursor, delta int) int {
	for i := cursor + delta; i >= 0 && i < len(rows); i += delta {
		if rows[i].kind != rowHeader {
			return i
		}
	}
	return cursor
}

// clampSelectableRow pulls cursor into range and off any header row,
// preferring the row above. Returns 0 when nothing is selectable.
func clampSelectableRow(rows []shortcutRow, cursor int) int {
	if len(rows) == 0 {
		return 0
	}
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	if rows[cursor].kind != rowHeader {
		return cursor
	}
	if i := nextSelectableRow(rows, cursor, -1); rows[i].kind != rowHeader {
		return i
	}
	if i := nextSelectableRow(rows, cursor, 1); rows[i].kind != rowHeader {
		return i
	}
	return cursor
}

// clampShortcutOffset scrolls the visible window so cursor stays inside it,
// mirroring how filterOffset tracks filterCursor for the file tree.
func clampShortcutOffset(cursor, offset, visible int) int {
	if visible <= 0 || offset < 0 {
		return 0
	}
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+visible {
		return cursor - visible + 1
	}
	return offset
}

// visibleShortcutRows is how many rows fit above the picker footer (the
// provider line and the hint line).
func visibleShortcutRows(height int) int {
	n := height - 2
	if n < 1 {
		n = 1
	}
	return n
}

// selectedRow returns the row under the cursor. An out-of-range cursor yields
// an unselectable header sentinel so callers can treat it like the section
// header without a separate bounds check.
func selectedRow(rows []shortcutRow, cursor int) shortcutRow {
	if cursor < 0 || cursor >= len(rows) {
		return shortcutRow{kind: rowHeader, index: -1}
	}
	return rows[cursor]
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run 'TestBuildShortcutRows|TestNextSelectableRow|TestClampSelectableRow|TestClampShortcutOffset|TestVisibleShortcutRows|TestSelectedRow' -v`
Expected: PASS (12 tests).

- [ ] **Step 5: Verify the whole tree**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 5: Funnel every AI run through one helper

Three code paths start a picker-initiated AI run: pressing Enter on a shortcut, pressing Enter on a pattern (Task 7), and resuming after the provider config wizard. The resume path in `plugin_input.go:72` currently re-derives the shortcut as `m.shortcuts[m.shortcutCursor]`, which breaks once the cursor indexes rows, and it always opens the diff view even for a `review`-type shortcut. Capturing the run as a value fixes both.

**Files:**
- Create: `ai_run.go`
- Modify: `shortcuts_input.go:38-80` (the `enter` case), `plugin_input.go:62-93`, `model.go:162` (replace the `shortcutPending` field)
- Test: `ai_run_test.go`

**Interfaces:**
- Consumes: `aiInputContent`, `newDiffViewports`, `streamPluginCmd`, `runShortcutStream`, `resolveShortcutType`.
- Produces:
  - `type aiRun struct { review bool; start func(ctx context.Context, content, provider string, cfg map[string]string) (<-chan string, <-chan error) }`
  - `func shortcutRun(s AIShortcut) aiRun`
  - `func (m *model) startAIRun(run aiRun, provider string, cfg map[string]string) tea.Cmd`
  - model field `pendingAIRun *aiRun` replacing `shortcutPending bool`

- [ ] **Step 1: Write the failing test**

Create `ai_run_test.go`:

```go
package main

import (
	"context"
	"testing"
)

func closedStream() (<-chan string, <-chan error) {
	chunks := make(chan string)
	errs := make(chan error)
	close(chunks)
	close(errs)
	return chunks, errs
}

func TestStartAIRun_ReviewOpensReadOnlyPane(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("note body")

	run := aiRun{review: true, start: func(context.Context, string, string, map[string]string) (<-chan string, <-chan error) {
		return closedStream()
	}}
	m.startAIRun(run, "openrouter", map[string]string{"api_key": "k", "model": "m"})

	if m.inputMode != inputPluginReview {
		t.Errorf("inputMode = %v, want inputPluginReview", m.inputMode)
	}
	if m.aiRunOnSelection {
		t.Error("aiRunOnSelection = true; a review must never edit the note")
	}
	if !m.pluginProcessing {
		t.Error("pluginProcessing = false, want true")
	}
}

func TestStartAIRun_ReplaceOpensDiff(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("note body")

	run := aiRun{review: false, start: func(context.Context, string, string, map[string]string) (<-chan string, <-chan error) {
		return closedStream()
	}}
	m.startAIRun(run, "openrouter", map[string]string{"api_key": "k", "model": "m"})

	if m.inputMode != inputPluginDiff {
		t.Errorf("inputMode = %v, want inputPluginDiff", m.inputMode)
	}
	if m.pluginDiffOriginal != "note body" {
		t.Errorf("pluginDiffOriginal = %q, want %q", m.pluginDiffOriginal, "note body")
	}
}

func TestShortcutRun_CarriesResolvedType(t *testing.T) {
	if got := shortcutRun(AIShortcut{Name: "critique", Type: "review"}); !got.review {
		t.Error("review-type shortcut produced a non-review run")
	}
	if got := shortcutRun(AIShortcut{Name: "tldr", Type: "replace"}); got.review {
		t.Error("replace-type shortcut produced a review run")
	}
	// Type inferred from the name when unset, as resolveShortcutType does.
	if got := shortcutRun(AIShortcut{Name: "questions"}); !got.review {
		t.Error("untyped 'questions' shortcut should infer review")
	}
}

func TestPluginConfigCompletion_ResumesPendingReviewRun(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("note body")
	plugin := &fakePlugin{name: "openrouter"}
	m.plugins = []Plugin{plugin}
	m.pluginActive = plugin
	m.pluginConfigFields = plugin.ConfigFields()
	m.pluginConfigIndex = len(m.pluginConfigFields) - 1
	m.pluginConfigValues = map[string]string{"api_key": "k"}
	m.pluginConfigInput.SetValue("some-model")
	m.inputMode = inputPluginConfig
	run := aiRun{review: true, start: func(context.Context, string, string, map[string]string) (<-chan string, <-chan error) {
		return closedStream()
	}}
	m.pendingAIRun = &run

	next, _ := m.handlePluginConfig(pressEnter())
	nm := next.(model)

	if nm.inputMode != inputPluginReview {
		t.Errorf("inputMode = %v, want inputPluginReview — a pending review run must not open the diff", nm.inputMode)
	}
	if nm.pendingAIRun != nil {
		t.Error("pendingAIRun still set after resume")
	}
}
```

`fakePlugin`, `pressEnter`, `newTestModel`, and `setEditorSize` already exist in the test suite (`plugin_selection_test.go` and its neighbours). If `fakePlugin.ConfigFields()` returns a single field, drop the `pluginConfigValues` seeding and set `m.pluginConfigIndex = 0` instead — check the definition before running.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestStartAIRun|TestShortcutRun|TestPluginConfigCompletion' -v`
Expected: FAIL — `undefined: aiRun`.

- [ ] **Step 3: Write `ai_run.go`**

```go
package main

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// aiRun is one invocation started from the shortcut picker — a native
// shortcut or a fabric pattern. Holding the run as a value (rather than a
// cursor into a slice) lets it survive the provider config wizard, which
// otherwise resumes against a cursor that may since have moved.
type aiRun struct {
	review bool // render the read-only review pane instead of the diff
	start  func(ctx context.Context, content, provider string, cfg map[string]string) (<-chan string, <-chan error)
}

// shortcutRun builds the run for a native AI shortcut.
func shortcutRun(s AIShortcut) aiRun {
	return aiRun{
		review: resolveShortcutType(s) == "review",
		start: func(ctx context.Context, content, provider string, cfg map[string]string) (<-chan string, <-chan error) {
			return runShortcutStream(ctx, s, content, provider, cfg)
		},
	}
}

// startAIRun begins streaming and switches to the diff or review view.
func (m *model) startAIRun(run aiRun, provider string, cfg map[string]string) tea.Cmd {
	content, onSelection := m.aiInputContent()
	m.aiRunOnSelection = onSelection
	m.pluginDiffOriginal = content
	m.pluginDiffResult = ""
	m.pluginProcessing = true
	ctx, cancel := context.WithCancel(context.Background())
	m.pluginCancel = cancel
	m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(content, "", m.editorWidth, m.editorHeight)
	m.paneFocus = paneFocusRight
	if run.review {
		m.inputMode = inputPluginReview
		m.aiRunOnSelection = false // a review is read-only, it never replaces a selection
	} else {
		m.inputMode = inputPluginDiff
	}
	chunks, errs := run.start(ctx, content, provider, cfg)
	m.activeChunks = chunks
	return streamPluginCmd(chunks, errs)
}
```

- [ ] **Step 4: Swap the model field**

In `model.go`, replace the `shortcutPending` field in the AI shortcuts block:

```go
	pendingAIRun             *aiRun // set when a picker run waits on the provider config wizard
```

- [ ] **Step 5: Rewrite the `enter` case in `shortcuts_input.go`**

Replace the whole `case "enter":` body in `handleShortcutSelect` with:

```go
	case "enter":
		if len(m.shortcuts) == 0 || m.shortcutCursor >= len(m.shortcuts) {
			return m, nil
		}
		run := shortcutRun(m.shortcuts[m.shortcutCursor])
		provider := m.activeShortcutProvider
		if provider == "" {
			provider = defaultAIShortcutProvider
		}
		plugin := pluginByName(m.plugins, provider)
		if plugin == nil {
			m.errMsg = "Unknown AI shortcut provider: " + provider
			return m, nil
		}
		cfg, err := loadPluginConfig(provider)
		if err != nil || !pluginConfigComplete(plugin.ConfigFields(), cfg) {
			m.pendingAIRun = &run
			m.pluginActive = plugin
			m.pluginConfigFields = plugin.ConfigFields()
			m.pluginConfigIndex = 0
			m.pluginConfigValues = make(map[string]string)
			m.inputMode = inputPluginConfig
			m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[0])
			return m, textinput.Blink
		}
		return m, m.startAIRun(run, provider, cfg)
```

Task 7 replaces the cursor arithmetic here with row lookups; this task only removes the duplicated stream-start block.

- [ ] **Step 6: Rewrite the resume path in `plugin_input.go`**

Replace lines 66-88 (the `m.shortcutPending` error reset and the pending-shortcut block) with:

```go
			if err := savePluginConfig(m.pluginActive.Name(), m.pluginConfigValues); err != nil {
				m.errMsg = "Failed to save plugin config: " + err.Error()
				m.inputMode = inputNone
				m.pendingAIRun = nil
				return m, nil
			}
			// If config was triggered from the shortcut picker, run what was
			// waiting on it.
			if m.pendingAIRun != nil {
				run := *m.pendingAIRun
				m.pendingAIRun = nil
				provider := m.pluginActive.Name()
				cfg, _ := loadPluginConfig(provider)
				return m, m.startAIRun(run, provider, cfg)
			}
```

Then delete the now-unused `"context"` import from `plugin_input.go` if `go vet` flags it.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./... -run 'TestStartAIRun|TestShortcutRun|TestPluginConfigCompletion' -v`
Expected: PASS (4 tests).

- [ ] **Step 8: Verify the whole tree**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS — including the pre-existing `TestShortcutSelect_WithSelection_SendsOnlySelection`, which exercises the rewritten `enter` path.

---

### Task 6: Render rows with a scroll window

`shortcutSelectorView` renders every shortcut and lets `MaxHeight` clip the overflow. With ~250 patterns the cursor would walk off the bottom into nothing. It now takes rows plus a scroll offset and renders only the visible slice.

**Files:**
- Modify: `shortcuts_modal.go:45-100`, `model.go:2151-2152` (view wiring), `model.go` (two new fields, `newModel` init, `shortcutRows` helper)
- Test: `shortcuts_modal_test.go`

**Interfaces:**
- Consumes: `shortcutRow`, `visibleShortcutRows`, `truncateRight`.
- Produces: `func shortcutSelectorView(rows []shortcutRow, cursor, offset int, provider string, filtering bool, width, height int) string`; `func (m model) shortcutRows() []shortcutRow`; model fields `fabricPatterns []FabricPattern`, `shortcutOffset int`, `shortcutFilterInput textinput.Model`.

- [ ] **Step 1: Write the failing tests**

Rewrite `shortcuts_modal_test.go`'s four existing calls to build rows first, e.g. the first test becomes:

```go
func TestShortcutSelectorView_ShowsDescriptions(t *testing.T) {
	rows := buildShortcutRows([]AIShortcut{
		{Name: "prd", Description: "Turn text into a PRD with TBDs for gaps"},
		{Name: "tldr", Description: "Add a TL;DR at the top"},
	}, nil, "")
	out := shortcutSelectorView(rows, 0, 0, "openrouter", false, 120, 20)
	// ...existing assertions unchanged...
}
```

Apply the same shape to `TestShortcutSelectorView_NamesAlignToLongest`, the truncation test at line 78 (`..., 0, 0, "openrouter", false, 30, 20`), and the empty test at line 90 (`shortcutSelectorView(nil, 0, 0, "openrouter", false, 80, 10)`).

Then append:

```go
func TestShortcutSelectorView_RendersFabricSection(t *testing.T) {
	rows := buildShortcutRows(
		[]AIShortcut{{Name: "tldr", Description: "Add a TL;DR"}},
		[]FabricPattern{{Name: "summarize", Description: "Summarize content"}},
		"")
	out := shortcutSelectorView(rows, 0, 0, "openrouter", false, 120, 20)
	if !strings.Contains(out, fabricSectionTitle) {
		t.Error("missing fabric section header")
	}
	if !strings.Contains(out, "summarize") {
		t.Error("missing fabric pattern name")
	}
	if !strings.Contains(out, "Summarize content") {
		t.Error("missing fabric pattern description")
	}
}

func TestShortcutSelectorView_ScrollsToOffset(t *testing.T) {
	var shortcuts []AIShortcut
	for _, n := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"} {
		shortcuts = append(shortcuts, AIShortcut{Name: n})
	}
	rows := buildShortcutRows(shortcuts, nil, "")
	// height 5 leaves 3 visible rows after the two footer lines.
	out := shortcutSelectorView(rows, 4, 2, "openrouter", false, 120, 5)
	if strings.Contains(out, "alpha") || strings.Contains(out, "bravo") {
		t.Error("rows above the offset are still rendered")
	}
	if !strings.Contains(out, "echo") {
		t.Error("cursor row 'echo' is not rendered")
	}
	if strings.Contains(out, "foxtrot") {
		t.Error("row past the window is rendered")
	}
}

func TestShortcutSelectorView_FilteringSwapsHint(t *testing.T) {
	rows := buildShortcutRows([]AIShortcut{{Name: "tldr"}}, nil, "")
	if out := shortcutSelectorView(rows, 0, 0, "openrouter", false, 120, 20); !strings.Contains(out, "/:filter") {
		t.Error("normal hint should advertise the filter key")
	}
	if out := shortcutSelectorView(rows, 0, 0, "openrouter", true, 120, 20); !strings.Contains(out, "Esc:clear") {
		t.Error("filter hint should advertise clearing the filter")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run TestShortcutSelectorView -v`
Expected: FAIL — argument count mismatch on `shortcutSelectorView`.

- [ ] **Step 3: Rewrite `shortcutSelectorView`**

In `shortcuts_modal.go`, add a style beside the existing ones and replace the function body:

```go
	shortcutSectionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Bold(true).
				PaddingLeft(1)
```

```go
// maxShortcutNameCol caps the name column so long fabric pattern names cannot
// squeeze the description column out of existence.
const maxShortcutNameCol = 30

// shortcutSelectorView renders the visible slice of picker rows. offset is the
// first row shown; the caller keeps the cursor inside the window via
// clampShortcutOffset.
func shortcutSelectorView(rows []shortcutRow, cursor, offset int, provider string, filtering bool, width, height int) string {
	box := lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)

	if len(rows) == 0 {
		msg := "No shortcuts. Press Ctrl+L to create one."
		if filtering {
			msg = "No matches."
		}
		return box.Height(height).Render(shortcutEmptyStyle.Render(msg))
	}

	maxName := 0
	for _, r := range rows {
		if r.kind == rowHeader {
			continue
		}
		if n := len([]rune(r.name)); n > maxName {
			maxName = n
		}
	}
	if maxName > maxShortcutNameCol {
		maxName = maxShortcutNameCol
	}
	nameCol := maxName + 2

	descBudget := width - 2 - 2 - nameCol - 3
	if descBudget < 0 {
		descBudget = 0
	}

	if offset < 0 || offset >= len(rows) {
		offset = 0
	}
	end := offset + visibleShortcutRows(height)
	if end > len(rows) {
		end = len(rows)
	}

	var out []string
	for i := offset; i < end; i++ {
		r := rows[i]
		if r.kind == rowHeader {
			out = append(out, shortcutSectionStyle.Render(r.name))
			continue
		}
		name := truncateRight(r.name, maxName)
		namePart := name + strings.Repeat(" ", nameCol-len([]rune(name)))
		var line string
		if i == cursor {
			line = shortcutCursorStyle.Render("> " + namePart)
		} else {
			line = shortcutItemStyle.Render("  " + namePart)
		}
		if r.description != "" {
			if desc := truncateRight(r.description, descBudget); desc != "" {
				line += shortcutDescStyle.Render("— " + desc)
			}
		}
		out = append(out, line)
	}

	providerLine := shortcutHintStyle.Render("Provider: " + provider + "  (p:cycle)")
	hint := shortcutHintStyle.Render("Enter:run  /:filter  e:edit  d:delete  Ctrl+↑/↓:reorder  Esc:close")
	if filtering {
		hint = shortcutHintStyle.Render("Enter:run  ↑/↓:move  Esc:clear filter")
	}
	return box.Render(strings.Join(out, "\n") + "\n" + providerLine + "\n" + hint)
}
```

- [ ] **Step 4: Add the model state and wire the view**

In `model.go`, add to the AI shortcuts field block:

```go
	fabricPatterns           []FabricPattern
	shortcutOffset           int
	shortcutFilterInput      textinput.Model
```

In `newModel`, beside the other `textinput.New()` blocks:

```go
	sfi := textinput.New()
	sfi.Placeholder = "filter shortcuts and patterns…"
	sfi.CharLimit = 128
```

and in the `m := model{` literal:

```go
		shortcutFilterInput:      sfi,
```

Add the row helper — put it in `shortcut_rows.go` below `selectedRow`:

```go
// shortcutRows is the picker's current row list: the user's shortcuts and the
// installed fabric patterns, narrowed by the active filter.
func (m model) shortcutRows() []shortcutRow {
	return buildShortcutRows(m.shortcuts, m.fabricPatterns, m.shortcutFilterInput.Value())
}
```

Replace the view branch at `model.go:2151-2152`:

```go
	} else if m.inputMode == inputShortcutSelect {
		rightView = shortcutSelectorView(m.shortcutRows(), m.shortcutCursor, m.shortcutOffset,
			m.activeShortcutProvider, false, m.editorWidth, m.editorHeight)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./... -run TestShortcutSelectorView -v`
Expected: PASS (7 tests).

- [ ] **Step 6: Verify the whole tree**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS. `m.fabricPatterns` is still empty at this point — Task 7 populates it.

---

### Task 7: Row-aware navigation and running a pattern

**Files:**
- Modify: `shortcuts_input.go` (`handleShortcutSelect`, `handleShortcutDeleteConfirm`), `model.go:847-853` (the `ctrl+g` case), `model.go:2309-2315` (delete-confirm status bar)
- Test: `shortcuts_input_test.go`

**Interfaces:**
- Consumes: everything from Tasks 2-6.
- Produces: `func (m model) selectedShortcutIndex() int`; `func fabricRun(dir string, p FabricPattern) (aiRun, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `shortcuts_input_test.go`:

```go
func TestShortcutSelector_DownSkipsFabricHeader(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{{Name: "tldr", Prompt: "p"}}
	m.fabricPatterns = []FabricPattern{{Name: "summarize"}}
	m.shortcutCursor = 0
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(tea.KeyMsg{Type: tea.KeyDown})
	nm := next.(model)
	if nm.shortcutCursor != 2 {
		t.Errorf("cursor = %d, want 2 (header at row 1 skipped)", nm.shortcutCursor)
	}
}

func TestShortcutSelector_EditOnPatternIsRejected(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{{Name: "tldr", Prompt: "p"}}
	m.fabricPatterns = []FabricPattern{{Name: "summarize"}}
	m.shortcutCursor = 2 // the pattern row
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	nm := next.(model)
	if nm.inputMode != inputShortcutSelect {
		t.Errorf("inputMode = %v, want inputShortcutSelect — patterns are not editable", nm.inputMode)
	}
	if nm.errMsg == "" {
		t.Error("expected an errMsg explaining patterns are read-only")
	}
}

func TestShortcutSelector_DeleteOnPatternIsRejected(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{{Name: "tldr", Prompt: "p"}}
	m.fabricPatterns = []FabricPattern{{Name: "summarize"}}
	m.shortcutCursor = 2
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if next.(model).inputMode != inputShortcutSelect {
		t.Error("'d' on a pattern row opened the delete confirmation")
	}
}

func TestShortcutSelector_EnterOnPatternOpensReview(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FABRIC_CONFIG_HOME", filepath.Dir(dir))
	patterns := filepath.Join(filepath.Dir(dir), "patterns")
	if err := os.MkdirAll(filepath.Join(patterns, "summarize"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(patterns, "summarize", "system.md"), []byte("# SYS"), 0o644); err != nil {
		t.Fatalf("write system.md: %v", err)
	}

	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("note body")
	provider := defaultAIShortcutProvider
	m.plugins = []Plugin{&fakePlugin{name: provider}}
	m.activeShortcutProvider = provider
	if err := savePluginConfig(provider, map[string]string{"api_key": "k", "model": "m"}); err != nil {
		t.Fatalf("savePluginConfig: %v", err)
	}
	m.shortcuts = nil
	m.fabricPatterns = listFabricPatterns(fabricPatternsDir())
	if len(m.fabricPatterns) != 1 {
		t.Fatalf("got %d patterns, want 1", len(m.fabricPatterns))
	}
	m.shortcutCursor = 1 // row 0 is the header
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(pressEnter())
	nm := next.(model)
	if nm.inputMode != inputPluginReview {
		t.Errorf("inputMode = %v, want inputPluginReview", nm.inputMode)
	}
	if nm.aiRunOnSelection {
		t.Error("aiRunOnSelection = true; a pattern run must never edit the note")
	}
}

func TestSelectedShortcutIndex_MinusOneOnPatternRow(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{{Name: "tldr"}}
	m.fabricPatterns = []FabricPattern{{Name: "summarize"}}
	m.shortcutCursor = 0
	if got := m.selectedShortcutIndex(); got != 0 {
		t.Errorf("on a shortcut row = %d, want 0", got)
	}
	m.shortcutCursor = 1 // header
	if got := m.selectedShortcutIndex(); got != -1 {
		t.Errorf("on the header = %d, want -1", got)
	}
	m.shortcutCursor = 2 // pattern
	if got := m.selectedShortcutIndex(); got != -1 {
		t.Errorf("on a pattern row = %d, want -1", got)
	}
}
```

Add `"os"`, `"path/filepath"` to the test file's imports if absent.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestShortcutSelector_Down|TestShortcutSelector_EditOn|TestShortcutSelector_DeleteOn|TestShortcutSelector_EnterOn|TestSelectedShortcutIndex' -v`
Expected: FAIL — cursor lands on 1 instead of 2, `e` opens the editor, `selectedShortcutIndex` undefined.

- [ ] **Step 3: Add `fabricRun` and `selectedShortcutIndex`**

Append to `ai_run.go`:

```go
// fabricRun builds the run for a fabric pattern. Patterns always render as a
// read-only review: most of them are analysis or extraction, so replacing the
// note with their output would destroy it.
func fabricRun(dir string, p FabricPattern) (aiRun, error) {
	system, user, err := loadFabricPattern(dir, p.Name)
	if err != nil {
		return aiRun{}, err
	}
	return aiRun{
		review: true,
		start: func(ctx context.Context, content, provider string, cfg map[string]string) (<-chan string, <-chan error) {
			return runFabricPatternStream(ctx, system, user, content, provider, cfg)
		},
	}, nil
}
```

Append to `shortcut_rows.go`:

```go
// selectedShortcutIndex is the index into m.shortcuts under the cursor, or -1
// when the cursor sits on the fabric header or a pattern row.
func (m model) selectedShortcutIndex() int {
	row := selectedRow(m.shortcutRows(), m.shortcutCursor)
	if row.kind != rowShortcut {
		return -1
	}
	return row.index
}
```

- [ ] **Step 4: Rewrite `handleShortcutSelect`**

Replace the navigation, reorder, enter, edit, and delete cases in `shortcuts_input.go`:

```go
func (m model) handleShortcutSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.shortcutRows()
	visible := visibleShortcutRows(m.editorHeight)
	switch msg.String() {
	case "up", "k":
		m.shortcutCursor = nextSelectableRow(rows, m.shortcutCursor, -1)
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visible)
	case "down", "j":
		m.shortcutCursor = nextSelectableRow(rows, m.shortcutCursor, 1)
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visible)
	case "ctrl+up", "ctrl+down":
		if m.shortcutFilterInput.Value() != "" {
			m.errMsg = "Clear the filter to reorder shortcuts"
			return m, nil
		}
		i := m.selectedShortcutIndex()
		if i < 0 {
			m.errMsg = "Fabric patterns are read-only"
			return m, nil
		}
		j := i - 1
		if msg.String() == "ctrl+down" {
			j = i + 1
		}
		if j < 0 || j >= len(m.shortcuts) {
			return m, nil
		}
		m.shortcuts[i], m.shortcuts[j] = m.shortcuts[j], m.shortcuts[i]
		m.shortcutCursor += j - i
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visible)
		if err := saveShortcuts(m.shortcuts); err != nil {
			m.errMsg = "Failed to save shortcuts: " + err.Error()
		}
	case "enter":
		row := selectedRow(rows, m.shortcutCursor)
		var run aiRun
		switch row.kind {
		case rowShortcut:
			run = shortcutRun(m.shortcuts[row.index])
		case rowFabric:
			var err error
			run, err = fabricRun(fabricPatternsDir(), m.fabricPatterns[row.index])
			if err != nil {
				m.errMsg = "Failed to load fabric pattern: " + err.Error()
				return m, nil
			}
		default:
			return m, nil
		}
		provider := m.activeShortcutProvider
		if provider == "" {
			provider = defaultAIShortcutProvider
		}
		plugin := pluginByName(m.plugins, provider)
		if plugin == nil {
			m.errMsg = "Unknown AI shortcut provider: " + provider
			return m, nil
		}
		cfg, err := loadPluginConfig(provider)
		if err != nil || !pluginConfigComplete(plugin.ConfigFields(), cfg) {
			m.pendingAIRun = &run
			m.pluginActive = plugin
			m.pluginConfigFields = plugin.ConfigFields()
			m.pluginConfigIndex = 0
			m.pluginConfigValues = make(map[string]string)
			m.inputMode = inputPluginConfig
			m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[0])
			return m, textinput.Blink
		}
		return m, m.startAIRun(run, provider, cfg)
	case "/":
		m.inputMode = inputShortcutFilter
		m.shortcutFilterInput.SetValue("")
		m.shortcutCursor = clampSelectableRow(m.shortcutRows(), 0)
		m.shortcutOffset = 0
		return m, m.shortcutFilterInput.Focus()
	case "p":
		// ...unchanged...
	case "e":
		i := m.selectedShortcutIndex()
		if i < 0 {
			m.errMsg = "Fabric patterns are read-only"
			return m, nil
		}
		m.shortcutEditing = i
		m.inputMode = inputShortcutName
		m.shortcutNameInput.SetValue(m.shortcuts[i].Name)
		cmd := m.shortcutNameInput.Focus()
		return m, cmd
	case "d":
		if m.selectedShortcutIndex() < 0 {
			m.errMsg = "Fabric patterns are read-only"
			return m, nil
		}
		m.inputMode = inputShortcutDeleteConfirm
	case "esc":
		m.inputMode = inputNone
	case "ctrl+q":
		// ...unchanged...
	}
	return m, nil
}
```

The `/` case references `inputShortcutFilter`, which Task 8 adds. To keep this task compiling on its own, add the constant now: in `model.go`, append `inputShortcutFilter` to the end of the `inputMode` const block (appending avoids renumbering the existing values).

- [ ] **Step 5: Make delete row-aware**

In `handleShortcutDeleteConfirm`, replace the `case "y":` body:

```go
	case "y":
		if i := m.selectedShortcutIndex(); i >= 0 {
			m.shortcuts = append(m.shortcuts[:i], m.shortcuts[i+1:]...)
			if err := saveShortcuts(m.shortcuts); err != nil {
				m.errMsg = "Failed to save shortcuts: " + err.Error()
			}
		}
		rows := m.shortcutRows()
		m.shortcutCursor = clampSelectableRow(rows, m.shortcutCursor)
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visibleShortcutRows(m.editorHeight))
		if len(rows) == 0 {
			m.inputMode = inputNone
		} else {
			m.inputMode = inputShortcutSelect
		}
```

In `model.go`, the delete-confirm status bar branch resolves through the helper:

```go
	} else if m.inputMode == inputShortcutDeleteConfirm {
		name := ""
		if i := m.selectedShortcutIndex(); i >= 0 {
			name = m.shortcuts[i].Name
		}
```

- [ ] **Step 6: Load patterns when the picker opens**

Replace the `ctrl+g` case in `model.go`:

```go
		case "ctrl+g":
			if m.currentFile != "" || m.newNoteDir != "" {
				m.shortcuts, _ = loadShortcuts()
				// Re-scan every open so patterns added to the fabric directory
				// show up without restarting clipad.
				m.fabricPatterns = listFabricPatterns(fabricPatternsDir())
				m.shortcutFilterInput.SetValue("")
				m.inputMode = inputShortcutSelect
				m.shortcutCursor = clampSelectableRow(m.shortcutRows(), 0)
				m.shortcutOffset = 0
			}
			return m, nil
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./... -run 'TestShortcutSelector|TestSelectedShortcutIndex' -v`
Expected: PASS, including the four pre-existing reorder tests (`CtrlDown_SwapsAndPersists`, `CtrlUp_SwapsAndPersists`, `CtrlUp_AtTop_NoOp`, `CtrlDown_AtBottom_NoOp`) — with no patterns loaded, row indexes still equal shortcut indexes.

- [ ] **Step 8: Verify the whole tree**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 8: The `/` filter mode

Mirrors the file tree's `inputFilter`: typed runes narrow the list, arrows navigate, Enter runs, Esc clears. Single-letter actions (`p`/`e`/`d`/`j`/`k`) are unavailable while filtering because those keystrokes are text — the same tradeoff the tree filter already makes.

**Files:**
- Modify: `shortcuts_input.go` (new handler), `model.go` (dispatch, view branch, status bar)
- Test: `shortcuts_input_test.go`

**Interfaces:**
- Consumes: `inputShortcutFilter` (added in Task 7), `m.shortcutFilterInput`, `clampSelectableRow`.
- Produces: `func (m model) handleShortcutFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd)`.

- [ ] **Step 1: Write the failing tests**

Append to `shortcuts_input_test.go`:

```go
func TestShortcutFilter_SlashEntersFilterMode(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{{Name: "tldr", Prompt: "p"}}
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if next.(model).inputMode != inputShortcutFilter {
		t.Errorf("inputMode = %v, want inputShortcutFilter", next.(model).inputMode)
	}
}

func TestShortcutFilter_TypingNarrowsRows(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{{Name: "tldr"}, {Name: "critique"}}
	m.fabricPatterns = []FabricPattern{{Name: "summarize"}}
	m.inputMode = inputShortcutFilter

	next, _ := m.handleShortcutFilter(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	nm := next.(model)
	if nm.shortcutFilterInput.Value() != "s" {
		t.Fatalf("filter = %q, want s", nm.shortcutFilterInput.Value())
	}
	rows := nm.shortcutRows()
	for _, r := range rows {
		if r.kind == rowShortcut && r.name == "critique" {
			t.Error("'critique' survived the 's' filter")
		}
	}
	if nm.shortcutCursor < 0 || nm.shortcutCursor >= len(rows) {
		t.Errorf("cursor = %d, out of range for %d rows", nm.shortcutCursor, len(rows))
	}
	if rows[nm.shortcutCursor].kind == rowHeader {
		t.Error("cursor parked on the section header after filtering")
	}
}

func TestShortcutFilter_EscClearsAndReturns(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{{Name: "tldr"}}
	m.inputMode = inputShortcutFilter
	m.shortcutFilterInput.SetValue("zzz")

	next, _ := m.handleShortcutFilter(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(model)
	if nm.inputMode != inputShortcutSelect {
		t.Errorf("inputMode = %v, want inputShortcutSelect", nm.inputMode)
	}
	if nm.shortcutFilterInput.Value() != "" {
		t.Errorf("filter = %q, want cleared", nm.shortcutFilterInput.Value())
	}
}

func TestShortcutFilter_ArrowsNavigateWithoutLeavingFilter(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{{Name: "alpha"}, {Name: "alpine"}}
	m.inputMode = inputShortcutFilter
	m.shortcutCursor = 0

	next, _ := m.handleShortcutFilter(tea.KeyMsg{Type: tea.KeyDown})
	nm := next.(model)
	if nm.inputMode != inputShortcutFilter {
		t.Errorf("inputMode = %v, want to stay in inputShortcutFilter", nm.inputMode)
	}
	if nm.shortcutCursor != 1 {
		t.Errorf("cursor = %d, want 1", nm.shortcutCursor)
	}
	if nm.shortcutFilterInput.Value() != "" {
		t.Errorf("arrow key typed into the filter: %q", nm.shortcutFilterInput.Value())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run TestShortcutFilter -v`
Expected: FAIL — `undefined: (model).handleShortcutFilter`.

- [ ] **Step 3: Write the handler**

Append to `shortcuts_input.go`:

```go
// handleShortcutFilter runs the picker's '/' filter, mirroring the file
// tree's inputFilter: runes narrow the list, arrows navigate, Enter runs the
// selection, Esc clears the filter and returns to the plain picker.
func (m model) handleShortcutFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.inputMode = inputShortcutSelect
		return m.handleShortcutSelect(msg)
	case "esc":
		m.shortcutFilterInput.SetValue("")
		m.shortcutFilterInput.Blur()
		m.inputMode = inputShortcutSelect
		m.shortcutCursor = clampSelectableRow(m.shortcutRows(), m.shortcutCursor)
		m.shortcutOffset = 0
		return m, nil
	case "up", "down":
		delta := 1
		if msg.String() == "up" {
			delta = -1
		}
		rows := m.shortcutRows()
		m.shortcutCursor = nextSelectableRow(rows, m.shortcutCursor, delta)
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visibleShortcutRows(m.editorHeight))
		return m, nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.shortcutFilterInput, cmd = m.shortcutFilterInput.Update(msg)
	m.shortcutCursor = clampSelectableRow(m.shortcutRows(), 0)
	m.shortcutOffset = 0
	return m, cmd
}
```

- [ ] **Step 4: Wire the mode into the model**

Dispatch — in the `switch m.inputMode` around `model.go:1156`:

```go
	case inputShortcutFilter:
		return m.handleShortcutFilter(msg)
```

View — extend the picker branch so filter mode renders the same modal:

```go
	} else if m.inputMode == inputShortcutSelect || m.inputMode == inputShortcutFilter {
		rightView = shortcutSelectorView(m.shortcutRows(), m.shortcutCursor, m.shortcutOffset,
			m.activeShortcutProvider, m.inputMode == inputShortcutFilter, m.editorWidth, m.editorHeight)
```

Status bar — beside the other shortcut input branches (around `model.go:2300`), where clipad already puts prompt text:

```go
	} else if m.inputMode == inputShortcutFilter {
		statusView = statusBarStyle.Width(m.width).Render(
			"Filter: " + m.shortcutFilterInput.View())
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./... -run TestShortcutFilter -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Verify the whole tree**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

---

### Task 9: Documentation

**Files:**
- Modify: `README.md`, `help_modal.go:80-83`

**Interfaces:**
- Consumes: the finished feature.
- Produces: nothing code-facing.

- [ ] **Step 1: Add the filter key to the in-app help**

In `help_modal.go`, the shortcut-picker key list gains a row after `{"Enter", "Run shortcut"}`:

```go
			{"/", "Filter shortcuts and fabric patterns"},
```

- [ ] **Step 2: Document fabric patterns in the README**

After the "Switching providers" paragraph (README line 233) and before "The default library covers:", insert:

```markdown
**Fabric patterns.** If you have [fabric](https://github.com/danielmiessler/fabric)
installed, the shortcut picker lists every pattern it finds under a
`── Fabric patterns ──` heading. Clipad reads the pattern files directly — it never
invokes the fabric CLI — so a pattern runs through whichever AI provider your
shortcuts use, not fabric's own model config. The pattern's `system.md` becomes the
system prompt and your note becomes the user message, which is how fabric itself
invokes them.

Patterns always open in the read-only review pane: most of them analyse or extract
rather than rewrite, so replacing your note with their output would destroy it. Press
`c` in the review to copy the result. Patterns cannot be edited, deleted, or
reordered from clipad — edit the files under `~/.config/fabric/patterns/` instead.

Clipad looks in `$FABRIC_CONFIG_HOME/patterns` when that variable is set, otherwise
`~/.config/fabric/patterns`. Descriptions come from `pattern_explanations.md` in the
same directory when fabric ships one. If fabric is not installed, the section simply
does not appear.

**Filtering.** With a few hundred patterns in the list, press `/` inside the picker to
fuzzy-filter shortcuts and patterns by name. Arrows move, `Enter` runs, `Esc` clears
the filter.
```

Also update line 104's keybinding table entry to mention the filter, adding a row after `Ctrl+G`:

```markdown
| `/` (in picker) | Filter shortcuts and fabric patterns |
```

- [ ] **Step 3: Verify**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS.

Then run a manual smoke check against the real fabric install:

```bash
go build -o /tmp/clipad-fabric . && /tmp/clipad-fabric
```

Confirm by hand: `Ctrl+G` shows the `── Fabric patterns ──` section below your own shortcuts; `j`/`k` scroll past the fold without the cursor disappearing; the header is never selected; `/` then `summ` narrows to matching rows; `Enter` on a pattern opens the read-only review; `e`/`d` on a pattern row report that patterns are read-only.

---

## Self-Review Notes

- **Spec coverage:** blackbox removal + migration → Task 1; pattern discovery and descriptions → Task 2; loading and fabric-semantics streaming → Task 3; row model, header suppression, nav, scroll math → Task 4; the `startAIRun` funnel that makes the deferred-run path row-safe → Task 5; scrolling render → Task 6; row-aware handlers, read-only guards, review-mode pattern runs, `ctrl+g` refresh → Task 7; `/` filter → Task 8; README and help → Task 9.
- **Deviation from the spec:** the spec described `FabricPattern` as carrying the prompt bodies; the plan splits listing (`listFabricPatterns`) from loading (`loadFabricPattern`) so opening the picker does not read ~250 files. The spec's Loading section already called for this; the struct just stays metadata-only.
- **Addition beyond the spec:** `fabricStreamURL` is a package-level variable so the pattern-run test can point at an httptest server. `shortcutProviderURL` has no other test seam.
- **Latent bug fixed in passing:** before Task 5, a `review`-type shortcut that triggered the provider config wizard resumed into the diff view instead of the review pane.
