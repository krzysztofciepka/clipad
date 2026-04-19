# Rename File or Folder Shortcut

## Problem

The file tree supports delete (Ctrl+D) and create-folder (Ctrl+F), but no rename. Users have to drop to a shell to rename a file or folder, which breaks the editor flow and leaves the open file pointing at a stale path.

## Design

### Trigger

- **Ctrl+E** in the tree panel when a file or folder is selected.
- No-op in the editor panel, in filter mode, or in any active input mode.

### State

Add to `model` struct (alongside `newFolderInput`):

- `renameInput textinput.Model` — placeholder `"new name"`, `CharLimit` 256
- `renameTarget string` — full path of the node being renamed, snapshotted when the prompt opens so tree refreshes don't shift the target
- `renameIsDir bool` — true if renaming a folder; controls extension handling

Add `inputRename` to the `inputMode` enum and dispatch it in `handleInputMode`.

### Open flow

When Ctrl+E is pressed in the tree panel:

1. Get the selected node. If none, no-op.
2. Snapshot `renameTarget = node.Path`, `renameIsDir = node.IsDir`.
3. Pre-fill input value:
   - **File:** base name without extension — `strings.TrimSuffix(node.Name, filepath.Ext(node.Name))`.
   - **Folder:** full `node.Name`.
4. `renameInput.CursorEnd()` so the user can append/edit immediately.
5. Set `inputMode = inputRename` and focus the input.

### Submit flow (Enter)

In `handleRename`:

1. `name := strings.TrimSpace(renameInput.Value())`. If empty, keep the prompt open (matches new-folder behavior).
2. Reject if `name` contains `/` or `\` — set `errMsg = "Name cannot contain path separators"` and keep the prompt open. Rename is not move.
3. Compute `target`:
   - **File:** `filepath.Join(filepath.Dir(renameTarget), name + filepath.Ext(renameTarget))` — extension is auto-reappended.
   - **Folder:** `filepath.Join(filepath.Dir(renameTarget), name)`.
4. If `target == renameTarget`, close the prompt as a no-op.
5. If `target` already exists (`os.Stat` returns nil error), set `errMsg = "Already exists: <basename>"` and keep the prompt open.
6. Call `os.Rename(renameTarget, target)`. On error, set `errMsg` and close the prompt.
7. **Open-file pointer fixups:**
   - If `m.currentFile == renameTarget` (file renamed while open), update both `m.currentFile = target` and `m.tree.currentFile = target`.
   - If a folder was renamed and contains the open file (`strings.HasPrefix(m.currentFile, renameTarget + string(os.PathSeparator))`), rewrite the open-file path by replacing the prefix.
8. **Clipboard fixup:** if `m.fileClip.path == renameTarget` or `m.tree.cutPath == renameTarget`, clear them. (For a renamed folder, also clear if the clipboard path lives under the old folder.)
9. Call `refreshTree()` and close the prompt.

### Cancel flow

`Esc` closes the prompt without changes. `Ctrl+Q` follows the same dirty-guard pattern as `handleNewFolder`.

### Status bar

In `View()`, add a branch alongside the other input-mode prompts:

```go
} else if m.inputMode == inputRename {
    statusView = statusBarStyle.Width(m.width).Render(
        "Rename: " + m.renameInput.View())
}
```

### Tree key handling

Add a case to `handleTreeKeys`:

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
```

## Files Changed

- **model.go**: new `inputRename` enum value, `renameInput`/`renameTarget`/`renameIsDir` fields, `handleRename` handler, `handleInputMode` dispatch, `handleTreeKeys` case, `View()` status-bar branch, `newModel` initializes `renameInput`.
- **rename_test.go** (new): unit tests for the rename behavior.
- **README.md**: add `Ctrl+E` row under the **File Tree** section.

## Tests

`rename_test.go` covers:

- File rename preserves the original extension when the user types only the base name.
- Folder rename rewrites `m.currentFile` when the open file lives inside the renamed folder.
- Reject names containing `/` or `\` — prompt stays open, no rename happens.
- Reject when target path already exists — prompt stays open, source untouched.
- Update `currentFile` and `tree.currentFile` when the renamed file is the open file.
- Clear `fileClip` and `tree.cutPath` when the clipboard path matches (or lives under) the renamed path.
- No-op when the typed name produces the same path as the source.

## Out of Scope

- Move semantics (path separators are rejected). Use cut+paste for moves.
- Multi-select rename.
- Undo.
- Rename from the editor panel or the filter view.
