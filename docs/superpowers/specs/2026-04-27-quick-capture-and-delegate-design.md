# Quick capture + delegate-to-new-note

Date: 2026-04-27

Zero-friction "dump a thought without losing context" flow, plus the
natural follow-up: split selected text out into a new sibling note.

## Goals

1. **Quick capture** (`Ctrl+J`): from anywhere in the app, pop a small
   modal, type a thought, hit Enter — the text gets appended to
   `<vault>/inbox.md` (creating the file if missing) prefixed with a
   timestamp. Modal closes; the user is back in exactly the same file at
   the same cursor position in the same mode (editor or tree). Esc cancels
   without writing. `Shift+Enter` inserts a newline; plain `Enter`
   submits.
2. **Delegate selected text** (`Ctrl+O`): with a selection active in the
   editor, hit a keybinding, type a filename in a status-bar prompt — the
   selection is cut from the current note and written to a new note in
   the same directory. Source file is auto-saved as part of the operation.

Both features ship under one spec because they share a conceptual frame
("park text without losing context") and the same module (`capture.go`).
They have no shared code paths beyond the file-IO helper style.

## Non-goals

- A general-purpose templating system for inbox entries.
- A multi-target capture (e.g., capturing into a chosen note rather than
  always `inbox.md`). The override is at the config level
  (`inbox_path`), not per-capture.
- Generalizing the watcher to reload arbitrary externally-changed open
  files. The capture flow handles its own reload; broader watcher work
  is out of scope.
- Symlink rejection or vault-sandboxing for `inbox_path`. Absolute paths
  are explicitly allowed.

## Keybindings

| Key | Action |
|-----|--------|
| `Ctrl+J` | Open quick-capture modal (any mode) |
| `Ctrl+O` | Open delegate-to-new-note prompt (editor mode, selection required) |

`Ctrl+Shift+N` was the user-suggested binding for capture; it was rejected
because Ctrl+Shift bindings have a poor track record in this codebase
(commit `d7eba8d` migrated `Ctrl+Shift+F`/`Ctrl+Shift+A` to
`Ctrl+T`/`Ctrl+K` for that reason). `Ctrl+J` is byte-equivalent to
Enter on most terminals — the user has confirmed it works in their
primary terminal. If it turns out to fire on every Enter press in some
terminal, the binding can be swapped without touching any of the rest
of the design.

## Configuration

A new `inbox_path` field in `~/.config/clipad/config.toml`:

```toml
inbox_path = "inbox.md"           # default — vault-relative
# inbox_path = "journals/inbox.md" # vault-relative subpath
# inbox_path = "/tmp/inbox.md"    # absolute
# inbox_path = "~/scratch.md"     # ~-expanded
```

Resolution rules (`resolveInboxPath(vault, configValue)`):
- Empty / missing → `inbox.md` (relative).
- Starts with `~` → `os.UserHomeDir()` substitution, treated as absolute.
- `filepath.IsAbs` → used as-is, cleaned.
- Otherwise → joined with vault root.

## Module layout

One new file: `capture.go` (state, handlers, IO helpers, view) and its
peer `capture_test.go`. Two new `inputMode` constants, two new fields
on the `model` struct, two new tea.Msg types. Small wiring edits in
`model.go` and `config.go`.

```
capture.go
├── pure helpers
│   ├── resolveInboxPath(vault, configValue) string
│   ├── formatCaptureLine(now time.Time, text string) string
│   ├── appendToInboxFile(path, line string) error
│   └── writeNewFile(path, content string) error
├── handlers
│   ├── handleCapture(msg tea.KeyMsg) (model, tea.Cmd)
│   └── handleDelegate(msg tea.KeyMsg) (model, tea.Cmd)
├── views
│   └── captureView(input string, w, h int) string
└── messages
    └── captureAppendedMsg{ err error; inboxPath string; reloadOpen bool }
```

Delegate doesn't need an async tea.Msg — the cut + save are quick
local operations and the existing `DeleteSelection()` /
`saveCurrentFile()` already do the work synchronously. See the
delegate flow section below.

## Data structures

