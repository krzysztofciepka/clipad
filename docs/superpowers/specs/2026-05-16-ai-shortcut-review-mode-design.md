# AI Shortcut Review Mode — Design

**Date:** 2026-05-16
**Task:** Task 69
**Status:** Approved

## Problem

AI shortcuts (`Ctrl+G`) currently support exactly one outcome: stream an AI
transformation and offer to **replace** the note (or selection) via the
side-by-side diff + `y/n` accept flow.

Some shortcuts don't produce a better version of the note — they produce
commentary *about* it (a critique, a list of open questions, a risk list).
Replacing the note with that output destroys the note. Users want a second
outcome: **generate the content and display it side-by-side, read-only,
without touching the note**.

## Solution Overview

Add a per-shortcut **type**:

- `replace` — existing behaviour (diff + accept/discard). Default.
- `review` — new read-only side-by-side view. Note is never modified.

Built-in shortcuts get an explicit type in `defaults/ai_shortcuts.toml`.
Existing user configs (no `type` field) resolve their type at read time, so
no migration write is required.

## Detailed Design

### 1. Data model

`AIShortcut` gains a `Type` field:

```go
type AIShortcut struct {
    Name        string `toml:"name"`
    Description string `toml:"description"`
    Prompt      string `toml:"prompt"`
    Type        string `toml:"type"` // "replace" (default) | "review"
}
```

A helper resolves the effective type:

```go
// resolveShortcutType returns "replace" or "review".
func resolveShortcutType(s AIShortcut) string
```

Resolution rules:

1. If `s.Type == "replace"` or `s.Type == "review"` → return it.
2. Otherwise (empty or unrecognised) → infer by name:
   - Name in the set `{critique, questions, risks, outline}` → `"review"`.
   - Anything else (other built-ins, custom shortcuts) → `"replace"`.

The inferred-review name set is a small package-level `map[string]bool`
constant. Resolution is read-time only; clipad never rewrites a user's
`ai_shortcuts.toml` to add the field.

`defaults/ai_shortcuts.toml` is updated so every entry has an explicit
`type`: `type = 'review'` for `critique`, `questions`, `risks`, `outline`;
`type = 'replace'` for the other 19.

### 2. Run dispatch

In `handleShortcutSelect` (the `"enter"` case), the streaming setup is
unchanged (`runShortcutStream`, context/cancel, `activeChunks`, viewport
creation). After setup, branch on `resolveShortcutType(shortcut)`:

- `replace` → `m.inputMode = inputPluginDiff` (unchanged path).
- `review`  → `m.inputMode = inputPluginReview` (new).

The streaming message handlers (`pluginChunkMsg`, `pluginDoneMsg`,
`pluginErrMsg`) are mode-agnostic: they accumulate into
`m.pluginDiffResult` and update the right viewport, gated by
`m.activeChunks`. No per-message branching needed, with one exception:

- The `pluginDoneMsg` "No changes" early-dismiss (fires when result equals
  the original or is empty) must **not** dismiss a review. For review mode:
  an identical result is still a valid thing to read; an **empty** result
  sets `m.errMsg = "No review generated"` and closes. The handler checks
  `m.inputMode` to apply the correct rule.

### 3. Review view & interaction (`inputPluginReview`)

New `inputPluginReview` value added to the `inputMode` enum.

**View** — `pluginReviewView(left, right viewport.Model, focus reviewFocus,
width, height int) string`, reusing the two-pane layout structure of
`pluginDiffView` but with:

- Left header: `── Note ──`
- Right header: `── Review ──`
- The focused pane's header/border is highlighted so focus is visible.

**Handler** — `handlePluginReview(msg tea.KeyMsg)`:

- `tab` — toggle `m.reviewFocus` between left (note) and right (review).
  Default focus when entering review mode = right (the review).
- `up`/`k`/`down`/`j` — scroll the **focused** viewport one line.
- `c` — copy the full review to the system clipboard via
  `clipboard.WriteAll(m.pluginDiffResult)` (`github.com/atotto/clipboard`,
  already a dependency, used in `selection.go`). Set a transient
  `m.errMsg`-style status ("Review copied").
- `esc` / `q` — close: `m.inputMode = inputNone`, clear `pluginActive`,
  `pluginDiffOriginal`, `pluginDiffResult`; editor value untouched.
- `ctrl+q` — unchanged dirty-guard / quit behaviour (mirrors
  `handlePluginDiff`).

`reviewFocus` is a small typed enum (`reviewFocusNote` /
`reviewFocusReview`) stored on the model.

**Status bar** — for `inputPluginReview`:
`Review — Tab:switch pane  c:copy  Esc:close`.

**Resize** — the `WindowSize`/recompute block that rebuilds the diff
viewports when `inputMode == inputPluginDiff` also rebuilds them when
`inputMode == inputPluginReview`.

### 4. Mouse scroll

`tea.MouseMsg` is currently dropped when `inputMode != inputNone` (the
`inputHelp` case is the lone exception). Add an `inputPluginReview` branch
that routes wheel events to a handler which maps the mouse X position to
the left or right half of the editor area and scrolls **that** (hovered)
pane's viewport. Wheel up/down → `LineUp(n)`/`LineDown(n)`. Strictly scoped
to review mode; diff/replace and all other modes are unaffected.

### 5. New / edited shortcut flow

The create/edit input chain is currently
name → description → prompt → save.

Insert a type step: name → description → prompt → **type** → save.

- New `inputShortcutType` inputMode with a minimal two-option selector
  ("replace" / "review").
- Keys: `↑`/`↓` move selection, `enter` confirms, `r`/`v` jump-select,
  `esc` cancels the whole create/edit (consistent with other steps),
  `ctrl+q` dirty-guard.
- The `saveShortcuts` call currently at the end of `handleShortcutPrompt`
  moves to the type-confirm step; the constructed `AIShortcut` now includes
  `Type`.
- When editing an existing shortcut, the selector is pre-set to that
  shortcut's resolved type.

### 6. Documentation

- README: document the two shortcut types and the review interaction
  (Tab / scroll / mouse / `c` copy / Esc).
- Help modal content: add the review-mode keys.

## Testing

- `resolveShortcutType`:
  - Explicit `"replace"` and `"review"` returned as-is.
  - Empty type for each of `critique`, `questions`, `risks`, `outline`
    → `"review"`.
  - Empty type for a replace built-in (e.g. `tighten`) and a custom name
    → `"replace"`.
  - Unrecognised string (e.g. `"foo"`) → `"replace"`.
- `defaults/ai_shortcuts.toml` parses; the 4 named entries resolve to
  `review`, the remaining 20 to `replace`.
- `handleShortcutSelect` routes to `inputPluginReview` for a review
  shortcut and `inputPluginDiff` for a replace shortcut.
- `handlePluginReview`:
  - `tab` toggles `reviewFocus`.
  - scroll keys move the focused viewport (assert via viewport offset or
    a focus-routing seam).
  - `c` writes the review text to the clipboard.
  - `esc` and `q` close with the editor value unchanged.
- Streaming `pluginDoneMsg` with an identical result does **not** dismiss
  review mode; an empty result closes with the "No review generated"
  message.
- New-shortcut chain advances to `inputShortcutType` and the saved
  shortcut carries the chosen `Type`; editing pre-selects the resolved
  type.

## Out of Scope (YAGNI)

- Appending the review into the note.
- Diff-style add/remove highlighting in the review pane.
- Persisting independent scroll positions beyond what `viewport.Model`
  already provides.
- Rewriting existing user `ai_shortcuts.toml` files to add the `type`
  field (resolution is read-time).
