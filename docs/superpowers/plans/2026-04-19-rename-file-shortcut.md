# Rename File or Folder Shortcut Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Ctrl+E shortcut in the file tree to rename the selected file or folder, with the open-file path and clipboard kept consistent across the rename.

**Architecture:** Mirror the existing `inputNewFolder` pattern: new `inputRename` enum value, a `renameInput` text field on the model, a tree-key handler that opens the prompt, and a dedicated `handleRename` input-mode handler. The actual filesystem work lives in a small extractable helper (`(m *model) doRename`) so it can be unit-tested without a Bubble Tea harness.

**Tech Stack:** Go, Bubble Tea (`textinput.Model`), `os.Rename`, existing `model.go` patterns.

**Spec:** [`docs/superpowers/specs/2026-04-19-rename-file-shortcut-design.md`](../specs/2026-04-19-rename-file-shortcut-design.md)

---

## File Map

- **Modify** `model.go` — add `inputRename` enum value, three new fields on `model`, init `renameInput` in `newModel`, dispatch in `handleInputMode`, status-bar branch in `View()`, `ctrl+e` case in `handleTreeKeys`, `handleRename` handler, `doRename` helper.
- **Create** `rename_test.go` — unit tests for `doRename`.
- **Modify** `README.md` — add `Ctrl+E` row under **File Tree**.

---

## Task 1: Add `inputRename` enum value and `renameInput` model fields

**Files:**
- Modify: `model.go` (enum block at lines 43-60; struct around line 95-99; `newModel` around line 138-200)

- [ ] **Step 1: Add the enum constant**

In `model.go`, locate the `inputMode` const block (lines 43-60). Add `inputRename` at the end:

```go
type inputMode int

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
	inputShortcutPrompt
	inputShortcutDeleteConfirm
	inputGitRemote
	inputRename
)
```

- [ ] **Step 2: Add fields to `model` struct**

Find the `newFolderInput` field declaration in the `model` struct (around line 95). Add the three rename fields directly after it:

```go
	newFolderInput     textinput.Model
	renameInput        textinput.Model
	renameTarget       string
	renameIsDir        bool
	replaceSearchInput textinput.Model
```

- [ ] **Step 3: Initialize `renameInput` in `newModel`**

In `newModel`, find where `nf` (the new-folder input) is created (around line 147-149) and add a sibling `rn` for rename:

```go
	nf := textinput.New()
	nf.Placeholder = "folder name"
	nf.CharLimit = 256

	rn := textinput.New()
	rn.Placeholder = "new name"
	rn.CharLimit = 256
```

Then in the struct literal where `newFolderInput: nf,` is set (around line 178), add the rename input alongside:

```go
		newFolderInput:     nf,
		renameInput:        rn,
		replaceSearchInput: rs,
```

- [ ] **Step 4: Verify the build**

Run: `go build ./...`
Expected: clean build, no errors.

- [ ] **Step 5: Commit**

```bash
git add model.go
git commit -m "feat(rename): add inputRename enum and renameInput model state"
```

---

## Task 2: Add `doRename` helper with failing tests

We extract the filesystem logic into a testable method. The handler in Task 4 will call it.

**Files:**
- Create: `rename_test.go`
- Modify: `model.go` (add `doRename` method)

- [ ] **Step 1: Write the failing tests**

