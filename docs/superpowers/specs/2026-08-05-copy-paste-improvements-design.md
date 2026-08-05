# Copy/Paste Improvements — Design

Task P12. Improve clipad's copy and paste behaviour: auto-copy highlighted
text, give every copy visible confirmation, and make terminal-originated paste
behave like the app's own paste.

## Background

The task asked for three things:

1. Bind copy to both `Ctrl+C` and `Ctrl+Shift+C`.
2. Bind paste to both `Ctrl+V` and `Ctrl+Shift+V`.
3. Auto-copy highlighted text to the clipboard, with a small green
   confirmation in the status bar's right slot.

Items 1 and 2 cannot be implemented as literally stated. Two independent
blockers:

- **Terminal emulators intercept the keys.** `Ctrl+Shift+C` and
  `Ctrl+Shift+V` are the terminal's own copy/paste bindings (kitty, Alacritty,
  GNOME Terminal, Konsole, WezTerm). They are consumed locally and never
  delivered to the application.
- **Bubble Tea v1.3.10 has no key name for them.** `key.go:311-330` defines
  only `ctrl+shift+home`, `end`, `up`, `down`, `left`, and `right`. Receiving
  `Ctrl+Shift+<letter>` requires the kitty keyboard protocol or
  `modifyOtherKeys`, neither of which Bubble Tea v1 negotiates.

What the keys do today:

- `Ctrl+Shift+C` copies the *terminal's* screen selection. Clipad's highlight
  is an application-internal concept the terminal knows nothing about, so this
  copies nothing and reads as broken.
- `Ctrl+Shift+V` pastes as a **bracketed paste**, which *does* reach clipad.
  Bubble Tea delivers it as a single `KeyMsg{Type: KeyRunes, Paste: true}`
  carrying every rune including newlines
  (`key_sequences.go:83-119`). Clipad currently handles it in the generic
  typing path (`selection.go:658`), so it coalesces into the surrounding
  `editKindTyping` undo group — one `Ctrl+Z` after a paste also undoes the
  typing around it.

Item 3 makes item 1 moot: once every highlight lands on the system clipboard
automatically, there is nothing left for `Ctrl+Shift+C` to do. Item 2 has real
work behind it: make bracketed paste an atomic undo step.

## Scope

**In scope**

1. Auto-copy the editor selection to the system clipboard when a mouse drag
   ends.
2. A green `Copied` flash in the status bar's right slot, shown for auto-copy,
   `Ctrl+C`, and `Ctrl+X`.
3. Bracketed paste handled as its own atomic undo operation.

**Out of scope**

- Binding `Ctrl+Shift+C` (not achievable — see Background).
- OSC 52 clipboard fallback for SSH/container environments.
- Auto-copy on keyboard selection (`shift+arrow`, `Ctrl+A`). Every extension
  keystroke would spawn a `wl-copy`/`xclip` process and re-trigger the flash.
  Keyboard selections still copy with `Ctrl+C`.
- Double-click word selection. Clipad has no double-click handling today; this
  design does not add it.

## Components

### 1. Copy flash state

**Files:** `clipboard.go`, `model.go`

Mirrors the existing `autoSaveFlash` (`autosave.go`) and `gitSyncFlash`
(`git_sync.go`) pattern:

- `model.copyFlash bool`
- `copyFlashMsg` message type
- `copyFlashTick() tea.Cmd` returning `tea.Tick(2*time.Second, ...)`, matching
  the 2-second fade of the auto-save and git-sync flashes
- An `Update` case for `copyFlashMsg` that clears `copyFlash`

The tick and message live in `clipboard.go`, which is small and already owns
clipboard concerns.

`clipboard.go` also gains a test seam so the suite can observe copies without
writing to the real clipboard:

```go
// writeClipboard writes text to the system clipboard. Indirected through a
// variable so tests can observe copies without clobbering the real clipboard.
var writeClipboard = clipboard.WriteAll
```

`copyToClipboard` (`selection.go:381`) calls `writeClipboard` instead of
`clipboard.WriteAll` directly. This follows the pattern already used for image
clipboard I/O (`runWithStdin`, `lookPath`, `clipEnvForPaste`, stubbed in
`image_copy_test.go:13-24`).

A single model helper is the only way callers set the flash:

```go
// flashCopied shows the green "Copied" confirmation and returns the tick that
// clears it.
func (m *model) flashCopied() tea.Cmd
```

**Status bar rendering.** `View()` (`model.go:2257`) currently resolves the
flash slot as `autoSaveFlash` then `gitSyncFlash`. That inline chain moves into
a pure helper so the priority is directly testable:

```go
// flashText resolves which non-error flash the status bar shows.
func flashText(copyFlash, autoSaveFlash bool, gitSyncFlash string) string
```

`copyFlash` is checked first — it acknowledges an explicit user action, where
the other two are background events the user did not just trigger. It renders through the existing `statusFlashStyle`
(foreground 76, green) in the same right-hand slot as `Auto-saved` and
`Synced`, so no new styling is introduced.