### Config (`config.go`)

```go
type Config struct {
    Vault              string     `toml:"vault"`
    InboxPath          string     `toml:"inbox_path,omitempty"`  // NEW
    GitRemote          string     `toml:"git_remote,omitempty"`
    LastSync           *time.Time `toml:"last_sync,omitempty"`
    AIShortcutProvider string     `toml:"ai_shortcut_provider,omitempty"`
    EmbeddingProvider  string     `toml:"embedding_provider,omitempty"`
    EmbeddingModel     string     `toml:"embedding_model,omitempty"`
    OllamaURL          string     `toml:"ollama_url,omitempty"`
}
```

Mirrored on `configTOML`. No load-time defaulting — the empty string is
preserved through to `resolveInboxPath`, which substitutes `inbox.md`.
This keeps the default tracking the current vault rather than freezing
at config-load time.

### Model fields (added to `model` in `model.go`)

```go
inboxPath     string           // raw config value; "" → default "inbox.md"
captureInput  textarea.Model   // multi-line, Shift+Enter newline
delegateInput textinput.Model  // single-line filename
```

No delegate-context struct: the source path comes from `m.currentFile`
(stable while the modal is open — modal blocks panel switches and
file opens), and the selected text comes from
`m.editor.SelectedText()` at Enter time. Selection state in the
editor is also stable while the delegate modal is open, since keys
go to the modal's textinput (the editor is blurred).

`inboxPath` is the *raw* config value (not resolved to absolute), stored
on the model so the rest of the code doesn't reach into the config
file at runtime. Resolution to an absolute path happens at use time
via `resolveInboxPath(m.vault, m.inboxPath)`. The `newModel`
constructor signature is extended to accept the value at startup —
matches how `activeShortcutProvider` is already passed in.

### Input mode constants

```go
inputCapture
inputDelegateName
```

Added to the existing `inputMode` enum in `model.go`. Both are routed
through `handleInputMode`.

### Widget initialization (in `newModel()`)

```go
ci := textarea.New()
ci.Placeholder = "Quick capture (Enter to save, Shift+Enter for newline, Esc to cancel)"
ci.CharLimit = 0
ci.SetWidth(56)
ci.SetHeight(6)
ci.ShowLineNumbers = false
m.captureInput = ci

di := textinput.New()
di.Placeholder = "filename (no .md needed)"
di.CharLimit = 200
di.Prompt = "Move to: "
m.delegateInput = di
```

## Capture flow (`Ctrl+J`)

### Open

```go
case "ctrl+j":
    if m.vault == "" {
        m.errMsg = "no vault configured"
        return m, nil
    }
    m.inputMode = inputCapture
    m.captureInput.Reset()
    cmd := m.captureInput.Focus()
    return m, cmd
```

No state on the underlying editor/tree is touched. Closing later via
Esc/Enter just sets `inputMode = inputNone`; the user is back exactly
where they were.

### Key handler

```go
func (m model) handleCapture(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc":
        m.inputMode = inputNone
        m.captureInput.Blur()
        return m, nil

    case "enter":  // plain Enter submits
        text := strings.TrimRight(m.captureInput.Value(), "\n")
        m.inputMode = inputNone
        m.captureInput.Blur()
        if strings.TrimSpace(text) == "" {
            return m, nil  // empty capture is a silent cancel
        }
        return m.dispatchCapture(text)
    }
    // Shift+Enter and all other keys fall through to textarea.Update,
    // which inserts a newline for Shift+Enter.
    var cmd tea.Cmd
    m.captureInput, cmd = m.captureInput.Update(msg)
    return m, cmd
}
```

### Dispatch — branching on inbox state

```go
func (m model) dispatchCapture(text string) (tea.Model, tea.Cmd) {
    line := formatCaptureLine(time.Now(), text)
    inboxPath := resolveInboxPath(m.vault, m.inboxPath)

    if m.currentFile == inboxPath {
        if m.isDirty() {
            // Inbox is open with unsaved edits: append in-memory only.
            m.appendLineToEditor(line)
            return m, nil  // editor stays dirty; user saves with Ctrl+S
        }
        // Inbox open and clean: disk write, then reload editor on completion.
        return m, captureAppendCmd(inboxPath, line, true)
    }
    // Inbox not open: just disk write.
    return m, captureAppendCmd(inboxPath, line, false)
}
```

