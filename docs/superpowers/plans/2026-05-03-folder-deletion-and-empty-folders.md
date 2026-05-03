# Folder deletion + empty folders — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render empty folders in the file tree and let `Ctrl+D` delete folders recursively, with confirmation, sane cursor placement, an unsaved-edits guard when the open file lives inside the target folder, and removal of the obsolete `untitled.md` placeholder workaround in `handleNewFolder`.

**Architecture:** Two narrow code changes plus three small additions.
1. Drop the `hasMarkdownFiles` filter in `populateChildren` so empty folders flow through naturally; remove the `untitled.md` workaround that compensated for it.
2. Generalise the existing file-delete flow (`Ctrl+D` → `inputConfirmDelete` → `handleDeleteConfirm`) to also handle directories: branch on `node.IsDir`, swap `os.Remove` for `os.RemoveAll`, generalise the "open file is gone" cleanup to a path-prefix match, render a count-aware prompt, and place the cursor on the next sibling — or the parent row if the folder was the last child.
3. Reuse the `inputUnsavedGuard` / `pendingAction` idiom (the same pattern `Ctrl+Q` uses) to gate folder deletes when the open file is dirty and inside the target folder.

Three small helpers carry the new logic and get unit tests of their own: `countTreeContents` (file IO walk), `(*TreePanel).indexOfPath`, and `(*TreePanel).hasFollowingSiblingAtSameDepth`. Two new model fields (`deleteCount`, `deleteTarget`) and one new `pendingActionType` value (`pendingDelete`) carry state between the key press, the optional unsaved-guard detour, and the confirmation render.

**Tech Stack:** Go, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`. Tests use the standard `testing` package and the existing `newTestModel(t)` helper from `shortcuts_input_test.go`.

**Spec:** `docs/superpowers/specs/2026-05-03-folder-deletion-and-empty-folders-design.md` (commit `ef58219`).

---

## File Map

- **Modify:** `filetree_item.go` — drop the `hasMarkdownFiles` gate inside `populateChildren`; delete the now-unused `hasMarkdownFiles` function.
- **Modify:** `filetree_item_test.go` — add empty-folder rendering tests; existing tests updated where the previous filter changed expected counts.
- **Modify:** `model.go` — new `pendingDelete` enum value; new model fields `deleteCount struct{ files, folders int }` and `deleteTarget string`; remove placeholder write in `handleNewFolder`; relax `Ctrl+D` to allow folders, capture state, compute counts; extend `handleDeleteConfirm` (recursive remove, generalised "current file" cleanup, cursor placement); branch the status-bar prompt on `node.IsDir`; route `pendingDelete` through `executePendingAction`.
- **Modify:** `tree.go` — add `(*TreePanel).indexOfPath` and `(*TreePanel).hasFollowingSiblingAtSameDepth` helpers.
- **Modify:** `tree_test.go` — unit tests for the two new `TreePanel` helpers.
- **Create:** `delete_test.go` — integration tests covering empty/non-empty folder delete, currently-open-file cleanup, last-child cursor placement, and the unsaved-edits guard.
- **Modify:** `help_modal.go` — keybinding label update.
- **Modify:** `README.md` — keybinding table update.

---

## Phase 0 — Empty folders render

### Task 1: Empty folders flow through `populateChildren`

**Files:**
- Modify: `filetree_item.go`
- Modify: `filetree_item_test.go`

- [ ] **Step 1: Add the failing tests**

Append to `filetree_item_test.go`:

```go
func TestBuildTree_EmptyFolderRenders(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}

	var found bool
	for _, child := range root.Children {
		if child.Name == "empty" {
			found = true
			if !child.IsDir {
				t.Errorf("empty entry IsDir = false, want true")
			}
			if len(child.Children) != 0 {
				t.Errorf("empty.Children len = %d, want 0", len(child.Children))
			}
		}
	}
	if !found {
		names := make([]string, len(root.Children))
		for i, c := range root.Children {
			names[i] = c.Name
		}
		t.Errorf("empty folder not in tree; got %v", names)
	}
}

func TestBuildTree_AllEmptyFoldersStillRender(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}
	if len(root.Children) != 3 {
		names := make([]string, len(root.Children))
		for i, c := range root.Children {
			names[i] = c.Name
		}
		t.Errorf("got %d children %v, want 3", len(root.Children), names)
	}
}