The writing-metrics segment (`sel: N words · N chars`) is unaffected: `View()`
composes metrics ahead of the flash/filename segment, so both remain visible.

### 2. Auto-copy on drag release

**File:** `mouse.go`

`handleEditorMouse` handles `MouseActionRelease` at `mouse.go:222` by calling
`m.editor.EndMouseDrag()` and returning a nil command. It gains: when the
release leaves an active selection whose text is non-empty, copy it and return
`m.flashCopied()`.

`EndMouseDrag` (`selection.go:298`) already clears the selection when the
cursor never left the anchor, so a plain click cannot trigger a copy. The
non-empty text check guards the remaining degenerate case where a selection is
active but spans nothing.

Copy happens once, on release. Nothing is written to the clipboard during drag
motion, so a long drag costs exactly one `wl-copy`/`xclip` invocation.

### 3. Flash on explicit copy and cut

**File:** `model.go`

The `ctrl+c` (`model.go:752`) and `ctrl+x` (`model.go:772`) editor branches
return `m.flashCopied()` when a selection was actually copied. Both
`editor.Copy()` and `editor.Cut()` are no-ops without a selection, so the
active-selection check must happen before calling them — `Cut` clears
`selActive` as part of its work.

Untouched:

- The image-element branch in both handlers returns early with its own
  `Copied image to clipboard` message.
- The tree panel's file copy/cut keeps its existing `Copied: <name>` /
  `Cut: <name>` messages.

### 4. Bracketed paste as an atomic operation

**File:** `selection.go`

`Paste()` splits so the insert logic can be reused without a clipboard read:

```go
// PasteText inserts text at the cursor as a single undo operation, replacing
// any active selection first.
func (e *SelectableEditor) PasteText(text string)

// Paste inserts the clipboard contents. Unchanged behaviour.
func (e *SelectableEditor) Paste() tea.Cmd
```

`Paste()` becomes `PasteText(e.readFromClipboard())` with the existing
empty-text short circuit preserved.

`HandleKey` gains an early branch, before the generic key switch:

```go
if msg.Type == tea.KeyRunes && msg.Paste {
    e.PasteText(string(msg.Runes))
    return nil
}
```

This routes the paste through `recordOp`/`commitOp`, which use `editKindOp`.
Per `undo.go:32-40`, `editKindOp` always starts a new undo group, so a
terminal paste becomes its own `Ctrl+Z` step instead of merging into
surrounding typing.

**Routing.** A bracketed paste's `msg.String()` is the pasted text wrapped in
square brackets (`key.go:73-85`), so it matches no case in the model's key
switch and falls through to `m.editor.HandleKey` — either via
`handleEditorKeys` (`model.go:1137`) or via the tree panel's printable-input
auto-switch (`model.go:1084`). Both reach the new branch.

**Limitation.** A terminal paste can only carry text. `Ctrl+V` additionally
inspects the clipboard for image data and inserts an image element
(`pasteImageOrText`, `model.go:406`); `Ctrl+Shift+V` cannot, because the
terminal has already converted the clipboard to a rune stream by the time
clipad sees it. Pasting images requires `Ctrl+V`.

## Data flow

Auto-copy:

```
MouseMsg(release, editor panel)
  → handleEditorMouse
  → editor.EndMouseDrag()          clears selection if no drag occurred
  → selActive && SelectedText()≠"" ?
      → editor.Copy()              writes system clipboard + textClip
      → m.flashCopied()            sets copyFlash, returns tick
  → View(): sb.flashMsg = "Copied" rendered in statusFlashStyle
  → copyFlashMsg after 2s          clears copyFlash
```

Terminal paste:

```
KeyMsg{KeyRunes, Paste:true}
  → model key switch (no match, String() is "[...]")
  → handleEditorKeys → editor.HandleKey
  → PasteText(runes)               recordOp → replace selection → insert → commitOp
```

## Error handling

- `clipboard.WriteAll` failure is already swallowed by
  `copyToClipboard` (`selection.go:381`), which also stores the text in the
  in-process `textClip` fallback. This design does not change that: the flash
  confirms the copy was attempted and the internal buffer is populated, which
  is what `Ctrl+V` within clipad relies on.
- An empty selection produces no clipboard write and no flash.
- An empty bracketed paste inserts nothing; `commitOp` reverts the undo push
  when content is unchanged.

## Testing

**`mouse_test.go`**

- Press → motion → release copies the selection and sets `copyFlash`.
- Press → release with no motion copies nothing and sets no flash.
- Release with an active but empty selection sets no flash.

**`selection_test.go`**

- `PasteText` inserts at the cursor as one undo step.
- Type, then bracketed paste, then undo → only the pasted text is removed.
- Bracketed paste over an active selection replaces it, and a single undo
  restores both the selection text and removes the paste.

**`statusbar_test.go`**

- `Copied` renders in the right slot using the green flash style.
- `copyFlash` takes priority over `Auto-saved` and `Synced`.

**Regression:** `go test ./...` stays green.
