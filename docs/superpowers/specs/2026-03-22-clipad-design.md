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
│ ^S save  ^N new  ^D delete  ^Q quit  ^M switch  ^P preview │
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

## Data Flows

### Open File
Select in tree -> read from disk -> load into textarea buffer -> clear dirty flag.

### Save
`Ctrl+S` -> write textarea content to disk -> clear dirty flag -> refresh tree.

### New Note
`Ctrl+N` -> prompt for filename (e.g. `daily/2026-03-22.md`) in bottom bar -> create intermediate directories if needed -> create file -> open in editor.

### Delete
`Ctrl+D` -> show confirmation prompt -> delete from disk -> refresh tree -> open next file or clear editor.

### Preview Toggle
`Ctrl+P` -> if in edit mode, render buffer through Glamour into viewport; if in preview, switch back to textarea.

### Filter
`/` (in tree panel) -> show filter input at top of tree -> fuzzy match all files by filename -> Enter opens selected match -> Esc cancels and restores tree.

### Unsaved Changes Guard
When switching files or quitting with `dirty == true`, show prompt: "Unsaved changes. Save? (y/n/cancel)".

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
- Tab inserts spaces (configurable width, default 4)
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
| `Ctrl+M` | Switch focus between tree and editor |
| `Ctrl+P` | Toggle edit/preview mode |
| `Ctrl+D` | Delete selected file (with confirmation) |

### Tree Panel

| Key | Action |
|-----|--------|
| `/` | Activate fuzzy filter |
| `Enter` | Open file / toggle folder |
| Up/Down | Navigate entries |

### Editor Panel
All normal text input passes through to the textarea. Arrow keys move the cursor.

### Bottom Bar Format
```
^S save  ^N new  ^D delete  ^Q quit  ^M switch  ^P preview  [line:col] [filename] [modified]
```

## Configuration

### Location
`~/.config/clipad/config.toml`

### Fields
```toml
vault = "/home/user/notes"
tab_width = 4
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