func TestBuildTree_HiddenDirStillExcluded_EmptyOrNot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}
	for _, child := range root.Children {
		if child.Name == ".hidden" {
			t.Error(".hidden directory should be excluded even when empty")
		}
	}
}
```

- [ ] **Step 2: Run the new tests, confirm they fail**

Run: `go test -run 'TestBuildTree_EmptyFolderRenders|TestBuildTree_AllEmptyFoldersStillRender|TestBuildTree_HiddenDirStillExcluded_EmptyOrNot' -v`
Expected: the first two FAIL (filter excludes empty dirs); the hidden-dir test PASSES (filter happens to also drop it via `hasMarkdownFiles` returning false, but the dotfile guard handles it cleanly post-change too).

- [ ] **Step 3: Drop the filter in `populateChildren` and delete `hasMarkdownFiles`**

Edit `filetree_item.go`. Replace the body of the `entry.IsDir()` branch in `populateChildren` (currently around lines 52–63) with:

```go
		if entry.IsDir() {
			child := &TreeNode{
				Name:  name,
				Path:  childPath,
				IsDir: true,
			}
			if err := populateChildren(child); err != nil {
				continue
			}
			dirs = append(dirs, child)
		} else if strings.HasSuffix(strings.ToLower(name), ".md") {
```

Then delete the `hasMarkdownFiles` function (currently lines 83–93). The full updated `populateChildren` reads:

```go
func populateChildren(node *TreeNode) error {
	entries, err := os.ReadDir(node.Path)
	if err != nil {
		return err
	}

	var dirs, files []*TreeNode
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		childPath := filepath.Join(node.Path, name)
		if entry.IsDir() {
			child := &TreeNode{
				Name:  name,
				Path:  childPath,
				IsDir: true,
			}
			if err := populateChildren(child); err != nil {
				continue
			}
			dirs = append(dirs, child)
		} else if strings.HasSuffix(strings.ToLower(name), ".md") {
			files = append(files, &TreeNode{
				Name: name,
				Path: childPath,
			})
		}
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	node.Children = append(dirs, files...)
	return nil
}
```

- [ ] **Step 4: Run the full filetree_item_test.go suite**

Run: `go test -run 'TestBuildTree|TestFlattenTree|TestCollectFiles' -v`
Expected: all PASS. The pre-existing `TestBuildTree`, `TestBuildTree_SortOrder`, `TestBuildTree_HiddenExcluded`, `TestFlattenTree`, and `TestCollectFiles` should still pass — the test vault built by `setupTestVault` has no empty dirs, so the change does not alter their expected counts (still 3 root children, still 7 flattened, still 4 files).

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS. If anything else fails (e.g., a model-level test that depended on a folder being filtered out), follow the failure to its root cause; do not patch around it.

- [ ] **Step 6: Commit**

```bash
git add filetree_item.go filetree_item_test.go
git commit -m "feat(tree): render empty folders in the file tree"
```

---

### Task 2: Stop seeding `untitled.md` in `handleNewFolder`

**Files:**
- Modify: `model.go`
- Modify: `filetree_item_test.go` (small integration assertion via the model)

The placeholder write in `handleNewFolder` exists only to defeat the filter we just dropped. With Task 1 in place, fresh folders show up natively, and the placeholder is just clutter.

- [ ] **Step 1: Write the failing test**

Append to `filetree_item_test.go`. (Imports `testing`, `os`, `path/filepath`, `strings` — already present except `strings`; add it if missing.)

```go
func TestHandleNewFolder_DoesNotSeedUntitledMd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	m := newModel(vault, nil, "", "")
	m.inputMode = inputNewFolder
	m.newFolderInput.SetValue("research")

	next, _ := m.handleNewFolder(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}

	folder := filepath.Join(vault, "research")
	st, err := os.Stat(folder)
	if err != nil || !st.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("new folder is not empty: %v", names)
	}
}
```

If `tea` and `strings` are not yet imported in `filetree_item_test.go`, add:

```go
import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)
```

(`strings` is not actually needed by this test; only add it if a later test in this file uses it.)

- [ ] **Step 2: Run the test, confirm it fails**

Run: `go test -run TestHandleNewFolder_DoesNotSeedUntitledMd -v`
Expected: FAIL — the new folder contains `untitled.md`.

- [ ] **Step 3: Remove the placeholder write**

Edit `model.go`. In `handleNewFolder` (currently lines 1293–1336), delete these two lines (currently 1316–1317):

```go
		// Create a placeholder note so the folder shows in the tree
		os.WriteFile(filepath.Join(folderPath, "untitled.md"), []byte(""), 0o644)
```

- [ ] **Step 4: Run the test, confirm it passes**

Run: `go test -run TestHandleNewFolder_DoesNotSeedUntitledMd -v`
Expected: PASS.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add model.go filetree_item_test.go
git commit -m "refactor(tree): drop untitled.md placeholder in handleNewFolder"
```

---

## Phase 1 — Helpers

These two tasks add small pure helpers used by the delete flow. Each has its own unit tests and lands as its own commit.

### Task 3: `countTreeContents` helper

**Files:**
- Create: `delete.go` — new file holding helpers and (in later tasks) `setCursorAfterDelete`. Starts here with `countTreeContents` only.
- Create: `delete_test.go` — accompanying tests.

- [ ] **Step 1: Write the failing tests**

Create `delete_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountTreeContents_Empty(t *testing.T) {
	dir := t.TempDir()

	files, folders, err := countTreeContents(dir)
	if err != nil {
		t.Fatalf("countTreeContents: %v", err)
	}
	if files != 0 || folders != 0 {
		t.Errorf("got (files=%d, folders=%d), want (0, 0)", files, folders)
	}
}

func TestCountTreeContents_FilesAndFolders(t *testing.T) {
	dir := t.TempDir()
	// dir/
	//   a.md
	//   b.txt          ← still counted (count is recursive over the FS, not filtered)
	//   sub/
	//     c.md
	//     deep/
	//       d.md
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "c.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "deep", "d.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, folders, err := countTreeContents(dir)
	if err != nil {
		t.Fatalf("countTreeContents: %v", err)
	}
	if files != 4 {
		t.Errorf("files = %d, want 4", files)
	}
	if folders != 2 {
		t.Errorf("folders = %d, want 2", folders)
	}
}

func TestCountTreeContents_ExcludesRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "only"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, folders, err := countTreeContents(dir)
	if err != nil {
		t.Fatalf("countTreeContents: %v", err)
	}
	if folders != 1 {
		t.Errorf("folders = %d, want 1 (root must be excluded)", folders)
	}
}

func TestCountTreeContents_MissingPathReturnsError(t *testing.T) {
	_, _, err := countTreeContents(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run TestCountTreeContents -v`
Expected: FAIL — `countTreeContents` is undefined.

- [ ] **Step 3: Implement `countTreeContents`**

Create `delete.go`:

```go
package main

import (
	"io/fs"
	"path/filepath"
)

// countTreeContents walks root and returns the number of files and the number
// of subdirectories beneath it. The root directory itself is not counted. Both
// markdown and non-markdown files are counted because the count reflects what
// os.RemoveAll will actually delete from the filesystem, not what the tree
// view renders.
func countTreeContents(root string) (files, folders int, err error) {
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			folders++
		} else {
			files++
		}
		return nil
	})
	return files, folders, walkErr
}
```

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run TestCountTreeContents -v`
Expected: PASS.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add delete.go delete_test.go
git commit -m "feat(delete): add countTreeContents helper"
```

---

### Task 4: `TreePanel` cursor helpers

**Files:**
- Modify: `tree.go`
- Modify: `tree_test.go`

These two helpers do exactly one thing each and exist to keep the cursor-placement logic in `handleDeleteConfirm` short and testable.

- [ ] **Step 1: Write the failing tests**

Append to `tree_test.go`:

```go
// helper: build a TreePanel from a list of (path, depth, isDir) triples.
func buildPanel(items []struct {
	Path  string
	Depth int
	IsDir bool
}) TreePanel {
	tp := TreePanel{height: 100, width: 40}
	tp.items = make([]FlatItem, len(items))
	for i, it := range items {
		tp.items[i] = FlatItem{
			Node:  &TreeNode{Path: it.Path, Name: it.Path, IsDir: it.IsDir},
			Depth: it.Depth,
		}
	}
	return tp
}

func TestTreePanel_IndexOfPath(t *testing.T) {
	tp := buildPanel([]struct {
		Path  string
		Depth int
		IsDir bool
	}{
		{"/v/a", 0, true},
		{"/v/a/x.md", 1, false},
		{"/v/b.md", 0, false},
	})

	if got := tp.indexOfPath("/v/a/x.md"); got != 1 {
		t.Errorf("indexOfPath = %d, want 1", got)
	}
	if got := tp.indexOfPath("/v/missing"); got != -1 {
		t.Errorf("indexOfPath missing = %d, want -1", got)
	}
}

func TestTreePanel_HasFollowingSiblingAtSameDepth(t *testing.T) {
	// Tree (flat view, expanded):
	//   0: foo/      depth=0
	//   1:   a/      depth=1
	//   2:     n.md  depth=2
	//   3:   b/      depth=1   ← follows a/ at same depth
	//   4: bar/      depth=0   ← follows foo/ at same depth
	tp := buildPanel([]struct {
		Path  string
		Depth int
		IsDir bool
	}{
		{"/v/foo", 0, true},
		{"/v/foo/a", 1, true},
		{"/v/foo/a/n.md", 2, false},
		{"/v/foo/b", 1, true},
		{"/v/bar", 0, true},
	})

	if !tp.hasFollowingSiblingAtSameDepth(0) {
		t.Error("foo/ should have a following sibling (bar/)")
	}
	if !tp.hasFollowingSiblingAtSameDepth(1) {
		t.Error("a/ should have a following sibling (b/) at depth=1")
	}
	if tp.hasFollowingSiblingAtSameDepth(3) {
		t.Error("b/ has no same-depth follower before depth drops below 1")
	}
	if tp.hasFollowingSiblingAtSameDepth(4) {
		t.Error("bar/ is the last item; no follower")
	}

	// Out-of-range and negative indices behave safely (return false).
	if tp.hasFollowingSiblingAtSameDepth(-1) {
		t.Error("negative idx must return false")
	}
	if tp.hasFollowingSiblingAtSameDepth(99) {
		t.Error("oob idx must return false")
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run 'TestTreePanel_IndexOfPath|TestTreePanel_HasFollowingSiblingAtSameDepth' -v`
Expected: FAIL — neither helper exists.

- [ ] **Step 3: Implement the helpers**

Append to `tree.go`:

```go
// indexOfPath returns the position of the flat item whose Node.Path matches
// path, or -1 if no such item is currently visible.
func (tp *TreePanel) indexOfPath(path string) int {
	for i, item := range tp.items {
		if item.Node.Path == path {
			return i
		}
	}
	return -1
}

// hasFollowingSiblingAtSameDepth reports whether any item after idx has the
// same Depth as items[idx] before encountering an item at a lower depth (which
// would mean the parent's subtree ended). False for out-of-range idx.
func (tp *TreePanel) hasFollowingSiblingAtSameDepth(idx int) bool {
	if idx < 0 || idx >= len(tp.items) {
		return false
	}
	depth := tp.items[idx].Depth
	for j := idx + 1; j < len(tp.items); j++ {
		if tp.items[j].Depth < depth {
			return false
		}
		if tp.items[j].Depth == depth {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the tests, confirm they pass**

Run: `go test -run 'TestTreePanel_IndexOfPath|TestTreePanel_HasFollowingSiblingAtSameDepth' -v`
Expected: PASS.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add tree.go tree_test.go
git commit -m "feat(tree): add indexOfPath and hasFollowingSiblingAtSameDepth helpers"
```

---

## Phase 2 — Delete-flow wiring

### Task 5: Add model state — `pendingDelete`, `deleteCount`, `deleteTarget`

This task adds enum and field declarations only. No behavior changes; no tests yet — the additions get exercised in Tasks 6–9.

**Files:**
- Modify: `model.go`

- [ ] **Step 1: Add `pendingDelete` to the `pendingActionType` enum**

Edit `model.go`. The enum block currently reads (around lines 34–41):

```go
type pendingActionType int

const (
	pendingNone pendingActionType = iota
	pendingSwitchFile
	pendingQuit
	pendingNewNote
)
```

Append `pendingDelete`:

```go
type pendingActionType int

const (
	pendingNone pendingActionType = iota
	pendingSwitchFile
	pendingQuit
	pendingNewNote
	pendingDelete
)
```

- [ ] **Step 2: Add `deleteCount` and `deleteTarget` fields to `model`**

In the `model` struct (around lines 70–185), find the existing `pendingAction` / `pendingSwitchPath` block (currently lines 101–102):

```go
	pendingAction     pendingActionType
	pendingSwitchPath string
```

Replace with:

```go
	pendingAction     pendingActionType
	pendingSwitchPath string
	pendingDeletePath string // node path captured when Ctrl+D detours via inputUnsavedGuard

	deleteCount   deleteCounts // (files, folders) inside the folder being confirmed; zeroed for files
	deleteTarget  string       // node.Path of the item awaiting confirmation; "" when not in inputConfirmDelete
```

Add the supporting type just below the `pendingActionType` const block (right after `pendingDelete`):

```go
type deleteCounts struct {
	files   int
	folders int
}
```

(Putting it adjacent to the enum keeps the small types together and keeps `model` readable.)

- [ ] **Step 3: Verify it builds**

