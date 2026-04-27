# Quick capture + delegate-to-new-note — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `Ctrl+J` quick capture (textarea modal → append timestamped bullet to `<vault>/inbox.md`) and `Ctrl+O` delegate (status-bar prompt → cut selection out into a new sibling note), with branching behavior when `inbox.md` is open dirty/clean and a new `inbox_path` config field.

**Architecture:** One new file `capture.go` holds pure file-IO helpers, the two key handlers (`handleCapture`, `handleDelegate`), the `captureView` overlay, and the `captureAppendedMsg` async message. Wiring lives in `model.go` (input-mode enum, `model` fields, `newModel`, key dispatcher, `handleInputMode`, `Update`, `View`) and `config.go` (new `InboxPath` field). The capture flow branches on inbox state: closed → disk write; open+clean → disk write + editor reload preserving cursor; open+dirty → in-memory editor buffer append (no disk write). Delegate uses the existing `(*SelectableEditor).DeleteSelection()` and `(*model).saveCurrentFile()` so it stays fully synchronous.

**Tech Stack:** Go, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles/textarea` (capture), `github.com/charmbracelet/bubbles/textinput` (delegate), `github.com/charmbracelet/lipgloss` (overlay rendering), `github.com/pelletier/go-toml/v2` (config).

**Spec:** `docs/superpowers/specs/2026-04-27-quick-capture-and-delegate-design.md` (commit `a9c066f`).

---

## File Map

- **Create:** `capture.go` — pure helpers (`resolveInboxPath`, `formatCaptureLine`, `appendToInboxFile`, `ensureTrailingNewline`, `writeNewFile`), handlers (`handleCapture`, `handleDelegate`), `(*model).appendLineToEditor`, `(model).dispatchCapture`, `captureView`, `captureAppendCmd`, `captureAppendedMsg` type.
- **Create:** `capture_test.go` — pure-helper unit tests + model-level integration tests for capture and delegate flows.
- **Modify:** `config.go` — add `InboxPath string` to `Config` and `configTOML`; load/save passthrough.
- **Modify:** `config_test.go` — verify `inbox_path` round-trip.
- **Modify:** `model.go` — add `inputCapture` and `inputDelegateName` to the `inputMode` enum; add `inboxPath`, `captureInput`, `delegateInput` fields to `model`; extend `newModel` signature to take `inboxPath string`; init the two widgets and store `inboxPath` in the constructor; add `Ctrl+J`/`Ctrl+O` cases in main key dispatcher; route `inputCapture`/`inputDelegateName` in `handleInputMode`; handle `captureAppendedMsg` in `Update`; add overlay branch in `View()` for capture; add status-bar branch for delegate.
- **Modify:** `main.go` — pass `cfg.InboxPath` to `newModel`.
- **Modify:** `shortcuts_input_test.go` — update `newTestModel` to pass `""` for the new `inboxPath` parameter.

---

## Phase 0 — Config schema

### Task 1: Add `InboxPath` to Config

**Files:**
- Modify: `config.go`
- Modify: `config_test.go`

- [ ] **Step 1: Write the failing test**

Append to `config_test.go`:

```go
func TestConfig_InboxPathRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := Config{
		Vault:     "/tmp/vault",
		InboxPath: "journals/inbox.md",
	}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if loaded.InboxPath != "journals/inbox.md" {
		t.Errorf("InboxPath = %q, want %q", loaded.InboxPath, "journals/inbox.md")
	}
}

func TestConfig_InboxPathOmittedWhenEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := Config{Vault: "/tmp/vault"}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	data, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(data), "inbox_path") {
		t.Errorf("config contains inbox_path when empty: %q", string(data))
	}
}
```

If `os` and `strings` aren't already imported in `config_test.go`, add them.

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestConfig_InboxPath -v`
Expected: FAIL — `Config` has no `InboxPath` field.

- [ ] **Step 3: Add the field to `Config` and `configTOML`**

Edit `config.go`. The `Config` struct (currently around lines 12–22) becomes:

```go
type Config struct {
	Vault              string     `toml:"vault"`
	InboxPath          string     `toml:"inbox_path,omitempty"`
	GitRemote          string     `toml:"git_remote,omitempty"`
	LastSync           *time.Time `toml:"last_sync,omitempty"`
	AIShortcutProvider string     `toml:"ai_shortcut_provider,omitempty"`

	EmbeddingProvider string `toml:"embedding_provider,omitempty"`
	EmbeddingModel    string `toml:"embedding_model,omitempty"`
	OllamaURL         string `toml:"ollama_url,omitempty"`
}
```

Mirror the same field in `configTOML` (currently around lines 33–43):

```go
type configTOML struct {
	Vault              string `toml:"vault"`
	InboxPath          string `toml:"inbox_path,omitempty"`
	GitRemote          string `toml:"git_remote,omitempty"`
	LastSync           string `toml:"last_sync,omitempty"`
	AIShortcutProvider string `toml:"ai_shortcut_provider,omitempty"`

	EmbeddingProvider string `toml:"embedding_provider,omitempty"`
	EmbeddingModel    string `toml:"embedding_model,omitempty"`
	OllamaURL         string `toml:"ollama_url,omitempty"`
}
```

In `loadConfig()`, after the existing `cfg := Config{...}` block, add `InboxPath: ct.InboxPath,` to the literal:

```go
cfg := Config{
	Vault:              ct.Vault,
	InboxPath:          ct.InboxPath,
	GitRemote:          ct.GitRemote,
	AIShortcutProvider: ct.AIShortcutProvider,
	EmbeddingProvider:  ct.EmbeddingProvider,
	EmbeddingModel:     ct.EmbeddingModel,
	OllamaURL:          ct.OllamaURL,
}
```

In `saveConfig()`, add `InboxPath: cfg.InboxPath,` to the `ct := configTOML{...}` literal:

```go
ct := configTOML{
	Vault:              cfg.Vault,
	InboxPath:          cfg.InboxPath,
	GitRemote:          cfg.GitRemote,
	AIShortcutProvider: cfg.AIShortcutProvider,
	EmbeddingProvider:  cfg.EmbeddingProvider,
	EmbeddingModel:     cfg.EmbeddingModel,
	OllamaURL:          cfg.OllamaURL,
}
```

No load-time defaulting — the empty string is preserved through to `resolveInboxPath`.

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestConfig_InboxPath -v`
Expected: PASS, both subtests.

- [ ] **Step 5: Run the full test suite to confirm nothing else broke**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add config.go config_test.go
git commit -m "feat(config): add inbox_path field

Round-trips through loadConfig/saveConfig with omitempty.
Default value resolution is intentionally deferred to use-time
(resolveInboxPath, in a later task) so the default tracks the
current vault rather than being frozen at load time.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 1 — Pure helpers

These tasks each create a single helper function with TDD. They build up `capture.go` and `capture_test.go` incrementally.

### Task 2: `resolveInboxPath`

**Files:**
- Create: `capture.go`
- Create: `capture_test.go`

- [ ] **Step 1: Write the failing tests**

Create `capture_test.go`:

```go
package main

import (
	"path/filepath"
	"testing"
)

