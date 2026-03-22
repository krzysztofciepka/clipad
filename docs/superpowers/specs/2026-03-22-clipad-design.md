# Clipad — TUI Note-Taking App Design Spec

## Overview

Clipad is a terminal-based note-taking application written in Go that resembles Obsidian's layout. It provides a file tree on the left, a markdown editor on the right, and a status bar at the bottom. Notes are stored as `.md` files in a configurable vault directory.

## Technology Stack

- **Language:** Go
- **TUI framework:** Bubble Tea (Elm architecture)
- **Styling/layout:** Lipgloss
- **Editor:** Bubbles `textarea` component
- **Scrollable views:** Bubbles `viewport` component
- **Markdown rendering:** Glamour
- **Config parsing:** `pelletier/go-toml/v2`
- **Fuzzy matching:** `sahilm/fuzzy`

## Architecture

Follows Bubble Tea's Elm architecture: a single `model` holds all application state, `Update` processes messages (keypresses, window resizes, file I/O), and `View` renders the UI.

### Layout

```
┌─────────────────────────────────────────────────┐
│                   clipad                        │
├──────────────┬──────────────────────────────────┤
│              │                                  │
│  File Tree   │   Editor (raw) / Preview (md)    │
│  (25%)       │   (75%)                          │
│              │                                  │
│  > notes/    │   # My Note                      │
│    > daily/  │                                  │
│      jan.md  │   Some content here...           │
│    ideas.md  │                                  │
│  > projects/ │                                  │
│              │                                  │
├──────────────┴──────────────────────────────────┤
│ ^S save  ^N new  ^D delete  ^Q quit  Tab switch  ^P preview │
└─────────────────────────────────────────────────┘
```

Two panels: file tree (left, ~25% width) and editor/preview (right, ~75%). One panel is focused at a time. Bottom status bar is always visible.

## Core State (Model)

| Field | Type | Description |
|-------|------|-------------|
| `activePanel` | enum | Which panel has focus (tree or editor) |
| `editorMode` | enum | Edit or preview |
| `vault` | string | Root path to the vault directory |
| `fileTree` | tree struct | Recursive tree of vault contents |
| `currentFile` | string | Path of the currently open file |
| `editor` | `textarea.Model` | Multiline text buffer |
| `preview` | `viewport.Model` | Glamour-rendered markdown viewport |
| `dirty` | bool | Whether the buffer has unsaved changes |
| `filterInput` | `textinput.Model` | Fuzzy filter text input |
| `filtering` | bool | Whether filter mode is active |
| `confirmDelete` | bool | Whether delete confirmation is showing |
| `pendingAction` | enum/nil | Action deferred by unsaved-changes guard (switchFile, quit, delete) |
| `width` | int | Terminal width (updated on WindowSizeMsg) |
| `height` | int | Terminal height (updated on WindowSizeMsg) |

## Data Flows

### Open File
Select in tree -> read from disk -> load into textarea buffer -> clear dirty flag. On read error, show error message in the status bar and keep the previous file open.

### Save
`Ctrl+S` -> write textarea content to disk -> clear dirty flag -> refresh tree. On write error (permissions, disk full), show error in status bar; buffer is preserved.

### New Note
`Ctrl+N` -> prompt for filename (e.g. `daily/2026-03-22.md`) in bottom bar -> if file already exists, open the existing file instead of overwriting -> create intermediate directories if needed -> create file -> open in editor.

### Delete
`Ctrl+D` (tree panel only) -> show confirmation in bottom bar: "Delete {filename}? (y/n)" -> on `y`: delete from disk -> refresh tree -> open next file or clear editor. On `n` or `Esc`: cancel.

### Preview Toggle
`Ctrl+P` -> if in edit mode, render buffer through Glamour into viewport; if in preview, switch back to textarea.

### Filter
`/` (in tree panel) -> show filter input at top of tree -> fuzzy match all files by filename -> Enter opens selected match -> Esc cancels and restores tree. Global keybindings (`Ctrl+S`, `Ctrl+Q`, etc.) remain active during filter mode; only printable characters go to the filter input.

### Unsaved Changes Guard
When switching files or quitting with `dirty == true`, show prompt in bottom bar: "Unsaved changes. Save? (y/n/Esc)". `y` = save then proceed with pending action. `n` = discard changes then proceed. `Esc` = cancel and stay. The `pendingAction` field tracks what action to resume after the user responds.

### Error Handling
All file I/O errors are shown as a message in the status bar (non-blocking). The app never crashes on I/O errors — it shows the error and preserves current state. If the vault directory becomes inaccessible, a persistent error is shown and the tree displays empty.

## File Tree Component

