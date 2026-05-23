# CLI Startup Arguments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let clipad perform a quick action at launch — `clipad -p <path>` (preview), `clipad <path>` (edit), `clipad --new`/`-n` (new note) — instead of always opening the default vault view.

**Architecture:** A pure `resolveStartup` function classifies the parsed flags + positional path into a `startupAction` (open file / new note in dir / new note). `prepareStartup` performs any filesystem creation in `main()` before the TUI starts. The action is stored on the model and applied exactly once on the first `tea.WindowSizeMsg` (after `recalcLayout` has sized the panes) via `applyStartup`, which sets `editorMode` / `activePanel` / `treeHidden` / `newNoteDir` and builds the preview viewport.

**Tech Stack:** Go 1.26.1, Bubble Tea + Bubbles (`viewport`, `textinput`), stdlib `flag`. Module `clipad`.

---

## File Structure

- **Create `startup.go`** — `startupKind` constants, `startupAction` struct, `resolveStartup` (pure classification), `prepareStartup` (filesystem prep), `applyStartup` (`*model` method, view-state mutation).
- **Create `startup_test.go`** — unit tests for all three functions.
- **Modify `model.go`** — add `startup` / `startupDone` fields to the `model` struct (~line 199); apply the action in the `tea.WindowSizeMsg` handler (line 477-481).
- **Modify `main.go`** — register `-p`/`--preview` and `-n`/`--new` flags; call `resolveStartup` + `prepareStartup`; set `m.startup`.
- **Modify `README.md`** — document the flags (CLI flags table, line 61) and a "Quick actions" subsection.

Each task ends in a compiling, fully-tested state.

---

### Task 1: `startupAction` type + `resolveStartup`

**Files:**
- Create: `startup.go`
- Test: `startup_test.go`

- [ ] **Step 1: Write the failing tests**

Create `startup_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStartup_ExistingFile_Edit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveStartup(false, false, file, dir, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.kind != startupOpenFile || got.path != file || got.preview || !got.hideTree {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_ExistingFile_Preview(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	os.WriteFile(file, []byte("hi"), 0o644)
	got, _ := resolveStartup(true, false, file, dir, dir)
	if got.kind != startupOpenFile || !got.preview || !got.hideTree {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_RelativePath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "rel.md"), []byte("x"), 0o644)
	got, _ := resolveStartup(false, false, "rel.md", dir, dir)
	if got.kind != startupOpenFile || got.path != filepath.Join(dir, "rel.md") {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_ExistingDir_NewNote(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	got, _ := resolveStartup(false, false, sub, dir, dir)
	if got.kind != startupNewNoteInDir || got.path != sub || !got.hideTree || got.needsMkdir {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_MissingFile_NeedsCreate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "new", "deep.md")
	got, _ := resolveStartup(false, false, target, dir, dir)
	if got.kind != startupOpenFile || !got.needsCreate || !got.hideTree {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_MissingDirTrailingSlash_NeedsMkdir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "newdir") + "/"
	got, _ := resolveStartup(false, false, target, dir, dir)
	if got.kind != startupNewNoteInDir || !got.needsMkdir {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_NewFlag_VaultRoot(t *testing.T) {
	vault := t.TempDir()
	got, _ := resolveStartup(false, true, "", "/tmp", vault)
	if got.kind != startupNewNote || got.path != vault || got.hideTree {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_PreviewNoPath_Errors(t *testing.T) {
	if _, err := resolveStartup(true, false, "", "/tmp", "/vault"); err == nil {
		t.Error("expected error for -p with no path")
	}
}

func TestResolveStartup_NoArgs_None(t *testing.T) {
	got, err := resolveStartup(false, false, "", "/tmp", "/vault")
	if err != nil || got.kind != startupNone {
		t.Errorf("got %+v err %v", got, err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run TestResolveStartup`
Expected: FAIL — `undefined: resolveStartup` / `undefined: startupOpenFile`.

- [ ] **Step 3: Implement `startup.go`**

