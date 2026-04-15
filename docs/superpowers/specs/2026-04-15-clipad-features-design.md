# Clipad Feature Batch: Move/Copy, Auto-Save, Text Selection, AI Shortcuts

**Date:** 2026-04-15
**Status:** Approved

## Overview

Four features added to clipad in a single batch. Each is independent but they interact in two places: text selection provides context for AI shortcuts, and Ctrl+C/X are context-dependent (tree panel = file operations, editor panel = text operations).

## Feature 1: Move/Copy Files (Cut/Copy/Paste in Tree)

### Keybindings

| Key | Panel | Action |
|-----|-------|--------|
| Ctrl+X | Tree | Cut selected file (mark for move) |
| Ctrl+C | Tree | Copy selected file (mark for copy) |
| Ctrl+V | Tree | Paste file into current location |

**Breaking change:** `Ctrl+C` is removed from quit. `Ctrl+Q` remains the sole quit binding.

### State

New `fileClipboard` struct on model:
- `path string` — source file path
- `op` — `clipCut` or `clipCopy`
- Zero value means empty clipboard

Only files can be cut/copied, not folders.

### Behavior

1. **Cut/Copy:** Stores the selected file's path and operation type. Status bar shows "Cut: filename" or "Copied: filename". Tree visually dims a cut file.
2. **Paste:** Determines target directory from current tree selection:
   - If selected node is a folder: paste into it
   - If selected node is a file: paste into its parent directory
   - If nothing selected: paste into vault root
3. **Move** (cut+paste): `os.Rename(src, dst)`. If the moved file was open in the editor, update `currentFile` to the new path.
4. **Copy** (copy+paste): Read source, write to destination.
5. **Name collision:** If target path exists, append ` (1)`, ` (2)`, etc. until a unique name is found.
6. **After paste:** Refresh tree, clear clipboard state.
7. **Paste without prior cut/copy:** No-op.

### New file

`clipboard.go` — `fileClipboard` struct, `handleCut()`, `handleCopy()`, `handlePaste()` methods, file copy helper.

## Feature 2: Auto-Save

### Mechanism

- `tea.Tick` command fires every 15 seconds, producing `autoSaveTickMsg`
- On tick: if a file is open (not a new unsaved note) and dirty, write to disk and update `cleanContent`
- Show "Auto-saved" in status bar for 2 seconds via `autoSaveFadeMsg` tick
- New unsaved notes (no filename yet) are skipped — user must write content and trigger manual Ctrl+S first
- On save error: show error in status bar (same as manual save errors), do not set flash

### State

- `autoSaveFlash bool` — when true, status bar shows "Auto-saved"

### Integration

- `Init()` adds the first auto-save tick to the batch: `tea.Batch(textarea.Blink, watchVault(m.vault), autoSaveTick())`
- Status bar rendering checks `autoSaveFlash` before rendering hints
- Reuses existing save-to-disk logic from `saveCurrentFile()`, extracted into a shared helper for the actual write operation

### New file

`autosave.go` — `autoSaveTickMsg`, `autoSaveFadeMsg`, `autoSaveTick()` command, `autoSaveFadeTick()` command, tick handler.

## Feature 3: Text Selection

### Architecture

`SelectableEditor` struct wrapping `textarea.Model` with selection state layered on top.

```
SelectableEditor
├── textarea.Model  (handles text editing, cursor, scrolling)
├── selAnchor       (line, col where selection started)
├── selActive       (bool)
├── textClipboard   (string, internal clipboard for copy/paste)
└── View()          (custom render when selection active, delegates to textarea otherwise)
```

### Keybindings

| Key | Action |
|-----|--------|
| Shift+Left/Right | Select character by character |
| Ctrl+Left/Right | Word-jump navigation (no selection) |
| Ctrl+Shift+Left/Right | Select word by word |
| Shift+Up/Down | Extend selection by line |
| Shift+Home/End | Select to start/end of line |
| Ctrl+A | Select all |
| Ctrl+C | Copy selection (editor panel, selection active) |
| Ctrl+X | Cut selection (editor panel, selection active) |
| Ctrl+V | Paste from clipboard (replaces selection if active) |
| Backspace/Delete | Delete selection |
| Any printable char | Replace selection with typed character |

### Ctrl+C/X Context Resolution

| Panel | Selection | Ctrl+C | Ctrl+X |
|-------|-----------|--------|--------|
| Tree | n/a | Copy file | Cut file |
| Editor | Active | Copy text | Cut text |
| Editor | Inactive | No-op | No-op |

### Clipboard

- **Write:** ANSI OSC 52 escape sequence to write to system clipboard. Works in most modern terminals without external dependencies.
- **Read/Paste:** Use `atotto/clipboard` library for cross-platform clipboard read on Ctrl+V. Fall back to internal `textClipboard` if system clipboard is unavailable.

### Rendering

When `selActive` is true:
- Render content manually instead of using `textarea.View()`
- Selected range highlighted with inverted colors (background swap)
- Line numbers rendered in same gray style as textarea
- Viewport offset tracked to render only visible lines
- Cursor position shown

When `selActive` is false:
- Delegate to `textarea.View()` as normal

### Word Boundary Detection