Run: `go build ./...`
Expected: success. The new fields default to their zero values; nothing reads them yet.

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`
Expected: PASS — purely additive.

- [ ] **Step 5: Commit**

```bash
git add model.go
git commit -m "feat(model): scaffold pendingDelete + deleteCount/deleteTarget state"
```

---

### Task 6: `Ctrl+D` accepts folders + status bar shows folder prompt

This task:
1. Drops the `!node.IsDir` check in the `Ctrl+D` case of `handleTreeKeys`.
2. Computes `deleteCount` and stores `deleteTarget` when entering `inputConfirmDelete`.
3. Updates the status-bar render to format folder prompts with the count (or simpler text when empty).

It does **not** yet wire the unsaved-edits guard or `os.RemoveAll`. The existing `handleDeleteConfirm` continues to use `os.Remove`, which means folder confirms still fail at this point — that's fine; the next task fixes it. The reason for slicing this way is that the prompt UX is its own change worth landing as its own commit.

**Files:**
- Modify: `model.go`
- Modify: `delete_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `delete_test.go`:

```go
import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)
```

(If `delete_test.go` already imports `tea` from a previous task, fold these in rather than re-declaring.)

```go
func TestCtrlD_OnEmptyFolder_EntersConfirmWithZeroCounts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, "research"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	// Cursor on "research".
	idx := m.tree.indexOfPath(filepath.Join(vault, "research"))
	if idx < 0 {
		t.Fatalf("research not in tree items")
	}
	m.tree.cursor = idx

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := next.(model)

	if nm.inputMode != inputConfirmDelete {
		t.Fatalf("inputMode = %v, want inputConfirmDelete", nm.inputMode)
	}
	if nm.deleteTarget != filepath.Join(vault, "research") {
		t.Errorf("deleteTarget = %q, want %q", nm.deleteTarget, filepath.Join(vault, "research"))
	}
	if nm.deleteCount.files != 0 || nm.deleteCount.folders != 0 {
		t.Errorf("deleteCount = %+v, want zero", nm.deleteCount)
	}
}

func TestCtrlD_OnNonEmptyFolder_CountsContents(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	if err := os.MkdirAll(filepath.Join(target, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "sub", "b.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	idx := m.tree.indexOfPath(target)
	if idx < 0 {
		t.Fatalf("proj not in tree items")
	}
	m.tree.cursor = idx

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := next.(model)

	if nm.inputMode != inputConfirmDelete {
		t.Fatalf("inputMode = %v, want inputConfirmDelete", nm.inputMode)
	}
	if nm.deleteCount.files != 2 {
		t.Errorf("deleteCount.files = %d, want 2", nm.deleteCount.files)
	}
	if nm.deleteCount.folders != 1 {
		t.Errorf("deleteCount.folders = %d, want 1", nm.deleteCount.folders)
	}
}

func TestCtrlD_OnAddNoteRow_NoOp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = -1

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.deleteTarget != "" {
		t.Errorf("deleteTarget = %q, want empty", nm.deleteTarget)
	}
}

func TestStatusBarPrompt_FolderEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, "research"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.width = 80
	m.height = 24
	idx := m.tree.indexOfPath(filepath.Join(vault, "research"))
	m.tree.cursor = idx

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	out := next.(model).View()
	if !strings.Contains(out, `Delete folder "research"? (y/n)`) {
		t.Errorf("View() missing empty-folder prompt; got:\n%s", out)
	}
}

func TestStatusBarPrompt_FolderNonEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	if err := os.MkdirAll(filepath.Join(target, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(target, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(target, "sub", "b.md"), []byte("x"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.width = 80
	m.height = 24
	m.tree.cursor = m.tree.indexOfPath(target)

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	out := next.(model).View()
	if !strings.Contains(out, `Delete folder "proj" (2 files, 1 folders)? (y/n)`) {
		t.Errorf("View() missing non-empty folder prompt; got:\n%s", out)
	}
}

func TestStatusBarPrompt_FileUnchanged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	file := filepath.Join(vault, "note.md")
	os.WriteFile(file, []byte("hi"), 0o644)
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.width = 80
	m.height = 24
	m.tree.cursor = m.tree.indexOfPath(file)

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	out := next.(model).View()
	if !strings.Contains(out, "Delete note.md? (y/n)") {
		t.Errorf("View() missing file prompt; got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run 'TestCtrlD_|TestStatusBarPrompt_' -v`
Expected: FAIL — `Ctrl+D` on a folder is currently a no-op (`!node.IsDir` guard) and `deleteCount`/`deleteTarget` are never populated.

- [ ] **Step 3: Update the `Ctrl+D` case in `handleTreeKeys`**

Edit `model.go`. The current `Ctrl+D` block (lines 887–891) reads:

```go
	case "ctrl+d":
		node := m.tree.selectedNode()
		if node != nil && !node.IsDir {
			m.inputMode = inputConfirmDelete
		}
```

Replace with:

```go
	case "ctrl+d":
		node := m.tree.selectedNode()
		if node == nil {
			return m, nil
		}
		m.deleteTarget = node.Path
		m.deleteCount = deleteCounts{}
		if node.IsDir {
			files, folders, err := countTreeContents(node.Path)
			if err != nil {
				m.errMsg = fmt.Sprintf("Delete failed: %v", err)
				m.deleteTarget = ""
				return m, nil
			}
			m.deleteCount = deleteCounts{files: files, folders: folders}
		}
		m.inputMode = inputConfirmDelete
		return m, nil
```

Note: Task 9 will splice the unsaved-changes guard in just before the final `m.inputMode = inputConfirmDelete`. We're not adding it now to keep this commit narrow.

- [ ] **Step 4: Update the status-bar render**

In `(model).View()` (`model.go`, around lines 1986–1993), replace:

```go
	} else if m.inputMode == inputConfirmDelete {
		node := m.tree.selectedNode()
		name := ""
		if node != nil {
			name = node.Name
		}
		statusView = statusBarStyle.Width(m.width).Render(
			fmt.Sprintf("Delete %s? (y/n)", name))
	}
```

with:

```go
	} else if m.inputMode == inputConfirmDelete {
		node := m.tree.selectedNode()
		name := ""
		isDir := false
		if node != nil {
			name = node.Name
			isDir = node.IsDir
		}
		var prompt string
		switch {
		case !isDir:
			prompt = fmt.Sprintf("Delete %s? (y/n)", name)
		case m.deleteCount.files == 0 && m.deleteCount.folders == 0:
			prompt = fmt.Sprintf("Delete folder %q? (y/n)", name)
		default:
			prompt = fmt.Sprintf("Delete folder %q (%d files, %d folders)? (y/n)",
				name, m.deleteCount.files, m.deleteCount.folders)
		}
		statusView = statusBarStyle.Width(m.width).Render(prompt)
	}
```