Create `startup.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type startupKind int

const (
	startupNone startupKind = iota
	startupNewNote      // --new: new note in the vault root
	startupNewNoteInDir // path resolves to a directory
	startupOpenFile     // path resolves to a file
)

// startupAction describes the one-shot action to perform when clipad launches
// with command-line arguments. It is resolved before the TUI starts and applied
// once on the first WindowSizeMsg.
type startupAction struct {
	kind        startupKind
	path        string // resolved absolute path (file for open; directory for new-note kinds)
	preview     bool   // open the file in preview mode (only meaningful for startupOpenFile)
	hideTree    bool   // hide the file tree on launch
	needsCreate bool   // create an empty file (plus parents) before opening
	needsMkdir  bool   // create the directory before starting the new note
}

// resolveStartup classifies command-line arguments into a startupAction. It
// performs read-only stat checks but no filesystem writes, so it is safe to
// unit test. cwd and vault are passed in (not read from globals) so tests need
// no chdir and can run in parallel.
func resolveStartup(preview, newNote bool, pathArg, cwd, vault string) (startupAction, error) {
	if newNote {
		return startupAction{kind: startupNewNote, path: vault, hideTree: false}, nil
	}
	if pathArg == "" {
		if preview {
			return startupAction{}, fmt.Errorf("-p requires a file path")
		}
		return startupAction{kind: startupNone}, nil
	}

	trailingSlash := strings.HasSuffix(pathArg, "/")

	p := pathArg
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	abs := filepath.Clean(p)

	info, err := os.Stat(abs)
	switch {
	case err == nil && info.IsDir():
		return startupAction{kind: startupNewNoteInDir, path: abs, hideTree: true}, nil
	case err == nil:
		return startupAction{kind: startupOpenFile, path: abs, preview: preview, hideTree: true}, nil
	case os.IsNotExist(err):
		if trailingSlash {
			return startupAction{kind: startupNewNoteInDir, path: abs, hideTree: true, needsMkdir: true}, nil
		}
		return startupAction{kind: startupOpenFile, path: abs, preview: preview, hideTree: true, needsCreate: true}, nil
	default:
		return startupAction{}, fmt.Errorf("cannot access %s: %w", abs, err)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run TestResolveStartup -v`
Expected: PASS (all 9 tests).

- [ ] **Step 5: Commit**

```bash
git add startup.go startup_test.go
git commit -m "feat(startup): resolveStartup classifies CLI args into startupAction"
```

---

### Task 2: `prepareStartup` (filesystem prep)

**Files:**
- Modify: `startup.go`
- Test: `startup_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `startup_test.go`:

```go
func TestPrepareStartup_CreatesFileAndParents(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "note.md")
	a := startupAction{kind: startupOpenFile, path: target, needsCreate: true}
	if err := prepareStartup(a); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestPrepareStartup_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "newdir")
	a := startupAction{kind: startupNewNoteInDir, path: target, needsMkdir: true}
	if err := prepareStartup(a); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Errorf("dir not created: err=%v", err)
	}
}

