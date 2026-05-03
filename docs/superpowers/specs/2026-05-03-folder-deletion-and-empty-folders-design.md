# Folder deletion + show empty folders in the tree

Date: 2026-05-03
Source task: `~/Notes/Notes/Tasks Completed/Task 27.md`

## Problem

Two related gaps in the file tree:

1. **Empty folders are hidden.** `populateChildren` in `filetree_item.go` filters directories through `hasMarkdownFiles` before adding them, so a folder created with `Ctrl+F` disappears until a `.md` file lands inside. The current workaround in `handleNewFolder` (`model.go:1317`) writes a stub `untitled.md` into every new folder, leaving phantom files behind.
2. **`Ctrl+D` only deletes files.** Folders cannot be deleted from the TUI. Users have to drop to the shell.

## Goals

- Empty folders render in the tree like any other directory. Dot-prefixed directories stay hidden.
- `Ctrl+D` deletes folders recursively, with confirmation and cursor-handling that matches the rest of Clipad's tree UX.
- The currently-open file is closed cleanly if it lives inside the deleted folder. Unsaved edits go through the same guard `Ctrl+Q` uses.
- The placeholder-file workaround in `handleNewFolder` is removed.

## Non-goals

- No trash / undo for deletes. `os.RemoveAll` is final, matching the existing file-delete semantics.
- No auto-cleanup of pre-existing `untitled.md` placeholder files. They're indistinguishable from real notes; users handle their own.
- No multi-select delete.
- No new keybinding. `Ctrl+D` is reused for both files and folders.

## Design

### 1. Empty folders render

`populateChildren` in `filetree_item.go` always appends directories after recursion. The `hasMarkdownFiles` gate (line 61) is removed, and the now-unused function (lines 83–93) is deleted along with it. Dot-prefixed directories continue to be skipped at line 48.

`handleNewFolder` in `model.go` no longer writes the placeholder `untitled.md` (line 1317 is removed). Folders are created with `os.MkdirAll` and refresh the tree directly.

### 2. `Ctrl+D` on a folder

**Key handler** (`model.go:887–891`): drop the `!node.IsDir` check. Files and folders both transition to `inputConfirmDelete`.

**Unsaved-changes guard.** When the user presses `Ctrl+D` on a folder, before transitioning to `inputConfirmDelete` we check whether the currently-open file is dirty AND whether it lives inside the target folder (or *is* the target file). If both are true, the model enters `inputUnsavedGuard` with a new `pendingDelete` action and stores `m.deleteTarget = node.Path`. The existing save/discard/cancel flow plays out first; the handler that resolves `inputUnsavedGuard` checks `m.pendingAction == pendingDelete` and, on save or discard, transitions to `inputConfirmDelete`. Cancel/Esc returns to the tree without deleting. This mirrors the `Ctrl+Q` → `pendingQuit` idiom (`model.go:1325–1331`).

The "currentFile lives inside deleted folder" check is `m.currentFile == node.Path || strings.HasPrefix(m.currentFile, node.Path + string(os.PathSeparator))`.

If the cursor is on the pinned "+ Add note" row (`m.tree.cursor == -1`), `selectedNode()` returns `nil` and `Ctrl+D` is a no-op — same as today.

**Confirmation prompt** (`model.go:1986–1993`): branches on `node.IsDir`. The (files, folders) count is computed *once* when the model transitions into `inputConfirmDelete` (using `filepath.WalkDir`, excluding the target folder itself) and stored on the model as `m.deleteCount` (a small struct: `{files, folders int}`). The renderer reads from `m.deleteCount` rather than walking on every frame.

Prompt copy:
- empty dir: `Delete folder "name"? (y/n)`
- non-empty dir: `Delete folder "name" (N files, M folders)? (y/n)`
- file (unchanged): `Delete name? (y/n)`

**Confirm handler** (`handleDeleteConfirm`, `model.go:1270`): branches on `node.IsDir`. Folders use `os.RemoveAll(node.Path)`; files keep `os.Remove(node.Path)`. The "current file is gone" cleanup generalises to the same prefix check above — clears `m.currentFile`, `m.editor`, and `m.cleanContent`.

### 3. Cursor placement after delete

Before deleting, capture:
- `parentPath` = `filepath.Dir(node.Path)`
- `wasLastChild` — true if the deleted node has no following sibling at the same flat-view depth before falling back to a lower depth.

After `refreshTree()`:
- If `wasLastChild`, scan `m.tree.items` for the index whose `Node.Path == parentPath` and set the cursor there. If the parent is the vault root (no row exists for the root), set `cursor = -1` (the pinned "Add note" row).
- Otherwise, leave the cursor index alone — natural list collapse lands it on the former next sibling.

`clampOffset()` runs after the assignment.

The same logic applies uniformly to file deletes — files almost always have a successor row, so the `wasLastChild` branch rarely fires for them, and the behaviour is unchanged in practice.

### 4. Documentation

- `help_modal.go:65` — `{"Ctrl+D", "Delete"}` → `{"Ctrl+D", "Delete file or folder"}`
- `README.md:96` — `| Ctrl+D | Delete file |` → `| Ctrl+D | Delete file or folder |`

The placeholder-file removal is mentioned in the GitHub release notes only (no README CHANGELOG section exists today).

## Tests

Added to `filetree_item_test.go`:
- Empty subdirectory renders in the tree.
- Vault where every directory is empty still renders all of them.
- Existing hidden-dir / sort-order / count tests stay as regression guards.

Added to a new `delete_test.go` (or `tree_test.go`):
- Deleting an empty folder via `handleDeleteConfirm` removes it and leaves the cursor on a sane row.
- Deleting a non-empty folder with the currently-open file inside clears the editor buffer and `currentFile`.
- Last-child folder delete lands the cursor on the parent row in the flat view.
- Unsaved-edits guard fires when `Ctrl+D` targets a folder containing the dirty open file; on save it proceeds to confirm; on cancel it returns to the tree without deleting.

## Touch points

- `filetree_item.go` — drop filter, delete `hasMarkdownFiles`.
- `filetree_item_test.go` — empty-folder rendering tests.
- `model.go` — key handler, dirty guard wiring (new `pendingDelete` value), confirmation prompt, `handleDeleteConfirm`, cursor logic, new model fields (`deleteCount {files, folders int}`, optionally `deleteTarget` if needed for the dirty-guard handoff), removal of placeholder-file write in `handleNewFolder`.
- `delete_test.go` (new) or extension of existing tree tests — delete-flow coverage.
- `help_modal.go` — keybinding label.
- `README.md` — keybinding table.

## Out-of-scope clean-ups noted but not done

None.