`%q` produces the double-quoted form `"name"` to match the prompt copy in the spec (e.g. `Delete folder "research"? (y/n)`).

- [ ] **Step 5: Run the new tests, confirm they pass**

Run: `go test -run 'TestCtrlD_|TestStatusBarPrompt_' -v`
Expected: PASS.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add model.go delete_test.go
git commit -m "feat(delete): Ctrl+D folder prompt with content counts"
```

---

### Task 7: Recursive folder removal in `handleDeleteConfirm`

**Files:**
- Modify: `model.go`
- Modify: `delete_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `delete_test.go`:

```go
func TestHandleDeleteConfirm_FolderRemovesRecursively(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	if err := os.MkdirAll(filepath.Join(target, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(target, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(target, "sub", "b.md"), []byte("x"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(target)
	m.inputMode = inputConfirmDelete
	m.deleteTarget = target

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err = %v", target, err)
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.deleteTarget != "" {
		t.Errorf("deleteTarget = %q, want empty", nm.deleteTarget)
	}
}

func TestHandleDeleteConfirm_FolderClearsOpenFileInside(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	open := filepath.Join(target, "a.md")
	os.MkdirAll(target, 0o755)
	os.WriteFile(open, []byte("hello"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	// Open the file as the editor would.
	m.openFile(open)
	if m.currentFile != open {
		t.Fatalf("openFile didn't take")
	}
	m.tree.cursor = m.tree.indexOfPath(target)
	m.inputMode = inputConfirmDelete
	m.deleteTarget = target

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if nm.currentFile != "" {
		t.Errorf("currentFile = %q, want empty", nm.currentFile)
	}
	if nm.editor.Value() != "" {
		t.Errorf("editor value = %q, want empty", nm.editor.Value())
	}
	if nm.cleanContent != "" {
		t.Errorf("cleanContent = %q, want empty", nm.cleanContent)
	}
	if nm.tree.currentFile != "" {
		t.Errorf("tree.currentFile = %q, want empty", nm.tree.currentFile)
	}
}

func TestHandleDeleteConfirm_FileStillWorks(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	file := filepath.Join(vault, "note.md")
	os.WriteFile(file, []byte("hi"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(file)
	m.inputMode = inputConfirmDelete
	m.deleteTarget = file

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err = %v", file, err)
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
}

func TestHandleDeleteConfirm_NCancels(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	os.MkdirAll(target, 0o755)
	os.WriteFile(filepath.Join(target, "a.md"), []byte("x"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(target)
	m.inputMode = inputConfirmDelete
	m.deleteTarget = target

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	nm := next.(model)

	if _, err := os.Stat(target); err != nil {
		t.Errorf("folder unexpectedly removed on 'n': %v", err)
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.deleteTarget != "" {
		t.Errorf("deleteTarget = %q, want empty", nm.deleteTarget)
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run 'TestHandleDeleteConfirm_' -v`
Expected: FAIL — current `handleDeleteConfirm` calls `os.Remove` on a folder (returns "directory not empty" error or similar) and doesn't generalise the open-file cleanup.

- [ ] **Step 3: Rewrite `handleDeleteConfirm`**

Edit `model.go`. The current handler (lines 1270–1291) reads:

```go
func (m model) handleDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		node := m.tree.selectedNode()
		if node != nil {
			if err := os.Remove(node.Path); err != nil {
				m.errMsg = fmt.Sprintf("Delete failed: %v", err)
			} else {
				if m.currentFile == node.Path {
					m.currentFile = ""
					m.editor.SetValue("")
					m.cleanContent = ""
				}
				m.refreshTree()
			}
		}
		m.inputMode = inputNone
	case "n", "esc", "ctrl+c":
		m.inputMode = inputNone
	}
	return m, nil
}
```

Replace with:

```go
func (m model) handleDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		node := m.tree.selectedNode()
		if node != nil {
			var err error
			if node.IsDir {
				err = os.RemoveAll(node.Path)
			} else {
				err = os.Remove(node.Path)
			}
			if err != nil {
				m.errMsg = fmt.Sprintf("Delete failed: %v", err)
			} else {
				if m.currentFile != "" && pathIsInside(m.currentFile, node.Path) {
					m.currentFile = ""
					m.editor.SetValue("")
					m.cleanContent = ""
					m.tree.currentFile = ""
				}
				m.refreshTree()
			}
		}
		m.inputMode = inputNone
		m.deleteTarget = ""
		m.deleteCount = deleteCounts{}
	case "n", "esc", "ctrl+c":
		m.inputMode = inputNone
		m.deleteTarget = ""
		m.deleteCount = deleteCounts{}
	}
	return m, nil
}
```

- [ ] **Step 4: Add the `pathIsInside` helper**

Append to `delete.go`:

```go
// pathIsInside reports whether path is equal to root or located somewhere
// beneath it. Both arguments must already be cleaned absolute paths in the
// same form os.ReadDir / filepath.Join produce, which is the case throughout
// the model (vault, node.Path, currentFile are all built from the vault root
// by filepath.Join).
func pathIsInside(path, root string) bool {
	if path == root {
		return true
	}
	prefix := root + string(os.PathSeparator)
	return strings.HasPrefix(path, prefix)
}
```

If `delete.go` does not yet import `os` and `strings`, add them:

```go
import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)
```

- [ ] **Step 5: Add a unit test for `pathIsInside`**

Append to `delete_test.go`:

```go
func TestPathIsInside(t *testing.T) {
	cases := []struct {
		path string
		root string
		want bool
	}{
		{"/v/a", "/v/a", true},
		{"/v/a/x.md", "/v/a", true},
		{"/v/a/sub/x.md", "/v/a", true},
		{"/v/ab", "/v/a", false},          // prefix overlap, not inside
		{"/v/a", "/v/a/sub", false},       // child cannot contain parent
		{"/v/b", "/v/a", false},
	}
	for _, tc := range cases {
		if got := pathIsInside(tc.path, tc.root); got != tc.want {
			t.Errorf("pathIsInside(%q, %q) = %v, want %v", tc.path, tc.root, got, tc.want)
		}
	}
}
```

