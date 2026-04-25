# Selection-Scoped AI Shortcuts — Design

**Date:** 2026-04-25
**Task:** Task 22

## Problem

When the user triggers an AI run while text is selected, the run currently sends the whole note to the LLM and the diff/accept flow rewrites the whole note. The user wants selection-scoped runs: when a selection is active, only the selected text is sent and only the selection is replaced; the rest of the note is untouched. With no selection, today's whole-note behavior is preserved.

## Current state

The codebase already has most of the wiring; the work is to finish it.

| Flow | Keybinding | Selection-scoped today? |
|---|---|---|
| AI Shortcuts (Ctrl+G → pick → run) | `Ctrl+G` | Yes — `shortcuts_input.go:43–47` reads `SelectedText()` when `selActive`; `plugin_diff.go:30–33` calls `ReplaceSelection` on accept. |
| AI Shortcut needing plugin config first | `Ctrl+G` → config wizard → resume | Yes — `plugin_input.go:73–76`. |
| Plugin custom prompt (Ctrl+Space → pick → enter prompt → run) | `Ctrl+Space` | **No** — `plugin_input.go:119` always uses `m.editor.Value()`. |

The diff/accept handler (`plugin_diff.go`) already branches on the boolean state field `shortcutOnSelection` and calls `ReplaceSelection` vs `SetValue` correctly. The reject branch resets the flag and leaves the selection alive (so the user can retry).

The only flow genuinely missing the behavior is the plugin custom-prompt path. A few related cleanups (helper extraction, field rename, README correction) bundle naturally with that fix.

## Scope

In scope:
1. Make the plugin custom-prompt path selection-aware, matching the shortcut path's behavior.
2. Extract a single helper used by all three call sites that need to compute "what content do we send + was it scoped to a selection?"
3. Rename the state field `shortcutOnSelection` → `aiRunOnSelection` so the name reflects that it now spans plugin and shortcut runs.
4. Fix `README.md:144`, which incorrectly says `Ctrl+Space` triggers AI shortcuts (the actual binding is `Ctrl+G`); add the missing `Ctrl+G` row to the Global keybindings table; document the new selection-scoped behavior.
5. Add model-level tests covering both flows, with and without selection, on both accept and reject.

Out of scope:
- A "scope: selection / whole note" indicator in the diff header.
- Letting the user toggle scope after triggering.
- Refactoring the duplicate `pluginResultMsg` / `shortcutResultMsg` handling in `model.go:315–355`.
- Any change to undo grouping; `ReplaceSelection` already records a single-entry op via `recordOp`.

## Architecture

No structural moves. One new helper, three call sites converging on it, one state field renamed, and one diff-accept handler that already does the right thing once the field reflects its broader meaning.

### Helper

```go
// aiInputContent returns the content to feed to an AI run plus a flag the
// diff-accept path uses to decide whether to replace just the selection or
// the whole buffer. selActive is sufficient as the "has selection" predicate
// because the editor already clears it on no-op clicks and on cursor moves
// without shift, so a true value implies a non-empty range.
func (m *model) aiInputContent() (content string, onSelection bool) {
    if m.editor.selActive {
        return m.editor.SelectedText(), true
    }
    return m.editor.Value(), false
}
```

Pointer receiver for consistency with other model methods. The helper is read-only; it does not write `aiRunOnSelection` and does not mutate selection. The caller writes the field, sets `pluginDiffOriginal`, and dispatches the LLM command — keeping each call site as a single coherent transaction.

### State field rename

`shortcutOnSelection` → `aiRunOnSelection`. Six occurrences across four files:

- `model.go:132` — declaration
- `plugin_input.go:74,75` — set + read in shortcut-pending-after-config branch
- `plugin_diff.go:30,33,50` — branch on accept, reset on accept, reset on reject
- `shortcuts_input.go:44,45` — set + read in shortcut path

Mechanical rename, no semantic change.

### Call site changes

**Site 1 — `handlePluginPrompt` (`plugin_input.go:106–138`).** The actual fix.

