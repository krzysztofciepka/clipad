# UI Polish — Design

**Date:** 2026-04-25
**Task:** Task clipad 1

## Problem

Five small UI/UX issues across the file tree, help modal, and AI shortcut selector:

1. Mouse wheel on the file tree only scrolls a "limited range" — the offset can't move past the cursor row, so once the cursor leaves the visible window the scroll stalls. Keyboard navigation works.
2. There's no way to create a new note from the file tree using the mouse — `Ctrl+N` is keyboard-only.
3. The help modal (`Ctrl+/`) clips long content because it has no scrollable viewport.
4. The AI shortcut selector (`Ctrl+G`) doesn't let users reorder shortcuts; today the only way to change order is to delete and recreate.
5. There's no keybinding to hide the file tree to give the editor more horizontal room. Today the tree only auto-hides on extremely narrow terminals.

## Current state

- **Tree scroll:** `TreePanel.clampOffset` (`tree.go:60–90`) keeps the cursor visible after any offset change. Mouse-wheel handlers (`mouse.go:214–225`) call `clampOffset` directly, which snaps `offset` back to the cursor's row whenever they diverge — that's the "limited range" symptom. Keyboard `moveUp`/`moveDown` works because the cursor moves first and `offset` then follows it.
- **File tree rows:** `TreePanel.View` (`tree.go:133–202`) iterates `tp.items` (a flat list of `FlatItem` produced by `flattenTree`). `tp.cursor` is an index into `tp.items`. The narrow-terminal codepath (`model.go:1231–1233`) sets `treeWidth = 0` to hide the tree.
- **Help modal:** `helpView` (`help_modal.go:91–127`) renders all sections into a `lipgloss` block with `MaxHeight(height)`. There's no viewport, so overflow is clipped. `handleHelp` (`model.go:701–713`) only handles close/quit keys.
- **Shortcut selector:** `handleShortcutSelect` (`shortcuts_input.go:8–88`) handles `up`/`down`/`enter`/`p`/`e`/`d`/`esc`/`ctrl+q`. `saveShortcuts` (`shortcuts.go:63–74`) writes the slice to `~/.config/clipad/ai_shortcuts.toml` in array order, and `loadShortcuts` reads it back in the same order — so persistence is just "save the slice after a swap."
- **Layout:** `recalcLayout` (`model.go:1218–1256`) computes `treeWidth` and `editorWidth` each frame; the auto-hide branch already sets `treeWidth = 0` on narrow terminals.

## Scope

In scope:
1. Decouple mouse-wheel scroll from cursor position in the file tree.
2. Add a pinned "Add note" row at the top of the tree that triggers the same flow as `Ctrl+N` when activated by mouse click or Enter.
3. Make the help modal scrollable via a `viewport.Model`.
4. Add `Ctrl+Up`/`Ctrl+Down` reordering in the shortcut selector, persisted to disk on each swap.
5. Add `Ctrl+B` to toggle file tree visibility (per session, not persisted).

Out of scope:
- Drag-and-drop reordering for shortcuts.
- A scroll indicator / scrollbar in the tree or help modal (viewport handles overflow naturally).
- Persisting tree-hidden state across sessions.
- Restructuring `flattenTree` or making "Add note" a real `FlatItem` (kept as a virtual row).
- Touching the editor's existing mouse-scroll behavior (already cursor-decoupled and correct).

## Architecture

No structural moves. Five independent fixes touching different files:

| Concern | Files |
|---|---|
| Tree mouse scroll | `tree.go`, `mouse.go`, `mouse_test.go`, `tree_test.go` |
| Add note row | `tree.go`, `model.go`, `mouse.go`, `tree_test.go`, `mouse_test.go` |
| Help modal viewport | `help_modal.go`, `model.go`, `mouse.go`, `help_modal_test.go`, `mouse_test.go` |
| Shortcut reorder | `shortcuts_input.go`, `shortcuts_modal.go`, `help_modal.go`, `shortcuts_input_test.go` |
| Toggle tree visibility | `model.go`, `help_modal.go`, `README.md`, `model_test.go` |

