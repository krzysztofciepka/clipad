# Mouse Support & Faster Text Editing — Design

## Problem

Clipad is keyboard-only. Three gaps slow down editing and navigation:

1. Text selection and line navigation work via keyboard but the app ignores mouse input entirely — no click-to-position, no drag-select, no wheel-scroll.
2. The file tree can only be driven with arrow keys; clicking a file does nothing.
3. Git sync runs automatically on startup and every 24 hours, but there's no keybinding to trigger an immediate push/pull when the user wants one.

## Goal

Add mouse support (click, drag-select, wheel) in the editor and file tree, make line-edge navigation explicit, and add a keybinding for manual git sync — without disturbing the existing keyboard flow.

## Scope

| # | Task | Status today | Work |
|---|------|-------------|------|
| 1 | Delete selected text (after shift+arrow or mouse) | Already works for keyboard selection via `backspace`/`delete` in `selection.go:341`. Will automatically work for mouse-selected text once mouse selection shares the same `selActive` state. | Verification + tests for mouse-selection path. |
| 2 | Jump to line start / end | Plain `Home`/`End` fall through to bubbletea textarea's built-in handling. `shift+Home`/`shift+End` already select-to-line-edge. | Add explicit tests; no code change unless textarea defaults fail in practice. |
| 3 | Mouse selection matching keyboard | Not implemented. | Enable `WithMouseCellMotion`; add `MouseMsg` handler; drag updates `selAnchor*` + cursor just like `shift+arrow`. |
| 4 | Keybinding for git sync | `runGitSync` exists; only triggered on startup / URL save / 24h timer. | Add `Ctrl+Y` → run sync immediately, bypassing the 24h guard. |
| 5 | Mouse control (click tree, wheel) | Not implemented. | Click in tree: move cursor + preview (file) or toggle expand (folder). Wheel scrolls the focused panel. |

**Out of scope:** double-click, right-click menus, mouse resize of the tree/editor split, drag-to-reorder tree rows, mouse events during modal overlays, selection across panel boundaries, horizontal-wheel events, terminal-native selection toggle.

## Architecture

Three new isolated units, one `main.go` one-liner, minor `model.go` edits. All mouse logic lives in a new `mouse.go` file so existing files stay focused.

### New file: `mouse.go`

**Pure helpers** — easy to unit-test, no bubbletea runtime needed:

- `hitTestPanel(treeWidth, editorWidth, height, x, y int) (hit panel, localX, localY int, ok bool)` — given a mouse `(x, y)` and the current layout, return which panel the click lands in (`treePanel` / `editorPanel`), the coordinates relative to that panel's top-left, and `ok=false` if the click is on the status bar or outside. Narrow-terminal path (`treeWidth == 0`) treats the whole width as editor.
- `mousePosToEditorCursor(content string, viewOffset, localX, localY, numWidth int) (line, col int)` — translate local editor coords to `(line, col)` in the content, accounting for:
  - `editorStyle`'s `Padding(0, 1)` → 1 char left/right
  - Line-number column: `numWidth + 1` chars
  - `viewOffset` as the top visible line
  - Clamp to the line's rune length; clamp `line` to content length.
- `mousePosToTreeRow(treeOffset, localY int) int` — translate local tree Y to absolute tree row index (`treeOffset + localY`); callers validate bounds against `len(tree.items)`.

**Dispatcher:**

- `handleMouseMsg(m model, msg tea.MouseMsg) (tea.Model, tea.Cmd)` — routes to:
  - `handleEditorMouse(m, localX, localY, msg)` for editor clicks / drags / wheel
  - `handleTreeMouse(m, localY, msg)` for tree clicks / wheel
  - Preview mode: wheel events forwarded to `m.preview` viewport via its `Update(msg)`
  - Any event during `m.inputMode != inputNone` or `m.pluginProcessing`: ignored

### Changes in existing files

**`selection.go`** — add one field to `SelectableEditor`:

```go
type SelectableEditor struct {
    ...
    mouseDragging bool // true between left-button press and release
}
```

Reason: distinguish "click = position cursor, no selection" from "drag = position anchor + extend selection." No other selection fields needed — we reuse `selActive`, `selAnchorLine`, `selAnchorCol`.

**`main.go`** — enable mouse:

```go
p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
```

`WithMouseCellMotion` sends motion events only while a button is held — exactly what drag-select needs, and no wheel-motion spam.