Before:
```go
content := m.editor.Value()
m.pluginDiffOriginal = content
m.pluginProcessing = true
m.inputMode = inputNone
return m, runPluginCmd(m.pluginActive, content, prompt, cfg)
```
After:
```go
content, onSelection := m.aiInputContent()
m.aiRunOnSelection = onSelection
m.pluginDiffOriginal = content
m.pluginProcessing = true
m.inputMode = inputNone
return m, runPluginCmd(m.pluginActive, content, prompt, cfg)
```

**Site 2 — `handleShortcutSelect` (`shortcuts_input.go:43–47`).** Refactor: replace inline three-liner with the helper. Behavior identical.

**Site 3 — `handlePluginConfig` shortcut-pending branch (`plugin_input.go:73–76`).** Refactor: replace inline three-liner with the helper. Behavior identical.

**Not touched:** `handlePluginDiff` (`plugin_diff.go:27–73`). The accept path branches on the state field and does the right thing today; the rename is the only edit.

## Documentation changes (`README.md`)

1. Add `| Ctrl+G | Open AI shortcut selector |` to the Global keybindings table (currently missing).
2. Fix line 144: replace "Press `Ctrl+Space`, pick a shortcut" with "Press `Ctrl+G`, pick a shortcut".
3. Append after the AI Shortcuts paragraph:

> If text is selected when you trigger a plugin or shortcut, only the selected text is sent to the LLM and the diff/accept flow replaces just that selection. With no selection, the whole note is rewritten as before.

## Testing strategy

New file: `plugin_selection_test.go`. Pattern follows `shortcuts_input_test.go` and `mouse_test.go`: construct a `model`, set up editor state, dispatch `tea.Msg` values into `Update`, assert state. LLM responses are simulated by dispatching a synthetic `pluginResultMsg{result: "...", err: nil}` directly — no network call.

| # | Test | Asserts |
|---|---|---|
| 1 | `TestPluginPrompt_NoSelection_SendsWholeContent` | `pluginDiffOriginal == whole buffer`; `aiRunOnSelection == false`. |
| 2 | `TestPluginPrompt_WithSelection_SendsOnlySelection` | `pluginDiffOriginal == SelectedText()`; `aiRunOnSelection == true`. |
| 3 | `TestPluginDiffAccept_OnSelection_ReplacesOnlySelection` | Buffer outside the selection range byte-identical; selection range replaced; cursor at end of replacement. |
| 4 | `TestPluginDiffAccept_NoSelection_ReplacesWholeContent` | Whole buffer replaced. Regression check. |
| 5 | `TestPluginDiffReject_OnSelection_LeavesContentUnchanged` | Buffer unchanged; `aiRunOnSelection == false`; selection still active. |
| 6 | `TestShortcutSelect_WithSelection_SendsOnlySelection` | Regression for the existing shortcut path after the helper refactor. |
| 7 | `TestAIInputContent_Helper` | Helper returns `(whole, false)` with no selection; `(selected, true)` with selection. |

Out of scope for new tests (existing coverage suffices): the network call in `runPluginCmd` / `runShortcutCmd`; `ReplaceSelection` undoability (`selection_test.go:447–500`); diff viewport rendering.

## Edge cases

1. **Selection spans the whole buffer.** `ReplaceSelection` deletes everything and inserts the result; equivalent end state to whole-note rewrite.
2. **Partial-line selection, multi-line LLM output.** `InsertString` handles embedded newlines; cursor ends at the end of the inserted text.
3. **LLM returns text identical to the selection.** `pluginDiffOriginal` holds the selection, so the existing `if msg.result == m.pluginDiffOriginal` check (`model.go:324`) fires "No changes" correctly.
4. **Reject then retry.** Reject leaves `selActive` alone; the next run uses the still-active selection.
5. **"Logically empty" selection.** Cannot happen by construction: `EndMouseDrag` clears `selActive` on no-drag; non-shift arrow keys clear it; typing clears it. `selActive == true` implies non-empty `SelectedText()`. No defensive whitespace check needed.
6. **File switch mid-run.** Modal blocks key input while `pluginProcessing` is true (`model.go:367`); the diff modal only handles `y/n/esc/arrows/ctrl+q`. No race.
7. **Auto-save mid-run.** Writes the buffer to disk; doesn't touch selection state or `pluginDiffOriginal`. Harmless.
8. **`shortcutPending` after config.** Falls through `handlePluginConfig` to run the shortcut once config is filled in; that path becomes the helper call (call site 3).
