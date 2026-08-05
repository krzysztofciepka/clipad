# Left-Pane Button Bar — Design

Date: 2026-08-05
Task: P11

## Problem

Several of clipad's most useful actions are reachable only by keyboard shortcut
(Ctrl+R find & replace, Ctrl+G AI shortcuts, Ctrl+K vault chat, Ctrl+T vault
search). They are invisible unless the user opens the help modal. The same is
true inside diff, review, and vault-chat modes, where the available actions are
single letters shown only in the status bar hint.

## Goal

Add a persistent, clickable button bar at the bottom of the left pane. It shows
a default action set in ordinary use and a mode-specific set in diff, review,
and vault-chat modes. It can be collapsed to a single row to give the file tree
back its space.

## Non-Goals

- No new keyboard shortcuts for the actions themselves; every button maps onto
  an existing key path.
- No persistence of the collapsed state across restarts. It is per-session,
  matching the existing `Ctrl+B` tree toggle.
- No mouse support inside modal overlays that currently swallow mouse input.

## Layout

The left column becomes two stacked regions:

```
┌ tree panel ────────┐
│ + Add note         │
│ ▼ notes            │
│   ideas.md         │
│   todo.md          │
│ ▶ archive          │
│                    │
│─[-]────────────────│   ← toggle rule (bar row 0)
│ Find & replace     │
│ AI tools           │
│ Ask                │
│ AI search          │
└────────────────────┘
```

Collapsed, the bar is exactly one row:

```
│─[+]────────────────│
```

### Sizing

`recalcLayout()` computes the bar height first and subtracts it from the tree:

```go
m.barHeight  = buttonBarHeight(m.treeWidth, m.buttonsCollapsed, len(m.barButtons()))
m.treeHeight = m.height - 2 - m.barHeight   // floor of 1
```

`buttonBarHeight` returns:

| Condition | Height |
|---|---|
| `treeWidth == 0` (tree hidden, or terminal too narrow) | 0 |
| collapsed | 1 |
| expanded | `1 + len(buttons)` |

The tree keeps its existing offset-clamping and scrolling behavior inside the
reduced height, so it never grows over the bar. The left column still totals
`m.height - 2` rows, matching the editor column, so the horizontal join stays
aligned.

The bar renders through a style mirroring `treePanelStyle` — `Padding(0, 1)`
plus `BorderRight` with `NormalBorder` — at `Width(m.treeWidth)`, so the
vertical divider between the left pane and the editor runs unbroken from the
top of the tree to the bottom of the bar.

### Related fix

`filterView()` currently renders with `MaxHeight(m.treeHeight)` rather than
`Height`. With nothing below it that was invisible; with the bar attached, a
short filter result list would let the bar float up mid-column and leave the
left column shorter than the editor. It changes to `Height(m.treeHeight)`.

## Button Sets

A single method `(m model) barButtons() []barButton` returns the active set.
Precedence, first match wins:

| Condition | Buttons |
|---|---|
| `m.inputMode == inputPluginDiff` | `Approve` · `Reject` |
| `m.inputMode == inputPluginReview` | `Copy` |
| `m.chatOpen` | `Input` · `Prev cite` · `Next cite` |
| otherwise | `Find & replace` · `AI tools` · `Ask` · `AI search` |

`barButton` carries a label, an action identifier, and an `enabled` flag.

Bar row 0 is always the toggle rule and is not part of `barButtons()`; the view
and the row→action mapping both account for the offset.

Because the button set drives the bar height, changing modes changes
`m.treeHeight`. Every transition into or out of diff, review, and chat modes
already calls `recalcLayout()` or is reachable from a path that does; any that
does not gets one added.

## Actions

Each button invokes exactly the same code path as its keyboard equivalent.

| Button | Existing key | Extracted method |
|---|---|---|
| Find & replace | `Ctrl+R` | `startFindReplace()` |
| AI tools | `Ctrl+G` | `openShortcutPicker()` |
| Ask | `Ctrl+K` | `toggleChat()` |
| AI search | `Ctrl+T` | `openVaultSearch()` |
| Approve | `y` (diff) | `acceptDiff()` |
| Reject | `n` (diff) | `rejectDiff()` |
| Copy | `c` (review) | `copyReview()` |
| Input | `i` (chat view mode) | `chatFocusInput()` |
| Prev cite / Next cite | — (new) | `jumpCitation(delta)` |

The bodies of those cases move out of the `Update` key switch and out of
`handlePluginDiff` / `handlePluginReview` / `handleChatPanel` into named
methods on `*model` in a new `actions.go`. The key switch then calls them. This
is behavior-preserving for the keyboard and keeps the two entry points from
drifting; it also removes bulk from `model.go`, which is 2561 lines.

### Enablement

A button whose guard makes it a silent no-op renders dim and ignores clicks and
Enter:

- `Find & replace` and `AI tools` require `m.currentFile != "" || m.newNoteDir != ""`.

Buttons whose guard produces a user-visible message stay live so the message
still reaches the user — `AI search` without a configured embedder sets
`"Configure embedding_provider in config.toml"`, exactly as `Ctrl+T` does today.

`Prev cite` / `Next cite` render dim when the chat has no citation-bearing
assistant turn yet.

### Dim during modal overlays