Create `rename_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func newRenameTestModel(t *testing.T, vault string) model {
	t.Helper()
	m := newModel(vault, nil)
	return m
}

func TestDoRename_FileReappendsExtension(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.md")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newRenameTestModel(t, dir)
	m.renameTarget = src
	m.renameIsDir = false

	if err := m.doRename("new"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	want := filepath.Join(dir, "new.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected %s to exist: %v", want, err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected %s to be gone, err=%v", src, err)
	}
}

func TestDoRename_FolderRewritesOpenFilePath(t *testing.T) {
	dir := t.TempDir()
	oldFolder := filepath.Join(dir, "oldfolder")
	if err := os.MkdirAll(oldFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	openFile := filepath.Join(oldFolder, "note.md")
	if err := os.WriteFile(openFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newRenameTestModel(t, dir)
	m.currentFile = openFile
	m.tree.currentFile = openFile
	m.renameTarget = oldFolder
	m.renameIsDir = true

	if err := m.doRename("newfolder"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	wantOpen := filepath.Join(dir, "newfolder", "note.md")
	if m.currentFile != wantOpen {
		t.Errorf("currentFile = %q, want %q", m.currentFile, wantOpen)
	}
	if m.tree.currentFile != wantOpen {
		t.Errorf("tree.currentFile = %q, want %q", m.tree.currentFile, wantOpen)
	}
}

func TestDoRename_RejectsPathSeparator(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	os.WriteFile(src, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.renameTarget = src
	m.renameIsDir = false

	err := m.doRename("sub/b")
	if err == nil {
		t.Fatal("expected error for path separator, got nil")
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("source should still exist, statErr=%v", statErr)
	}
}

func TestDoRename_RejectsExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	dst := filepath.Join(dir, "b.md")
	os.WriteFile(src, []byte("x"), 0o644)
	os.WriteFile(dst, []byte("y"), 0o644)
	m := newRenameTestModel(t, dir)
	m.renameTarget = src
	m.renameIsDir = false

	err := m.doRename("b")
	if err == nil {
		t.Fatal("expected error for existing target, got nil")
	}
	data, _ := os.ReadFile(src)
	if string(data) != "x" {
		t.Errorf("source clobbered: %q", data)
	}
}

func TestDoRename_UpdatesOpenFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "open.md")
	os.WriteFile(src, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.currentFile = src
	m.tree.currentFile = src
	m.renameTarget = src
	m.renameIsDir = false

	if err := m.doRename("renamed"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	want := filepath.Join(dir, "renamed.md")
	if m.currentFile != want {
		t.Errorf("currentFile = %q, want %q", m.currentFile, want)
	}
	if m.tree.currentFile != want {
		t.Errorf("tree.currentFile = %q, want %q", m.tree.currentFile, want)
	}
}

func TestDoRename_ClearsClipboardOnMatch(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	os.WriteFile(src, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.fileClip = fileClipboard{path: src, op: clipCut}
	m.tree.cutPath = src
	m.renameTarget = src
	m.renameIsDir = false

	if err := m.doRename("b"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	if !m.fileClip.empty() {
		t.Errorf("fileClip should be cleared, got %+v", m.fileClip)
	}
	if m.tree.cutPath != "" {
		t.Errorf("cutPath should be cleared, got %q", m.tree.cutPath)
	}
}

func TestDoRename_ClearsClipboardOnFolderContains(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "f")
	os.MkdirAll(folder, 0o755)
	clip := filepath.Join(folder, "a.md")
	os.WriteFile(clip, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.fileClip = fileClipboard{path: clip, op: clipCopy}
	m.tree.cutPath = clip
	m.renameTarget = folder
	m.renameIsDir = true

	if err := m.doRename("g"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	if !m.fileClip.empty() {
		t.Errorf("fileClip should be cleared, got %+v", m.fileClip)
	}
	if m.tree.cutPath != "" {
		t.Errorf("cutPath should be cleared, got %q", m.tree.cutPath)
	}
}

func TestDoRename_NoOpSameName(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	os.WriteFile(src, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.renameTarget = src
	m.renameIsDir = false

	if err := m.doRename("a"); err != nil {
		t.Fatalf("doRename: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source should still exist: %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run: `go test ./... -run TestDoRename`
Expected: FAIL with `m.doRename undefined`.

- [ ] **Step 3: Implement `doRename` in `model.go`**

Add the following method at the end of `model.go` (after `updateLastSync`). The method returns an error; the caller decides whether to surface it via `errMsg` or keep the prompt open.

```go
func (m *model) doRename(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("name cannot contain path separators")
	}

	dir := filepath.Dir(m.renameTarget)
	var target string
	if m.renameIsDir {
		target = filepath.Join(dir, name)
	} else {
		target = filepath.Join(dir, name+filepath.Ext(m.renameTarget))
	}

	if target == m.renameTarget {
		return nil
	}

	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("already exists: %s", filepath.Base(target))
	}

	if err := os.Rename(m.renameTarget, target); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}

	if m.renameIsDir {
		prefix := m.renameTarget + string(os.PathSeparator)
		if strings.HasPrefix(m.currentFile, prefix) {
			rest := strings.TrimPrefix(m.currentFile, prefix)
			m.currentFile = filepath.Join(target, rest)
			m.tree.currentFile = m.currentFile
		}
		if m.fileClip.path == m.renameTarget || strings.HasPrefix(m.fileClip.path, prefix) {
			m.fileClip = fileClipboard{}
		}
		if m.tree.cutPath == m.renameTarget || strings.HasPrefix(m.tree.cutPath, prefix) {
			m.tree.cutPath = ""
		}
	} else {
		if m.currentFile == m.renameTarget {
			m.currentFile = target
			m.tree.currentFile = target
		}
		if m.fileClip.path == m.renameTarget {
			m.fileClip = fileClipboard{}
		}
		if m.tree.cutPath == m.renameTarget {
			m.tree.cutPath = ""
		}
	}

	return nil
}
```

- [ ] **Step 4: Run tests and verify they pass**

Run: `go test ./... -run TestDoRename -v`
Expected: all 8 `TestDoRename_*` tests pass.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: all tests pass — no regressions in `TestBuildTree`, `TestFlattenTree`, etc.

- [ ] **Step 6: Commit**

```bash
git add rename_test.go model.go
git commit -m "feat(rename): add doRename helper with unit tests"
```

---

## Task 3: Wire Ctrl+E into the tree key handler

**Files:**
- Modify: `model.go` — add `ctrl+e` case in `handleTreeKeys` (around lines 484-541)

- [ ] **Step 1: Add the `ctrl+e` case**

In `handleTreeKeys`, locate the `case "ctrl+f":` block (around line 527-531). Insert a new case directly above it:

```go
	case "ctrl+e":
		node := m.tree.selectedNode()
		if node != nil {
			m.renameTarget = node.Path
			m.renameIsDir = node.IsDir
			prefill := node.Name
			if !node.IsDir {
				prefill = strings.TrimSuffix(node.Name, filepath.Ext(node.Name))
			}
			m.renameInput.SetValue(prefill)
			m.renameInput.CursorEnd()
			m.inputMode = inputRename
			cmd := m.renameInput.Focus()
			return m, cmd
		}
	case "ctrl+f":
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add model.go
git commit -m "feat(rename): wire Ctrl+E to open rename prompt in tree panel"
```

---

## Task 4: Add `handleRename` input handler and dispatch

**Files:**
- Modify: `model.go` — add `handleRename`, dispatch from `handleInputMode` (line 597), status-bar branch in `View()` (around line 1256)

- [ ] **Step 1: Add the handler dispatch**

In `handleInputMode` (around line 597), add a case at the end of the switch:

```go
	case inputGitRemote:
		return m.handleGitRemoteInput(msg)
	case inputRename:
		return m.handleRename(msg)
	}
	return m, nil
}
```

- [ ] **Step 2: Add the `handleRename` method**

Add directly after `handleNewFolder` (which ends around line 769):

```go
func (m model) handleRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.renameInput.Value())
		if name == "" {
			return m, nil
		}
		if err := m.doRename(name); err != nil {
			m.errMsg = err.Error()
			// Keep prompt open for recoverable validation errors;
			// close on filesystem failures so the user isn't stuck.
			if strings.HasPrefix(err.Error(), "rename failed") {
				m.inputMode = inputNone
			}
			return m, nil
		}
		m.refreshTree()
		m.inputMode = inputNone
		m.errMsg = ""
		return m, nil
	case "esc":
		m.inputMode = inputNone
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
	m.renameInput, cmd = m.renameInput.Update(msg)
	return m, cmd
}
```

- [ ] **Step 3: Add the status-bar branch**

In `View()`, find the `} else if m.inputMode == inputNewFolder {` branch (around line 1256). Add a sibling branch directly after the `inputNewFolder` block:

```go
	} else if m.inputMode == inputNewFolder {
		statusView = statusBarStyle.Width(m.width).Render(
			"New folder: " + m.newFolderInput.View())
	} else if m.inputMode == inputRename {
		statusView = statusBarStyle.Width(m.width).Render(
			"Rename: " + m.renameInput.View())
	} else if m.inputMode == inputReplaceSearch {
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: all pass.

- [ ] **Step 6: Manual smoke test**

Run: `go run . `

In the running app:
1. Navigate the tree to a markdown file. Press `Ctrl+E`. Status bar should show `Rename: <basename>` with the cursor at the end. Type `_renamed`, press Enter. Verify the file is renamed in the tree (extension preserved) and the open file pointer follows.
2. Open one of the files inside a folder. Navigate cursor to the parent folder. Press `Ctrl+E`. Rename the folder. Verify the open file in the editor still points at the right path (status bar filename should reflect the new folder).
3. Press `Ctrl+E`, type `foo/bar`, press Enter. Verify `Name cannot contain path separators` shows in the status bar and the prompt stays open.
4. Press `Esc` while the prompt is open. Verify it closes without renaming.
5. Press `Ctrl+E`, type the existing name of another file in the same directory, press Enter. Verify `Already exists: <name>` shows and the prompt stays open.

- [ ] **Step 7: Commit**

```bash
git add model.go
git commit -m "feat(rename): add handleRename input handler and status-bar prompt"
```

---

## Task 5: Document the shortcut in the README

**Files:**
- Modify: `README.md` (File Tree keybindings table around lines 60-69)

- [ ] **Step 1: Add the row**

In `README.md`, find the **File Tree** table:

```markdown
### File Tree

| Key | Action |
|-----|--------|
| `Up/Down` | Navigate (previews file content) |
| `Enter` | Open file in editor / toggle folder |
| `Right` | Open file in editor |
| `/` | Fuzzy filter |
| `Ctrl+D` | Delete file |
| `Ctrl+F` | Create folder |
```

Add a `Ctrl+E` row directly above `Ctrl+D`:

```markdown
| `Ctrl+E` | Rename file or folder |
| `Ctrl+D` | Delete file |
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: document Ctrl+E rename shortcut in README"
```

---

## Self-Review Checklist (verified during plan writing)

- **Spec coverage:** every spec section has a task — enum + state (Task 1), open flow (Task 3), submit flow (Task 2 logic + Task 4 handler), cancel flow (Task 4), status bar (Task 4), tree key handling (Task 3), tests (Task 2), README (Task 5).
- **Placeholder scan:** no TBD/TODO; all code is concrete.
- **Type consistency:** `doRename(name string) error`, `renameInput textinput.Model`, `renameTarget string`, `renameIsDir bool` — same signatures used everywhere they appear.