**`model.go`** — add to `Update`:

```go
case tea.MouseMsg:
    if m.pluginProcessing {
        return m, nil
    }
    if m.inputMode != inputNone {
        return m, nil
    }
    return handleMouseMsg(m, msg)
```

And a new key case in the existing top-level key switch:

```go
case "ctrl+y":
    return m.triggerManualGitSync()
```

**`git_sync.go`** — new method:

```go
func (m model) triggerManualGitSync() (tea.Model, tea.Cmd) {
    if m.gitSyncRunning {
        return m, nil
    }
    cfg, err := loadConfig()
    if err != nil {
        m.errMsg = "Git sync: " + err.Error()
        return m, nil
    }
    if cfg.GitRemote == "" {
        m.inputMode = inputGitRemote
        m.gitRemoteInput.SetValue("")
        cmd := m.gitRemoteInput.Focus()
        return m, cmd
    }
    m.gitSyncRunning = true
    m.gitSyncError = ""
    return m, runGitSync(m.vault, cfg.GitRemote)
}
```

Bypasses the 24h guard used by the periodic `gitSyncCheckMsg`. Concurrent safety via the existing `gitSyncRunning` flag.

## Mouse data flow

### Editor

```
Press Left (in editor panel)
  → mousePosToEditorCursor → (line, col)
  → e.moveTo(line, col)
  → e.selAnchorLine, e.selAnchorCol = line, col
  → e.selActive = true          // provisional; release may clear
  → e.mouseDragging = true
  → if activePanel == treePanel: switch to editorPanel, e.Focus()
  → if editorMode == modePreview: switch to modeEdit

Motion with Button=Left (drag in same panel)
  → mousePosToEditorCursor → (line, col)
  → e.moveTo(line, col)         // anchor stays, cursor moves
  → adjustViewOffset            // auto-scroll on edge overshoot

Release Left
  → if (e.Line(), e.cursorCol()) == (anchor): e.ClearSelection()
    i.e. it was a click, not a drag
  → e.mouseDragging = false

Wheel Up in editor
  → e.viewOffset -= 3; clamp ≥ 0
Wheel Down in editor
  → e.viewOffset += 3; clamp ≤ max(len(lines) - 1, 0)
```

Scroll keeps the cursor line where it is unless it falls outside the new viewport — matches most editors' feel.

### Tree

```
Press Left (in tree panel, on row i)
  → if i >= len(tp.items): ignore
  → tp.cursor = i; clampOffset
  → node := tp.items[i].Node
  → if node.IsDir:
        node.Expanded = !node.Expanded
        tp.rebuildItems()
  → else (file):
        previewSelectedFile()   // existing: opens + modePreview + blur editor
  → activePanel = treePanel

Wheel Up in tree
  → tp.offset -= 3; clamp ≥ 0
Wheel Down in tree
  → tp.offset += 3; clamp so last row still visible
```

If the cursor would go offscreen after wheel, it follows the viewport.

### Preview & modals

Preview-mode wheel events (`activePanel == editorPanel && editorMode == modePreview`) forward to `m.preview` viewport, which already interprets `tea.MouseMsg` wheel events.

Any mouse event during an `inputMode != inputNone` overlay (plugin picker, shortcut picker, replace, help, confirm dialogs) is ignored, preserving the keyboard-only flow of those flows.

## Git sync keybinding

- `Ctrl+Y` is unused today (verified by grepping every `ctrl+` key case in the codebase).
- On `Ctrl+Y` with no remote configured: opens the existing `inputGitRemote` prompt; after the user enters the URL, `handleGitRemoteInput` already issues `gitSyncCheckImmediate()`, so the first sync kicks off automatically.
- Re-entrancy blocked by `m.gitSyncRunning`.
- Status bar already shows "Syncing..." via the existing `gitSyncRunning` branch in `View()`. Flash messages ("Synced" / "Backed up" / "Synced from remote") fade via the existing `gitSyncFadeTick`. Errors surface in `m.gitSyncError` → existing status-bar error branch.

## Home / End

Bubble Tea's `bubbles/textarea` binds `Home`/`End` to line-start / line-end internally. In our code:

- `HandleKey` does **not** intercept plain `Home`/`End`; they fall through to `e.Model.Update(msg)`.
- `HandleKey`'s `selActive + tea.KeyHome/KeyEnd` branch (lines 352–354) clears the selection before the textarea handles the move — so pressing `Home`/`End` after a shift-selection jumps and deselects in one step.
- `shift+Home` / `shift+End` already extend selection to line edges (lines 312–321).