`appendLineToEditor` reads `m.editor.Value()`, ensures it ends in `\n`
(prepends one if not), appends `line + "\n"`, calls `m.editor.SetValue`,
then restores the cursor at its original `(row, col)` clamped to the
new content bounds. The editor's existing dirty-tracking mechanism
flags the buffer as dirty automatically when content changes.

### Async append

```go
func captureAppendCmd(inboxPath, line string, reloadOpen bool) tea.Cmd {
    return func() tea.Msg {
        if err := appendToInboxFile(inboxPath, line); err != nil {
            return captureAppendedMsg{err: err}
        }
        return captureAppendedMsg{inboxPath: inboxPath, reloadOpen: reloadOpen}
    }
}
```

### Update handler

```go
case captureAppendedMsg:
    if msg.err != nil {
        m.errMsg = "capture failed: " + msg.err.Error()
        return m, nil
    }
    if msg.reloadOpen && m.currentFile == msg.inboxPath {
        if data, err := os.ReadFile(msg.inboxPath); err == nil {
            line, col := editorCursorPos(m.editor)
            m.editor.SetValue(string(data))
            m.cleanContent = string(data)  // editor stays clean
            m.editor.MoveTo(line, col)      // clamps internally
        }
    }
    // No tree refresh here — the watcher's debounced fileChangedMsg fires
    // shortly and refreshes the tree (esp. if inbox.md was just created).
    return m, nil
```

### View

In `View()`, alongside existing modal branches:

```go
} else if m.inputMode == inputCapture {
    modal := captureView(m.captureInput.View(), m.editorWidth, m.editorHeight)
    rightView = lipgloss.Place(m.editorWidth, m.editorHeight,
        lipgloss.Center, lipgloss.Center, modal)
}
```

`captureView` renders a bordered ~60×8 box with a title line
`Quick capture →  <inbox path>` over the textarea, in the same lipgloss
style family as `vaultSearchView`.

### Observable behavior

| State at `Ctrl+J` | At Enter |
|---|---|
| Inbox not open | Disk write only. Watcher fires `fileChangedMsg` → tree refresh. |
| Inbox open, clean | Disk write + editor reload. Cursor preserved. Editor stays clean. |
| Inbox open, dirty | In-memory append to editor buffer. Editor stays dirty. No disk write. |
| Esc | Modal closes. No state change. |
| Enter on empty / whitespace-only input | Silent cancel. |

## Delegate flow (`Ctrl+O`)

### Open

```go
case "ctrl+o":
    if m.activePanel != editorPanel || m.currentFile == "" {
        m.errMsg = "open a file in the editor first"
        return m, nil
    }
    if !m.editor.selActive || m.editor.SelectedText() == "" {
        m.errMsg = "select text first"
        return m, nil
    }
    m.inputMode = inputDelegateName
    m.delegateInput.Reset()
    cmd := m.delegateInput.Focus()
    return m, cmd
```

(`selActive` is a private field on `SelectableEditor` — accessed
directly because `capture.go` is in the same package. If the
implementer prefers, a tiny `HasSelection() bool` wrapper method can be
added to `selection.go`; functionally equivalent.)

No snapshotting needed: the editor's selection state is stable while
the delegate modal is open. The textinput consumes all key events and
the editor is blurred, so neither selection nor cursor in the editor
can move. `m.currentFile` is also stable — the modal blocks panel
switching and file opens. So at Enter time, both the source path and
the selection content are exactly what they were at `Ctrl+O` time.

### Key handler