- Recursive tree built by walking the vault directory
- Only `.md` files shown; hidden files/folders excluded
- Folders sorted first, then files, both alphabetical
- Enter on folder toggles expand/collapse
- Enter on file opens it in the editor
- Currently open file is visually highlighted
- Expanded/collapsed state kept in memory (not persisted)

### Fuzzy Filter
- `/` activates a text input at the top of the tree panel
- Filters all files (flattened, ignoring folder structure) by fuzzy match on filename
- Results shown as flat list while filtering
- Enter opens selected match, Esc exits filter mode

### New Note
- `Ctrl+N` opens a text input prompt in the bottom bar area
- User enters a path like `daily/2026-03-22.md`
- Missing intermediate directories are created automatically
- File is created empty and opened in the editor

## Editor & Preview

### Editor
- Uses Bubble Tea's `textarea` bubble
- Multiline text editing with cursor movement (arrows, home/end)
- Vertical scrolling when content exceeds viewport
- Indentation is done with spaces via the spacebar (Tab is reserved for panel switching)
- Cursor position shown in bottom bar (line:col)

### Preview
- Renders current textarea content through Glamour
- Displayed in a `viewport` bubble (scrollable, read-only)
- Preview is a snapshot of the buffer at the moment `Ctrl+P` is pressed (no live rendering)
- Arrow keys / j,k scroll in preview mode

## Keybindings

### Global (both panels)

| Key | Action |
|-----|--------|
| `Ctrl+S` | Save current file |
| `Ctrl+N` | New note |
| `Ctrl+Q` | Quit (with unsaved changes guard) |
| `Tab` | Switch focus between tree and editor |
| `Ctrl+P` | Toggle edit/preview mode |

Note: `Tab` is used instead of `Ctrl+M` because `Ctrl+M` produces the same byte as Enter in most terminals. `Tab` is unambiguous and not needed for markdown editing (indentation uses spaces).

### Tree Panel

| Key | Action |
|-----|--------|
| `/` | Activate fuzzy filter |
| `Enter` | Open file / toggle folder |
| `Ctrl+D` | Delete selected file (with confirmation) |
| Up/Down | Navigate entries |

`Ctrl+D` is tree-panel only to avoid conflicting with editor text input.

### Editor Panel
All normal text input passes through to the textarea. Arrow keys move the cursor.

### Bottom Bar Format
```
^S save  ^N new  ^D del  ^Q quit  Tab switch  ^P preview  [line:col] [filename] [modified]
```
`^D del` is only shown when the tree panel is focused.

## Configuration

### Location
`$XDG_CONFIG_HOME/clipad/config.toml` (defaults to `~/.config/clipad/config.toml` if `XDG_CONFIG_HOME` is unset).

### Fields
```toml
vault = "/home/user/notes"
# tab_width removed — indentation uses spacebar
```

### First Run
- App checks for config file at startup
- If missing: TUI prompt asking "Enter vault path:" with text input
- Validates path exists or offers to create it
- Writes config file and proceeds to main UI

## Project Structure

```
clipad/
├── main.go              # Entry point, config loading, first-run flow
├── config.go            # Config struct, read/write TOML, path validation
├── model.go             # Main Bubble Tea model, Update, View
├── tree.go              # File tree component (build, navigate, expand/collapse)
├── editor.go            # Editor wrapper around textarea bubble
├── preview.go           # Glamour rendering, viewport wrapper
├── filter.go            # Fuzzy filter logic for file tree
├── statusbar.go         # Bottom bar rendering
├── filetree_item.go     # Tree node struct (file/folder, children, expanded state)
├── go.mod
└── go.sum
```

Single `main` package. One file per concern.

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/bubbles` | textarea, viewport, textinput |
| `github.com/charmbracelet/lipgloss` | Styling and layout |
| `github.com/charmbracelet/glamour` | Markdown rendering |
| `github.com/pelletier/go-toml/v2` | Config file parsing |
| `github.com/sahilm/fuzzy` | Fuzzy matching for file filter |

## Window Resize

The layout recomputes panel widths and heights on every `tea.WindowSizeMsg`. The 25%/75% split is recalculated dynamically. Minimum usable terminal size is 60 columns by 15 rows — below this, a "terminal too small" message is shown instead of the UI.

## Known Limitations & Out of Scope (v1)

- **No rename/move:** Manage file renames outside the app. May be added in a future version.
- **No file watching:** Changes made outside the app (e.g. git pull) are not detected. The tree refreshes only after internal operations (save, new, delete).
- **Large files:** No file size limit is enforced. Very large files (>1MB) may cause slow rendering in the textarea. This is a known limitation of the textarea bubble.
- **Cursor state not preserved:** When switching between files, cursor resets to position 0,0. Scroll position is not retained.
- **Startup:** When launching into a vault with existing files, the editor starts empty with no file selected. The user selects a file from the tree.