While a modal input mode other than diff and review is active
(`inputVaultSearch`, `inputShortcutSelect`, `inputCapture`, `inputRename`,
`inputFilter`, …), the whole bar renders dim and ignores clicks. The modal owns
the keyboard and mouse events are already dropped for those modes; dimming makes
the bar visibly inert rather than looking live and doing nothing.

## Mouse

`hitTestPanel` is unchanged. A new helper on the model splits the left column:

```go
func (m model) buttonBarRowAt(x, y int) (row int, ok bool)
```

It returns the bar-local row when the coordinate lands in the left column at
`localY >= m.treeHeight`, and `ok == false` otherwise (including when
`m.barHeight == 0`).

Two call sites use it:

1. `handleMouseMsg` — a `treePanel` hit with `localY >= m.treeHeight` routes to
   `handleButtonBarMouse` instead of `handleTreeMouse`.
2. `Update`'s `tea.MouseMsg` branch — for `inputPluginDiff` / `inputPluginReview`
   the bar is checked before falling through to `handlePaneMouse`, which
   currently claims every mouse event in those modes.

Only left-button press activates. Wheel events over the bar are ignored.

Clicking bar row 0 toggles `m.buttonsCollapsed` and calls `recalcLayout()`.

## Keyboard Focus

New model state, all per-session:

```go
buttonsCollapsed bool // bar minified to the toggle row
buttonFocused    bool // keyboard focus is on the bar
buttonCursor     int  // 0 = toggle row, 1..N = buttons
```

### Tab cycle

- Ordinary modes: tree → bar → editor → tree. The bar is skipped when
  `m.barHeight == 0`, preserving today's tree ↔ editor cycle when the tree is
  hidden.
- Diff and review: right pane → left pane → bar → right pane. `Tab` currently
  flips `paneFocus` between two values; it gains the bar as a third stop.

### While focused

| Key | Effect |
|---|---|
| `↑` / `↓` | Move `buttonCursor` within the bar's rows (no wrap) |
| `Enter` / `Space` | Activate the row under the cursor |
| `Esc` | Drop focus, return it to the file tree |
| `Tab` | Continue the focus cycle |
| anything else | Swallowed |

Swallowing other keys matters: without it, a keystroke while the bar is focused
would fall through to `handleEditorKeys` and type into the buffer.

The focused row renders with the existing `treeSelectedStyle` inverse
highlight, matching how the tree marks its cursor. Focus is dropped whenever
the bar's height goes to zero (tree hidden via `Ctrl+B`, or the terminal
narrows past the tree threshold).

Entering a modal input mode drops bar focus, since the bar is inert there.

## Citation Jump

New state: `chatCiteCursor int`, indexing into the citations of the most recent
assistant turn that has any — the same list `mostRecentCitation` already walks.

- `0` means "no citation selected yet". The first `Next cite` click opens
  citation 1; the first `Prev cite` click opens the last citation.
- Subsequent clicks move by ±1 with wraparound within the list.
- Each move immediately opens the cited file at the cited line, reusing the
  existing `1`–`9` open path — including its `isDirty()` guard, which detours
  through `inputUnsavedGuard` before switching files.
- The cursor resets to `0` when a new assistant turn is appended, so a fresh
  answer starts from its own first citation.

No new chat *key* bindings. The buttons are the requested affordance and are
Tab-reachable.

## Files

| File | Change |
|---|---|
| `buttons.go` | New. `barButton`, `barButtons()`, `buttonBarHeight`, `buttonBarView`, `buttonBarRowAt`, `handleButtonBarMouse`, focus-key handling |
| `actions.go` | New. Extracted action methods shared by keys and buttons |
| `model.go` | New state fields; `recalcLayout` bar height; `View` left-column join; `Update` Tab cycle, focus keys, mouse routing; key cases delegate to `actions.go` |
| `mouse.go` | Left-column split routing in `handleMouseMsg` |
| `tree.go` | Unchanged (already renders to a fixed height) |
| `plugin_diff.go`, `plugin_review.go` | `Tab` gains the bar as a focus stop; `y`/`n`/`c` delegate to `actions.go` |
| `help_modal.go` | Document the bar and its focus keys |
| `README.md` | Document the bar |

## Testing

New `buttons_test.go`, following the repo's existing table-test style:

- `buttonBarHeight` across collapsed, expanded, and `treeWidth == 0`.
- `barButtons()` set selection per mode, including precedence when both
  `chatOpen` and a diff mode are somehow active.
- Row → action mapping, including the toggle at row 0 and out-of-range rows.
- `buttonBarRowAt` at the tree/bar boundary, the bar's last row, the status bar
  row, and with `barHeight == 0`.
- Tab cycle transitions in ordinary, diff, and review modes.
- Focused-key handling: cursor clamping, activation, `Esc`, and that an
  unrelated key does not reach the editor.
- Citation cursor: first next, first prev, wraparound both directions, reset on
  a new turn, and the dirty-buffer detour.
- Enablement: dim when no file is open, dim during modal overlays, and that a
  dim button ignores clicks.

Extending `model_test`-style coverage:

- `recalcLayout` shrinks `treeHeight` by the bar height and the left column
  still totals `height - 2`.
- Collapsing the bar returns rows to the tree.
- A tree with more items than the reduced height still scrolls rather than
  overlapping the bar.

`go test ./...` and `go vet ./...` must pass.