```go
func (m model) handleDelegate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc":
        m.inputMode = inputNone
        m.delegateInput.Blur()
        return m, nil

    case "enter":
        raw := strings.TrimSpace(m.delegateInput.Value())
        if raw == "" {
            return m, nil  // ignore; user keeps typing
        }
        name := raw
        if filepath.Ext(name) == "" {
            name += ".md"  // A1: auto-append if no extension
        }
        if strings.ContainsAny(name, "/\\") {
            m.errMsg = "filename only — no slashes"
            return m, nil
        }
        srcDir := filepath.Dir(m.currentFile)
        dstPath := filepath.Join(srcDir, name)

        if _, err := os.Stat(dstPath); err == nil {
            m.errMsg = "file exists: " + name
            return m, nil
        }

        // Get the selection text now (will be lost after DeleteSelection).
        selText := m.editor.SelectedText()

        // 1. Write the new file atomically (O_CREATE|O_EXCL).
        if err := writeNewFile(dstPath, ensureTrailingNewline(selText)); err != nil {
            m.errMsg = "delegate failed: " + err.Error()
            return m, nil
        }

        // 2. Cut the selection from the editor. DeleteSelection mutates
        //    the editor in place, integrates with the existing undo
        //    history, and moves the cursor to the start of where the
        //    selection used to be.
        m.editor.DeleteSelection()

        // 3. Persist the editor to disk. saveCurrentFile signals failure
        //    via m.errMsg (no return value); after success isDirty is
        //    false. Note: even if this fails, the new file is already
        //    on disk, so no text is lost — the user can Ctrl+S to retry.
        m.errMsg = ""
        m.saveCurrentFile()

        m.inputMode = inputNone
        m.delegateInput.Blur()
        return m, nil
    }

    var cmd tea.Cmd
    m.delegateInput, cmd = m.delegateInput.Update(msg)
    return m, cmd
}
```

`writeNewFile` uses `O_CREATE|O_EXCL` for an atomic create-only write —
defends against a race where the target file appeared between the
`os.Stat` collision check and the actual write.

The cursor lands at the start of where the selection used to be — that
behavior is built into `DeleteSelection()`. The new file is **not**
opened automatically (per the brainstorming decision: keeps the user in
flow). The watcher will refresh the tree shortly so the new note becomes
visible there.

### Failure modes

| Failure point | Outcome |
|---|---|
| `os.Stat` says target exists | No filesystem change. Modal stays open. |
| `writeNewFile` fails (permissions, disk full) | No filesystem change. Modal closes? Actually: the handler aborts and modal stays open. Refining: stay in `inputMode == inputDelegateName` so the user can retry with a different name. |
| `DeleteSelection` (after new file written) | Can't fail — pure in-memory mutation. |
| `saveCurrentFile` fails | New file exists on disk; editor reflects post-cut state but isn't persisted. `errMsg` has the reason; `isDirty()` is true. User can `Ctrl+S` to retry. No data lost (selection content is in the new file). |

### View — status-bar inline (not a centered modal)

In `View()`, the status bar already branches by `inputMode`:

```go
} else if m.inputMode == inputDelegateName {
    srcDir := filepath.Dir(m.currentFile)
    statusBar = "Move to " + srcDir + string(filepath.Separator) +
                m.delegateInput.View()
}
```

Same one-line treatment as new-folder, rename, and git-remote prompts.
The editor stays fully visible underneath, including the active
selection — useful as visual confirmation of what's about to be cut.

### Observable behavior

