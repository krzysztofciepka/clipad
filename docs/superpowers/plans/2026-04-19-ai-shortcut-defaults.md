# AI Shortcut Built-in Defaults Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the 23-shortcut AI library as built-in defaults in the clipad binary; seed `~/.config/clipad/ai_shortcuts.toml` with these defaults the first time `loadShortcuts` runs against a missing file.

**Architecture:** Embed `defaults/ai_shortcuts.toml` via `go:embed` into a package-level `defaultShortcutsTOML []byte`. Modify `loadShortcuts` to write that byte slice to the config path when the file is missing, then continue with the existing parse path. Existing files (including intentionally empty ones) are never touched.

**Tech Stack:** Go (`embed`, `os`, `path/filepath`), `github.com/pelletier/go-toml/v2` (already a dep).

---

## Spec Reference

`/home/kc/repos/clipad/docs/superpowers/specs/2026-04-19-ai-shortcut-library-design.md` (Part 2 — Built-in defaults in clipad)

## File Structure

- **Create:** `defaults/ai_shortcuts.toml` — single source of truth for the embedded defaults; same 23-entry content already living at `~/.config/clipad/ai_shortcuts.toml`.
- **Modify:** `shortcuts.go`
  - Add `_ "embed"` import.
  - Add `//go:embed defaults/ai_shortcuts.toml` declaration with `var defaultShortcutsTOML []byte`.
  - Modify `loadShortcuts()` so the `os.IsNotExist(err)` branch seeds the file from `defaultShortcutsTOML` instead of returning `nil, nil`.
- **Modify:** `shortcuts_test.go`
  - Replace `TestLoadShortcuts_Missing` with `TestLoadShortcuts_SeedsWhenMissing` (asserts new behavior).
  - Add `TestDefaultShortcutsEmbeddedTOMLParses`, `TestLoadShortcuts_DoesNotOverwriteExisting`, `TestLoadShortcuts_KeepsExplicitlyEmpty`.
- **Modify:** `README.md` — add a paragraph and a name list under Plugins describing the default AI shortcut library and how seeding works.

`shortcuts.go` will gain only ~6 lines net. No new files in the package other than the embedded data file.

---

### Task 1: Add the embedded defaults file

**Files:**
- Create: `defaults/ai_shortcuts.toml`

- [ ] **Step 1: Create the `defaults/` directory**

Run:
```bash
mkdir -p /home/kc/repos/clipad/defaults
```

- [ ] **Step 2: Write the file**

Use the `Write` tool to create `/home/kc/repos/clipad/defaults/ai_shortcuts.toml` with **exactly** the same content as the user's local config produced by the prior plan. The full content is at `~/.config/clipad/ai_shortcuts.toml` and can be copied verbatim. As a one-shot the executor may use:

```bash
cp /home/kc/.config/clipad/ai_shortcuts.toml /home/kc/repos/clipad/defaults/ai_shortcuts.toml
```

- [ ] **Step 3: Sanity-check the copy**

Run:
```bash
diff /home/kc/.config/clipad/ai_shortcuts.toml /home/kc/repos/clipad/defaults/ai_shortcuts.toml && grep -c '^\[\[shortcuts\]\]' /home/kc/repos/clipad/defaults/ai_shortcuts.toml
```

Expected: `diff` produces no output (files identical), and the count is `23`.

- [ ] **Step 4: Commit**

Run:
```bash
git -C /home/kc/repos/clipad add defaults/ai_shortcuts.toml && git -C /home/kc/repos/clipad commit -m "feat(shortcuts): add embedded default ai-shortcut library

Single source of truth for the 23 default shortcuts that will be
seeded into the user's config on first run.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Wire `go:embed` into shortcuts.go (no behavior change yet)

**Files:**
- Modify: `/home/kc/repos/clipad/shortcuts.go`

- [ ] **Step 1: Write a failing test asserting the embedded TOML parses**

Append to `/home/kc/repos/clipad/shortcuts_test.go`:

```go
import (
	"github.com/pelletier/go-toml/v2"
)