Each section below describes its part in isolation; they don't interact at runtime.

### 1. Tree mouse scroll (decoupled from cursor)

Split `clampOffset`'s two responsibilities. Today it both (a) clamps `offset` to valid bounds and (b) snaps `offset` so the cursor is visible. Mouse wheel only wants (a).

Shared helper for the items-visible budget (consumed by both `scrollBy` and `clampOffset`):

```go
// itemsHeight returns how many real-item rows fit, accounting for the pinned
// "Add note" row that always consumes one line of tp.height.
func (tp *TreePanel) itemsHeight() int {
    h := tp.height - 1
    if h < 0 {
        h = 0
    }
    return h
}
```

New scroll method:

```go
// scrollBy adjusts offset by delta without touching the cursor and without
// snapping back to keep the cursor visible. The cursor may end up off-screen;
// the next moveUp/moveDown will snap the view back via clampOffset.
func (tp *TreePanel) scrollBy(delta int) {
    h := tp.itemsHeight()
    if h <= 0 || len(tp.items) == 0 {
        tp.offset = 0
        return
    }
    maxOffset := len(tp.items) - h
    if maxOffset < 0 {
        maxOffset = 0
    }
    tp.offset += delta
    if tp.offset < 0 {
        tp.offset = 0
    }
    if tp.offset > maxOffset {
        tp.offset = maxOffset
    }
}
```

Mouse handlers in `mouse.go:214–225` change from `m.tree.offset += 3; m.tree.clampOffset()` to `m.tree.scrollBy(3)` (and `-3` for wheel up).

`clampOffset` is updated in two ways: (a) replace the bare `tp.height` with `tp.itemsHeight()` everywhere it computes the visible budget for items, and (b) skip the cursor-visibility snap when `tp.cursor == -1` (the pinned row is always visible regardless of `offset`). Body otherwise unchanged. It's still called from `recalcLayout`, `moveUp`/`moveDown` (transitively), `rebuildItems` callers, and the click handler in `handleTreeMouse` (which moves the cursor and then needs cursor visibility). It now correctly represents "post-cursor-move re-alignment" and is no longer abused for plain scroll.

### 2. "Add note" pinned row

Treat "Add note" as a virtual row, not a real `FlatItem`. The `tp.items` slice stays files-only; the view layer prepends one row above it.