| State at `Ctrl+O` | Result |
|---|---|
| No selection / no file open / wrong panel | Status flash, no modal opens. |
| Selection + clean source | Cut from editor + create new note + save source. Cursor → former selection start. |
| Selection + dirty source | Cut from editor (carries any prior dirty edits) + create new note + save source. The single end-of-flow save also persists the unrelated dirty edits. |
| Filename has `/` or `\` | Refuse, status flash, modal stays open. |
| Filename target exists | Refuse, status flash, modal stays open. |
| Filename without extension | `.md` auto-appended. |
| `writeNewFile` fails | New file not created. Editor not modified. `errMsg` set. Modal stays open. |
| `saveCurrentFile` fails (post-cut) | New file exists. Editor shows post-cut content but is dirty (not on disk). `errMsg` set. User can Ctrl+S to retry. |
| Esc | Modal closes. Selection on source editor remains as-is. |

## Pure file-IO helpers

### `resolveInboxPath`

```go
func resolveInboxPath(vault, configValue string) string {
    if configValue == "" {
        configValue = "inbox.md"
    }
    if strings.HasPrefix(configValue, "~") {
        if home, err := os.UserHomeDir(); err == nil {
            configValue = filepath.Join(home, strings.TrimPrefix(configValue, "~"))
        }
    }
    if filepath.IsAbs(configValue) {
        return filepath.Clean(configValue)
    }
    return filepath.Join(vault, configValue)
}
```

### `formatCaptureLine`

```go
func formatCaptureLine(now time.Time, text string) string {
    return fmt.Sprintf("- %s — %s",
        now.Format("2006-01-02 15:04"),
        text)
}
```

Local time, em-dash (U+2014) separator, minute precision per the task
example. Multi-line `text` (from `Shift+Enter` newlines) embeds literal
`\n`s as-is — the timestamp prefixes only the first line; subsequent
lines render as continuation under the bullet.

### `appendToInboxFile`

```go
func appendToInboxFile(path, line string) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return err
    }
    existing, err := os.ReadFile(path)
    if err != nil && !os.IsNotExist(err) {
        return err
    }
    var b strings.Builder
    b.Write(existing)
    if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
        b.WriteByte('\n')
    }
    b.WriteString(line)
    b.WriteByte('\n')
    return os.WriteFile(path, []byte(b.String()), 0o644)
}
```

Rules:
- File and parent dir created if missing.
- If existing file lacks a trailing `\n`, one is inserted before the
  new bullet.
- Result always ends in exactly one `\n`.
- Existing trailing blank lines (`\n\n`) are preserved as-is; the new
  bullet is appended after.

### `ensureTrailingNewline`

```go
func ensureTrailingNewline(s string) string {
    if s == "" || strings.HasSuffix(s, "\n") {
        return s
    }
    return s + "\n"
}
```

Used to ensure the new note (delegate target) ends in `\n`.

The actual cut from the source editor is done by the existing
`(*SelectableEditor).DeleteSelection()`, not by a new helper. Strict
character-range semantics are inherent in `DeleteSelection`'s
existing behavior — line-rounding was rejected as patronizing given
clipad's character-precise selection model, and the existing API
already implements strict cuts (see `selection.go::deleteText`).

### `writeNewFile`

```go
func writeNewFile(path, content string) error {
    f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
    if err != nil {
        return err  // os.ErrExist if file already exists
    }
    defer f.Close()
    _, err = f.WriteString(content)
    return err
}
```

## Why the watcher reload gap stays open

The current watcher (`watcher.go`) emits `fileChangedMsg` on any vault
change, but the model only refreshes the tree — never the editor for
the open file. This means external edits to the open file (e.g., from
git pull, or a separate process) don't reach the editor. The capture
flow handles its own reload (see the `captureAppendedMsg` handler), so
the user-visible promise is met for capture. Generalizing the watcher
to reload arbitrary externally-changed open files is an independent
piece of work — useful, but not required by this task.

## Edge cases not handled

- **Symlinks at inbox path**: followed (default Go fs behavior).
- **Inbox path with `..` escaping the vault**: allowed (absolute paths
  are explicitly permitted, so `..` is a strict subset of that).
- **Filesystem permission errors**: surfaced via `errMsg`. No retry.
- **Disk full / partial writes**: not specifically defended beyond Go's
  normal error returns.
- **Concurrent capture firing twice in rapid succession**: serialized
  through Bubble Tea's single-threaded Update loop. The async commands
  produce ordered messages; both appends will land, in order.

## Testing

### Unit tests (`capture_test.go`) — pure helpers

Project style matches `vault_search_test.go` and `git_sync_test.go`:
plain `testing.T`, `t.TempDir()` for fs, no testify, no table-test
framework — direct assertions.

**`resolveInboxPath`:**
- Empty config → `<vault>/inbox.md`
- Bare filename `foo.md` → `<vault>/foo.md`
- Subpath `journals/inbox.md` → `<vault>/journals/inbox.md`
- Absolute `/tmp/inbox.md` → `/tmp/inbox.md`
- `~`-prefixed → home-expanded
- `..` in path → cleaned

**`formatCaptureLine`:**
- Fixed `time.Time` → exact `"- 2006-01-02 15:04 — text"` string
- Em-dash literal byte sequence verified
- Multi-line `text` → embedded `\n`s preserved
- Empty `text` → safe (no panic)

**`appendToInboxFile`:**
- Missing file → file created with bullet + `\n`
- Missing parent dir → dir created
- Existing file ends in `\n` → no extra blank line
- Existing file lacks `\n` → newline inserted before bullet
- Existing file ends in `\n\n` → blank line preserved
- Two captures in sequence → both bullets in order, exactly one trailing `\n`
- File mode is `0o644`

**`ensureTrailingNewline`:**
- Empty string → empty string (no spurious `\n`)
- String already ending in `\n` → unchanged
- String not ending in `\n` → `\n` appended

**`writeNewFile`:**
- Target doesn't exist → file created
- Target exists → returns `os.ErrExist`
- Parent missing → returns error

### Integration tests — model level

Style matches `git_sync_test.go`: real fs, real model, no UI snapshots.

**Capture:**
- Closed inbox: `Ctrl+J` → type → Enter → file exists with one bullet.
- Open + clean inbox: file on disk gets new bullet; editor matches disk.
- Open + dirty inbox: file on disk unchanged; editor buffer has new
  bullet appended; editor remains dirty.
- Cursor preservation: editor at row 5 col 3, capture into open+clean
  inbox, cursor still at row 5 col 3.
- Esc cancels: no file change, no editor change.
- Empty Enter: silent cancel, no file write.

**Delegate:**
- Happy path: source on disk has cut applied; destination exists with
  selection content; editor reflects new source; cursor at former
  selection start; source editor is clean post-save.
- No selection (`selActive` false): refused with status flash, modal
  not opened.
- Collision (target file pre-exists): no filesystem change; source
  editor unchanged; modal stays open with `errMsg` set.
- Dirty source on entry: end-of-flow `saveCurrentFile` persists both
  the cut and any prior dirty edits in one write.
- Filename with `/` or `\`: refused; modal stays open.
- Filename without extension: `.md` auto-appended.
- Mid-line cut: strict character-range semantics (e.g., cutting `bar`
  from `foo bar baz` leaves `foo  baz` with double space).

### Manual smoke checklist

- Capture modal centers correctly at narrow + wide terminal widths.
- Status-bar delegate prompt readable when source path is long.
- `Ctrl+J` doesn't fire on plain Enter in the user's primary terminal.
- Tree refreshes within ~100ms after a capture creates `inbox.md`.

### Test fixture conventions

- Vault root: `t.TempDir()`.
- `time.Now` not globally mocked: `formatCaptureLine` takes `time.Time`
  as a parameter; tests pass deterministic instants.
- No fs mocking: real files in temp dirs.
- Standard `go test ./...` — no flags.

## Wiring summary (touch points in existing files)

**`model.go`:**
- 2 new `inputMode` constants (`inputCapture`, `inputDelegateName`).
- 3 new fields in `model` struct (`inboxPath`, `captureInput`, `delegateInput`).
- 2 new widget initializations in `newModel()` (`textarea` for capture,
  `textinput` for delegate); plus storing the passed-in `inboxPath`.
- `Ctrl+J` and `Ctrl+O` cases in main key dispatcher.
- `inputCapture` and `inputDelegateName` cases in `handleInputMode`.
- `captureAppendedMsg` case in main `Update` (delegate is fully
  synchronous, no message needed).
- 1 overlay branch in `View()` for capture; 1 status-bar branch for
  delegate.
- Extend `newModel` signature to accept `inboxPath string`; thread it
  from `main.go` (which calls `loadConfig().InboxPath`).

**`config.go`:**
- `InboxPath string` added to `Config` and `configTOML` structs.
- Load/save passthrough (no defaulting at load time — handled in
  `resolveInboxPath`).

**New: `capture.go`, `capture_test.go`** as described above.