func TestDefaultShortcutsEmbeddedTOMLParses(t *testing.T) {
	var cfg aiShortcutsConfig
	if err := toml.Unmarshal(defaultShortcutsTOML, &cfg); err != nil {
		t.Fatalf("embedded defaults failed to parse: %v", err)
	}
	if len(cfg.Shortcuts) != 23 {
		t.Fatalf("embedded defaults: want 23 shortcuts, got %d", len(cfg.Shortcuts))
	}

	want := []string{
		"prd",
		"userstory", "acceptance", "critique",
		"todos", "prioritize", "breakdown",
		"onboard", "explain",
		"tighten", "tldr", "outline", "questions", "examples", "diagram", "glossary", "risks",
		"bullets", "steps", "table", "headers", "fmtjson", "markdown",
	}
	for i, n := range want {
		if cfg.Shortcuts[i].Name != n {
			t.Errorf("shortcut %d: want name %q, got %q", i, n, cfg.Shortcuts[i].Name)
		}
		if cfg.Shortcuts[i].Prompt == "" {
			t.Errorf("shortcut %q: empty prompt", n)
		}
	}
}
```

Note: `shortcuts_test.go` does not currently import `toml`. Add the import block at the top of the file. If the import block already only contains `"testing"`, replace it with:

```go
import (
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)
```

- [ ] **Step 2: Run the test and confirm it fails to compile**

Run:
```bash
cd /home/kc/repos/clipad && go test -run TestDefaultShortcutsEmbeddedTOMLParses -v ./...
```

Expected: build failure with `undefined: defaultShortcutsTOML`.

- [ ] **Step 3: Add the embed declaration to `shortcuts.go`**

Modify `/home/kc/repos/clipad/shortcuts.go` to add `_ "embed"` to the import block and a package-level embed declaration. The current import block is:

```go
import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	toml "github.com/pelletier/go-toml/v2"
)
```

Change it to:

```go
import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	toml "github.com/pelletier/go-toml/v2"
)

//go:embed defaults/ai_shortcuts.toml
var defaultShortcutsTOML []byte
```

The `//go:embed` directive and the `var` it documents must be on consecutive lines with no blank line between them. Place this block immediately after the import block, before the existing `type AIShortcut struct` declaration.

- [ ] **Step 4: Run the test and confirm it passes**

Run:
```bash
cd /home/kc/repos/clipad && go test -run TestDefaultShortcutsEmbeddedTOMLParses -v ./...
```

Expected: `PASS`. Also run the full test suite to make sure nothing else broke:

```bash
cd /home/kc/repos/clipad && go test -v ./...
```

Expected: all tests pass (existing `TestLoadShortcuts_Missing` still passes since `loadShortcuts` behavior is unchanged at this point).

- [ ] **Step 5: Commit**

Run:
```bash
git -C /home/kc/repos/clipad add shortcuts.go shortcuts_test.go && git -C /home/kc/repos/clipad commit -m "feat(shortcuts): embed default library via go:embed

Exposes the 23-entry default set as defaultShortcutsTOML []byte.
No behavior change yet — loadShortcuts still returns nil for
missing files. Test asserts the embedded data parses to the
expected 23 names in spec order.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Replace TestLoadShortcuts_Missing with the new seed behavior test

**Files:**
- Modify: `/home/kc/repos/clipad/shortcuts_test.go`

- [ ] **Step 1: Replace the existing test**

Find this block in `/home/kc/repos/clipad/shortcuts_test.go`:

```go
func TestLoadShortcuts_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() should not error for missing file: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected empty slice, got %d shortcuts", len(loaded))
	}
}
```

Replace it with:

```go
func TestLoadShortcuts_SeedsWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}
	if len(loaded) != 23 {
		t.Fatalf("expected 23 seeded shortcuts, got %d", len(loaded))
	}
	if loaded[0].Name != "prd" {
		t.Errorf("first seeded shortcut: want %q, got %q", "prd", loaded[0].Name)
	}

	path := filepath.Join(tmpDir, "clipad", "ai_shortcuts.toml")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected seeded file at %s: %v", path, err)
	}
	if string(got) != string(defaultShortcutsTOML) {
		t.Errorf("seeded file content does not match embedded defaults")
	}
}
```

This test references `os` and `filepath`; add them to the test file's import block:

```go
import (
	"os"
	"path/filepath"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)