No code change required. The design documents the behavior and adds explicit tests. If during implementation the textarea defaults don't match (edge case — different bubbles versions), add an explicit handler mirroring the shift variants without the selection side-effect.

## Delete selected text

`HandleKey` (selection.go:341) calls `DeleteSelection()` on `backspace` or `delete` when a selection is active. `DeleteSelection` uses the same `selActive` / `selAnchor*` state that mouse selection will write to, so deletion works for mouse selection automatically once mouse selection is wired.

No code change required. Tests verify both keyboard-selected and mouse-selected paths hit the same deletion code.

## Testing

### `mouse_test.go` (new)

Pure-function tests:

- `TestHitTestPanel` — tree area, border column, editor area, status-bar row, outside, narrow-terminal (`treeWidth == 0`) cases.
- `TestMousePosToEditorCursor` — viewOffset=0 baseline; clamp past line length; clamp past content height; viewOffset > 0 shift; click in padding or line-number column → col 0.
- `TestMousePosToTreeRow` — simple offset + localY; beyond visible → caller rejects via `len(items)` check.

### `selection_test.go` (extensions)

- `TestHandleKeyDeleteSelection` — build a `SelectableEditor`, drive shift+right a few times through `HandleKey`, press backspace, assert `Value()` reflects deletion.
- `TestHandleKeyHomeEnd` — plain Home → col 0; plain End → line length.
- `TestHomeClearsSelection` — create selection via shift+right, plain Home → `selActive == false` and cursor at col 0.
- `TestMouseDragSelection` — simulate `MouseMsg{Left, Press, x, y}` + motion + release through `handleEditorMouse`, assert `SelectedText()` returns the expected substring. If routing through `handleEditorMouse` proves too coupled to the model, this test pokes `selAnchorLine/Col` + `moveTo` directly to verify the drag→selection mapping shape.

### `git_sync_test.go` (extensions)

- `TestTriggerManualGitSync_NoRemotePromptsForURL` — call with a config that has no `GitRemote`; assert the returned model has `inputMode == inputGitRemote`. No real git invocation.
- `TestTriggerManualGitSync_SkipsIfAlreadyRunning` — set `gitSyncRunning = true`, call, assert no state change and no command returned.
- Do **not** test "bypasses 24h guard" with a real git run — too much setup. Instead, assert `triggerManualGitSync` returns a non-nil command regardless of `cfg.LastSync`, using a stubbed config load (or by asserting the code path reached, not the command contents).

### Manual integration checklist

After implementation:

- Type in editor, shift+arrow select, press backspace → text deleted.
- Click-drag in editor → text selected (highlight via `renderWithSelection`).
- Click-drag then backspace → text deleted.
- Click file in tree → preview opens in right pane; editor blurred; tree stays focused.
- Click folder in tree → expand/collapse; cursor moves to clicked row.
- Wheel in editor scrolls the view; wheel in tree scrolls the tree.
- Press `Home` then `End` in a file → cursor jumps to line-start / line-end.
- Press `Ctrl+Y` → status bar shows "Syncing..." then a flash message; errors surface in status bar.

## Files touched

### Source

- `main.go` — add `tea.WithMouseCellMotion()` to program options.
- `model.go` — add `case tea.MouseMsg:` in `Update`; add `case "ctrl+y":` in main key switch.
- `selection.go` — add `mouseDragging bool` field.
- `mouse.go` (new) — pure helpers and mouse dispatcher / handlers.
- `git_sync.go` — add `triggerManualGitSync` method.
- `README.md` — document `Ctrl+Y` and mouse behavior.

### Tests

- `mouse_test.go` (new) — hit-test, cursor translation, tree-row translation.
- `selection_test.go` — delete-selection, home/end, home-clears-selection, mouse-drag-selection.
- `git_sync_test.go` — `triggerManualGitSync` tests.

## Non-goals

- Double-click (select word, open file without preview).
- Right-click context menus.
- Mouse resize of the tree/editor split.
- Mouse drag to reorder tree rows.
- Mouse events during modal overlays — all explicitly ignored.
- Mouse events on the status bar — ignored.
- Terminal-native selection toggle — per earlier decision, full mouse capture only.
- Mouse selection across panel boundaries.
- Horizontal-wheel events.