func TestPrepareStartup_ExistingFile_NoOp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.md")
	os.WriteFile(file, []byte("keep"), 0o644)
	a := startupAction{kind: startupOpenFile, path: file} // no needsCreate
	if err := prepareStartup(a); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "keep" {
		t.Errorf("file content changed: %q", data)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run TestPrepareStartup`
Expected: FAIL — `undefined: prepareStartup`.

- [ ] **Step 3: Implement `prepareStartup`**

Append to `startup.go`:

```go
// prepareStartup performs the filesystem side effects implied by an action:
// creating a missing file (and its parents) or a missing directory. It is
// called from main() before the TUI starts so errors can exit cleanly.
func prepareStartup(a startupAction) error {
	switch {
	case a.needsCreate:
		if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		return f.Close()
	case a.needsMkdir:
		return os.MkdirAll(a.path, 0o755)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run TestPrepareStartup -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add startup.go startup_test.go
git commit -m "feat(startup): prepareStartup creates missing files/dirs"
```

---

### Task 3: Model fields + `applyStartup` + WindowSizeMsg wiring

**Files:**
- Modify: `model.go` (struct ~line 199; WindowSizeMsg handler 477-481)
- Modify: `startup.go` (add imports + `applyStartup`)
- Test: `startup_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `startup_test.go`. Add `tea "github.com/charmbracelet/bubbletea"` to its import block (final import block becomes `"os"`, `"path/filepath"`, `"testing"`, `tea "github.com/charmbracelet/bubbletea"`):

```go
func newStartupTestModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	return newModel(t.TempDir(), nil, "", "")
}

func TestApplyStartup_OpenFileEdit(t *testing.T) {
	m := newStartupTestModel(t)
	file := filepath.Join(t.TempDir(), "note.md")
	os.WriteFile(file, []byte("body text"), 0o644)
	m.startup = startupAction{kind: startupOpenFile, path: file, hideTree: true}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm := next.(model)
	if nm.currentFile != file {
		t.Errorf("currentFile = %q, want %q", nm.currentFile, file)
	}
	if nm.editorMode != modeEdit {
		t.Errorf("editorMode = %v, want modeEdit", nm.editorMode)
	}
	if nm.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", nm.activePanel)
	}
	if !nm.treeHidden {
		t.Error("treeHidden should be true")
	}
	if !nm.startupDone {
		t.Error("startupDone should be true")
	}
}

func TestApplyStartup_OpenFilePreview(t *testing.T) {
	m := newStartupTestModel(t)
	file := filepath.Join(t.TempDir(), "note.md")
	os.WriteFile(file, []byte("preview body"), 0o644)
	m.startup = startupAction{kind: startupOpenFile, path: file, preview: true, hideTree: true}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm := next.(model)
	if nm.editorMode != modePreview {
		t.Errorf("editorMode = %v, want modePreview", nm.editorMode)
	}
	if nm.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", nm.activePanel)
	}
	if nm.currentFile != file {
		t.Errorf("currentFile = %q, want %q", nm.currentFile, file)
	}
}

func TestApplyStartup_NewNoteInDir(t *testing.T) {
	m := newStartupTestModel(t)
	dir := t.TempDir()
	m.startup = startupAction{kind: startupNewNoteInDir, path: dir, hideTree: true}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm := next.(model)
	if nm.newNoteDir != dir {
		t.Errorf("newNoteDir = %q, want %q", nm.newNoteDir, dir)
	}
	if nm.currentFile != "" {
		t.Errorf("currentFile = %q, want empty", nm.currentFile)
	}
	if nm.editorMode != modeEdit {
		t.Errorf("editorMode = %v, want modeEdit", nm.editorMode)
	}
	if nm.editor.Value() != "" {
		t.Errorf("editor not empty: %q", nm.editor.Value())
	}
}

func TestApplyStartup_RunsOnce(t *testing.T) {
	m := newStartupTestModel(t)
	file := filepath.Join(t.TempDir(), "note.md")
	os.WriteFile(file, []byte("orig"), 0o644)
	m.startup = startupAction{kind: startupOpenFile, path: file, hideTree: true}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm := next.(model)
	nm.editor.SetValue("user typed")
	next2, _ := nm.Update(tea.WindowSizeMsg{Width: 80, Height: 25})
	nm2 := next2.(model)
	if nm2.editor.Value() != "user typed" {
		t.Errorf("startup re-applied on second resize; editor = %q", nm2.editor.Value())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run TestApplyStartup`
Expected: FAIL — `m.startup undefined` / `m.startupDone undefined` (compile error).

- [ ] **Step 3: Add the model fields**

In `model.go`, locate the end of the `model` struct (the quick-capture block ending with `delegateInput textinput.Model`, ~line 199). Add immediately after it, before the closing `}`:

```go
	// Startup action (applied once on the first WindowSizeMsg)
	startup     startupAction
	startupDone bool
```

- [ ] **Step 4: Implement `applyStartup`**

In `startup.go`, extend the import block to:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)
```

Append the method:

```go
// applyStartup mutates the model to reflect the resolved startup action. It runs
// once, on the first WindowSizeMsg, after recalcLayout has sized the panes. All
// referenced paths already exist (prepareStartup ran in main). It returns a
// command to focus the editor where appropriate.
func (m *model) applyStartup() tea.Cmd {
	m.treeHidden = m.startup.hideTree
	m.recalcLayout() // re-flow now that treeHidden may have changed

	switch m.startup.kind {
	case startupOpenFile:
		m.openFile(m.startup.path)
		if m.startup.preview {
			vp := viewport.New(m.editorWidth-2, m.editorHeight)
			vp.SetContent(wordWrap(m.editor.Value(), m.editorWidth-4))
			m.preview = vp
			m.editorMode = modePreview
			m.editor.Blur()
			m.activePanel = editorPanel
			return nil
		}
		m.editorMode = modeEdit
		m.activePanel = editorPanel
		return m.editor.Focus()

	case startupNewNote, startupNewNoteInDir:
		m.newNoteDir = m.startup.path
		m.currentFile = ""
		m.editor.ClearHistory()
		m.editor.SetValue("")
		m.cleanContent = ""
		m.editorMode = modeEdit
		m.activePanel = editorPanel
		return m.editor.Focus()
	}
	return nil
}
```

- [ ] **Step 5: Wire the WindowSizeMsg handler**

In `model.go`, replace the `tea.WindowSizeMsg` case (currently lines 477-481):

```go
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil
```

with:

```go
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		if !m.startupDone && m.startup.kind != startupNone {
			cmd := m.applyStartup()
			m.startupDone = true
			return m, cmd
		}
		return m, nil
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./... -run TestApplyStartup -v`
Expected: PASS (4 tests).

- [ ] **Step 7: Commit**

```bash
git add startup.go model.go startup_test.go
git commit -m "feat(startup): apply startup action on first WindowSizeMsg"
```

---

### Task 4: Wire CLI flags in `main.go`

**Files:**
- Modify: `main.go`

No new unit test — `main()` orchestrates already-tested functions. Verified by `go build` and a manual smoke run.

- [ ] **Step 1: Register the flags**

In `main.go`, replace the flag block (currently lines 101-107):

```go
	var (
		showVersion bool
		doUpgrade   bool
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&doUpgrade, "upgrade", false, "fetch the latest release and replace this binary")
	flag.Parse()
```

with:

```go
	var (
		showVersion bool
		doUpgrade   bool
		previewFlag bool
		newFlag     bool
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&doUpgrade, "upgrade", false, "fetch the latest release and replace this binary")
	flag.BoolVar(&previewFlag, "p", false, "open the given file in preview mode (tree hidden)")
	flag.BoolVar(&previewFlag, "preview", false, "open the given file in preview mode (tree hidden)")
	flag.BoolVar(&newFlag, "n", false, "start in new-note mode")
	flag.BoolVar(&newFlag, "new", false, "start in new-note mode")
	flag.Parse()
```

- [ ] **Step 2: Resolve and prepare the startup action**

In `main.go`, immediately after the vault-existence check (the block ending at line 145 with `os.Exit(1)` / `}` for "Vault directory not found"), insert:

```go
	cwd, _ := os.Getwd()
	startup, err := resolveStartup(previewFlag, newFlag, flag.Arg(0), cwd, cfg.Vault)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := prepareStartup(startup); err != nil {
		fmt.Fprintf(os.Stderr, "cannot prepare %s: %v\n", startup.path, err)
		os.Exit(1)
	}
```

(`err` is already declared earlier in `main`, so `startup, err :=` reuses it — valid because `startup` and `cwd` are new.)

- [ ] **Step 3: Attach the action to the model**

In `main.go`, immediately after `m := newModel(cfg.Vault, plugins, cfg.AIShortcutProvider, cfg.InboxPath)` (line 163), add:

```go
	m.startup = startup
```

- [ ] **Step 4: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: no output, exit 0.

- [ ] **Step 5: Manual smoke check**

Run (in a scratch dir, with a configured vault):
```bash
go build -o /tmp/clipad-smoke . && cd /tmp && echo "# hi" > smoke.md
```
Then verify by inspection / interactive run:
- `/tmp/clipad-smoke -p /tmp/smoke.md` → opens preview, tree hidden, a keystroke switches to edit.
- `/tmp/clipad-smoke /tmp/smoke.md` → opens in edit, tree hidden.
- `/tmp/clipad-smoke --new` → new-note mode, tree visible.
- `/tmp/clipad-smoke /tmp/newdir/deep.md` → creates the file + dirs, opens in edit.
- `/tmp/clipad-smoke -p` → prints `-p requires a file path` and exits non-zero.

Quit with `Ctrl+Q`. (This step is observational; no automated assertion.)

- [ ] **Step 6: Commit**

```bash
git add main.go
git commit -m "feat(cli): wire -p/--preview, -n/--new flags and positional path"
```

---

### Task 5: Documentation

**Files:**
- Modify: `README.md` (CLI flags table at line 61; add "Quick actions" subsection)

- [ ] **Step 1: Extend the CLI flags table**

In `README.md`, the CLI flags table currently has rows for `--version` and `--upgrade`. Add these two rows after the `--upgrade` row:

```markdown
| `-p`, `--preview` `<path>` | Open `<path>` in preview mode with the file tree hidden; typing switches to edit mode |
| `-n`, `--new` | Start in new-note mode (same as "+ Add note"); the file tree stays visible |
```

- [ ] **Step 2: Add the "Quick actions" subsection**

In `README.md`, immediately after the CLI flags table (before the `## Keybindings` heading), add:

```markdown
### Quick actions

Open or create a note straight from the shell. Paths may be relative or
absolute and can point anywhere on the filesystem.

```bash
clipad path/to/note.md      # open in edit mode, file tree hidden
clipad -p path/to/note.md   # open in preview mode; start typing to edit
clipad --new                # start a new note in the vault root
clipad path/to/dir/         # start a new note in that directory
```

- A path to an existing file opens it; a path to a directory starts a new note in it.
- A non-existing path is created — the file, plus any missing parent directories.
- Flags must come before the path, e.g. `clipad -p note.md`.
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document CLI startup arguments"
```

---

### Task 6: Final verification + release

**Files:** none (build + release only)

- [ ] **Step 1: Full test suite + vet + build**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: `ok  clipad` (all tests pass), no vet output, clean build.

- [ ] **Step 2: Build the release binary (linux/amd64)**

The latest release is `v0.0.36`; the next patch is `v0.0.37`. The `--upgrade` path expects an asset named `clipad-<tag>-<goos>-<goarch>`.

```bash
GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=v0.0.37" -o clipad-v0.0.37-linux-amd64 .
./clipad-v0.0.37-linux-amd64 --version   # expect: clipad v0.0.37
```

- [ ] **Step 3: Push the branch**

```bash
git push -u origin "$(git branch --show-current)"
```

- [ ] **Step 4: Create the GitHub release with the binary asset**

Per the project release policy, every release must include the linux/amd64 binary asset.

```bash
gh release create v0.0.37 \
  --title "clipad v0.0.37" \
  --generate-notes \
  clipad-v0.0.37-linux-amd64
```

- [ ] **Step 5: Verify the release**

```bash
gh release view v0.0.37 --json tagName,assets -q '.tagName, (.assets[].name)'
```
Expected: `v0.0.37` and `clipad-v0.0.37-linux-amd64`.

Then remove the local build artifact:
```bash
rm clipad-v0.0.37-linux-amd64
```

---

## Notes / decisions captured from the spec

- **Paths are unrestricted** (anywhere on the filesystem). Files outside the vault won't appear in the tree when toggled visible with `Ctrl+B`; this is expected.
- **Directory paths** → new note in that directory for both `clipad <dir>` and `clipad -p <dir>` (preview is a no-op for an empty new note).
- **`--new` keeps the tree visible**; the path forms (`-p`/positional) hide it. This asymmetry follows the task's explicit wording.
- **Flags must precede the path** — a stdlib `flag` limitation, documented in the README.
- **`resolveStartup` reads `$HOME`** (via `os.UserHomeDir`) only for `~` expansion; tests set `HOME` where relevant.