```

- [ ] **Step 2: Run the new test and confirm it fails**

Run:
```bash
cd /home/kc/repos/clipad && go test -run TestLoadShortcuts_SeedsWhenMissing -v ./...
```

Expected: `FAIL` with `expected 23 seeded shortcuts, got 0` (current `loadShortcuts` still returns `nil, nil` for missing files).

---

### Task 4: Implement seed-on-missing in `loadShortcuts`

**Files:**
- Modify: `/home/kc/repos/clipad/shortcuts.go`

- [ ] **Step 1: Replace the `loadShortcuts` function**

Find this function in `/home/kc/repos/clipad/shortcuts.go`:

```go
func loadShortcuts() ([]AIShortcut, error) {
	data, err := os.ReadFile(shortcutsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading shortcuts: %w", err)
	}
	var cfg aiShortcutsConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing shortcuts: %w", err)
	}
	return cfg.Shortcuts, nil
}
```

Replace it with:

```go
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
```

The change: the `IsNotExist` branch now seeds the file with `defaultShortcutsTOML`, then sets `data` to the embedded bytes so the existing parse path runs against it. The directory is created first (mirrors `saveShortcuts`).

- [ ] **Step 2: Run the seed test and confirm it passes**

Run:
```bash
cd /home/kc/repos/clipad && go test -run TestLoadShortcuts_SeedsWhenMissing -v ./...
```

Expected: `PASS`.

- [ ] **Step 3: Run the full test suite**

Run:
```bash
cd /home/kc/repos/clipad && go test -v ./...
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

Run:
```bash
git -C /home/kc/repos/clipad add shortcuts.go shortcuts_test.go && git -C /home/kc/repos/clipad commit -m "feat(shortcuts): seed default library on first run

When ai_shortcuts.toml does not exist, write the embedded
defaults to it and return them. Existing files are never
touched. Replaces TestLoadShortcuts_Missing (which asserted the
old 'return empty' behavior) with TestLoadShortcuts_SeedsWhenMissing.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Add tests for the never-overwrite invariant

**Files:**
- Modify: `/home/kc/repos/clipad/shortcuts_test.go`

- [ ] **Step 1: Add the no-overwrite test**

Append to `/home/kc/repos/clipad/shortcuts_test.go`:

```go
func TestLoadShortcuts_DoesNotOverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "clipad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := []byte("[[shortcuts]]\nname = 'mine'\nprompt = 'do my thing'\n")
	path := filepath.Join(dir, "ai_shortcuts.toml")
	if err := os.WriteFile(path, custom, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("want 1 shortcut, got %d", len(loaded))
	}
	if loaded[0].Name != "mine" {
		t.Errorf("want name %q, got %q", "mine", loaded[0].Name)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(custom) {
		t.Errorf("file was overwritten:\nwant: %q\ngot:  %q", custom, got)
	}
}
```

- [ ] **Step 2: Add the explicitly-empty test**

Append to `/home/kc/repos/clipad/shortcuts_test.go`:

```go
func TestLoadShortcuts_KeepsExplicitlyEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "clipad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	empty := []byte("# user has no shortcuts\n")
	path := filepath.Join(dir, "ai_shortcuts.toml")
	if err := os.WriteFile(path, empty, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("want 0 shortcuts (file present, empty intent), got %d", len(loaded))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(empty) {
		t.Errorf("file was overwritten with defaults:\nwant: %q\ngot:  %q", empty, got)
	}
}
```

- [ ] **Step 3: Run the new tests and confirm they pass**

Run:
```bash
cd /home/kc/repos/clipad && go test -run "TestLoadShortcuts_DoesNotOverwriteExisting|TestLoadShortcuts_KeepsExplicitlyEmpty" -v ./...
```

Expected: both `PASS`. (They should pass without code changes — the implementation in Task 4 already preserves these invariants.)

- [ ] **Step 4: Run the full test suite**

Run:
```bash
cd /home/kc/repos/clipad && go test -v ./...
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

