# Clipad

A terminal-based note-taking app with an Obsidian-like layout. File tree on the left, markdown editor on the right, plugin system for LLM-powered note transformation.

Built with Go and the [Charm](https://charm.sh) ecosystem (Bubble Tea, Lipgloss, Glamour).

## Features

- **File tree** with nested folders, expand/collapse, fuzzy search
- **Markdown editor** with line numbers and preview rendering
- **Plugin system** with blackbox.ai and OpenRouter integrations for LLM-powered note transformation (rephrase, translate, redraft)
- **AI shortcuts** with two modes per shortcut: *replace* (diff + accept) or *review* (read-only side-by-side commentary that never edits the note)
- **Find & replace** with live highlighting and match count
- **Side-by-side diff view** for reviewing plugin changes, plus a read-only review view for commentary-style shortcuts
- **Adaptive layout** that scales to narrow terminals
- **First-run setup** with interactive vault path configuration

## Install

### From release

Download the binary from the [latest release](https://github.com/krzysztofciepka/clipad/releases) and place it in your `PATH`.

To upgrade an existing installation in place:

```bash
clipad --upgrade
```

This downloads the latest release, verifies its sha256 checksum, and atomically replaces the running binary.

### From source

```bash
go install github.com/krzysztofciepka/clipad@latest
```

Or build manually:

```bash
git clone https://github.com/krzysztofciepka/clipad.git
cd clipad
go build -o clipad .
```

For a release build that knows its own version (so `--version` and `--upgrade` work correctly):

```bash
TAG=v0.0.22
go build -ldflags "-X main.version=$TAG" -o clipad-$TAG-linux-amd64 .
```

## Usage

```bash
clipad
```

On first run, you'll be prompted to set your vault path (the directory where your notes live). The config is stored at `~/.config/clipad/config.toml`.

### CLI flags

| Flag | Action |
|------|--------|
| `--version` | Print the embedded version and exit |
| `--upgrade` | Fetch the latest GitHub release, verify its sha256, and replace the current binary in place. Restart clipad afterwards. Linux/amd64 only. |
| `-p`, `--preview` `<path>` | Open `<path>` in preview mode with the file tree hidden; typing switches to edit mode |
| `-n`, `--new` | Start in new-note mode (same as "+ Add note"); the file tree stays visible |

### Quick actions

Open or create a note straight from the shell. Paths may be relative or
absolute and can point anywhere on the filesystem.

```bash
clipad path/to/note.md      # open in edit mode, file tree hidden
clipad -p path/to/note.md   # open in preview mode; start typing to edit
clipad --new                # start a new note in the vault root
clipad path/to/dir/         # start a new note in that directory
```

- A path to an existing file opens it; a path to a directory starts a new note in it.
- A non-existing path is created — the file, plus any missing parent directories.
- Flags must come before the path, e.g. `clipad -p note.md`.

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `Ctrl+S` | Save |
| `Ctrl+N` | New note (filename derived from first line) |
| `Ctrl+J` | Quick capture — append timestamped bullet to `<vault>/inbox.md` |
| `Ctrl+O` | Move selected text to a new note in the same directory |
| `Ctrl+R` | Find & replace |
| `Ctrl+P` | Toggle markdown preview |
| `Ctrl+B` | Toggle file tree visibility |
| `F5` | Sync with git remote (push/pull) |
| `Ctrl+Q` | Quit |
| `Tab` | Switch panels |
| `Ctrl+Space` | Open plugin selector |
| `Ctrl+G` | Open AI shortcut selector |
| `Ctrl+L` | Create AI shortcut |
| `Ctrl+K` | Open the notes **agent** panel (ask about or manage your notes) |

### File Tree

| Key | Action |
|-----|--------|
| `Up/Down` | Navigate (previews file content) |
| `Enter` | Open file in editor / toggle folder |
| `Right` | Open file in editor |
| `/` | Fuzzy filter |
| `Ctrl+E` | Rename file or folder |
| `Ctrl+D` | Delete file or folder |
| `Ctrl+F` | Create folder |

### Editor

| Key | Action |
|-----|--------|
| `Esc` | Return to file tree |
| `Ctrl+Z` | Undo last edit |
| `Ctrl+Shift+Z` / `Ctrl+Y` | Redo |
| All other keys | Normal text editing |

### Mouse

| Action | Effect |
|--------|--------|
| Click in editor | Move cursor to clicked position |
| Click-drag in editor | Select text (same as shift+arrow) |
| Wheel up / down in editor | Scroll editor contents |
| Click on file in tree | Move tree cursor and open file in preview |
| Click on folder in tree | Expand / collapse the folder |
| Wheel up / down in tree | Scroll tree |

Terminal-native selection (dragging with the OS to copy outside the app) is disabled while clipad has the mouse. Most terminals still allow Shift+drag to bypass the app and use the OS selection.

## Plugins

Plugins process your notes through external services. Press `Ctrl+Space` to open the plugin selector.

### Blackbox

LLM-powered note transformation via [blackbox.ai](https://blackbox.ai). Supports any model available on the platform.

On first use, you'll be prompted for:
- **API Key** - your blackbox.ai API key
- **Model** - e.g. `blackboxai/minimax/minimax-m2.5`

After processing, a side-by-side diff shows the original and modified note. Press `y` to accept or `n` to reject.

Plugin config is stored at `~/.config/clipad/plugins/blackbox.toml`.

### OpenRouter

LLM-powered note transformation via [OpenRouter](https://openrouter.ai). Supports any model available on the platform.

On first use, you'll be prompted for:
- **API Key** - your OpenRouter API key
- **Model** - e.g. `openai/gpt-4o`, `anthropic/claude-sonnet-4`

Plugin config is stored at `~/.config/clipad/plugins/openrouter.toml`.

### AI Shortcuts

Quick text transformations powered by your configured LLM. Press `Ctrl+G`, pick a shortcut, and the model rewrites or augments the current note. The diff view lets you accept or reject the change. Each AI shortcut has a type — `replace` rewrites the note via the diff+accept flow; `review` opens a read-only side-by-side pane you can scroll and copy from. When creating a shortcut you choose its type as the final step.

If text is selected when you trigger a plugin or shortcut, only the selected text is sent to the LLM and the diff/accept flow replaces just that selection. With no selection, the whole note is rewritten as before.

Shortcuts live in `~/.config/clipad/ai_shortcuts.toml` as `[[shortcuts]]` blocks (`name` + `prompt`). On first run the file is seeded with a default library of 23 shortcuts; you can edit, delete, or add entries freely afterward — clipad never overwrites your file.

**Switching providers.** Inside the shortcut picker, press `p` to cycle the active AI provider (Blackbox ⇄ OpenRouter). The current provider is shown in the picker hint line and persisted to `~/.config/clipad/config.toml` as `ai_shortcut_provider`. If you select a provider that has not been configured yet, the next shortcut run will trigger its setup wizard.

The default library covers:

- **Requirements** — `prd`, `userstory`, `acceptance`, `critique`
- **Todos** — `todos`, `prioritize`, `breakdown`
- **Tech notes** — `onboard`, `explain`
- **Universal utilities** — `tighten`, `tldr`, `outline`, `questions`, `examples`, `diagram`, `glossary`, `risks`
- **Formatting** — `bullets`, `steps`, `table`, `headers`, `fmtjson`, `markdown`

## Agent

Press `Ctrl+K` to open the agent — a continuous chat in the right-hand panel
that can both answer questions about your notes and manage them. It uses your
active AI provider (blackbox.ai by default) with native tool-calling and has two
tools:

- **search_vault** — semantic search over your notes (cited inline; press `1`–`9`
  to open a citation). Requires `embedding_provider` configured; before each
  search it prunes index entries for files that no longer exist.
- **bash** — runs shell commands (cd, mv, cp, cat, sed, awk, …) in your vault to
  inspect and edit notes. Commands run with the vault as the working directory
  and a best-effort guard that blocks paths escaping the vault and `sudo`.

Example: *"rename all Task <N> files so N is sequential starting from 1, only in
the Prywatne directory."*

Slash commands: `/clear` (reset the conversation), `/exit` (close), `/model`
(show the model), `/help`. Press `Esc` to stop a run.

The agent's bash commands run automatically and are scoped to the vault by
working directory plus a heuristic guard — this is a safety rail against
accidents, not a security sandbox.

## Configuration

| File | Purpose |
|------|---------|
| `~/.config/clipad/config.toml` | Vault path; optional `inbox_path` override (defaults to `inbox.md` relative to the vault — accepts vault-relative subpaths, absolute paths, and `~`-prefixed paths) |
| `~/.config/clipad/plugins/*.toml` | Plugin settings |

Respects `$XDG_CONFIG_HOME` if set.

## License

MIT