(The cases use forward-slash literals; on Windows `string(os.PathSeparator)` differs, but clipad targets Linux per its release workflow, so `/`-only test data is fine.)

- [ ] **Step 6: Run the new tests, confirm they pass**

Run: `go test -run 'TestHandleDeleteConfirm_|TestPathIsInside' -v`
Expected: PASS.

- [ ] **Step 7: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add model.go delete.go delete_test.go
git commit -m "feat(delete): recursive folder removal + clear open file inside"
```

---

### Task 8: Cursor placement after delete

**Files:**
- Modify: `model.go`
- Modify: `delete_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `delete_test.go`:

```go
func TestHandleDeleteConfirm_CursorOnNextSibling(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	// vault/
	//   a/  ← delete this
	//   b/  ← cursor should land here
	//     keep.md
	os.MkdirAll(filepath.Join(vault, "a"), 0o755)
	os.MkdirAll(filepath.Join(vault, "b"), 0o755)
	os.WriteFile(filepath.Join(vault, "b", "keep.md"), []byte("x"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(filepath.Join(vault, "a"))
	m.inputMode = inputConfirmDelete
	m.deleteTarget = filepath.Join(vault, "a")

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if got := nm.tree.selectedNode(); got == nil || got.Path != filepath.Join(vault, "b") {
		var p string
		if got != nil {
			p = got.Path
		}
		t.Errorf("cursor on %q, want %q", p, filepath.Join(vault, "b"))
	}
}

func TestHandleDeleteConfirm_LastChildLandsOnParent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	// vault/
	//   foo/        ← parent (expanded)
	//     a/        ← delete this; foo's only child
	os.MkdirAll(filepath.Join(vault, "foo", "a"), 0o755)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	// Expand foo.
	fooNode := findNodeByPath(m.treeRoot, filepath.Join(vault, "foo"))
	if fooNode == nil {
		t.Fatal("foo not in tree")
	}
	fooNode.Expanded = true
	m.tree.rebuildItems()

	m.tree.cursor = m.tree.indexOfPath(filepath.Join(vault, "foo", "a"))
	m.inputMode = inputConfirmDelete
	m.deleteTarget = filepath.Join(vault, "foo", "a")

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if got := nm.tree.selectedNode(); got == nil || got.Path != filepath.Join(vault, "foo") {
		var p string
		if got != nil {
			p = got.Path
		}
		t.Errorf("cursor on %q, want %q (parent)", p, filepath.Join(vault, "foo"))
	}
}

func TestHandleDeleteConfirm_LastTopLevelLandsOnAddNoteRow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	os.MkdirAll(filepath.Join(vault, "only"), 0o755)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(filepath.Join(vault, "only"))
	m.inputMode = inputConfirmDelete
	m.deleteTarget = filepath.Join(vault, "only")

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if nm.tree.cursor != -1 {
		t.Errorf("cursor = %d, want -1 (Add note row)", nm.tree.cursor)
	}
}

// helper used by the last-child test.
func findNodeByPath(root *TreeNode, path string) *TreeNode {
	if root == nil {
		return nil
	}
	if root.Path == path {
		return root
	}
	for _, c := range root.Children {
		if hit := findNodeByPath(c, path); hit != nil {
			return hit
		}
	}
	return nil
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run 'TestHandleDeleteConfirm_CursorOnNextSibling|TestHandleDeleteConfirm_LastChildLandsOnParent|TestHandleDeleteConfirm_LastTopLevelLandsOnAddNoteRow' -v`
Expected: FAIL — current `handleDeleteConfirm` doesn't reposition the cursor.

- [ ] **Step 3: Capture cursor state before remove and reposition after refresh**

Edit `model.go`. In `handleDeleteConfirm`, replace the `case "y":` body with the version below. The new lines compute `parentPath` and `wasLastChild` before the `os.Remove*` call, and reposition the cursor after `refreshTree()`:

```go
	case "y":
		node := m.tree.selectedNode()
		if node != nil {
			parentPath := filepath.Dir(node.Path)
			wasLastChild := !m.tree.hasFollowingSiblingAtSameDepth(m.tree.cursor)

			var err error
			if node.IsDir {
				err = os.RemoveAll(node.Path)
			} else {
				err = os.Remove(node.Path)
			}
			if err != nil {
				m.errMsg = fmt.Sprintf("Delete failed: %v", err)
			} else {
				if m.currentFile != "" && pathIsInside(m.currentFile, node.Path) {
					m.currentFile = ""
					m.editor.SetValue("")
					m.cleanContent = ""
					m.tree.currentFile = ""
				}
				m.refreshTree()
				m.placeCursorAfterDelete(parentPath, wasLastChild)
			}
		}
		m.inputMode = inputNone
		m.deleteTarget = ""
		m.deleteCount = deleteCounts{}
```

- [ ] **Step 4: Add `placeCursorAfterDelete`**

Append to `model.go` (after `refreshTree` and its helpers, around line 1685 — anywhere in the file is fine, but grouping with other tree-state helpers reads better):

```go
// placeCursorAfterDelete repositions the tree cursor following a delete. If
// the deleted node had a sibling that followed it in the flat view, the
// natural list collapse already lands the cursor on that sibling, so we leave
// it alone. Otherwise we set the cursor to the parent's row, or to -1 (the
// pinned "Add note" row) when the parent is the vault root.
func (m *model) placeCursorAfterDelete(parentPath string, wasLastChild bool) {
	if !wasLastChild {
		m.tree.clampOffset()
		return
	}
	if parentPath == m.vault {
		m.tree.cursor = -1
		m.tree.clampOffset()
		return
	}
	if idx := m.tree.indexOfPath(parentPath); idx >= 0 {
		m.tree.cursor = idx
	}
	m.tree.clampOffset()
}
```

- [ ] **Step 5: Run the new tests, confirm they pass**