Run:
```bash
git -C /home/kc/repos/clipad add shortcuts_test.go && git -C /home/kc/repos/clipad commit -m "test(shortcuts): cover never-overwrite invariants for default seeding

Adds tests asserting that an existing custom ai_shortcuts.toml
is left untouched, and that an existing-but-intentionally-empty
file is not reseeded with defaults.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Build verify

- [ ] **Step 1: Build the binary**

Run:
```bash
cd /home/kc/repos/clipad && go build -o /tmp/clipad-build-check . && ls -l /tmp/clipad-build-check && rm /tmp/clipad-build-check
```

Expected: build succeeds, binary is created and removed cleanly. Confirms `go:embed` resolved the `defaults/ai_shortcuts.toml` path correctly relative to the package source.

- [ ] **Step 2: Run `go vet`**

Run:
```bash
cd /home/kc/repos/clipad && go vet ./...
```

Expected: no output.

---

### Task 7: README update

**Files:**
- Modify: `/home/kc/repos/clipad/README.md`

- [ ] **Step 1: Read current Plugins section**

Read `/home/kc/repos/clipad/README.md` and locate the `## Plugins` section (around line 78 in the current file). The new content goes inside that section.

- [ ] **Step 2: Insert a new subsection after the OpenRouter subsection**

Place the new `### AI Shortcuts` subsection immediately after the OpenRouter section ends and before `## Configuration`. The OpenRouter section currently ends with the line `Plugin config is stored at \`~/.config/clipad/plugins/openrouter.toml\`.` Insert the new subsection after that paragraph (with a blank line between them) and before the `## Configuration` heading.

The new subsection should look like:

```markdown
### AI Shortcuts

Quick text transformations powered by your configured LLM. Press `Ctrl+Space`, pick a shortcut, and the model rewrites or augments the current note. The diff view lets you accept or reject the change.

Shortcuts live in `~/.config/clipad/ai_shortcuts.toml` as `[[shortcuts]]` blocks (`name` + `prompt`). On first run the file is seeded with a default library of 23 shortcuts; you can edit, delete, or add entries freely afterward — clipad never overwrites your file.

The default library covers:

- **Requirements** — `prd`, `userstory`, `acceptance`, `critique`
- **Todos** — `todos`, `prioritize`, `breakdown`
- **Tech notes** — `onboard`, `explain`
- **Universal utilities** — `tighten`, `tldr`, `outline`, `questions`, `examples`, `diagram`, `glossary`, `risks`
- **Formatting** — `bullets`, `steps`, `table`, `headers`, `fmtjson`, `markdown`

```

(Note the trailing blank line so the next `### OpenRouter` heading is well-spaced.)

- [ ] **Step 3: Commit**

Run:
```bash
git -C /home/kc/repos/clipad add README.md && git -C /home/kc/repos/clipad commit -m "docs(readme): describe AI shortcut library and default seeding

Adds an AI Shortcuts subsection under Plugins covering what the
shortcuts do, where the file lives, the seed-on-first-run
behavior, and the names of the 23 defaults grouped by category.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Final verification and report

- [ ] **Step 1: Run the full test suite one more time**

Run:
```bash
cd /home/kc/repos/clipad && go test -v ./...
```

Expected: all tests pass, including the four new shortcut tests.

- [ ] **Step 2: Confirm the embed file is referenced correctly**

Run:
```bash
cd /home/kc/repos/clipad && grep -n 'go:embed' shortcuts.go && ls -l defaults/ai_shortcuts.toml
```

Expected: one `//go:embed` line targeting `defaults/ai_shortcuts.toml`; that file exists and is non-empty.

- [ ] **Step 3: Print the final git log for this feature**

Run:
```bash
git -C /home/kc/repos/clipad log --oneline @{u}..HEAD
```

Expected: a clean stack of commits — one per task — sitting on top of the previously-pushed master.

- [ ] **Step 4: Report**

> Implementation complete. Embedded 23-entry default library in `defaults/ai_shortcuts.toml`, taught `loadShortcuts` to seed the user's config on first run, added four new tests covering parse/seed/no-overwrite/keep-empty invariants, updated README. All tests pass. Ready for push and release.

---

## Closing Notes

- **Push and release** are explicitly out of this plan — they belong to a separate decision step where the user signs off on the version bump and release notes.
- **No backwards-compatibility hack** for users who relied on the old "missing file → empty list" behavior. The new behavior is strictly more useful and any user who actively wants no shortcuts can write a one-line empty TOML file.
- **Spec coverage check:** every requirement in spec Part 2 maps to a task — embed file (Task 1), embed declaration (Task 2), loader change (Task 4), the four required tests (Tasks 2, 3, 5), README update (Task 7), build verify (Task 6).
