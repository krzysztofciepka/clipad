# Clipad

A terminal-based note-taking app with an Obsidian-like layout. File tree on the left, markdown editor on the right, plugin system for LLM-powered note transformation.

Built with Go and the [Charm](https://charm.sh) ecosystem (Bubble Tea, Lipgloss, Glamour).

## Features

- **File tree** with nested folders, expand/collapse, fuzzy search
- **Markdown editor** with line numbers and preview rendering
- **Plugin system** with OpenRouter integration for LLM-powered note transformation (rephrase, translate, redraft)
- **Find & replace** with live highlighting and match count
- **Side-by-side diff view** for reviewing plugin changes
- **Adaptive layout** that scales to narrow terminals
- **First-run setup** with interactive vault path configuration

## Install

### From release

Download the binary from the [latest release](https://github.com/krzysztofciepka/clipad/releases) and place it in your PATH.

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

## Usage

```bash
clipad
```

On first run, you'll be prompted to set your vault path (the directory where your notes live). The config is stored at `~/.config/clipad/config.toml`.

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `Ctrl+S` | Save |
| `Ctrl+N` | New note (filename derived from first line) |
| `Ctrl+R` | Find & replace |
| `Ctrl+P` | Toggle markdown preview |
| `Ctrl+Q` | Quit |
| `Tab` | Switch panels |
| `Ctrl+Space` | Open plugin selector |

### File Tree

| Key | Action |
|-----|--------|
| `Up/Down` | Navigate (previews file content) |
| `Enter` | Open file in editor / toggle folder |
| `Right` | Open file in editor |
| `/` | Fuzzy filter |
| `Ctrl+D` | Delete file |
| `Ctrl+F` | Create folder |

### Editor

| Key | Action |
|-----|--------|
| `Esc` | Return to file tree |
| All other keys | Normal text editing |

## Plugins

Plugins process your notes through external services. Press `Ctrl+Space` to open the plugin selector.

### OpenRouter

LLM-powered note transformation via [OpenRouter](https://openrouter.ai). Supports any model available on the platform.

On first use, you'll be prompted for:
- **API Key** - your OpenRouter API key
- **Model** - e.g. `openai/gpt-4o`, `anthropic/claude-sonnet-4`

After processing, a side-by-side diff shows the original and modified note. Press `y` to accept or `n` to reject.

Plugin config is stored at `~/.config/clipad/plugins/openrouter.toml`.

## Configuration

| File | Purpose |
|------|---------|
| `~/.config/clipad/config.toml` | Vault path |
| `~/.config/clipad/plugins/*.toml` | Plugin settings |

Respects `$XDG_CONFIG_HOME` if set.

## License

MIT