Run: `go test -run 'TestHandleDeleteConfirm_CursorOnNextSibling|TestHandleDeleteConfirm_LastChildLandsOnParent|TestHandleDeleteConfirm_LastTopLevelLandsOnAddNoteRow' -v`
Expected: PASS.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add model.go delete_test.go
git commit -m "feat(delete): place cursor on next sibling or parent after delete"
```

---

### Task 9: Unsaved-edits guard for folder-with-dirty-open-file

**Files:**
- Modify: `model.go`
- Modify: `delete_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `delete_test.go`:

```go
func TestCtrlD_DirtyOpenFileInsideTarget_GoesToUnsavedGuard(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	open := filepath.Join(target, "a.md")
	os.MkdirAll(target, 0o755)
	os.WriteFile(open, []byte("hello"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.openFile(open)
	m.editor.SetValue("hello dirty")            // make it dirty
	m.tree.cursor = m.tree.indexOfPath(target)

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := next.(model)

	if nm.inputMode != inputUnsavedGuard {
		t.Fatalf("inputMode = %v, want inputUnsavedGuard", nm.inputMode)
	}
	if nm.pendingAction != pendingDelete {
		t.Errorf("pendingAction = %v, want pendingDelete", nm.pendingAction)
	}
	if nm.pendingDeletePath != target {
		t.Errorf("pendingDeletePath = %q, want %q", nm.pendingDeletePath, target)
	}
}

func TestCtrlD_DirtyOpenFileOutsideTarget_NoGuard(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	other := filepath.Join(vault, "other.md")
	os.MkdirAll(target, 0o755)
	os.WriteFile(other, []byte("hello"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.openFile(other)
	m.editor.SetValue("hello dirty")
	m.tree.cursor = m.tree.indexOfPath(target)

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := next.(model)

	if nm.inputMode != inputConfirmDelete {
		t.Errorf("inputMode = %v, want inputConfirmDelete (no guard for outside dirty file)", nm.inputMode)
	}
}

func TestUnsavedGuard_DiscardProceedsToConfirm(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	open := filepath.Join(target, "a.md")
	os.MkdirAll(target, 0o755)
	os.WriteFile(open, []byte("hello"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.openFile(open)
	m.editor.SetValue("hello dirty")
	m.tree.cursor = m.tree.indexOfPath(target)

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	guarded := next.(model)
	next2, _ := guarded.handleUnsavedGuard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	nm := next2.(model)

	if nm.inputMode != inputConfirmDelete {
		t.Errorf("after discard inputMode = %v, want inputConfirmDelete", nm.inputMode)
	}
	if nm.pendingAction != pendingNone {
		t.Errorf("pendingAction = %v, want pendingNone", nm.pendingAction)
	}
	if nm.deleteTarget != target {
		t.Errorf("deleteTarget = %q, want %q", nm.deleteTarget, target)
	}
}

func TestUnsavedGuard_EscCancelsDelete(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	open := filepath.Join(target, "a.md")
	os.MkdirAll(target, 0o755)
	os.WriteFile(open, []byte("hello"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.openFile(open)
	m.editor.SetValue("hello dirty")
	m.tree.cursor = m.tree.indexOfPath(target)

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	guarded := next.(model)
	next2, _ := guarded.handleUnsavedGuard(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next2.(model)

	if nm.inputMode != inputNone {
		t.Errorf("after esc inputMode = %v, want inputNone", nm.inputMode)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("target unexpectedly removed on esc: %v", err)
	}
}

func TestUnsavedGuard_SavePersistsThenConfirmDeletesEverything(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	open := filepath.Join(target, "a.md")
	os.MkdirAll(target, 0o755)
	os.WriteFile(open, []byte("hello"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.openFile(open)
	m.editor.SetValue("hello dirty")
	m.tree.cursor = m.tree.indexOfPath(target)

	// Ctrl+D → unsaved guard.
	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	// 'y' → save and continue.
	next2, _ := next.(model).handleUnsavedGuard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	confirming := next2.(model)
	if confirming.inputMode != inputConfirmDelete {
		t.Fatalf("after save inputMode = %v, want inputConfirmDelete", confirming.inputMode)
	}
	// Verify the save did happen — file on disk now matches dirty content.
	saved, err := os.ReadFile(open)
	if err != nil {
		t.Fatalf("read open file: %v", err)
	}
	if string(saved) != "hello dirty" {
		t.Errorf("file on disk = %q, want %q", string(saved), "hello dirty")
	}

	// Confirm 'y' → folder gone, editor cleared.
	next3, _ := confirming.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next3.(model)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected target removed, stat err = %v", err)
	}
	if nm.currentFile != "" {
		t.Errorf("currentFile = %q, want empty", nm.currentFile)
	}
}
```

- [ ] **Step 2: Run the tests, confirm they fail**

Run: `go test -run 'TestCtrlD_DirtyOpenFileInsideTarget|TestCtrlD_DirtyOpenFileOutsideTarget|TestUnsavedGuard_' -v`
Expected: FAIL — guard never fires; `pendingDelete` is unhandled.

- [ ] **Step 3: Splice the guard into the `Ctrl+D` handler**

Edit `model.go`. In the `case "ctrl+d":` block (rewritten in Task 6), replace its body with:

```go
	case "ctrl+d":
		node := m.tree.selectedNode()
		if node == nil {
			return m, nil
		}
		if node.IsDir && m.isDirty() && m.currentFile != "" && pathIsInside(m.currentFile, node.Path) {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingDelete
			m.pendingDeletePath = node.Path
			return m, nil
		}
		m.deleteTarget = node.Path
		m.deleteCount = deleteCounts{}
		if node.IsDir {
			files, folders, err := countTreeContents(node.Path)
			if err != nil {
				m.errMsg = fmt.Sprintf("Delete failed: %v", err)
				m.deleteTarget = ""
				return m, nil
			}
			m.deleteCount = deleteCounts{files: files, folders: folders}
		}
		m.inputMode = inputConfirmDelete
		return m, nil
```

- [ ] **Step 4: Route `pendingDelete` through `executePendingAction`**

Edit `model.go`. The current `executePendingAction` (lines 1460–1479) reads:

```go
func (m model) executePendingAction() (tea.Model, tea.Cmd) {
	m.inputMode = inputNone
	switch m.pendingAction {
	case pendingQuit:
		m.pendingAction = pendingNone
		if m.gitSyncRunning {
			m.gitSyncQuitting = true
			return m, nil
		}
		return m, tea.Quit
	case pendingSwitchFile:
		m.openFile(m.pendingSwitchPath)
		m.pendingAction = pendingNone
		m.pendingSwitchPath = ""
	case pendingNewNote:
		m.pendingAction = pendingNone
		m.startNewNote()
	}
	return m, nil
}
```

Append a `pendingDelete` arm and a small wrapper that hands off to the existing confirm flow. Replace the function body with:

```go
func (m model) executePendingAction() (tea.Model, tea.Cmd) {
	m.inputMode = inputNone
	switch m.pendingAction {
	case pendingQuit:
		m.pendingAction = pendingNone
		if m.gitSyncRunning {
			m.gitSyncQuitting = true
			return m, nil
		}
		return m, tea.Quit
	case pendingSwitchFile:
		m.openFile(m.pendingSwitchPath)
		m.pendingAction = pendingNone
		m.pendingSwitchPath = ""
	case pendingNewNote:
		m.pendingAction = pendingNone
		m.startNewNote()
	case pendingDelete:
		path := m.pendingDeletePath
		m.pendingAction = pendingNone
		m.pendingDeletePath = ""
		return m.beginDeleteConfirm(path)
	}
	return m, nil
}
```

Add `beginDeleteConfirm` next to `executePendingAction`. It centralises the count-and-store logic so the `Ctrl+D` direct path and the post-guard path share one implementation.

```go
// beginDeleteConfirm transitions the model into inputConfirmDelete for the
// node at path. Used both directly from Ctrl+D and from the unsaved-edits
// guard handoff.
func (m model) beginDeleteConfirm(path string) (tea.Model, tea.Cmd) {
	idx := m.tree.indexOfPath(path)
	if idx < 0 {
		// Path no longer in the tree (renamed/deleted out from under us); bail.
		return m, nil
	}
	m.tree.cursor = idx
	node := m.tree.items[idx].Node
	m.deleteTarget = path
	m.deleteCount = deleteCounts{}
	if node.IsDir {
		files, folders, err := countTreeContents(path)
		if err != nil {
			m.errMsg = fmt.Sprintf("Delete failed: %v", err)
			m.deleteTarget = ""
			return m, nil
		}
		m.deleteCount = deleteCounts{files: files, folders: folders}
	}
	m.inputMode = inputConfirmDelete
	return m, nil
}
```

`beginDeleteConfirm` is required (the `pendingDelete` arm above calls it). The `Ctrl+D` direct path still inlines the equivalent logic from Task 6 — that's intentional duplication for now to keep this commit narrow. If you want to consolidate, replace the inlined `Ctrl+D` body with `return m.beginDeleteConfirm(node.Path)` after Step 5 confirms tests pass; then rerun `go test ./...`.

- [ ] **Step 5: Run the new tests, confirm they pass**

Run: `go test -run 'TestCtrlD_DirtyOpenFileInsideTarget|TestCtrlD_DirtyOpenFileOutsideTarget|TestUnsavedGuard_' -v`
Expected: PASS.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add model.go delete_test.go
git commit -m "feat(delete): unsaved-edits guard for folders containing the open dirty file"
```

---

## Phase 3 — Documentation

### Task 10: Update keybinding labels

**Files:**
- Modify: `help_modal.go`
- Modify: `README.md`

- [ ] **Step 1: Update the in-app help modal**

Edit `help_modal.go` line 65:

```go
			{"Ctrl+D", "Delete file or folder"},
```

(Was: `{"Ctrl+D", "Delete"}`.)

- [ ] **Step 2: Update the README keybinding table**

Edit `README.md` line 96:

```markdown
| `Ctrl+D` | Delete file or folder |
```

(Was: `| `Ctrl+D` | Delete file |`.)

- [ ] **Step 3: Run the full test suite as a sanity check**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Build the binary**

Run: `go build -o /tmp/clipad-folder-delete .`
Expected: success.

- [ ] **Step 5: Manual smoke test**

Manually verify the new behavior in a scratch vault:

```bash
SCRATCH=$(mktemp -d)
mkdir -p "$SCRATCH/proj/sub"
echo "hi" > "$SCRATCH/proj/note.md"
echo "ok" > "$SCRATCH/top.md"
mkdir "$SCRATCH/empty"
/tmp/clipad-folder-delete  # set vault to $SCRATCH on first-run prompt
```

Inside clipad:
- Confirm `empty/` is visible in the tree without ever putting a `.md` file in it.
- Move cursor to `empty`, press `Ctrl+D` → status bar shows `Delete folder "empty"? (y/n)`. Press `n` — folder remains.
- Move cursor to `proj`, press `Ctrl+D` → `Delete folder "proj" (1 files, 1 folders)? (y/n)`. Press `n`.
- Open `proj/note.md`, edit (don't save), `Esc` to tree, cursor on `proj`, `Ctrl+D` → unsaved-changes guard fires. Press `n` (discard) → confirmation prompt appears. Press `y` → folder gone, editor cleared, cursor on `top.md`.
- Create a new folder with `Ctrl+F` → it appears immediately, contains no files.

If any step misbehaves, stop and debug. Don't paper over it in this commit.

- [ ] **Step 6: Commit**

```bash
git add help_modal.go README.md
git commit -m "docs: Ctrl+D deletes files or folders"
```

---

## Done

At this point:
- Empty folders render in the tree.
- `Ctrl+D` deletes folders recursively with an accurate confirmation prompt.
- The currently-open file is cleaned up when its folder (or any ancestor folder) is deleted.
- Cursor lands on next sibling, or the parent row when the deleted folder was the last child.
- Unsaved edits are protected behind the same guard `Ctrl+Q` uses.
- The `untitled.md` placeholder workaround is gone.
- Help modal and README reflect the new keybinding behavior.

Run `go test ./...` once more for a final green check, then push the branch.