- Word = contiguous run of `[a-zA-Z0-9_]` characters
- Ctrl+Left: jump to start of previous word (skip whitespace/punctuation, then skip word chars)
- Ctrl+Right: jump to start of next word (skip word chars, then skip whitespace/punctuation)

### Key Implementation Detail

Shift+arrow and Ctrl+arrow keys are intercepted in `handleEditorKeys` before reaching textarea. The SelectableEditor processes them:
1. Record anchor on first Shift press (if not already active)
2. Move cursor via textarea methods
3. Update selection range
4. On any non-shift movement key: clear selection, revert to normal textarea rendering

### New file

`selection.go` — `SelectableEditor` struct, selection tracking, key interception, custom rendering, word boundary detection, clipboard operations (OSC 52 write, atotto/clipboard read).

## Feature 4: AI Shortcuts

### Data Model

```go
type AIShortcut struct {
    Name   string `toml:"name"`
    Prompt string `toml:"prompt"`
}

type AIShortcutsConfig struct {
    Shortcuts []AIShortcut `toml:"shortcuts"`
}
```

Stored at `~/.config/clipad/ai_shortcuts.toml`:

```toml
[[shortcuts]]
name = "Fix grammar"
prompt = "Fix grammar and spelling errors in this text. Return only the corrected text."

[[shortcuts]]
name = "Summarize"
prompt = "Summarize this text concisely. Return only the summary."
```

### Keybindings

| Key | Action |
|-----|--------|
| Ctrl+G | Open AI shortcuts context menu |
| Ctrl+L | Open new AI shortcut form |

### Context Menu (Ctrl+G)

- Modal list rendered over the editor area (similar to plugin selector)
- Up/Down arrows to navigate, Enter to execute
- `e` on highlighted entry: edit (pre-fills form with existing name/prompt)
- `d` on highlighted entry: delete with y/n confirmation in status bar
- Esc to close
- Empty state: "No shortcuts. Press Ctrl+L to create one."

### Shortcut Form (Ctrl+L or `e` from menu)

Two-step status bar input:
1. "Shortcut name: " — text input, Enter to proceed
2. "Prompt: " — text input, Enter to save

For editing: fields pre-filled with existing values. Esc at any step cancels.

### Execution Flow

1. User selects shortcut from menu, presses Enter
2. Determine content: if text selection is active, use selected text; otherwise, entire file content
3. Load OpenRouter config from `~/.config/clipad/plugins/openrouter.toml`
4. If OpenRouter not configured: trigger existing plugin config flow first
5. HTTP POST to OpenRouter API:
   - System: "You are a text processing assistant. Apply the following instruction to the provided text. Return ONLY the processed text, nothing else."
   - User: "Instruction: {shortcut.Prompt}\n\nText:\n{content}"
6. Show "Processing..." in status bar
7. On result: show side-by-side diff (reuse existing `pluginDiff` infrastructure)
8. `y` to accept: replace selection or entire file content. If selection, clear selection after replacing.
9. `n` to reject: revert to original

### Input Modes

- `inputShortcutSelect` — context menu navigation
- `inputShortcutName` — name input field
- `inputShortcutPrompt` — prompt input field
- `inputShortcutDeleteConfirm` — delete y/n confirmation

### New files

- `shortcuts.go` — `AIShortcut` struct, TOML load/save, LLM execution (reuses OpenRouter HTTP logic)
- `shortcuts_input.go` — input handlers for menu, form, delete
- `shortcuts_modal.go` — context menu rendering

## Model Changes Summary

New fields on `model`:
```go
// File clipboard (cut/copy/paste)
fileClip fileClipboard

// Auto-save
autoSaveFlash bool

// Text selection (replaces editor textarea.Model)
editor SelectableEditor

// AI shortcuts
shortcuts        []AIShortcut
shortcutCursor   int
shortcutEditing  int          // -1 for new, >= 0 for editing index
shortcutNameInput  textinput.Model
shortcutPromptInput textinput.Model
```

New input modes added to `inputMode` enum:
```go
inputShortcutSelect
inputShortcutName
inputShortcutPrompt
inputShortcutDeleteConfirm
```

## Keybinding Changes Summary

| Key | Before | After |
|-----|--------|-------|
| Ctrl+C | Quit (with Ctrl+Q) | Copy file (tree) / Copy text (editor) |
| Ctrl+X | — | Cut file (tree) / Cut text (editor) |
| Ctrl+V | — | Paste file (tree) / Paste text (editor) |
| Ctrl+G | — | AI shortcuts menu |
| Ctrl+L | — | New AI shortcut form |
| Ctrl+Left/Right | — | Word-jump cursor movement |
| Shift+arrows | — | Text selection |
| Shift+Home/End | — | Select to line start/end |
| Ctrl+Shift+Left/Right | — | Select by word |
| Ctrl+A | — | Select all |

## New Dependencies

- `github.com/atotto/clipboard` — cross-platform clipboard read for Ctrl+V paste

## New Files

| File | Purpose |
|------|---------|
| `clipboard.go` | File cut/copy/paste in tree |
| `autosave.go` | 15s auto-save tick, flash message |
| `selection.go` | SelectableEditor, text selection, word-jump, clipboard |
| `shortcuts.go` | AI shortcut types, TOML persistence, LLM calls |
| `shortcuts_input.go` | Shortcut menu/form/delete input handlers |
| `shortcuts_modal.go` | Shortcut context menu rendering |