func TestResolveInboxPath_EmptyDefaultsToInboxMd(t *testing.T) {
	got := resolveInboxPath("/vault", "")
	want := filepath.Join("/vault", "inbox.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_BareFilenameVaultRelative(t *testing.T) {
	got := resolveInboxPath("/vault", "scratch.md")
	want := filepath.Join("/vault", "scratch.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_SubpathVaultRelative(t *testing.T) {
	got := resolveInboxPath("/vault", "journals/inbox.md")
	want := filepath.Join("/vault", "journals", "inbox.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_AbsoluteUsedAsIs(t *testing.T) {
	got := resolveInboxPath("/vault", "/tmp/inbox.md")
	want := "/tmp/inbox.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_TildeExpanded(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	got := resolveInboxPath("/vault", "~/scratch.md")
	want := filepath.Join(tmp, "scratch.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveInboxPath_DotDotCleaned(t *testing.T) {
	got := resolveInboxPath("/vault", "a/../b.md")
	want := filepath.Join("/vault", "b.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestResolveInboxPath -v`
Expected: FAIL — `resolveInboxPath` is undefined.

- [ ] **Step 3: Create `capture.go` with the helper**

```go
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// resolveInboxPath converts the raw config value into an absolute path
// to inbox.md, applying these rules:
//   - empty → "inbox.md" (relative)
//   - "~" prefix → home-dir expansion, treated as absolute
//   - filepath.IsAbs → used as-is, cleaned
//   - otherwise → joined with vault root
func resolveInboxPath(vault, configValue string) string {
	if configValue == "" {
		configValue = "inbox.md"
	}
	if strings.HasPrefix(configValue, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			configValue = filepath.Join(home, strings.TrimPrefix(configValue, "~"))
		}
	}
	if filepath.IsAbs(configValue) {
		return filepath.Clean(configValue)
	}
	return filepath.Join(vault, configValue)
}
```

(The unused imports — `bytes`, `fmt`, `tea`, `lipgloss`, `time` — will be used by helpers in later tasks; keep them now to avoid editing the import block multiple times. If `go vet` complains, comment-out the unused ones temporarily; later tasks will activate them.)

Actually: the Go compiler errors on unused imports. To avoid churn, only import what `resolveInboxPath` uses right now. Replace the import block with:

```go
import (
	"os"
	"path/filepath"
	"strings"
)
```

Later tasks will add imports as needed.

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestResolveInboxPath -v`
Expected: PASS, all six subtests.

- [ ] **Step 5: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): resolveInboxPath helper

Maps the raw inbox_path config value to an absolute path:
empty → inbox.md, ~ → home, absolute → as-is, otherwise
vault-relative.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 3: `formatCaptureLine`

**Files:**
- Modify: `capture.go`
- Modify: `capture_test.go`

- [ ] **Step 1: Write the failing tests**

Add `"strings"` and `"time"` to the imports in `capture_test.go`:

```go
import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)
```

Append to `capture_test.go`:

```go
func TestFormatCaptureLine_FixedTime(t *testing.T) {
	when := time.Date(2026, 4, 27, 14, 22, 0, 0, time.UTC)
	got := formatCaptureLine(when, "hello world")
	want := "- 2026-04-27 14:22 — hello world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatCaptureLine_EmDashLiteralBytes(t *testing.T) {
	// Em-dash (U+2014) is the 3-byte sequence 0xE2 0x80 0x94 in UTF-8.
	// Verify the literal bytes are present (catches accidental hyphen).
	when := time.Date(2026, 4, 27, 14, 22, 0, 0, time.UTC)
	got := formatCaptureLine(when, "x")
	if !strings.Contains(got, "—") {
		t.Errorf("output %q missing em-dash U+2014", got)
	}
	// Reject if it accidentally used a plain hyphen as the separator.
	// The line begins "- 2026-..."; the separator we care about is
	// between the timestamp and the text.
	if !strings.Contains(got, " — ") {
		t.Errorf("output %q does not contain ' — ' as separator", got)
	}
}

func TestFormatCaptureLine_MultilineEmbedded(t *testing.T) {
	when := time.Date(2026, 4, 27, 14, 22, 0, 0, time.UTC)
	got := formatCaptureLine(when, "first\nsecond\nthird")
	want := "- 2026-04-27 14:22 — first\nsecond\nthird"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatCaptureLine_EmptyTextSafe(t *testing.T) {
	when := time.Date(2026, 4, 27, 14, 22, 0, 0, time.UTC)
	got := formatCaptureLine(when, "")
	want := "- 2026-04-27 14:22 — "
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestFormatCaptureLine -v`
Expected: FAIL — `formatCaptureLine` is undefined.

- [ ] **Step 3: Add the helper to `capture.go`**

Add `fmt` and `time` to the imports in `capture.go`:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)
```

Append:

```go
// formatCaptureLine renders one inbox.md bullet for the given timestamp
// and capture text. Format: "- 2026-04-27 14:22 — <text>" (em-dash,
// minute precision, local time). Multi-line text embeds literal "\n"s;
// only the first line gets the bullet/timestamp prefix.
func formatCaptureLine(now time.Time, text string) string {
	return fmt.Sprintf("- %s — %s",
		now.Format("2006-01-02 15:04"),
		text)
}
```

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestFormatCaptureLine -v`
Expected: PASS, all four subtests.

- [ ] **Step 5: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): formatCaptureLine helper

Renders an inbox.md bullet: '- YYYY-MM-DD HH:MM — <text>'.
Em-dash (U+2014) separator, minute precision, local time.
Multi-line text embeds literal newlines.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 4: `appendToInboxFile`

**Files:**
- Modify: `capture.go`
- Modify: `capture_test.go`

- [ ] **Step 1: Write the failing tests**

Add `"os"` to the imports in `capture_test.go`:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)
```

Append to `capture_test.go`:

```go
func TestAppendToInboxFile_CreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")

	if err := appendToInboxFile(path, "- 2026-04-27 14:22 — hi"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "- 2026-04-27 14:22 — hi\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_CreatesMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journals", "inbox.md")

	if err := appendToInboxFile(path, "- t — x"); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestAppendToInboxFile_PreservesTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")
	os.WriteFile(path, []byte("existing line\n"), 0o644)

	if err := appendToInboxFile(path, "- t — new"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, _ := os.ReadFile(path)
	want := "existing line\n- t — new\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_AddsNewlineWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")
	os.WriteFile(path, []byte("existing line"), 0o644) // no trailing \n

	if err := appendToInboxFile(path, "- t — new"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, _ := os.ReadFile(path)
	want := "existing line\n- t — new\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_PreservesBlankLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")
	os.WriteFile(path, []byte("a\n\n"), 0o644) // intentional blank line

	if err := appendToInboxFile(path, "- t — new"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, _ := os.ReadFile(path)
	want := "a\n\n- t — new\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_TwoCapturesInSequence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")

	if err := appendToInboxFile(path, "- t — one"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := appendToInboxFile(path, "- t — two"); err != nil {
		t.Fatalf("second: %v", err)
	}

	data, _ := os.ReadFile(path)
	want := "- t — one\n- t — two\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendToInboxFile_FileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.md")

	if err := appendToInboxFile(path, "- t — x"); err != nil {
		t.Fatalf("append: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestAppendToInboxFile -v`
Expected: FAIL — `appendToInboxFile` is undefined.

- [ ] **Step 3: Add the helper to `capture.go`**

Add `bytes` to the imports:

```go
import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)
```

Append the function:

```go
// appendToInboxFile appends one bullet line to the given path, creating
// the file (and any missing parent dirs) if needed. Trailing-newline
// rules:
//   - the result always ends in exactly one "\n"
//   - if the existing file lacks a trailing "\n", one is inserted
//     before the new bullet (so we never produce "…texthello- ...")
//   - existing trailing blank lines (e.g. "\n\n") are preserved as-is
//     and the bullet is appended after them
func appendToInboxFile(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var b strings.Builder
	b.Write(existing)
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		b.WriteByte('\n')
	}
	b.WriteString(line)
	b.WriteByte('\n')
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
```

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestAppendToInboxFile -v`
Expected: PASS, all seven subtests.

- [ ] **Step 5: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): appendToInboxFile helper

Appends a bullet to inbox.md, creating the file and any missing
parent dirs. Ensures the result always ends with exactly one
newline; inserts a newline before the new bullet if the existing
file didn't have one; preserves intentional blank lines.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 5: `ensureTrailingNewline`

**Files:**
- Modify: `capture.go`
- Modify: `capture_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `capture_test.go`:

```go
func TestEnsureTrailingNewline_Empty(t *testing.T) {
	if got := ensureTrailingNewline(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestEnsureTrailingNewline_AlreadyHasOne(t *testing.T) {
	if got := ensureTrailingNewline("foo\n"); got != "foo\n" {
		t.Errorf("got %q, want %q", got, "foo\n")
	}
}

func TestEnsureTrailingNewline_AddsOne(t *testing.T) {
	if got := ensureTrailingNewline("foo"); got != "foo\n" {
		t.Errorf("got %q, want %q", got, "foo\n")
	}
}

func TestEnsureTrailingNewline_OnlyNewline(t *testing.T) {
	if got := ensureTrailingNewline("\n"); got != "\n" {
		t.Errorf("got %q, want %q", got, "\n")
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestEnsureTrailingNewline -v`
Expected: FAIL — `ensureTrailingNewline` is undefined.

- [ ] **Step 3: Add the helper to `capture.go`**

Append:

```go
// ensureTrailingNewline returns s with a single "\n" at the end.
// Empty strings are returned unchanged (no spurious "\n").
func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
```

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestEnsureTrailingNewline -v`
Expected: PASS, all four subtests.

- [ ] **Step 5: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): ensureTrailingNewline helper

Used by writeNewFile (delegate) to ensure new notes end in \n
without inflicting a spurious newline on truly empty content.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 6: `writeNewFile`

**Files:**
- Modify: `capture.go`
- Modify: `capture_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `capture_test.go`:

```go
func TestWriteNewFile_CreatesIfMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.md")

	if err := writeNewFile(path, "hello\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello\n" {
		t.Errorf("got %q, want %q", string(data), "hello\n")
	}
}

func TestWriteNewFile_RefusesIfExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.md")
	os.WriteFile(path, []byte("preexisting"), 0o644)

	err := writeNewFile(path, "should not overwrite")
	if !os.IsExist(err) {
		t.Fatalf("err = %v, want os.IsExist", err)
	}
	// Verify original content is intact.
	data, _ := os.ReadFile(path)
	if string(data) != "preexisting" {
		t.Errorf("file overwritten: %q", string(data))
	}
}

func TestWriteNewFile_ParentMissingReturnsError(t *testing.T) {
	dir := t.TempDir()
	// Parent dir "nope" does not exist; writeNewFile does NOT create it
	// (delegate's parent dir is always the source file's parent, which
	// by definition exists).
	path := filepath.Join(dir, "nope", "child.md")

	err := writeNewFile(path, "x")
	if err == nil {
		t.Error("expected error when parent dir is missing")
	}
	if os.IsExist(err) {
		t.Errorf("err = %v, want a 'no such file' error, not ErrExist", err)
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestWriteNewFile -v`
Expected: FAIL — `writeNewFile` is undefined.

- [ ] **Step 3: Add the helper to `capture.go`**

Append:

```go
// writeNewFile writes content to path with O_CREATE|O_EXCL semantics:
// returns os.ErrExist if the file already exists. This is the atomic
// create-only primitive used by the delegate flow — it survives a
// TOCTOU race between the os.Stat collision check and the actual write.
func writeNewFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
```

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestWriteNewFile -v`
Expected: PASS, both subtests.

- [ ] **Step 5: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): writeNewFile helper

Atomic create-only write (O_CREATE|O_EXCL). Used by the delegate
flow to defend against TOCTOU between collision check and write.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 2 — Model wiring scaffolding

### Task 7: Extend model + newModel signature

This task wires up everything that the handlers in later tasks will reference: input-mode constants, model fields, widget initialization, the `newModel` parameter, and the `main.go` call site. After this task the project still builds and all existing tests still pass; no new behavior yet.

**Files:**
- Modify: `model.go`
- Modify: `main.go`
- Modify: `shortcuts_input_test.go` (the `newTestModel` helper)

- [ ] **Step 1: Add the two input-mode constants**

In `model.go`, find the `inputMode` enum (currently lines 45–66). Append the two new constants at the end:

```go
const (
	inputNone inputMode = iota
	inputFilter
	inputConfirmDelete
	inputUnsavedGuard
	inputPluginSelect
	inputPluginConfig
	inputPluginPrompt
	inputPluginDiff
	inputNewFolder
	inputReplaceSearch
	inputReplaceWith
	inputShortcutSelect
	inputShortcutName
	inputShortcutDescription
	inputShortcutPrompt
	inputShortcutDeleteConfirm
	inputGitRemote
	inputRename
	inputHelp
	inputVaultSearch
	inputCapture
	inputDelegateName
)
```

- [ ] **Step 2: Add three fields to the `model` struct**

In `model.go`, in the `model` struct (anywhere after the existing fields and before the closing brace at line 178). The cleanest placement is right at the end, just before the `}` at line 178. Add a blank line to separate, then:

```go
	// Quick capture (Ctrl+J) and delegate-to-new-note (Ctrl+O)
	inboxPath     string         // raw config value; "" → default "inbox.md"
	captureInput  textarea.Model // multi-line, Shift+Enter for newline
	delegateInput textinput.Model
```

`textarea` is already imported (line 9 `"github.com/charmbracelet/bubbles/textarea"`); verify by searching the file. `textinput` is also already imported.

- [ ] **Step 3: Extend the `newModel` signature**

Change the `newModel` declaration (currently line 180):

```go
func newModel(vault string, plugins []Plugin, activeShortcutProvider, inboxPath string) model {
```

- [ ] **Step 4: Initialize the two new widgets and store `inboxPath`**

In `newModel`, just before the `m := model{...}` literal (around line 229), add the widget setup:

```go
	cap := textarea.New()
	cap.Placeholder = "Quick capture (Enter saves, Shift+Enter for newline, Esc cancels)"
	cap.CharLimit = 0
	cap.SetWidth(56)
	cap.SetHeight(6)
	cap.ShowLineNumbers = false

	del := textinput.New()
	del.Placeholder = "filename (no .md needed)"
	del.CharLimit = 200
	del.Prompt = "Move to: "
```

Then in the `m := model{...}` literal, add three lines (next to the other init values, alphabetical order or grouped — doesn't matter; just inside the braces):

```go
		captureInput:           cap,
		delegateInput:          del,
		inboxPath:              inboxPath,
```

- [ ] **Step 5: Update the `main.go` call site**

In `main.go` line 163, change:

```go
m := newModel(cfg.Vault, plugins, cfg.AIShortcutProvider)
```

to:

```go
m := newModel(cfg.Vault, plugins, cfg.AIShortcutProvider, cfg.InboxPath)
```

- [ ] **Step 6: Update the `newTestModel` helper**

In `shortcuts_input_test.go` line 17, change:

```go
return newModel(vault, nil, "")
```

to:

```go
return newModel(vault, nil, "", "")
```

- [ ] **Step 7: Build and run all tests**

Run: `go build ./...`
Expected: clean build.

Run: `go test ./...`
Expected: PASS — no behavior changes yet, just plumbing.

- [ ] **Step 8: Commit**

```bash
git add model.go main.go shortcuts_input_test.go
git commit -m "feat(model): wire capture/delegate input modes and fields

- Add inputCapture and inputDelegateName to inputMode enum
- Add inboxPath, captureInput (textarea), delegateInput (textinput)
  fields to model
- Extend newModel signature: take inboxPath string
- Thread cfg.InboxPath through main.go
- Update newTestModel helper for the new parameter

No behavior changes yet — handlers and wiring follow.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 3 — Capture handlers and view

### Task 8: `captureView` overlay rendering

**Files:**
- Modify: `capture.go`

This is a small visual function. It's hard to assert on UI strings exactly, so we just verify it produces a non-empty string containing the input text and the resolved inbox path. Visual correctness is on the manual smoke checklist.

- [ ] **Step 1: Write the failing test**

Append to `capture_test.go`:

```go
func TestCaptureView_ContainsInputAndPath(t *testing.T) {
	out := captureView("typed text", "/vault/inbox.md", 80, 24)
	if out == "" {
		t.Fatal("expected non-empty render")
	}
	if !strings.Contains(out, "typed text") {
		t.Errorf("render missing input text: %q", out)
	}
	if !strings.Contains(out, "/vault/inbox.md") {
		t.Errorf("render missing inbox path: %q", out)
	}
}
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `go test -run TestCaptureView -v`
Expected: FAIL — `captureView` is undefined.

- [ ] **Step 3: Add the lipgloss import + the view function**

Add `"github.com/charmbracelet/lipgloss"` to the import block in `capture.go`:

```go
import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)
```

Append:

```go
var captureModalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("62")).
	Padding(0, 1)

// captureView renders the centered capture modal: a title line
// "Quick capture → <inbox path>" above the textarea contents.
// Caller is responsible for wrapping the result with lipgloss.Place
// to center it within the editor pane.
func captureView(textareaView, inboxPath string, screenWidth, screenHeight int) string {
	title := "Quick capture → " + inboxPath
	body := title + "\n\n" + textareaView + "\n\nEnter: save · Shift+Enter: newline · Esc: cancel"
	w := 60
	if screenWidth > 0 && w > screenWidth-4 {
		w = screenWidth - 4
	}
	return captureModalStyle.Width(w).Render(body)
}
```

- [ ] **Step 4: Run the test, confirm it passes**

Run: `go test -run TestCaptureView -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): captureView modal renderer

Bordered box with title (inbox path), textarea body, and a
keybinding hint footer. Caller wraps it with lipgloss.Place
to center within the editor pane.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 9: `handleCapture` — Esc and empty Enter

This task introduces the handler with the two simple cases (cancel paths). The disk-write path comes in Task 10.

**Files:**
- Modify: `capture.go`
- Modify: `capture_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `capture_test.go`:

```go
func TestHandleCapture_EscClosesModal(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputCapture
	m.captureInput.SetValue("not committed")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}

func TestHandleCapture_EmptyEnterCancelsSilently(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputCapture
	// captureInput is empty by default

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}

func TestHandleCapture_WhitespaceOnlyEnterCancelsSilently(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputCapture
	m.captureInput.SetValue("   \n\t  ")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}
```

Add the bubbletea import to `capture_test.go`:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestHandleCapture -v`
Expected: FAIL — `handleCapture` is undefined.

- [ ] **Step 3: Implement `handleCapture` with the two cancel paths**

Add `tea "github.com/charmbracelet/bubbletea"` to the `capture.go` import block (it isn't there yet from Task 2):

```go
import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)
```

Append to `capture.go`:

```go
// handleCapture handles key events while inputMode == inputCapture.
//
// Esc cancels (modal closes; underlying state untouched).
// Plain Enter submits — empty/whitespace-only input is a silent cancel.
// All other keys (including Shift+Enter for a newline) fall through
// to textarea.Update so the textarea handles them natively.
func (m model) handleCapture(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = inputNone
		m.captureInput.Blur()
		return m, nil

	case "enter":
		text := strings.TrimRight(m.captureInput.Value(), "\n")
		m.inputMode = inputNone
		m.captureInput.Blur()
		if strings.TrimSpace(text) == "" {
			return m, nil // empty / whitespace-only is a silent cancel
		}
		return m.dispatchCapture(text)
	}

	var cmd tea.Cmd
	m.captureInput, cmd = m.captureInput.Update(msg)
	return m, cmd
}

// dispatchCapture is filled in by Task 10. Stub returns no-op so
// handleCapture compiles.
func (m model) dispatchCapture(text string) (tea.Model, tea.Cmd) {
	return m, nil
}
```

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestHandleCapture -v`
Expected: PASS, all three subtests.

- [ ] **Step 5: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): handleCapture key handler — cancel paths

Esc and empty/whitespace Enter both close the modal silently.
dispatchCapture is currently a no-op stub — disk write logic
follows in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 10: `dispatchCapture` + async append (closed inbox path)

This task fills in `dispatchCapture` for the simplest branch (inbox not currently open in the editor) and introduces the async `captureAppendCmd` and `captureAppendedMsg` machinery.

**Files:**
- Modify: `capture.go`
- Modify: `capture_test.go`
- Modify: `model.go` (add `captureAppendedMsg` case in `Update`)

- [ ] **Step 1: Write the failing test**

Append to `capture_test.go`:

```go
func TestCapture_ClosedInbox_WritesToDisk(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputCapture
	m.captureInput.SetValue("hello world")
	// m.currentFile is "" — inbox is not open

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for non-empty capture")
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}

	// Run the cmd to perform the disk write.
	resultMsg := cmd().(captureAppendedMsg)
	if resultMsg.err != nil {
		t.Fatalf("captureAppendedMsg err: %v", resultMsg.err)
	}
	if resultMsg.reloadOpen {
		t.Errorf("reloadOpen = true, want false (inbox not open)")
	}

	inboxPath := filepath.Join(m.vault, "inbox.md")
	data, err := os.ReadFile(inboxPath)
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, " — hello world\n") {
		t.Errorf("inbox content %q does not contain ' — hello world\\n'", got)
	}
	if !strings.HasPrefix(got, "- ") {
		t.Errorf("inbox content %q does not start with bullet", got)
	}
}
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `go test -run TestCapture_ClosedInbox -v`
Expected: FAIL — `captureAppendedMsg` is undefined.

- [ ] **Step 3: Add the message type, the cmd, and replace the `dispatchCapture` stub**

In `capture.go`, append the message type and the cmd:

```go
// captureAppendedMsg is emitted by captureAppendCmd after the disk
// write completes (or fails). Carries enough info for the model to
// decide whether to reload an open editor view of inbox.md.
type captureAppendedMsg struct {
	err        error
	inboxPath  string
	reloadOpen bool
}

// captureAppendCmd performs the actual disk append off the main loop.
func captureAppendCmd(inboxPath, line string, reloadOpen bool) tea.Cmd {
	return func() tea.Msg {
		if err := appendToInboxFile(inboxPath, line); err != nil {
			return captureAppendedMsg{err: err}
		}
		return captureAppendedMsg{inboxPath: inboxPath, reloadOpen: reloadOpen}
	}
}
```

Then replace the stub `dispatchCapture` with the closed-inbox branch (the dirty/clean branches come in the next task):

```go
// dispatchCapture decides what to do with the captured text based on
// whether inbox.md is currently open in the editor and dirty.
//
// Branches:
//   - inbox not open → disk write only
//   - inbox open + clean → disk write + editor reload (next task)
//   - inbox open + dirty → in-memory editor append (next task)
func (m model) dispatchCapture(text string) (tea.Model, tea.Cmd) {
	line := formatCaptureLine(time.Now(), text)
	inboxPath := resolveInboxPath(m.vault, m.inboxPath)

	if m.currentFile != inboxPath {
		// Inbox is not the currently open file — just disk write.
		return m, captureAppendCmd(inboxPath, line, false)
	}
	// Other branches added in the next task.
	return m, captureAppendCmd(inboxPath, line, true)
}
```

- [ ] **Step 4: Wire `captureAppendedMsg` into the main `Update` switch**

In `model.go`'s `Update` method, find the existing `tea.Msg` type-switch (starts at line 292). Add a new case before the `tea.WindowSizeMsg` or `tea.KeyMsg` case — anywhere among the message cases is fine. Suggested placement: right after the `fileDeletedMsg` block (around line 308):

```go
	case captureAppendedMsg:
		if msg.err != nil {
			m.errMsg = "capture failed: " + msg.err.Error()
			return m, nil
		}
		if msg.reloadOpen && m.currentFile == msg.inboxPath {
			if data, err := os.ReadFile(msg.inboxPath); err == nil {
				line, col := editorCursorPos(m.editor)
				m.editor.SetValue(string(data))
				m.cleanContent = string(data)
				m.editor.MoveTo(line, col)
			}
		}
		return m, nil
```

If `os` isn't already imported in `model.go`, it is — search to confirm. If it isn't, add it.

- [ ] **Step 5: Run the test, confirm it passes**

Run: `go test -run TestCapture_ClosedInbox -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add capture.go capture_test.go model.go
git commit -m "feat(capture): dispatchCapture + async append (closed inbox)

Adds captureAppendedMsg + captureAppendCmd for off-loop disk
writes. dispatchCapture handles the closed-inbox branch (just
disk write); open+clean and open+dirty branches follow.

The Update handler for captureAppendedMsg also handles the
reload-open case, which the next task exercises.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 11: Capture branches — open+clean and open+dirty

This task adds the `appendLineToEditor` helper and exercises the open-clean (disk + reload) and open-dirty (in-memory) branches.

**Files:**
- Modify: `capture.go`
- Modify: `capture_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `capture_test.go`:

```go
func TestCapture_OpenCleanInbox_WritesAndReloads(t *testing.T) {
	m := newTestModel(t)
	inboxPath := filepath.Join(m.vault, "inbox.md")
	// Pre-populate inbox with an existing line.
	if err := os.WriteFile(inboxPath, []byte("- old — first\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Open inbox.md in the editor (clean).
	m.currentFile = inboxPath
	m.editor.SetValue("- old — first\n")
	m.cleanContent = "- old — first\n"
	if m.isDirty() {
		t.Fatal("editor should be clean after seed")
	}

	m.inputMode = inputCapture
	m.captureInput.SetValue("new entry")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if cmd == nil {
		t.Fatal("expected cmd")
	}

	// Run the cmd → produces captureAppendedMsg.
	msg := cmd().(captureAppendedMsg)
	if msg.err != nil {
		t.Fatalf("append err: %v", msg.err)
	}
	if !msg.reloadOpen {
		t.Error("reloadOpen = false, want true (inbox is open and was clean)")
	}

	// Disk should have both lines.
	data, _ := os.ReadFile(inboxPath)
	if !strings.Contains(string(data), "- old — first\n") {
		t.Errorf("disk missing original line: %q", string(data))
	}
	if !strings.Contains(string(data), " — new entry\n") {
		t.Errorf("disk missing new line: %q", string(data))
	}

	// Feed the captureAppendedMsg back through Update to exercise the
	// reload branch.
	next2, _ := nm.Update(msg)
	nm2 := next2.(model)
	if nm2.editor.Value() != string(data) {
		t.Errorf("editor not reloaded: got %q, want %q", nm2.editor.Value(), string(data))
	}
	if nm2.cleanContent != string(data) {
		t.Error("cleanContent not updated; editor would be dirty")
	}
}

func TestCapture_OpenDirtyInbox_AppendsInMemory(t *testing.T) {
	m := newTestModel(t)
	inboxPath := filepath.Join(m.vault, "inbox.md")
	original := "- old — first\n"
	if err := os.WriteFile(inboxPath, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Open inbox.md and make a dirty edit.
	m.currentFile = inboxPath
	m.editor.SetValue("- old — first\nDIRTY EDIT\n")
	m.cleanContent = original
	if !m.isDirty() {
		t.Fatal("editor should be dirty after the edit")
	}

	m.inputMode = inputCapture
	m.captureInput.SetValue("from capture")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	if cmd != nil {
		t.Errorf("expected nil cmd (no disk write), got %v", cmd)
	}

	// Disk content should be unchanged (still original).
	data, _ := os.ReadFile(inboxPath)
	if string(data) != original {
		t.Errorf("disk modified; got %q, want %q", string(data), original)
	}

	// Editor buffer should contain the dirty edit AND the new bullet.
	got := nm.editor.Value()
	if !strings.Contains(got, "DIRTY EDIT") {
		t.Errorf("editor lost dirty edit: %q", got)
	}
	if !strings.Contains(got, " — from capture") {
		t.Errorf("editor missing captured line: %q", got)
	}

	// Editor must remain dirty (cleanContent unchanged).
	if !nm.isDirty() {
		t.Error("editor should still be dirty after in-memory capture")
	}
}

func TestCapture_OpenCleanInbox_PreservesCursor(t *testing.T) {
	m := newTestModel(t)
	inboxPath := filepath.Join(m.vault, "inbox.md")
	// Multi-line file so we can position cursor at row 2.
	original := "line zero\nline one\nline two\n"
	os.WriteFile(inboxPath, []byte(original), 0o644)

	m.currentFile = inboxPath
	m.editor.SetValue(original)
	m.cleanContent = original
	m.editor.MoveTo(2, 3) // row 2, col 3 (within "line two")

	m.inputMode = inputCapture
	m.captureInput.SetValue("appended")

	next, cmd := m.handleCapture(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)
	msg := cmd().(captureAppendedMsg)
	next2, _ := nm.Update(msg)
	nm2 := next2.(model)

	gotLine, gotCol := editorCursorPos(nm2.editor)
	if gotLine != 2 || gotCol != 3 {
		t.Errorf("cursor = (%d, %d), want (2, 3)", gotLine, gotCol)
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestCapture_Open -v`
Expected: FAIL — open+dirty currently writes to disk because `dispatchCapture` doesn't yet branch on dirty.

- [ ] **Step 3: Add `appendLineToEditor` and the dirty branch in `dispatchCapture`**

In `capture.go`, append the editor-mutation helper:

```go
// appendLineToEditor appends a single bullet line to the in-memory
// editor buffer, ensuring the result is well-formed:
//   - existing buffer is given a trailing "\n" if it lacks one
//   - the new line is added with its own trailing "\n"
//   - the cursor's logical (row, col) is preserved across the
//     SetValue call (clamped to the new content bounds)
//
// The editor's existing isDirty mechanism flags the buffer as dirty
// automatically once SetValue runs.
func (m *model) appendLineToEditor(line string) {
	row, col := editorCursorPos(m.editor)

	old := m.editor.Value()
	var b strings.Builder
	b.WriteString(old)
	if len(old) > 0 && !strings.HasSuffix(old, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(line)
	b.WriteByte('\n')

	m.editor.SetValue(b.String())
	m.editor.MoveTo(row, col)
}
```

Replace the body of `dispatchCapture` to branch on dirty:

```go
func (m model) dispatchCapture(text string) (tea.Model, tea.Cmd) {
	line := formatCaptureLine(time.Now(), text)
	inboxPath := resolveInboxPath(m.vault, m.inboxPath)

	if m.currentFile == inboxPath {
		if m.isDirty() {
			// Inbox is open with unsaved edits. Append in-memory
			// only; user keeps their dirty state and saves later
			// with Ctrl+S. The editor stays dirty automatically
			// because cleanContent is unchanged.
			m.appendLineToEditor(line)
			return m, nil
		}
		// Inbox is open and clean — disk write + reload editor on
		// completion (the captureAppendedMsg handler does the reload).
		return m, captureAppendCmd(inboxPath, line, true)
	}
	// Inbox is not open — just disk write; no reload needed.
	return m, captureAppendCmd(inboxPath, line, false)
}
```

- [ ] **Step 4: Run the new tests, confirm they pass**

Run: `go test -run TestCapture_Open -v`
Expected: PASS, all three subtests.

- [ ] **Step 5: Run the full file's tests for safety**

Run: `go test -run TestCapture -v && go test -run TestHandleCapture -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): open-clean reload + open-dirty in-memory branches

dispatchCapture now branches on (m.currentFile == inboxPath)
and m.isDirty():
  - inbox not open → disk write only
  - inbox open + clean → disk write + editor reload (cursor preserved)
  - inbox open + dirty → in-memory editor append, no disk write

appendLineToEditor handles the in-memory mutation including
trailing-newline normalization and cursor preservation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 12: Wire `Ctrl+J` + `inputCapture` routing + `View` overlay

**Files:**
- Modify: `model.go`

This task connects the capture handler to the rest of the app. After this, pressing Ctrl+J in a running clipad invokes the modal.

- [ ] **Step 1: Add the `Ctrl+J` case to the main key dispatcher**

In `model.go`, find the existing global keybinding cases in `Update` (around line 710 for `ctrl+t`, line 723 for `ctrl+k`). Add a new case immediately after `ctrl+k` and before `tab`:

```go
		case "ctrl+j":
			if m.vault == "" {
				m.errMsg = "no vault configured"
				return m, nil
			}
			m.inputMode = inputCapture
			m.captureInput.Reset()
			cmd := m.captureInput.Focus()
			return m, cmd
```

- [ ] **Step 2: Add `inputCapture` to `handleInputMode`**

In `model.go` around line 953, after the `case inputVaultSearch:` line, append:

```go
	case inputCapture:
		return m.handleCapture(msg)
```

- [ ] **Step 3: Add the overlay branch to `View()`**

In `model.go`, find the View() overlay chain (starts at line 1751 with `if m.inputMode == inputVaultSearch {`). Add a new branch in the `else if` chain — before `inputHelp` is fine. Insert at line 1760:

```go
	} else if m.inputMode == inputCapture {
		modal := captureView(
			m.captureInput.View(),
			resolveInboxPath(m.vault, m.inboxPath),
			m.editorWidth, m.editorHeight,
		)
		rightView = lipgloss.Place(m.editorWidth, m.editorHeight,
			lipgloss.Center, lipgloss.Center, modal)
```

The full `else if` should be inserted between the `} else if m.inputMode == inputVaultSearch {` block and the existing `} else if m.inputMode == inputHelp {` block.

- [ ] **Step 4: Build and run all tests**

Run: `go build ./...`
Expected: clean build.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Manual smoke (optional but encouraged)**

```bash
go run . &  # or just `go build && ./clipad`
```

Press Ctrl+J. The capture modal should appear centered. Type something. Press Enter. Modal closes; `<vault>/inbox.md` should now contain a timestamped bullet. Press Esc the next time — modal should close without writing.

- [ ] **Step 6: Commit**

```bash
git add model.go
git commit -m "feat(capture): wire Ctrl+J and View overlay

Adds the global Ctrl+J case (set inputMode = inputCapture, reset
and focus the textarea), routes inputCapture to handleCapture in
handleInputMode, and adds the centered overlay branch in View().

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 4 — Delegate handler and view

### Task 13: `handleDelegate` — Esc and validation

**Files:**
- Modify: `capture.go`
- Modify: `capture_test.go`

This task introduces the delegate handler with the cancel + validation paths. The happy path comes in the next task.

- [ ] **Step 1: Write the failing tests**

Append to `capture_test.go`:

```go
func TestHandleDelegate_EscClosesModal(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("foo")

	next, cmd := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
}

func TestHandleDelegate_EmptyEnterIgnored(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputDelegateName
	// delegateInput is empty

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	// Empty input means "user keeps typing" — modal stays open.
	if nm.inputMode != inputDelegateName {
		t.Errorf("inputMode = %v, want inputDelegateName (modal stays open)", nm.inputMode)
	}
}

func TestHandleDelegate_SlashRejected(t *testing.T) {
	m := newTestModel(t)
	srcDir := m.vault
	srcPath := filepath.Join(srcDir, "src.md")
	os.WriteFile(srcPath, []byte("hello world"), 0o644)
	m.currentFile = srcPath
	m.editor.SetValue("hello world")
	m.cleanContent = "hello world"
	// Simulate a selection (selection mechanics handled by editor).
	m.editor.selActive = true
	m.editor.selAnchorLine = 0
	m.editor.selAnchorCol = 0
	m.editor.MoveTo(0, 5)

	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("subdir/foo")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputDelegateName {
		t.Errorf("inputMode = %v, want stays inputDelegateName", nm.inputMode)
	}
	if nm.errMsg == "" {
		t.Error("errMsg should be set on slash rejection")
	}
	// New file should NOT have been created.
	if _, err := os.Stat(filepath.Join(srcDir, "subdir")); err == nil {
		t.Error("subdir was created — handler should have rejected before any IO")
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestHandleDelegate -v`
Expected: FAIL — `handleDelegate` is undefined.

- [ ] **Step 3: Implement `handleDelegate` with cancel + validation only**

Append to `capture.go`:

```go
// handleDelegate handles key events while inputMode == inputDelegateName.
//
// Esc cancels (modal closes; selection on editor untouched).
// Empty Enter is ignored (user keeps typing).
// Filenames containing "/" or "\" are rejected — names only, target
// dir is fixed at the source file's parent directory.
// Filenames without an extension get ".md" auto-appended.
//
// On a valid name, the handler:
//   1. checks for collision against the target path
//   2. reads the editor's currently selected text
//   3. atomically writes the new file (writeNewFile / O_EXCL)
//   4. removes the selection from the editor (DeleteSelection)
//   5. saves the source via saveCurrentFile
//
// All other keys fall through to textinput.Update.
func (m model) handleDelegate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = inputNone
		m.delegateInput.Blur()
		return m, nil

	case "enter":
		raw := strings.TrimSpace(m.delegateInput.Value())
		if raw == "" {
			return m, nil // ignore; user keeps typing
		}
		name := raw
		if filepath.Ext(name) == "" {
			name += ".md"
		}
		if strings.ContainsAny(name, "/\\") {
			m.errMsg = "filename only — no slashes"
			return m, nil
		}
		// Happy path implemented in the next task.
		return m, nil
	}

	var cmd tea.Cmd
	m.delegateInput, cmd = m.delegateInput.Update(msg)
	return m, cmd
}
```

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestHandleDelegate -v`
Expected: PASS, all three subtests.

- [ ] **Step 5: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): handleDelegate skeleton — cancel + validation

Esc closes; empty Enter ignored; slash in name rejected; .md
auto-appended when no extension is given. Happy path (file
write + selection delete + save) follows in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 14: `handleDelegate` — happy path + collision

**Files:**
- Modify: `capture.go`
- Modify: `capture_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `capture_test.go`:

```go
// delegateSetup creates a temp vault, writes a source file, opens it
// in the model with a selection covering the middle word of the
// content. Returns the configured model and the source path.
func delegateSetup(t *testing.T, srcContent string, selectFromCol, selectToCol int) (model, string) {
	t.Helper()
	m := newTestModel(t)
	srcPath := filepath.Join(m.vault, "src.md")
	if err := os.WriteFile(srcPath, []byte(srcContent), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	m.currentFile = srcPath
	m.editor.SetValue(srcContent)
	m.cleanContent = srcContent
	// Set selection on row 0 from selectFromCol to selectToCol.
	m.editor.selActive = true
	m.editor.selAnchorLine = 0
	m.editor.selAnchorCol = selectFromCol
	m.editor.MoveTo(0, selectToCol)
	m.activePanel = editorPanel
	return m, srcPath
}

func TestDelegate_HappyPath(t *testing.T) {
	// Source content "foo bar baz". Select "bar" (cols 4..7).
	m, srcPath := delegateSetup(t, "foo bar baz", 4, 7)
	srcDir := filepath.Dir(srcPath)

	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("notes")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	// Modal should be closed.
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}

	// Destination file should exist with the selection content + \n.
	dstPath := filepath.Join(srcDir, "notes.md")
	dst, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("destination missing: %v", err)
	}
	if string(dst) != "bar\n" {
		t.Errorf("destination = %q, want %q", string(dst), "bar\n")
	}

	// Source on disk should have the cut applied (saved).
	// Seed was "foo bar baz" (no trailing \n). After DeleteSelection
	// of "bar" the editor holds "foo  baz" verbatim, and saveCurrentFile
	// writes that bytes-exactly via os.WriteFile.
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("source missing: %v", err)
	}
	if string(src) != "foo  baz" {
		t.Errorf("source on disk = %q, want %q", string(src), "foo  baz")
	}

	// Editor must reflect the post-cut state and be clean.
	if !strings.HasPrefix(nm.editor.Value(), "foo  baz") {
		t.Errorf("editor = %q, want post-cut content", nm.editor.Value())
	}
	if nm.isDirty() {
		t.Error("editor should be clean after delegate save")
	}
}

func TestDelegate_AutoAppendsMd(t *testing.T) {
	m, srcPath := delegateSetup(t, "abcdef", 0, 3)
	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("typed")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	_ = next

	dstPath := filepath.Join(filepath.Dir(srcPath), "typed.md")
	if _, err := os.Stat(dstPath); err != nil {
		t.Errorf(".md not auto-appended; expected %s", dstPath)
	}
}

func TestDelegate_DirtySource_SinglePersistedSave(t *testing.T) {
	// Source on disk: "foo bar baz". Editor has been dirtied with an
	// unrelated edit on a second line, AND a selection on "bar". The
	// delegate flow's end-of-flow saveCurrentFile must persist BOTH
	// the cut and the unrelated dirty edit in one write.
	m := newTestModel(t)
	srcPath := filepath.Join(m.vault, "src.md")
	onDisk := "foo bar baz\n"
	if err := os.WriteFile(srcPath, []byte(onDisk), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m.currentFile = srcPath
	// Dirty edit: append a second line.
	m.editor.SetValue("foo bar baz\nDIRTY\n")
	m.cleanContent = onDisk
	if !m.isDirty() {
		t.Fatal("editor must be dirty for this scenario")
	}
	// Select "bar" on row 0 (cols 4..7).
	m.editor.selActive = true
	m.editor.selAnchorLine = 0
	m.editor.selAnchorCol = 4
	m.editor.MoveTo(0, 7)
	m.activePanel = editorPanel

	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("notes")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	// Editor must be clean post-save.
	if nm.isDirty() {
		t.Error("editor should be clean after delegate save")
	}

	// On-disk source must have BOTH the cut applied AND the dirty edit.
	src, _ := os.ReadFile(srcPath)
	if !strings.Contains(string(src), "foo  baz") {
		t.Errorf("on-disk source missing cut: %q", string(src))
	}
	if !strings.Contains(string(src), "DIRTY") {
		t.Errorf("on-disk source missing dirty edit: %q", string(src))
	}

	// New file has the selection content.
	dst, err := os.ReadFile(filepath.Join(m.vault, "notes.md"))
	if err != nil {
		t.Fatalf("destination missing: %v", err)
	}
	if string(dst) != "bar\n" {
		t.Errorf("destination = %q, want %q", string(dst), "bar\n")
	}
}

func TestDelegate_CollisionRefused(t *testing.T) {
	m, srcPath := delegateSetup(t, "abcdef", 0, 3)
	srcDir := filepath.Dir(srcPath)
	// Pre-create the target file.
	collidePath := filepath.Join(srcDir, "taken.md")
	if err := os.WriteFile(collidePath, []byte("preexisting"), 0o644); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	m.inputMode = inputDelegateName
	m.delegateInput.SetValue("taken")

	next, _ := m.handleDelegate(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputDelegateName {
		t.Errorf("inputMode = %v, want stays inputDelegateName", nm.inputMode)
	}
	if nm.errMsg == "" {
		t.Error("errMsg should be set on collision")
	}

	// Existing file unchanged.
	data, _ := os.ReadFile(collidePath)
	if string(data) != "preexisting" {
		t.Errorf("collision file overwritten: %q", string(data))
	}
	// Source on disk unchanged.
	src, _ := os.ReadFile(srcPath)
	if string(src) != "abcdef" {
		t.Errorf("source modified despite collision refusal: %q", string(src))
	}
	// Editor unchanged (cut not applied).
	if nm.editor.Value() != "abcdef" {
		t.Errorf("editor modified despite collision refusal: %q", nm.editor.Value())
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestDelegate_ -v`
Expected: FAIL — current handler returns `m, nil` after validation without doing the actual cut/write.

- [ ] **Step 3: Implement the happy path in `handleDelegate`**

Replace the `// Happy path implemented in the next task.` line in `handleDelegate` with the real implementation. The full `case "enter":` block becomes:

```go
	case "enter":
		raw := strings.TrimSpace(m.delegateInput.Value())
		if raw == "" {
			return m, nil // ignore; user keeps typing
		}
		name := raw
		if filepath.Ext(name) == "" {
			name += ".md"
		}
		if strings.ContainsAny(name, "/\\") {
			m.errMsg = "filename only — no slashes"
			return m, nil
		}
		srcDir := filepath.Dir(m.currentFile)
		dstPath := filepath.Join(srcDir, name)

		// Collision check (best-effort; writeNewFile re-checks atomically
		// via O_EXCL in case the file appears between here and the write).
		if _, err := os.Stat(dstPath); err == nil {
			m.errMsg = "file exists: " + name
			return m, nil
		}

		// Snapshot the selection text — DeleteSelection wipes it.
		selText := m.editor.SelectedText()

		// Step 1: write the new file.
		if err := writeNewFile(dstPath, ensureTrailingNewline(selText)); err != nil {
			m.errMsg = "delegate failed: " + err.Error()
			return m, nil
		}

		// Step 2: cut the selection from the editor (mutates in-place,
		// integrates with undo, moves cursor to the start of where the
		// selection used to be).
		m.editor.DeleteSelection()

		// Step 3: persist the source. saveCurrentFile is a *model method
		// that signals failure by setting m.errMsg (no return value);
		// after success isDirty() returns false. We clear errMsg first
		// to detect a fresh failure cleanly.
		m.errMsg = ""
		m.saveCurrentFile()

		m.inputMode = inputNone
		m.delegateInput.Blur()
		return m, nil
```

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestDelegate -v`
Expected: PASS, all three subtests.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add capture.go capture_test.go
git commit -m "feat(capture): handleDelegate happy path + collision check

On valid filename: stat-check for collision, write new file with
selection content (O_EXCL atomic create), DeleteSelection mutates
editor + cursor lands at former selection start, saveCurrentFile
persists. Auto-appends .md when no extension is provided.
Collision refusal leaves filesystem and editor untouched.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

### Task 15: Wire `Ctrl+O` + `inputDelegateName` routing + status-bar branch

**Files:**
- Modify: `model.go`

- [ ] **Step 1: Write the failing test**

Add to `capture_test.go`:

```go
func TestCtrlO_NoSelection_RefusedWithFlash(t *testing.T) {
	m := newTestModel(t)
	srcPath := filepath.Join(m.vault, "src.md")
	os.WriteFile(srcPath, []byte("content"), 0o644)
	m.currentFile = srcPath
	m.editor.SetValue("content")
	m.cleanContent = "content"
	m.activePanel = editorPanel
	// No selection: m.editor.selActive is false (default)

	// We exercise the Update path so the global Ctrl+O case fires.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone (modal must NOT open)", nm.inputMode)
	}
	if nm.errMsg == "" {
		t.Error("errMsg should be set when no selection")
	}
}

func TestCtrlO_WithSelection_OpensModal(t *testing.T) {
	m, _ := delegateSetup(t, "hello", 0, 3)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	nm := next.(model)

	if nm.inputMode != inputDelegateName {
		t.Errorf("inputMode = %v, want inputDelegateName", nm.inputMode)
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestCtrlO -v`
Expected: FAIL — there is no `Ctrl+O` case yet.

- [ ] **Step 3: Add the `Ctrl+O` case to the main key dispatcher**

In `model.go`, immediately after the new `ctrl+j` case from Task 12 (in the global keybinding block, before `tab`), add:

```go
		case "ctrl+o":
			if m.activePanel != editorPanel || m.currentFile == "" {
				m.errMsg = "open a file in the editor first"
				return m, nil
			}
			if !m.editor.selActive || m.editor.SelectedText() == "" {
				m.errMsg = "select text first"
				return m, nil
			}
			m.inputMode = inputDelegateName
			m.delegateInput.Reset()
			cmd := m.delegateInput.Focus()
			return m, cmd
```

- [ ] **Step 4: Add `inputDelegateName` to `handleInputMode`**

In `model.go`, after the `case inputCapture:` line added in Task 12, append:

```go
	case inputDelegateName:
		return m.handleDelegate(msg)
```

- [ ] **Step 5: Add the status-bar branch in `View()`**

In `model.go`, find the status-bar branching chain (starts at line 1857 with `if m.gitSyncQuitting`). Add a new `else if` for `inputDelegateName` — placement next to the other status-bar input modes (e.g., after `inputRename` at line 1879) is appropriate:

```go
	} else if m.inputMode == inputDelegateName {
		statusView = statusBarStyle.Width(m.width).Render(
			"Move to " + filepath.Dir(m.currentFile) + string(filepath.Separator) +
				m.delegateInput.View())
```

If `path/filepath` isn't imported in `model.go`, search the file — it likely is. If not, add it.

- [ ] **Step 6: Run the wire-up tests, confirm they pass**

Run: `go test -run TestCtrlO -v`
Expected: PASS.

- [ ] **Step 7: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 8: Manual smoke (optional)**

```bash
go build && ./clipad
```

Open a file. Select some text (Shift+arrow keys). Press Ctrl+O. Status bar should show "Move to /path/to/dir/" with a textinput. Type a name, press Enter. The selected text should be removed from the editor and a new file should appear in the same directory. Press Esc next time — modal closes without writing.

- [ ] **Step 9: Commit**

```bash
git add model.go
git commit -m "feat(capture): wire Ctrl+O and delegate status-bar prompt

Adds the global Ctrl+O case (gated on editorPanel + non-empty
selection), routes inputDelegateName to handleDelegate in
handleInputMode, and adds the status-bar inline prompt branch
in View() showing 'Move to <dir>/<input>'.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 5 — Final verification

### Task 16: Full sweep

**Files:** none (verification only)

- [ ] **Step 1: Clean build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 2: Run full test suite with race detector**

Run: `go test -race ./...`
Expected: all tests PASS.

- [ ] **Step 3: Run `go vet`**

Run: `go vet ./...`
Expected: no warnings.

- [ ] **Step 4: Check that capture and delegate are documented in README.md keybindings**

Read `README.md` lines 67–104 (the Keybindings section). Add two new rows to the Global table:

```markdown
| `Ctrl+J` | Quick capture — append timestamped bullet to `<vault>/inbox.md` |
| `Ctrl+O` | Move selected text to a new note in the same directory |
```

These should slot in alongside the other global keys.

- [ ] **Step 5: Commit the README update**

```bash
git add README.md
git commit -m "docs: document Ctrl+J quick capture and Ctrl+O delegate

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 6: Final manual smoke checklist**

Run `./clipad` against a real vault and verify:
- Ctrl+J in tree mode opens capture; Enter writes to inbox.md; user is back in tree mode at the same cursor.
- Ctrl+J in editor mode (with a different file open) opens capture; Enter writes to inbox.md; cursor in the editor unchanged.
- Ctrl+J with inbox.md open and clean: Enter appends, editor reflects new bullet, editor cursor preserved, editor stays clean.
- Ctrl+J with inbox.md open and dirty: Enter appends in-memory, file on disk unchanged, editor stays dirty until Ctrl+S.
- Capture modal: Shift+Enter inserts a newline; plain Enter submits.
- Esc cancels capture without writing.
- Ctrl+O without a selection: status flash, no modal.
- Ctrl+O with a selection: status-bar prompt opens. Type "foo", Enter → `foo.md` appears next to the source file, selection removed from editor, source saved.
- Ctrl+O collision: type a name that already exists, Enter → status flash, modal stays open, no filesystem change.
- Ctrl+O slash: type "subdir/foo", Enter → status flash, modal stays open.

---

## Decision log

- **No `m.editor.HasSelection()` wrapper added.** The handler accesses `m.editor.selActive` directly because `capture.go` is in the same package. If a future cross-package consumer needs the predicate, that's a 2-line addition to `selection.go` and not load-bearing here.
- **No `delegateContext` struct.** The delegate happy path reads `m.currentFile` and `m.editor.SelectedText()` at Enter time. Both are stable while the modal is open (textinput consumes keys; editor is blurred; no panel switch or file open is reachable).
- **Tests use the real `selActive`/`selAnchorLine`/`selAnchorCol` fields** to simulate selections — same way `selection.go` and existing tests do internally. This stays in-package; no new public API.
- **`m.saveCurrentFile()` is `*model`-receiver, void return, signals via `m.errMsg`.** Calling sites clear `m.errMsg` before invoking and inspect it (or `m.isDirty()`) afterwards — pattern matches existing call sites at `model.go:423, 635, 1180, 1383`.
- **Watcher reload generalization is out of scope.** The capture flow handles its own reload via `captureAppendedMsg`; broader watcher work stays a separate task.
