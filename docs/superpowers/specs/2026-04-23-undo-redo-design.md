# Undo / redo in the editor — design

## Goal

Let the user revert the last edit in the editor with **Ctrl+Z** and re-apply it with **Ctrl+Shift+Z** (or **Ctrl+Y**). Edits coalesce into groups so undoing feels like rolling back a phrase or operation rather than a single keystroke.

## Scope

**In scope:** mutations of the `SelectableEditor` buffer — typing, backspace/delete, paste, cut, delete-selection, replace-selection, find & replace apply, plugin/shortcut diff accept.

**Out of scope:** tree operations (delete, rename, move), save, git sync, plugin and shortcut config edits, and anything else outside the editor buffer.

**Activation conditions:** the key combos are only honored when the editor panel is focused, `editorMode == modeEdit`, and `inputMode == inputNone`. In any modal (filter, replace, plugin-diff, etc.) the keys are ignored so they can't accidentally rewind the buffer behind a modal.

## Keybindings

| Key | Action |
|-----|--------|
| Ctrl+Z | Undo |
| Ctrl+Shift+Z | Redo |
| Ctrl+Y | Redo (alias, since some terminals deliver Ctrl+Shift+Z as Ctrl+Z) |

## History model

A new type `editHistory` lives in `undo.go` and is embedded (as field `history`) in `SelectableEditor`.

```go
type snapshot struct {
    content       string
    line, col     int
    selActive     bool
    selAnchorLine int
    selAnchorCol  int
}

type editHistory struct {
    undoStack []snapshot
    redoStack []snapshot
    groupOpen bool   // true while a run of coalesced edits is in progress
    lastKind  editKind // what the last recorded edit was (typing vs. delete vs. op)
}
```

- Both stacks are bounded to **100** entries. When full, the oldest entry is dropped.
- Pushing a new snapshot to `undoStack` clears `redoStack` (standard behavior).
- Snapshots hold full buffer content — not diffs. Notes are small enough that 100 × buffer-size is negligible.

## Edit grouping

The central idea: **a group is a run of same-kind typing with no cursor break.** A group begins when an edit happens in a new context; the `pre-edit` state is pushed once at the start of the group.

### What counts as a group boundary

A new group starts (meaning: push `pre-edit` snapshot before the current edit) when the next edit happens and any of these is true since the last recorded edit:

1. **Cursor movement** — arrow keys, word-nav (Ctrl+Left/Right), Home/End, Shift+movement, mouse click, scroll-driven cursor moves.
2. **Kind transition** — switching between *inserting* (rune input, Enter, Tab) and *deleting* (Backspace, Delete without selection). Each kind gets its own group so "type hello, backspace twice" is two undo steps.
3. **Non-typing op** — paste, cut, delete-selection, replace-selection, find & replace apply, plugin/shortcut diff accept, SelectAll-then-replace. Each is a single standalone group.
4. **External reset** — `openFile` or `startNewNote` clears the history entirely.

### What coalesces into one group

- Consecutive rune input (including spaces, newlines, Tab) with no cursor break — one group.
- Consecutive Backspace/Delete (no selection) with no cursor break — one group.

### When does a group close?

A group is closed implicitly by any of the "new group" triggers above. There is no explicit "commit" step; the next edit decides.

No time-based coalescing — the boundary rules above are deterministic and easier to test.

## Implementation placement

A new file `undo.go` holds:

- `snapshot`, `editHistory`, `editKind` types
- `(*editHistory) push(s snapshot)` — append with cap, clear redoStack
- `(*editHistory) popUndo() (snapshot, bool)` — returns false on empty
- `(*editHistory) popRedo() (snapshot, bool)`
- `(*editHistory) clear()`
- Helpers to classify a `tea.KeyMsg` into `editKindTyping`, `editKindDeleting`, or `editKindNonEdit` / `editKindMovement`

`SelectableEditor` gets:

- Field `history editHistory`
- `(*SelectableEditor) snapshotNow() snapshot` — capture current state
- `(*SelectableEditor) Undo()` / `Redo()` methods that swap stacks
- `(*SelectableEditor) beginGroup(kind editKind)` — called before each mutation; pushes pre-edit snapshot if this edit starts a new group
- `ClearHistory()` — called on file switch

Wiring:

- `HandleKey` classifies each incoming key and calls `beginGroup` with the right kind before delegating to the textarea model. Pure-movement keys call `noteMovement()` so the next edit is treated as a group start.
- `Paste`, `Cut`, `DeleteSelection`, `ReplaceSelection` each call `beginGroup(editKindOp)` with a dedicated "single-op" kind so they each become standalone groups.
- `StartMouseDrag`, `ScrollUp`, `ScrollDown` all call `noteMovement()` — any cursor reposition from mouse or scroll breaks the group.
- `model.handleReplaceWith` (find & replace apply) calls `m.editor.beginGroup(editKindOp)` before `SetValue`.
- `model.handlePluginDiff` (accept path) calls `m.editor.beginGroup(editKindOp)` before `SetValue`.
- `model.openFile` and `model.startNewNote` call `m.editor.ClearHistory()`.

### Undo / redo key handling

In `SelectableEditor.HandleKey`, add cases:

- `ctrl+z` → `e.Undo()`; consumed (do not pass to textarea model).
- `ctrl+shift+z`, `ctrl+y` → `e.Redo()`; consumed.

`Undo()` / `Redo()` restore content via `SetValue`, then `moveTo(line, col)` to place the cursor, and set selection fields from the snapshot. `syncVisualYOffset()` is called after so the viewport follows.

## Dirty state & save interaction

- Undo/redo do not touch `m.cleanContent`. The existing `isDirty()` compares `editor.Value()` to `cleanContent`, so undoing back to the saved state correctly shows clean, and undoing past it shows dirty.
- Save does not interact with history — it continues to be purely an I/O operation plus a `cleanContent` update.
- Auto-save likewise leaves history untouched.

## Edge cases

- **Empty undo/redo stack** — the key is a no-op; no status-bar message (stays quiet, matches most editors).
- **Undo during active selection** — `Undo()` clears the current selection before applying the snapshot, then restores the snapshot's selection state.
- **Undo while a plugin diff is open** — cannot happen; `inputMode != inputNone` blocks the key at the model level.
- **No-op edit** (e.g. Backspace at position 0) — after the textarea update, if the buffer content is unchanged from the snapshot just pushed, the snapshot is popped back off. This keeps the history free of useless entries.
- **Window resize** — not an edit; no history interaction.

## Testing

`undo_test.go` covers:

1. Type, undo → empty. Redo → restored.
2. Type `"hello world"` (single run), undo → empty (confirms coalescing).
3. Type `"a"`, move cursor right, type `"b"`, undo → `"a"`, undo again → empty (confirms movement breaks group).
4. Type `"ab"`, backspace, backspace, undo → `"ab"`; undo again → empty (confirms type/delete kind split).
5. Paste, undo → pre-paste buffer and cursor.
6. Select + cut, undo → pre-cut buffer with original content.
7. Select all + replace, undo → pre-replace buffer.
8. Find & replace (via the model), undo → pre-replace buffer.
9. Stack cap: push 150 entries, undo 100 times succeeds, 101st is a no-op.
10. Redo stack clears: type, undo, type, redo is a no-op.
11. `ClearHistory` on file switch: undo after `openFile` is a no-op.
12. Cursor restoration: edits in the middle of a buffer, undo restores cursor to pre-edit position.

## Docs

- Add `Ctrl+Z` and `Ctrl+Shift+Z` rows to the Editor section of `helpSections()` in `help.go`.
- Add matching rows to the Editor table in `README.md`.