Cursor sentinel: `tp.cursor == -1` means "Add note row is selected". Otherwise `tp.cursor` is an index into `tp.items` (today's meaning, unchanged).

Changes to `TreePanel`:

- `selectedNode()` returns `nil` when `cursor == -1`. Existing callers already null-check.
- New `(tp TreePanel) onAddNote() bool` returning `tp.cursor == -1`.
- `moveUp`: when `tp.cursor == 0`, set `tp.cursor = -1` and clear `tp.offset = 0` (so the pinned row is visible — though it's always visible regardless). When `tp.cursor == -1`, no-op.
- `moveDown`: when `tp.cursor == -1`, set `tp.cursor = 0` if `len(tp.items) > 0`. Otherwise existing logic.
- `clampOffset`: tolerate `tp.cursor == -1` by leaving it alone (don't clamp it to `[0, len(items)-1]`).
- `toggleOrSelect`: when `cursor == -1`, return a sentinel `addNoteSentinel` (a package-level `*TreeNode` value) so callers can distinguish "user activated Add note" from "user activated a file". Alternative considered: have `toggleOrSelect` return `nil` and require callers to call `onAddNote()` first — rejected because today `nil` means "directory toggled" and conflating the two would make the call site error-prone.

Initial cursor: `newTreePanel` (and any path that resets the cursor) sets `cursor = -1` if `len(items) == 0`, else `0`.

Changes to `View()`:

- Render the pinned row at panel-local line 0 unconditionally (never affected by `tp.offset`):
  ```
  + Add note
  ```
  Style: `Color("240")` (muted) when not focused or not on Add note; `treeSelectedStyle` background applied when `cursor == -1` and `focused`.
- Continue rendering items starting at panel-local row 1, using `tp.itemsHeight()` as the visible-item count.
- Click hit-testing: panel-local `localY == 0` always means the Add note row, independent of `tp.offset`.

Changes to `handleTreeKeys` (`model.go:532–604`):

- `case "enter":` if `m.tree.onAddNote()` → `m.startNewNote()`, switch focus to editor (matches `Ctrl+N`).
- `case "up", "k":` and `case "down", "j":` already call `moveUp`/`moveDown`; `previewSelectedFile` is called after, which today reads `selectedNode()`. With `cursor == -1`, `selectedNode()` returns `nil` and `previewSelectedFile` no-ops (existing safe path).
- `right` key: when `onAddNote()`, no-op (no file to open in editor).
- `ctrl+d` / `ctrl+e`: `selectedNode()` returns nil → no-op (existing behavior for nil).

Changes to `handleTreeMouse` (`mouse.go:193–227`):

- Map `localY == 0` to "Add note row hit". On `MouseButtonLeft + MouseActionPress`, set `m.tree.cursor = -1`, `m.activePanel = treePanel`, run `m.startNewNote()`, return.
- `localY > 0`: subtract 1 from the local row, then run today's logic (`mousePosToTreeRow(m.tree.offset, localY-1)`), so the existing item-row indexing is preserved.

### 3. Help modal viewport

Wrap the help content in a `viewport.Model` owned by the model.

New field on `model`:

```go
helpViewport viewport.Model
```

Refactor `help_modal.go`:

- Extract the section-rendering loop into `helpContent(width int) string` (returns the raw multiline content with the existing styling, no outer `Width`/`MaxHeight`/`Background`).
- Remove `helpView`. The modal now uses `helpContent` + viewport directly. There are no other callers (`helpView` is referenced only at `model.go:1275`, which is rewritten in this change).

Changes around `inputHelp`:

- In `model.go:499–501` (open help), construct `m.helpViewport = viewport.New(m.editorWidth, m.editorHeight)`, `SetContent(helpContent(m.editorWidth))`, then set `inputMode = inputHelp`.
- `recalcLayout` (`model.go:1252–1255`-ish): when `inputMode == inputHelp`, resize `m.helpViewport` to the new editor dimensions and `SetContent` again so wrapping recomputes.
- `handleHelp` (`model.go:701–713`): add cases for `up`/`down`/`pgup`/`pgdown`/`home`/`end` — delegate to `m.helpViewport.Update(msg)`. Keep existing close keys.
- `View()` (`model.go:1274–1275`): when `inputMode == inputHelp`, render `m.helpViewport.View()` inside the existing background-styled wrapper.
- Mouse wheel routing: in `handleMouseMsg` (`mouse.go:231`), add an early branch — if `inputMode == inputHelp` and the event is a wheel event, route to `m.helpViewport.Update`. Mirrors the existing preview-mode branch at `mouse.go:240–245`.

### 4. Shortcut reorder (`Ctrl+Up`/`Ctrl+Down`)

Add two cases to `handleShortcutSelect` (`shortcuts_input.go:8–88`), placed after the existing `up`/`down` cases:

```go
case "ctrl+up":
    if m.shortcutCursor > 0 {
        i := m.shortcutCursor
        m.shortcuts[i-1], m.shortcuts[i] = m.shortcuts[i], m.shortcuts[i-1]
        m.shortcutCursor--
        if err := saveShortcuts(m.shortcuts); err != nil {
            m.errMsg = "Failed to save shortcuts: " + err.Error()
        }
    }
case "ctrl+down":
    if m.shortcutCursor < len(m.shortcuts)-1 {
        i := m.shortcutCursor
        m.shortcuts[i], m.shortcuts[i+1] = m.shortcuts[i+1], m.shortcuts[i]
        m.shortcutCursor++
        if err := saveShortcuts(m.shortcuts); err != nil {
            m.errMsg = "Failed to save shortcuts: " + err.Error()
        }
    }
```

Persistence is automatic: `saveShortcuts` writes the slice in array order to `~/.config/clipad/ai_shortcuts.toml`.

UI hint: append `Ctrl+↑/↓:reorder` to the hint line in `shortcuts_modal.go:90`.

Help modal: add a `Ctrl+↑ / Ctrl+↓` row to the "Shortcut Picker" section in `helpSections` (`help_modal.go:73–81`).

### 5. Toggle tree visibility (`Ctrl+B`)

New field on `model`:

```go
treeHidden bool // per-session toggle; not persisted
```

Update `recalcLayout` (`model.go:1228–1244`):

```go
const minTreeWidth = 20
if m.treeHidden || m.width < minTreeWidth+10 {
    m.treeWidth = 0
    m.editorWidth = m.width
} else {
    // existing branch unchanged
}
```

Auto-hide on narrow terminals continues to win — the `||` ensures hidden stays hidden in either condition.

Add `Ctrl+B` handling alongside other globals in `Update` (near `model.go:499`):

```go
case "ctrl+b":
    m.treeHidden = !m.treeHidden
    if m.treeHidden && m.activePanel == treePanel {
        m.activePanel = editorPanel
        if m.currentFile != "" || m.newNoteDir != "" {
            cmd := m.editor.Focus()
            m.recalcLayout()
            return m, cmd
        }
    }
    m.recalcLayout()
    return m, nil
```

The handler sits inside the existing `inputMode == inputNone` branch, so it's automatically ignored during any modal/input flow.

Help modal: add `Ctrl+B | Toggle file tree` row to the Global section.

README: add the same row to the Global keybindings table.

## Testing strategy

New tests follow the project's existing patterns: table-driven, model-level, simulating `tea.Msg` values into `Update` and asserting state. No network or filesystem mocks beyond a temp `XDG_CONFIG_HOME` for shortcut persistence (already used in `shortcuts_test.go`).

| # | Test | Asserts |
|---|---|---|
| 1 | `TestTreeScrollBy_DecouplesFromCursor` | 50 items, height 10, cursor=0; `scrollBy(20)` → `offset==20`, `cursor==0`. |
| 2 | `TestTreeScrollBy_ClampsAtBounds` | `scrollBy(1000)` → `offset == len-height`; `scrollBy(-1000)` → `offset == 0`. |
| 3 | `TestTreeMoveDown_AfterScroll_SnapsViewToCursor` | scroll cursor off-screen, `moveDown` → view repositions so cursor visible. |
| 4 | `TestTreeWheelDown_ScrollsPastCursor` | regression for the original mouse-scroll bug. |
| 5 | `TestTreeAddNoteRow_EmptyTree_CursorOnAddNote` | empty tree → `cursor == -1`. |
| 6 | `TestTreeAddNoteRow_NonEmpty_CursorOnFirstFile` | files present → `cursor == 0`. |
| 7 | `TestTreeMoveUpFromFirstFile_LandsOnAddNote` | cursor=0 → moveUp → cursor=-1. |
| 8 | `TestTreeMoveDownFromAddNote_LandsOnFirstFile` | cursor=-1 → moveDown → cursor=0. |
| 9 | `TestTreeOnAddNote_EnterTriggersNewNote` | model with cursor=-1, dispatch enter → `newNoteDir` non-empty (or `inputMode`/state matches `Ctrl+N` flow). |
| 10 | `TestTreeClick_OnAddNoteRow_TriggersNewNote` | mouse click on row 0 → same effect as #9. |
| 11 | `TestHelpModal_WheelScrollsViewport` | open help, dispatch wheel events → `m.helpViewport.YOffset` advances. |
| 12 | `TestHelpModal_DownArrowScrolls` | down/pgdn/end advance the viewport. |
| 13 | `TestHelpModal_EscClosesAndDoesntScroll` | Esc → `inputMode == inputNone`. |
| 14 | `TestShortcutSelector_CtrlUp_SwapsAndPersists` | two shortcuts, cursor=1, dispatch ctrl+up → order swapped, cursor=0, on-disk file reflects new order. |
| 15 | `TestShortcutSelector_CtrlDown_SwapsAndPersists` | mirror of #14. |
| 16 | `TestShortcutSelector_CtrlUp_AtTop_NoOp` | cursor=0, ctrl+up → unchanged. |
| 17 | `TestShortcutSelector_CtrlDown_AtBottom_NoOp` | cursor=last, ctrl+down → unchanged. |
| 18 | `TestCtrlB_TogglesTreeHidden` | dispatch ctrl+b → `treeHidden==true`, `treeWidth==0`, `editorWidth==m.width`. Toggle again → restored. |
| 19 | `TestCtrlB_TreeHiddenAndActivePanel_FocusFollowsToEditor` | activePanel=tree, ctrl+b → activePanel=editor. |
| 20 | `TestCtrlB_NarrowTerminal_StaysHiddenAfterToggle` | width below threshold, ctrl+b twice → tree never reappears (auto-hide overrides). |
| 21 | `TestCtrlB_IgnoredDuringInputMode` | `inputMode = inputFilter`, dispatch ctrl+b → no change. |

Out of scope for new tests: `viewport.Model` internals (charm-tested), the underlying file I/O of `saveShortcuts` (covered by `shortcuts_test.go`).

## Edge cases

1. **Tree empty, "Add note" cursor focus.** `selectedNode()` returns nil; existing `previewSelectedFile`, `right` arrow, `ctrl+d`, `ctrl+e` already null-check and no-op. Enter still triggers new note.
2. **Tree mouse scroll past cursor, then keyboard `up`.** `scrollBy` left cursor where it was; `moveUp` decrements cursor and `clampOffset` snaps offset back to keep cursor visible. Expected.
3. **`scrollBy` when `len(items) <= height`.** `maxOffset` becomes 0; offset stays 0. Wheel is a no-op, matching existing behavior.
4. **Help modal opened, terminal resized.** `recalcLayout` resizes the viewport and re-`SetContent`s with the new width, so wrapping recomputes. Scroll position may reset; acceptable for a help view.
5. **Help modal mouse wheel hits during `pluginProcessing`.** The pre-existing guard at `model.go:367` blocks input dispatch entirely while processing; help modal can't be open simultaneously because opening it requires `inputMode == inputNone`.
6. **Shortcut reorder with single shortcut.** Both ctrl+up and ctrl+down no-op (cursor already at 0 == last index).
7. **Shortcut reorder save error.** Slice is updated in memory regardless; error surfaces in `m.errMsg`. Next launch reloads from disk — if disk write failed, the order from before the swap is what persists. Same failure mode as edit/delete today.
8. **Ctrl+B with tree already auto-hidden by narrow terminal.** Toggling `treeHidden = true` is a no-op visually (already hidden); toggling back to `false` doesn't bring the tree back because the narrow-terminal guard in `recalcLayout` still applies.
9. **Ctrl+B during `inputHelp`.** Ignored — the global key handler runs only when `inputMode == inputNone`. The help modal itself doesn't bind `Ctrl+B`, so it falls through and is consumed silently.
10. **Ctrl+B then resize terminal narrow.** `treeWidth` recomputes via `recalcLayout` on each resize. If terminal becomes narrow, tree stays hidden (both conditions true). If it becomes wide again and `treeHidden == true`, tree stays hidden until user toggles back.
11. **Pinned "Add note" row when `tp.height == 1`.** Only the pinned row is visible; no items shown. `clampOffset`'s `maxOffset = len(items) - (height - 1) = len(items)` — guarded by `len(items) > 0` check; no items rendered. Acceptable.
12. **Click hits the pinned-row position when `treeHidden == true`.** The hit-test runs against `treeWidth == 0`, so the click never lands in the tree panel; routes to editor instead. Safe.
